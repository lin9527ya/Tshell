package connection

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// TaskFunc 任务处理函数类型
type TaskFunc func(ctx context.Context) error

// Task 任务结构体
type Task struct {
	ID       string
	Function TaskFunc
	Priority int // 优先级，数字越小优先级越高
	Created  time.Time
	Timeout  time.Duration
}

// WorkerPool 工作池结构体
type WorkerPool struct {
	workerCount  int
	workersMu    sync.RWMutex // 保护 workers 切片与 workerCount（ResizePool 与统计读取并发安全）
	taskQueue    chan Task
	resultQueue  chan TaskResult
	workers      []*Worker
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	stats        *WorkerPoolStats
	maxQueueSize int
	isRunning    int32 // 原子操作标志
}

// TaskResult 任务结果
type TaskResult struct {
	TaskID    string
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

// Worker 工作协程
type Worker struct {
	id         int
	pool       *WorkerPool
	taskCount  int64
	isActive   int32
	lastActive time.Time
	mu         sync.RWMutex
	stopCh     chan struct{} // 缩容信号：非 nil 时 worker 处理完当前任务后退出
}

// WorkerPoolStats 工作池统计信息
type WorkerPoolStats struct {
	TotalTasks        int64
	CompletedTasks    int64
	FailedTasks       int64
	ActiveWorkers     int32
	QueuedTasks       int32
	AverageTaskTime   time.Duration
	TotalTaskTime     time.Duration
	WorkerUtilization float64
	LastUpdate        time.Time
	mu                sync.RWMutex
}

// NewWorkerPool 创建新的工作池
// 【P1-死代码优化】workerCount<=0 不再默认扩为 2×NumCPU——SubmitTask 全项目零调用，
// 原实现启动即常驻 2×NumCPU 个空闲 goroutine。现在 0 表示不预启 worker，
// 由管理员经 /worker/resize 按需扩容（ResizePool 动态 spawn）。
func NewWorkerPool(workerCount, maxQueueSize int) *WorkerPool {
	if maxQueueSize <= 0 {
		maxQueueSize = 1000 // 队列缓冲下限：0 会退化为无缓冲通道
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		workerCount:  workerCount,
		taskQueue:    make(chan Task, maxQueueSize),
		resultQueue:  make(chan TaskResult, maxQueueSize),
		ctx:          ctx,
		cancel:       cancel,
		maxQueueSize: maxQueueSize,
		stats: &WorkerPoolStats{
			LastUpdate: time.Now(),
		},
	}

	return pool
}

// Start 启动工作池
func (wp *WorkerPool) Start() error {
	if !atomic.CompareAndSwapInt32(&wp.isRunning, 0, 1) {
		return fmt.Errorf("工作池已在运行中")
	}

	// 创建工作协程
	wp.workersMu.Lock()
	wp.workers = make([]*Worker, wp.workerCount)
	for i := 0; i < wp.workerCount; i++ {
		worker := &Worker{
			id:         i,
			pool:       wp,
			lastActive: time.Now(),
			stopCh:     make(chan struct{}),
		}
		wp.workers[i] = worker

		wp.wg.Add(1)
		go worker.run()
	}
	wp.workersMu.Unlock()

	// 【P1-死代码优化】workerCount==0 时不启动统计/结果后台协程（避免常驻空转 goroutine）；
	// 后续 ResizePool 扩容后这些协程由 worker 内的原子统计兜底
	if wp.workerCount > 0 {
		// 启动统计信息收集器
		go wp.startStatsCollector()

		// 启动结果处理器
		go wp.startResultProcessor()
	}

	logx.Infof("✅ 工作池已启动，工作协程数: %d，最大队列大小: %d\n", wp.workerCount, wp.maxQueueSize)
	return nil
}

// Stop 停止工作池
func (wp *WorkerPool) Stop() error {
	if !atomic.CompareAndSwapInt32(&wp.isRunning, 1, 0) {
		return fmt.Errorf("工作池未在运行")
	}

	// 发送取消信号（worker 通过 ctx.Done 退出；不关闭 taskQueue，避免与 SubmitTask 竞态 panic）
	wp.cancel()

	// J3: 等待所有工作协程完成，带超时兜底（无超时任务可能永久阻塞 wg.Wait → Stop 挂起）
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		logx.Println("⚠️ 工作池停止超时（30s），存在未结束任务，强制返回")
	}

	// 关闭结果队列
	select {
	case <-wp.resultQueue:
	default:
		close(wp.resultQueue)
	}

	logx.Println("🔒 工作池已停止")
	return nil
}

// SubmitTask 提交任务
func (wp *WorkerPool) SubmitTask(task Task) error {
	if atomic.LoadInt32(&wp.isRunning) == 0 {
		return fmt.Errorf("工作池未运行")
	}

	if task.ID == "" {
		task.ID = fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	task.Created = time.Now()

	// 检查队列是否已满（同时监听停止信号，避免向已关闭队列发送）
	select {
	case wp.taskQueue <- task:
		atomic.AddInt32(&wp.stats.QueuedTasks, 1)
		atomic.AddInt64(&wp.stats.TotalTasks, 1)
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("工作池已停止")
	default:
		return fmt.Errorf("任务队列已满，无法提交任务")
	}
}

// SubmitTaskWithTimeout 提交带超时的任务
func (wp *WorkerPool) SubmitTaskWithTimeout(taskFunc TaskFunc, timeout time.Duration) error {
	task := Task{
		Function: taskFunc,
		Timeout:  timeout,
		Priority: 5, // 默认优先级
	}
	return wp.SubmitTask(task)
}

// SubmitHighPriorityTask 提交高优先级任务
func (wp *WorkerPool) SubmitHighPriorityTask(taskFunc TaskFunc) error {
	task := Task{
		Function: taskFunc,
		Priority: 1, // 高优先级
		Timeout:  30 * time.Second,
	}
	return wp.SubmitTask(task)
}

// Worker运行逻辑
func (w *Worker) run() {
	defer w.pool.wg.Done()
	atomic.AddInt32(&w.pool.stats.ActiveWorkers, 1)
	defer atomic.AddInt32(&w.pool.stats.ActiveWorkers, -1)

	for {
		select {
		case <-w.pool.ctx.Done():
			return
		case <-w.stopCh:
			// 缩容信号：处理完当前任务后退出
			return
		case task, ok := <-w.pool.taskQueue:
			if !ok {
				return
			}
			w.processTask(task)
			// 任务处理后再次检查缩容信号（任务可能在 select 前已到达）
			select {
			case <-w.stopCh:
				return
			default:
			}
		}
	}
}

// 处理单个任务
func (w *Worker) processTask(task Task) {
	atomic.StoreInt32(&w.isActive, 1)
	defer atomic.StoreInt32(&w.isActive, 0)

	w.mu.Lock()
	w.lastActive = time.Now()
	w.taskCount++
	w.mu.Unlock()

	startTime := time.Now()
	var err error

	// 创建任务上下文
	ctx := w.pool.ctx
	if task.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(w.pool.ctx, task.Timeout)
		defer cancel()
	}

	// 执行任务
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("任务panic: %v", r)
			}
		}()
		err = task.Function(ctx)
	}()

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// 发送任务结果
	result := TaskResult{
		TaskID:    task.ID,
		Error:     err,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  duration,
	}

	select {
	case w.pool.resultQueue <- result:
	case <-w.pool.ctx.Done():
		return
	default:
		// 结果队列满了，丢弃结果（或记录警告）
		logx.Warnf("⚠️ 结果队列已满，丢弃任务结果: %s\n", task.ID)
	}

	atomic.AddInt32(&w.pool.stats.QueuedTasks, -1)
}

// 启动结果处理器
func (wp *WorkerPool) startResultProcessor() {
	for {
		select {
		case <-wp.ctx.Done():
			return
		case result, ok := <-wp.resultQueue:
			if !ok {
				return
			}
			wp.processResult(result)
		}
	}
}

// 处理任务结果
func (wp *WorkerPool) processResult(result TaskResult) {
	wp.stats.mu.Lock()
	defer wp.stats.mu.Unlock()

	wp.stats.TotalTaskTime += result.Duration

	if result.Error != nil {
		wp.stats.FailedTasks++
		logx.Errorf("❌ 任务执行失败 %s: %v (耗时: %v)\n",
			result.TaskID, result.Error, result.Duration)
	} else {
		wp.stats.CompletedTasks++
	}

	// 更新平均任务时间
	totalCompleted := wp.stats.CompletedTasks + wp.stats.FailedTasks
	if totalCompleted > 0 {
		wp.stats.AverageTaskTime = wp.stats.TotalTaskTime / time.Duration(totalCompleted)
	}
}

// 启动统计信息收集器
func (wp *WorkerPool) startStatsCollector() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case <-ticker.C:
			wp.updateStats()
		}
	}
}

// 更新统计信息
func (wp *WorkerPool) updateStats() {
	wp.stats.mu.Lock()
	defer wp.stats.mu.Unlock()

	// 计算工作协程利用率
	wp.workersMu.RLock()
	activeWorkers := 0
	for _, worker := range wp.workers {
		if atomic.LoadInt32(&worker.isActive) == 1 {
			activeWorkers++
		}
	}
	workerCount := wp.workerCount
	wp.workersMu.RUnlock()

	if workerCount > 0 {
		wp.stats.WorkerUtilization = float64(activeWorkers) / float64(workerCount)
	}

	wp.stats.LastUpdate = time.Now()
}

// GetStats 获取工作池统计信息
func (wp *WorkerPool) GetStats() WorkerPoolStats {
	// 原子字段（SubmitTask/run/processTask 用 atomic 写）用 atomic 读；
	// mutex 字段（processResult/updateStats 持 stats.mu 写）在锁内读。
	// 混用会触发 race detector。
	wp.stats.mu.RLock()
	completed := wp.stats.CompletedTasks
	failed := wp.stats.FailedTasks
	avgTaskTime := wp.stats.AverageTaskTime
	totalTaskTime := wp.stats.TotalTaskTime
	utilization := wp.stats.WorkerUtilization
	lastUpdate := wp.stats.LastUpdate
	wp.stats.mu.RUnlock()
	total := atomic.LoadInt64(&wp.stats.TotalTasks)
	active := atomic.LoadInt32(&wp.stats.ActiveWorkers)
	queued := atomic.LoadInt32(&wp.stats.QueuedTasks)
	return WorkerPoolStats{
		TotalTasks:        total,
		CompletedTasks:    completed,
		FailedTasks:       failed,
		ActiveWorkers:     active,
		QueuedTasks:       queued,
		AverageTaskTime:   avgTaskTime,
		TotalTaskTime:     totalTaskTime,
		WorkerUtilization: utilization,
		LastUpdate:        lastUpdate,
	}
}

// GetWorkerInfo 获取工作协程信息
func (wp *WorkerPool) GetWorkerInfo() []map[string]interface{} {
	wp.workersMu.RLock()
	workers := make([]*Worker, len(wp.workers))
	copy(workers, wp.workers)
	wp.workersMu.RUnlock()

	result := make([]map[string]interface{}, len(workers))
	for i, worker := range workers {
		worker.mu.RLock()
		info := map[string]interface{}{
			"id":          worker.id,
			"task_count":  worker.taskCount,
			"is_active":   atomic.LoadInt32(&worker.isActive) == 1,
			"last_active": worker.lastActive,
		}
		worker.mu.RUnlock()
		result[i] = info
	}

	return result
}

// GetQueueSize 获取当前队列大小
func (wp *WorkerPool) GetQueueSize() int {
	return len(wp.taskQueue)
}

// IsRunning 检查工作池是否在运行
func (wp *WorkerPool) IsRunning() bool {
	return atomic.LoadInt32(&wp.isRunning) == 1
}

// ResizePool 动态调整工作池大小
func (wp *WorkerPool) ResizePool(newSize int) error {
	if newSize <= 0 {
		return fmt.Errorf("工作池大小必须大于0")
	}

	wp.workersMu.Lock()
	defer wp.workersMu.Unlock()

	if newSize == wp.workerCount {
		return nil
	}

	if newSize > wp.workerCount {
		// 增加工作协程
		for i := wp.workerCount; i < newSize; i++ {
			worker := &Worker{
				id:         i,
				pool:       wp,
				lastActive: time.Now(),
				stopCh:     make(chan struct{}),
			}
			wp.workers = append(wp.workers, worker)

			wp.wg.Add(1)
			go worker.run()
		}
		logx.Infof("📈 工作池大小增加到: %d\n", newSize)
	} else {
		// 减少工作协程：向多余的 worker 发缩容信号，处理完当前任务后自然退出
		toRemove := wp.workerCount - newSize
		for i := 0; i < toRemove && i < len(wp.workers); i++ {
			select {
			case <-wp.workers[i].stopCh:
				// 该 worker 已退出，跳过
			default:
				close(wp.workers[i].stopCh)
			}
		}
		wp.workers = wp.workers[toRemove:]
		logx.Infof("📉 工作池大小减少到: %d（已发缩容信号）\n", newSize)
	}

	wp.workerCount = newSize
	return nil
}

// 全局工作池实例
var GlobalWorkerPool *WorkerPool

// InitializeWorkerPool 初始化全局工作池
func InitializeWorkerPool(workerCount, maxQueueSize int) error {
	// 重复初始化前先停止旧实例（取消后台 goroutine：worker/统计收集/结果处理），防泄漏
	if GlobalWorkerPool != nil {
		_ = GlobalWorkerPool.Stop()
	}

	GlobalWorkerPool = NewWorkerPool(workerCount, maxQueueSize)

	if err := GlobalWorkerPool.Start(); err != nil {
		return fmt.Errorf("启动工作池失败: %v", err)
	}

	logx.Println("✅ 全局工作池初始化成功")
	return nil
}

// StopGlobalWorkerPool 停止全局工作池
func StopGlobalWorkerPool() error {
	if GlobalWorkerPool != nil {
		return GlobalWorkerPool.Stop()
	}
	return nil
}

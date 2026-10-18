package backup

import (
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// StartScheduler 按配置的间隔周期备份（interval <= 0 时不启动）。
// 返回停止函数，便于测试与优雅退出。
func (m *Manager) StartScheduler() (stop func()) {
	if m.opts.Interval <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(m.opts.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				info, err := m.Create("scheduled")
				if err != nil {
					// 备份失败必须显式告警：静默失败的备份系统等于没有备份
					logx.Errorf("❌ [备份] 定时备份失败: %v\n", err)
					continue
				}
				logx.Infof("🗄️ [备份] 完成 %s (%.1f MB)\n", info.Name, float64(info.Size)/1024/1024)
				if removed, err := m.Cleanup(); err != nil {
					logx.Warnf("⚠️ [备份] 清理旧备份失败: %v\n", err)
				} else if len(removed) > 0 {
					logx.Infof("🗑️ [备份] 按保留策略删除 %d 份旧备份\n", len(removed))
				}
			}
		}
	}()
	// 等待在途的一轮备份结束再返回：避免"停完还在写"，测试与优雅退出都需要这个保证
	return func() {
		close(done)
		<-stopped
	}
}

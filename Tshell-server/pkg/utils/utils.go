package utils

import (
	"bytes"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

//go:embed av.json
var avDataFS embed.FS

func BytesCombine(pBytes ...[]byte) []byte {
	return bytes.Join(pBytes, []byte(""))
}

// Paginate takes a slice of any type, and returns a paginated slice.
// It returns an empty slice if the input is invalid or out of bounds.
func Paginate(slice interface{}, page, pageSize int) interface{} {
	// 获取输入切片的反射值
	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice || rv.IsNil() {
		return nil
	}

	// 如果 pageSize <= 0，则直接返回空切片
	if pageSize <= 0 {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}

	// 计算总项数
	totalItems := rv.Len()

	// 边界处理：如果 totalItems <= 0，返回空切片
	if totalItems == 0 {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}

	// 计算总页数
	totalPages := (totalItems + pageSize - 1) / pageSize

	// 边界处理：如果 page <= 0，返回第一页
	if page <= 0 {
		page = 1
	}

	// 边界处理：如果 page 超出范围，返回最后一页
	if page > totalPages {
		page = totalPages
	}

	// 计算当前页的起始和结束索引
	start := (page - 1) * pageSize

	// 边界处理：确保 start 不小于 0
	if start < 0 {
		start = 0
	}

	end := start + pageSize

	// 确保 end 不超过切片的长度
	if end > totalItems {
		end = totalItems
	}

	// 使用反射创建一个新的切片，并复制数据
	paginatedSlice := reflect.MakeSlice(rv.Type(), end-start, end-start)
	reflect.Copy(paginatedSlice, rv.Slice(start, end))

	// 返回分页后的切片
	return paginatedSlice.Interface()
}

// ProcessInfo 结构体代表一行进程信息
type ProcessInfo struct {
	Pid         string `json:"PID"`
	PPid        string `json:"PPID"`
	Name        string `json:"Name"`
	Arch        string `json:"Arch"`
	User        string `json:"User"`
	ProcessInfo string `json:"ProcessInfo"` // 新增进程信息字段
}

// AV信息结构体
type AVInfo struct {
	Processes []string `json:"processes"`
	URL       string   `json:"url"`
}

// AV数据缓存
var (
	avDataCache   map[string]AVInfo
	avCacheMutex  sync.RWMutex
	avCacheLoaded bool = false
)

// 加载av.json文件
func LoadAVData() error {
	avCacheMutex.Lock()
	defer avCacheMutex.Unlock()

	if avCacheLoaded {
		return nil
	}

	// 从嵌入文件读取av.json
	data, err := avDataFS.ReadFile("av.json")
	if err != nil {
		return fmt.Errorf("failed to read embedded av.json: %v", err)
	}

	// 解析JSON
	avData := make(map[string]AVInfo)
	if err := json.Unmarshal(data, &avData); err != nil {
		return fmt.Errorf("failed to parse av.json: %v", err)
	}

	avDataCache = avData
	avCacheLoaded = true
	return nil
}

// 根据进程名称获取AV信息
func GetProcessAVInfo(processName string) string {
	// 先检查是否已加载，如果没有则加载
	if !avCacheLoaded {
		if err := LoadAVData(); err != nil {
			return ""
		}
	}

	avCacheMutex.RLock()
	defer avCacheMutex.RUnlock()

	// 遵循大小写不敏感匹配逻辑
	lowerProcessName := strings.ToLower(processName)

	for avName, avInfo := range avDataCache {
		for _, process := range avInfo.Processes {
			if strings.ToLower(process) == lowerProcessName {
				return avName
			}
		}
	}

	return ""
}

// ParsePid 解析进程信息字符串并返回ProcessInfo切片
func ParsePid(input string) []ProcessInfo {
	var processes []ProcessInfo
	lines := strings.Split(strings.TrimSpace(input), "\n")

	lines = lines[1:]

	// 确保 AV 数据已加载
	if !avCacheLoaded {
		LoadAVData()
	}

	for _, line := range lines {
		fields := strings.Split(line, "\t")

		// 获取进程名称并查询AV信息
		processName := fields[0]
		avInfo := GetProcessAVInfo(processName)

		processes = append(processes, ProcessInfo{
			Name:        processName,
			PPid:        fields[1],
			Pid:         fields[2],
			Arch:        fields[3],
			User:        fields[4],
			ProcessInfo: avInfo, // 填充AV信息
		})
	}

	return processes
}

// bytesToSize 将字节数转换为合适的单位，并保留一位小数。
func BytesToSize(bytesStr string) string {
	if bytesStr == "0" {
		return ""
	}
	// 将字符串转换为整数
	bytes, err := strconv.ParseInt(bytesStr, 10, 64)
	if err != nil {
		return ""
	}

	var units = []string{"B", "KB", "MB", "GB", "TB"}
	var unitIndex int
	var size float64
	size = float64(bytes)

	// 如果字节小于 1024 KB，则直接返回字节数
	if size < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}

	// 找到合适的单位并计算大小
	for size >= 1024 && unitIndex < len(units)-1 {
		unitIndex++
		size /= 1024
	}

	// 格式化输出，保留一位小数
	return fmt.Sprintf("%.1f%s", size, units[unitIndex])
}
func SplitByteArray(data []byte, chunkSize int) [][]byte {
	var result [][]byte
	for i := 0; i < len(data); i += chunkSize {
		// 如果剩余的字节长度小于 chunkSize，直接取剩余部分
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		result = append(result, data[i:end])
	}
	return result
}
func InitFunction() {
	_, err := os.Stat("./Downloads")
	if os.IsNotExist(err) {
		// 文件夹不存在，创建文件夹；失败只告警（下载目录缺失不应阻断启动）
		if err = os.MkdirAll("./Downloads", os.ModePerm); err != nil {
			fmt.Printf("⚠️ 创建 Downloads 目录失败: %v\n", err)
		}
	}
}

// getExistingDrives 从给定的掩码中提取存在的逻辑驱动器
func GetExistingDrives(drivesMask []byte) []string {
	var existingDrives []string
	drivers := string(drivesMask)
	
	// 修复Linux根目录处理问题
	// 如果驱动器字符串是"/"，表示Linux系统，直接返回根目录
	if drivers == "/" {
		return []string{"/"}
	}
	
	// Windows系统处理：为每个驱动器字符添加冒号
	for _, driver := range drivers {
		existingDrives = append(existingDrives, string(driver)+":")
	}
	return existingDrives
}
func Uint32ToIP(ip uint32) net.IP {
	// 将uint32类型的数据转换为字节切片
	bytes := make([]byte, 4)
	bytes[0] = byte((ip >> 24) & 0xFF)
	bytes[1] = byte((ip >> 16) & 0xFF)
	bytes[2] = byte((ip >> 8) & 0xFF)
	bytes[3] = byte(ip & 0xFF)
	return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3])
}
func WriteInt(nInt int) []byte {
	bBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bBytes, uint32(nInt))
	return bBytes
}

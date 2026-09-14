package utils

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

// TestMetaInfoUidConsistency 客户端本地 cid 必须等于 MD5(实际发送的 metainfo):
// 不一致会导致服务端会话密钥派生 uid 不同 → 命令解密失败
func TestMetaInfoUidConsistency(t *testing.T) {
	meta := MakeMetaInfo()
	cid, err := GetMetaInfoClientID()
	if err != nil {
		t.Fatalf("GetMetaInfoClientID err: %v", err)
	}
	h := md5.Sum(meta)
	want := hex.EncodeToString(h[:])
	if cid != want {
		t.Fatalf("cid 不一致!\n  本地: %s\n  发送: %s\n(元数据构造必须在 MakeMetaInfo 与 GetMetaInfoClientID 完全一致)", cid, want)
	}
}

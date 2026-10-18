package cred

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
)

// DPAPI 主密钥集是一个 ZIP 制品（manifest.json + 每个主密钥一份 json 明文）。
//
// 为什么必须解压校验而不是只看文件头：ZIP 的中央目录写在文件尾部，回传中断时
// 头部仍是 "PK"，只看魔数会把半截包当成完整包登记进凭据库。
// 逐个条目读到 CRC 校验通过，才算"这个包能打开"。
func ValidateZip(path string) (ArtifactInfo, error) {
	var info ArtifactInfo
	info.Format = FormatZip

	st, err := os.Stat(path)
	if err != nil {
		return info, err
	}
	info.Size = st.Size()
	if st.Size() == 0 {
		return info, fmt.Errorf("制品为空文件")
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return info, fmt.Errorf("不是完整的 ZIP 制品（中央目录缺失或损坏）: %w", err)
	}
	defer zr.Close()

	if len(zr.File) == 0 {
		return info, fmt.Errorf("ZIP 制品不含任何条目")
	}
	if len(zr.File) > MaxZipEntries {
		return info, fmt.Errorf("ZIP 条目数异常: %d", len(zr.File))
	}
	info.Entries = len(zr.File)

	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > MaxZipUncompressedBytes {
			return info, fmt.Errorf("ZIP 解压后体积超限（>%d MB）", MaxZipUncompressedBytes>>20)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "manifest.json" {
			info.HasManifest = true
		}
		// 真正读一遍：CRC 不符说明包在传输/写入时坏了
		rc, err := f.Open()
		if err != nil {
			return info, fmt.Errorf("条目 %s 无法读取: %w", f.Name, err)
		}
		n, err := io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			return info, fmt.Errorf("条目 %s 读取中断（制品不完整）: %w", f.Name, err)
		}
		if uint64(n) != f.UncompressedSize64 {
			return info, fmt.Errorf("条目 %s 长度不符（%d != %d）", f.Name, n, f.UncompressedSize64)
		}
	}
	if !info.HasManifest {
		return info, fmt.Errorf("ZIP 制品缺少 manifest.json（无法确认归属与内容）")
	}
	return info, nil
}

package delivery

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
)

// elfNew 复用 randomizeGoBuildID 的 ELF 解析入口
func elfNew(data []byte) (*elf.File, error) {
	return elf.NewFile(bytes.NewReader(data))
}

// readELFBuildID 读取 ELF .note.go.buildid note desc
func readELFBuildID(f *elf.File) string {
	sec := f.Section(".note.go.buildid")
	if sec == nil {
		return ""
	}
	noteData, err := sec.Data()
	if err != nil || len(noteData) < 16 {
		return ""
	}
	namesz := binary.LittleEndian.Uint32(noteData[0:4])
	descsz := binary.LittleEndian.Uint32(noteData[4:8])
	namePadded := (namesz + 3) &^ 3
	descStart := 12 + int(namePadded)
	if descStart+int(descsz) > len(noteData) {
		return ""
	}
	return string(noteData[descStart : descStart+int(descsz)])
}

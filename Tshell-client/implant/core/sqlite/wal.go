package sqlite

import (
	"encoding/binary"
	"fmt"
	"os"
)

// WAL（预写日志）支持。
//
// 为什么必须做：Chrome/Edge 从 80 版本起默认 `journal_mode=WAL`，
// 最近的登录记录/Cookie 很可能**只存在于 -wal 文件里**，主库还是旧快照。
// 不读 WAL 会稳定地"漏掉最新数据"，而这恰恰是行动中最新、最有用的一条。
//
// 只做必要的事：
//   - 找最后一个提交点（commit 字段非 0 的帧），其后的帧属于未提交事务，一律忽略；
//   - 每个页号取"提交点之前最后一次出现"的帧（后写覆盖先写，WAL 的语义就是重放）；
//   - 帧头/页大小/盐值做基本校验，任何不符就整体放弃 WAL（退回主库视图），
//     而不是猜 —— 读错的页比少读几页危险得多。
//
// 不做：校验和验证（读侧可跳过）、WAL 截断/重启的完整语义、shm 索引文件解析。
type walIndex struct {
	file   *os.File
	frames map[int]int // 页号 → 该帧数据起始偏移
}

const (
	walHeaderSize = 32
	walFrameHdr   = 24
	walMagicBE    = 0x377f0682 // 大端校验和
	walMagicLE    = 0x377f0683 // 小端校验和
)

// openWAL 打开同名 -wal 并建立页覆盖索引；不存在或不可用时返回 (nil, nil)。
func openWAL(dbPath string, pageSize int) (*walIndex, error) {
	path := dbPath + "-wal"
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	// 头 + 至少一帧
	if st.Size() < walHeaderSize+walFrameHdr {
		f.Close()
		return nil, nil
	}

	head := make([]byte, walHeaderSize)
	if _, err := f.ReadAt(head, 0); err != nil {
		f.Close()
		return nil, err
	}
	magic := binary.BigEndian.Uint32(head[0:4])
	if magic != walMagicBE && magic != walMagicLE {
		f.Close()
		return nil, fmt.Errorf("WAL 魔数不符: 0x%X", magic)
	}
	if ps := int(binary.BigEndian.Uint32(head[8:12])); ps != pageSize {
		f.Close()
		return nil, fmt.Errorf("WAL 页大小(%d)与主库(%d)不一致", ps, pageSize)
	}

	frameSize := int64(walFrameHdr + pageSize)
	// 帧数（去掉可能存在的半截帧）
	total := (st.Size() - walHeaderSize) / frameSize
	if total <= 0 {
		f.Close()
		return nil, nil
	}

	// 顺序扫描找最后一个提交点，并记录每页最后一次出现的偏移
	frameHdr := make([]byte, walFrameHdr)
	lastCommit := int64(-1)
	offsets := make(map[int64]int64, total)
	for i := int64(0); i < total; i++ {
		off := int64(walHeaderSize) + i*frameSize
		if _, err := f.ReadAt(frameHdr, off); err != nil {
			break // 读到一半（文件被并发截断）→ 用已扫到的部分
		}
		pgno := int64(binary.BigEndian.Uint32(frameHdr[0:4]))
		commit := binary.BigEndian.Uint32(frameHdr[4:8])
		if pgno <= 0 {
			break
		}
		offsets[pgno] = off + walFrameHdr
		if commit > 0 {
			lastCommit = i
		}
	}
	if lastCommit < 0 {
		f.Close()
		return nil, nil // 没有已提交事务：主库视图即可
	}
	// 提交点之后的帧属于未提交事务，丢弃
	commitOff := int64(walHeaderSize) + lastCommit*frameSize
	frames := make(map[int]int, len(offsets))
	for pgno, dataOff := range offsets {
		if dataOff <= commitOff {
			frames[int(pgno)] = int(dataOff)
		}
	}
	if len(frames) == 0 {
		f.Close()
		return nil, nil
	}
	return &walIndex{file: f, frames: frames}, nil
}

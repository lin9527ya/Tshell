package sqlite

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf16"
)

// Open 打开数据库（只读）：校验文件头，解析页参数，尝试加载同名 -wal。
//
// 主库整文件读入内存：浏览器库通常在几 MB 到几十 MB，读一次比逐页 seek
// 简单得多，也不会因为文件在被读期间被浏览器改写而读到半页。
func Open(path string) (*DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < headerSize {
		return nil, fmt.Errorf("文件过小（%d 字节），不是 SQLite 数据库", len(data))
	}
	if string(data[:16]) != headerMagic {
		return nil, fmt.Errorf("文件头不是 SQLite 格式（SQLite format 3）")
	}

	pageSize := int(binary.BigEndian.Uint16(data[16:18]))
	switch pageSize {
	case 1: // 65536 在 2 字节字段里写作 1
		pageSize = maxPageSize
	case minPageSize, 1024, 2048, 4096, 8192, 16384, 32768:
	default:
		return nil, fmt.Errorf("页大小非法: %d", pageSize)
	}
	reserved := int(data[20])
	if reserved >= pageSize {
		return nil, fmt.Errorf("保留区大小非法: %d", reserved)
	}

	// 头里声明的页数只在"无 WAL 且文件大小与声明一致"时才可信；
	// 以实际文件长度为准，避免读到声明的页数之外的空洞
	pages := (len(data) + pageSize - 1) / pageSize
	if declared := int(binary.BigEndian.Uint32(data[28:32])); declared > 0 && declared < pages {
		pages = declared
	}

	enc := int(binary.BigEndian.Uint32(data[56:60]))
	if enc == 0 {
		enc = 1 // 0 视为 UTF-8（老库缺省）
	}

	db := &DB{
		data:       data,
		pageSize:   pageSize,
		usableSize: pageSize - reserved,
		reserved:   reserved,
		encoding:   enc,
		pages:      pages,
	}
	if w, err := openWAL(path, pageSize); err == nil && w != nil {
		db.wal = w
		db.walSource = w.file
	}
	return db, nil
}

// Close 释放 WAL 句柄（主库是内存副本，无需操作）
func (db *DB) Close() error {
	if db.walSource != nil {
		err := db.walSource.Close()
		db.walSource = nil
		db.wal = nil
		return err
	}
	return nil
}

// PageSize 页大小（测试与诊断用）
func (db *DB) PageSize() int { return db.pageSize }

// HasWAL 是否存在已提交的 WAL 覆盖（测试与诊断用）
func (db *DB) HasWAL() bool { return db.wal != nil && len(db.wal.frames) > 0 }

// page 取一页（1 基）。WAL 里已提交的版本优先。
func (db *DB) page(pgno int) ([]byte, error) {
	if pgno <= 0 {
		return nil, fmt.Errorf("页号非法: %d", pgno)
	}
	if db.wal != nil {
		if off, ok := db.wal.frames[pgno]; ok {
			if db.walSource == nil {
				return nil, fmt.Errorf("WAL 句柄已关闭")
			}
			buf := make([]byte, db.pageSize)
			if _, err := db.walSource.ReadAt(buf, int64(off)); err != nil {
				return nil, fmt.Errorf("读取 WAL 帧失败(页 %d): %w", pgno, err)
			}
			return buf, nil
		}
	}
	start := (pgno - 1) * db.pageSize
	if start < 0 || start+db.pageSize > len(db.data) {
		return nil, fmt.Errorf("页 %d 越界（库共 %d 页）", pgno, db.pages)
	}
	return db.data[start : start+db.pageSize], nil
}

// Table 定位一张表并读出列清单（表结构来自 sqlite_master 的建表 SQL）
func (db *DB) Table(name string) (*Table, error) {
	root, sql, err := db.findTable(name)
	if err != nil {
		return nil, err
	}
	cols := parseColumnNames(sql)
	if len(cols) == 0 {
		return nil, fmt.Errorf("表 %s 的建表语句无法解析列名", name)
	}
	t := &Table{db: db, name: name, root: root, byName: make(map[string]int, len(cols)), rowidCol: -1}
	for i, c := range cols {
		t.cols = append(t.cols, Column{Name: c, Pos: i})
		t.byName[c] = i
	}
	// INTEGER PRIMARY KEY 是 rowid 的别名：**该列在记录里存的是 NULL**，
	// 值要从单元格的 rowid 字段取。Chrome 的 logins.id / cookies 系列都这么定义，
	// 不识别就会把主键读成空。
	if idx, isAlias := rowidAliasColumn(sql); isAlias && idx >= 0 && idx < len(t.cols) {
		t.rowidCol = idx
	}
	return t, nil
}

// rowidAliasColumn 判断建表语句里有没有 "INTEGER PRIMARY KEY"（rowid 别名），
// 返回它的列序号（按解析出的列顺序数）。
func rowidAliasColumn(sql string) (int, bool) {
	idx := 0
	for _, part := range splitTopLevel(innerColumns(sql)) {
		up := strings.ToUpper(strings.Join(strings.Fields(part), " "))
		if strings.HasPrefix(up, "PRIMARY ") || strings.HasPrefix(up, "UNIQUE") ||
			strings.HasPrefix(up, "CHECK") || strings.HasPrefix(up, "FOREIGN") ||
			strings.HasPrefix(up, "CONSTRAINT") {
			continue
		}
		if strings.Contains(up, "INTEGER PRIMARY KEY") {
			return idx, true
		}
		idx++
	}
	return -1, false
}

// equalFoldASCII 按 ASCII 规则做大小写不敏感比较（SQLite 标识符的语义）
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// findTable 在 sqlite_master（根页 1）里找表的根页号与建表 SQL
func (db *DB) findTable(name string) (int, string, error) {
	rows, _, err := db.readTableRows(1, MaxRows, nil, 0)
	if err != nil {
		return 0, "", fmt.Errorf("读取 sqlite_master 失败: %w", err)
	}
	// sqlite_master 列：type, name, tbl_name, rootpage, sql
	const (
		colType = 0
		colName = 1
		colRoot = 3
		colSQL  = 4
	)
	for _, r := range rows {
		if len(r) <= colSQL {
			continue
		}
		// SQLite 的表名是**大小写不敏感**的（ASCII），这里必须跟着不敏感：
		// 真实文件里就有 `metaData`（Firefox key4.db）这种驼峰写法，
		// 而调用方习惯写全小写的 `metadata`。按精确匹配会报"表不存在"，
		// 而那个错看起来像文件损坏/格式不支持，很难反查到只是大小写。
		if !equalFoldASCII(asString(r[colName]), name) {
			continue
		}
		if asString(r[colType]) != "table" {
			continue
		}
		root := int(asInt(r[colRoot]))
		if root <= 0 {
			continue
		}
		return root, asString(r[colSQL]), nil
	}
	return 0, "", fmt.Errorf("表不存在: %s", name)
}

// Rows 读整表（全表扫描），可带 rowid 过滤。
// 返回的 Row 以列名索引，调用方不必关心列顺序。
func (t *Table) Rows(filter *Filter) ([]Row, error) {
	raw, rowids, err := t.db.readTableRows(t.root, MaxRows, filter, len(t.cols))
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(raw))
	for i, vals := range raw {
		row := make(Row, len(t.cols))
		for _, c := range t.cols {
			if c.Pos < len(vals) {
				row[c.Name] = vals[c.Pos]
			} else {
				row[c.Name] = nil // 记录列数少于表定义（老库/迁移中）：缺的列按 NULL
			}
		}
		// INTEGER PRIMARY KEY 列在记录里存 NULL，真实值来自单元格的 rowid
		if t.rowidCol >= 0 && i < len(rowids) {
			row[t.cols[t.rowidCol].Name] = rowids[i]
		}
		out = append(out, row)
	}
	return out, nil
}

// readTableRows 遍历表 b-tree，返回解码后的行（按列序号切片）与对应 rowid
func (db *DB) readTableRows(root int, maxRows int, filter *Filter, expectCols int) ([][]Value, []int64, error) {
	var out [][]Value
	var rowids []int64
	seenPages := make(map[int]bool)
	var walk func(pgno, depth int) error
	walk = func(pgno, depth int) error {
		if len(out) >= maxRows {
			return nil
		}
		if depth > 32 {
			return fmt.Errorf("b-tree 深度异常（可能存在环）")
		}
		if seenPages[pgno] {
			return fmt.Errorf("b-tree 页重复访问（页 %d），结构畸形", pgno)
		}
		seenPages[pgno] = true

		page, err := db.page(pgno)
		if err != nil {
			return err
		}
		// 第 1 页的前 100 字节是数据库头，b-tree 页头从第 100 字节开始；
		// 其余页从页首开始。Cell 指针里的偏移是**页内绝对偏移**，不受此影响。
		pageHdr := 0
		if pgno == 1 {
			pageHdr = headerSize
		}
		if pageHdr+12 > len(page) {
			return fmt.Errorf("页 %d 头部不完整", pgno)
		}
		typ := page[pageHdr]
		if typ == pageInteriorIndex || typ == pageLeafIndex {
			return fmt.Errorf("页 %d 是索引页，本读取器只支持表 b-tree", pgno)
		}
		if typ != pageInteriorTable && typ != pageLeafTable {
			return fmt.Errorf("页 %d 类型非法: %d", pgno, typ)
		}
		cells := int(binary.BigEndian.Uint16(page[pageHdr+3 : pageHdr+5]))
		cellPtrStart := pageHdr + 8
		if typ == pageInteriorTable {
			cellPtrStart = pageHdr + 12
		}
		if cellPtrStart+cells*2 > len(page) {
			return fmt.Errorf("页 %d 单元格指针数组越界（%d 个）", pgno, cells)
		}
		if typ == pageInteriorTable {
			rightMost := int(binary.BigEndian.Uint32(page[pageHdr+8 : pageHdr+12]))
			for i := 0; i < cells; i++ {
				off := int(binary.BigEndian.Uint16(page[cellPtrStart+i*2:]))
				if off+4 > len(page) {
					return fmt.Errorf("页 %d 第 %d 个单元格越界", pgno, i)
				}
				child := int(binary.BigEndian.Uint32(page[off : off+4]))
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
			if rightMost > 0 {
				return walk(rightMost, depth+1)
			}
			return nil
		}

		// 叶子表页：cell = payloadLen(varint) + rowid(varint) + payload
		for i := 0; i < cells; i++ {
			off := int(binary.BigEndian.Uint16(page[cellPtrStart+i*2:]))
			if off <= 0 || off >= len(page) {
				return fmt.Errorf("页 %d 第 %d 个单元格偏移非法: %d", pgno, i, off)
			}
			payloadLen, n1 := getVarint(page[off:])
			if n1 <= 0 {
				return fmt.Errorf("页 %d 第 %d 个单元格载荷长度畸形", pgno, i)
			}
			rowid, n2 := getVarint(page[off+n1:])
			if n2 <= 0 {
				return fmt.Errorf("页 %d 第 %d 个单元格 rowid 畸形", pgno, i)
			}
			bodyOff := off + n1 + n2
			candidates, err := db.payloadCandidates(page, bodyOff, int(payloadLen), int(typ))
			if err != nil {
				return fmt.Errorf("页 %d 第 %d 个单元格: %w", pgno, i, err)
			}
			if !filter.Match(rowid) {
				continue
			}
			// 溢出单元格的局部长度按实测口径枚举（见 payloadCandidates），
			// 用**记录结构**认领正确的那一份：列数对得上、头长度与各列长度之和也吻合。
			// 错位的切法要么链走不通，要么解出来的列数不对，因此这个判据足够强。
			var vals []Value
			var decodeErr error
			ok := false
			for _, cand := range candidates {
				v, err := db.decodeRecord(cand)
				if err != nil {
					decodeErr = err
					continue
				}
				if expectCols <= 0 || len(v) == expectCols {
					vals, ok = v, true
					break
				}
				if vals == nil {
					vals = v // 列数不符时先记着，实在没有更好的就用它
				}
			}
			if !ok && vals == nil {
				return fmt.Errorf("页 %d 第 %d 个单元格记录解码失败: %w", pgno, i, decodeErr)
			}
			out = append(out, vals)
			rowids = append(rowids, rowid)
			if len(out) >= maxRows {
				return nil
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, nil, err
	}
	return out, rowids, nil
}

// payloadCandidates 返回单元格载荷的候选组装结果（正常只有一个）。
//
// 溢出单元格"页内放多少字节"没有可靠的公开解析式：SQLite file format §1.5 的写法
// 以及若干变体，在实测样本（scripts/fit_sqlite_overflow.py，4 种页大小 × 9 种载荷，
// 每个真实切分都用"链正好用完 + 末页指针为 0 + 记录头自证"确认）上都有对不上的情形。
// 所以这里走两层：
//
//  1. **结构反推（首选）**：页内数据的末尾 4 字节就是首个溢出页号；顺着链走到底
//     （末页指针为 0）即得链上字节数 = 载荷 - 局部长度。链本身是自证的，
//     不需要任何"余数规则"。
//  2. 公式候选（兜底）：链指针读不出来时，用实测主公式加一圈邻近值试。
//
// 两条路的结果都要过 readOverflowChain 的严格校验（链必须正好用完），
// 最后由调用方用记录列数认领。
func (db *DB) payloadCandidates(page []byte, off, payloadLen, pageType int) ([][]byte, error) {
	usable := db.usableSize
	if pageType == pageLeafTable {
		usable = usable - 35
	} else {
		usable = usable - 12
	}
	maxLocal := usable * 64 / 255
	if maxLocal > maxLocalTable {
		maxLocal = maxLocalTable
	}

	if payloadLen < 0 || off < 0 || off > len(page) {
		return nil, fmt.Errorf("载荷偏移非法: off=%d len=%d", off, payloadLen)
	}
	if payloadLen <= maxLocal {
		if off+payloadLen > len(page) {
			return nil, fmt.Errorf("页内载荷越界（需要 %d，页内可用 %d）", payloadLen, len(page)-off)
		}
		return [][]byte{page[off : off+payloadLen]}, nil
	}
	if db.usableSize <= 4 {
		return nil, fmt.Errorf("可用页大小异常: %d", db.usableSize)
	}
	maxPossible := len(page) - off

	var sizes []int
	seen := map[int]bool{}
	add := func(n int) {
		if n < 4 || n > payloadLen || n > maxPossible || seen[n] {
			return
		}
		seen[n] = true
		sizes = append(sizes, n)
	}

	// 1) 结构反推优先：从"页内尽量多放"往回退，第一个满足
	// "链上字节数 == 载荷 - 该位置"的切法就是它（不依赖任何公式）
	if _, size, ok := db.firstOverflowPageFromCell(page, off, payloadLen, maxPossible); ok {
		add(size)
	}

	// 2) 公式候选（实测主口径 + 邻近容差）：覆盖链指针恰好不可读的极端情形
	pageSize := db.usableSize + db.reserved
	if pageSize <= 0 {
		pageSize = db.pageSize
	}
	rest := payloadLen - maxLocal
	primary := maxLocal + rest%(pageSize-4) + 4
	add(primary)
	for _, d := range []int{-1, 1, -2, 2, -4, 4, -8, 8, -16, 16} {
		add(primary + d)
	}
	add(maxLocal + rest%(pageSize-8) + 8)
	add(maxLocal + rest%(pageSize-8))
	add(maxLocal + rest%(pageSize-4))

	// 3) 穷举兜底：把页内所有可能的切分点都试一遍（每页最多几百个，
	// 每个候选只做几次内存内的指针比较）。正确性由此**不依赖任何公式**：
	// 只有真实切分能让溢出链正好用完，其余会在 readOverflowChain 里被否掉。
	for size := 4; size <= maxPossible && size <= payloadLen; size++ {
		add(size)
	}

	var out [][]byte
	var lastErr error
	for _, size := range sizes {
		payload, err := db.readOverflowChain(page[off:off+size], payloadLen)
		if err != nil {
			if lastErr == nil {
				lastErr = fmt.Errorf("局部=%d: %w", size, err)
			}
			continue
		}
		out = append(out, payload)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("溢出载荷无法组装（载荷 %d 字节，页内最多 %d；首个失败 %v）",
			payloadLen, maxPossible, lastErr)
	}
	return out, nil
}

// chainDataBytes 顺着溢出页链走到底（末页指针为 0），返回链上的数据字节数。
// 链上字节数必须小于载荷长度，否则说明这个起点不是真的溢出页。
func (db *DB) chainDataBytes(first, payloadLen int) (int, bool) {
	perPage := db.usableSize - 4
	if perPage <= 0 {
		return 0, false
	}
	total := 0
	pg := first
	for i := 0; i <= payloadLen/perPage+2; i++ {
		if pg <= 0 || pg > db.pages {
			return 0, false
		}
		p, err := db.page(pg)
		if err != nil {
			return 0, false
		}
		total += perPage
		next := int(binary.BigEndian.Uint32(p[0:4]))
		if next == 0 {
			if total >= payloadLen {
				return 0, false
			}
			return total, true
		}
		pg = next
	}
	return 0, false
}

// firstOverflowPageFromCell 从"局部末尾 4 字节"的**每个可能位置**里，挑出真正像
// 首个溢出页号的那个：它的链走完后，链上字节数必须正好等于 (载荷 - 该位置)。
// 纯结构判据，不依赖"局部长度怎么算"的任何公式；搜索方向从"页内尽量多放"往下退，
// 与 SQLite 的填充方向一致（先命中真实切分点，避免被随机字节抢先）。
// 返回 (首个溢出页号, 局部长度, 是否找到)。
func (db *DB) firstOverflowPageFromCell(page []byte, off, payloadLen, maxPossible int) (int, int, bool) {
	if db.usableSize <= 4 {
		return 0, 0, false
	}
	hi := maxPossible
	if hi > payloadLen-1 {
		hi = payloadLen - 1
	}
	for size := hi; size >= 4; size-- {
		at := off + size - 4
		if at < 0 || at+4 > len(page) {
			continue
		}
		v := int(binary.BigEndian.Uint32(page[at : at+4]))
		if v < 2 || v > db.pages {
			continue
		}
		if got, ok := db.chainDataBytes(v, payloadLen); ok && got == payloadLen-size {
			return v, size, true
		}
	}
	return 0, 0, false
}

// readOverflowChain 把页内片段与溢出页链拼成完整载荷。
//
// 判据严格到"链必须正好用完"：最后一页的数据长度正好等于剩余字节数，
// 且该页的"下一页"指针为 0。只检查"读到的字节数够不够"会把载荷里随机出现的
// 4 字节误当成页号（实测踩过：本地切法因此被整体判断偏了 60 多字节）。
func (db *DB) readOverflowChain(local []byte, payloadLen int) ([]byte, error) {
	if len(local) < 4 || len(local) > payloadLen {
		return nil, fmt.Errorf("局部长度非法: %d", len(local))
	}
	perPage := db.usableSize - 4
	remaining := payloadLen - len(local)
	if remaining < 0 {
		return nil, fmt.Errorf("残留长度为负")
	}
	buf := make([]byte, 0, payloadLen)
	buf = append(buf, local...)
	if remaining == 0 {
		return buf, nil
	}

	next := int(binary.BigEndian.Uint32(local[len(local)-4:]))
	guard := 0
	for remaining > 0 {
		guard++
		if guard > payloadLen/perPage+8 {
			return nil, fmt.Errorf("溢出页链过长（可能存在环）")
		}
		if next <= 0 {
			return nil, fmt.Errorf("溢出页链提前结束（还差 %d 字节）", remaining)
		}
		op, err := db.page(next)
		if err != nil {
			return nil, err
		}
		n := remaining
		if n > perPage {
			n = perPage
		}
		if 4+n > len(op) {
			return nil, fmt.Errorf("溢出页 %d 越界", next)
		}
		buf = append(buf, op[4:4+n]...)
		remaining -= n
		next = int(binary.BigEndian.Uint32(op[0:4]))
		if remaining == 0 && next != 0 {
			// 链在这里结束，指针却还指向下一页 —— 说明这不是真实的切分点
			return nil, fmt.Errorf("溢出链在载荷用尽处仍未结束（下一页 %d）", next)
		}
	}
	return buf, nil
}

// readPayload 组装载荷（单候选便捷入口，测试与诊断用）
func (db *DB) readPayload(page []byte, off, payloadLen, pageType int) ([]byte, error) {
	cands, err := db.payloadCandidates(page, off, payloadLen, pageType)
	if err != nil {
		return nil, err
	}
	return cands[0], nil
}

// decodeRecord 解码一条记录：headerSize(varint) + serial types + body
func (db *DB) decodeRecord(payload []byte) ([]Value, error) {
	headerLen64, n := getVarint(payload)
	headerLen := int(headerLen64)
	if n <= 0 || headerLen < n || headerLen > len(payload) {
		return nil, fmt.Errorf("记录头长度非法: %d", headerLen64)
	}
	serialTypes := make([]int64, 0, 8)
	pos := n
	for pos < headerLen {
		st, m := getVarint(payload[pos:])
		if m <= 0 {
			return nil, fmt.Errorf("serial type 畸形")
		}
		serialTypes = append(serialTypes, st)
		pos += m
	}

	body := headerLen
	vals := make([]Value, 0, len(serialTypes))
	for _, st := range serialTypes {
		size := serialTypeLen(st)
		if size < 0 {
			return nil, fmt.Errorf("未知 serial type: %d", st)
		}
		if size > len(payload) || body > len(payload)-size {
			return nil, fmt.Errorf("记录体越界（serial=%d 需要 %d，剩余 %d）", st, size, len(payload)-body)
		}
		raw := payload[body : body+size]
		body += size

		switch {
		case st == 0:
			vals = append(vals, nil)
		case st >= 1 && st <= 6:
			vals = append(vals, int64(intBE(raw)))
		case st == 7:
			vals = append(vals, math.Float64frombits(binary.BigEndian.Uint64(raw)))
		case st == 8:
			vals = append(vals, int64(0))
		case st == 9:
			vals = append(vals, int64(1))
		case st >= 12 && st%2 == 0:
			vals = append(vals, append([]byte(nil), raw...)) // BLOB
		default: // 13 及以上的奇数：TEXT
			vals = append(vals, db.decodeText(raw))
		}
	}
	return vals, nil
}

// decodeText 按库的文本编码解码（Chrome/Edge 库都是 UTF-8，UTF-16 分支为兼容）
func (db *DB) decodeText(raw []byte) string {
	if db.encoding == 1 {
		return string(raw)
	}
	u16 := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		u16 = append(u16, binary.BigEndian.Uint16(raw[i:i+2]))
	}
	return string(utf16.Decode(u16))
}

// ===== 基础工具 =====

// getVarint 读 SQLite 的 1~9 字节变长整数，返回 (值, 消耗字节数)
func getVarint(b []byte) (int64, int) {
	var v uint64
	for i := 0; i < 9 && i < len(b); i++ {
		c := b[i]
		if i == 8 {
			v = (v << 8) | uint64(c)
			return int64(v), 9
		}
		v = (v << 7) | uint64(c&0x7F)
		if c < 0x80 {
			return int64(v), i + 1
		}
	}
	return 0, 0
}

// serialTypeLen serial type 对应的字节数（-1 = 非法）
func serialTypeLen(st int64) int {
	switch {
	case st == 0 || st == 8 || st == 9:
		return 0
	case st >= 1 && st <= 4:
		return int(st)
	case st == 5:
		return 6
	case st == 6 || st == 7:
		return 8
	case st >= 12 && st%2 == 0:
		return int((st - 12) / 2)
	case st >= 13 && st%2 == 1:
		return int((st - 13) / 2)
	default:
		return -1
	}
}

// intBE 大端有符号整数（SQLite 的 1/2/3/4/6 字节整数都是大端补码）
func intBE(b []byte) int64 {
	var v int64
	for _, c := range b {
		v = (v << 8) | int64(c)
	}
	if len(b) > 0 && len(b) < 8 && b[0]&0x80 != 0 {
		v -= int64(1) << (8 * uint(len(b)))
	}
	return v
}

func asString(v Value) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}

func asInt(v Value) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	default:
		return 0
	}
}

// parseColumnNames 从建表 SQL 里抽列名。
//
// 只做括号内顶层逗号切分：跳过约束子句（PRIMARY KEY / UNIQUE / FOREIGN KEY /
// CHECK / CONSTRAINT），列名取第一段标识符（去掉引号/方括号/反引号）。
// Chrome/Edge 的建表语句形态固定（列定义 + PRIMARY KEY 后缀），这套解析足够；
// 解析不出任何列名时调用方会显式报错，而不会静默按位置错位读值。
func parseColumnNames(sql string) []string {
	var cols []string
	for _, part := range splitTopLevel(innerColumns(sql)) {
		upper := strings.ToUpper(part)
		skip := false
		for _, kw := range []string{"PRIMARY ", "UNIQUE", "CHECK", "FOREIGN ", "CONSTRAINT ", "KEY "} {
			if strings.HasPrefix(upper, kw) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if name := firstIdentifier(part); name != "" {
			cols = append(cols, name)
		}
	}
	return cols
}

// innerColumns 取建表语句最外层括号内的内容（没有括号时返回空）
func innerColumns(sql string) string {
	start := strings.Index(sql, "(")
	end := strings.LastIndex(sql, ")")
	if start < 0 || end <= start {
		return ""
	}
	return sql[start+1 : end]
}

// splitTopLevel 按顶层逗号切分（忽略括号内与引号内的逗号），并去掉每段首尾空白
func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	inStr := byte(0)
	var cur strings.Builder
	flush := func() {
		part := strings.TrimSpace(cur.String())
		cur.Reset()
		if part != "" {
			parts = append(parts, part)
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			cur.WriteByte(c)
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			inStr = c
			cur.WriteByte(c)
		case '[':
			inStr = ']'
			cur.WriteByte(c)
		case '(':
			depth++
			cur.WriteByte(c)
		case ')':
			depth--
			cur.WriteByte(c)
		case ',':
			if depth == 0 {
				flush()
				continue
			}
			cur.WriteByte(c)
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return parts
}

// firstIdentifier 取一段列定义里的标识符（支持 "x" / [x] / `x` / x）
func firstIdentifier(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return ""
	}
	switch part[0] {
	case '"', '`':
		q := part[0]
		if i := strings.IndexByte(part[1:], q); i > 0 {
			return part[1 : 1+i]
		}
	case '[':
		if i := strings.IndexByte(part, ']'); i > 1 {
			return part[1:i]
		}
	}
	for i := 0; i < len(part); i++ {
		switch part[i] {
		case ' ', '\t', '\n', '\r', '(':
			return part[:i]
		}
	}
	return part
}

#!/usr/bin/env python3
"""精确测量溢出单元格的"页内局部长度"，并拟合出规则。

关键修正：**局部长度 = 页内数据字节数**，而载荷包含记录头（record header），
所以要用"记录头自证"把两者分开 —— 早期版本把"局部"当成载荷里的一段，
量出来的值整体偏小（差了整个记录头长度），据此拟合出的公式自然对不上。

用法：python scripts/fit_sqlite_overflow.py
"""
import os
import sqlite3
import struct
import sys
import tempfile


def build(path, page_size, blob_len, seed=7):
    for suffix in ("", "-wal", "-shm"):
        if os.path.exists(path + suffix):
            os.remove(path + suffix)
    con = sqlite3.connect(path)
    con.execute("PRAGMA page_size=%d" % page_size)
    con.execute("PRAGMA journal_mode=DELETE")
    con.execute("CREATE TABLE t (id INTEGER PRIMARY KEY, v BLOB)")
    con.execute("INSERT INTO t (id, v) VALUES (1, ?)",
                (bytes((j * seed + 11) % 256 for j in range(blob_len)),))
    con.commit()
    con.close()
    with open(path, "rb") as f:
        return f.read()


def varint(b, pos):
    v = 0
    for i in range(9):
        c = b[pos + i]
        if i == 8:
            return (v << 8) | c, 9
        v = (v << 7) | (c & 0x7F)
        if c < 0x80:
            return v, i + 1
    raise ValueError("bad varint")


def serial_size(st):
    if st in (0, 8, 9):
        return 0
    if 1 <= st <= 4:
        return st
    if st == 5:
        return 6
    if st in (6, 7):
        return 8
    if st >= 12 and st % 2 == 0:
        return (st - 12) // 2
    if st >= 13 and st % 2 == 1:
        return (st - 13) // 2
    return -1


def header_len(payload):
    """记录头长度（第一个 varint），非法返回 0"""
    hlen, n = varint(payload, 0)
    if hlen < n or hlen > len(payload):
        return 0
    pos = n
    while pos < hlen:
        st, m = varint(payload, pos)
        if serial_size(st) < 0:
            return 0
        pos += m
    return hlen


def page_bytes(data, page_size, pgno):
    return data[(pgno - 1) * page_size: pgno * page_size]


def measure(data, page_size):
    """返回 (载荷, 链上数据字节, 局部数据字节=载荷-链, usable, 溢出页数)"""
    usable = page_size - data[20]
    per = usable - 4
    total_pages = len(data) // page_size
    for pg in range(1, total_pages + 1):
        page = page_bytes(data, page_size, pg)
        hdr = 100 if pg == 1 else 0
        if page[hdr] != 13:
            continue
        n = struct.unpack(">H", page[hdr + 3:hdr + 5])[0]
        if n > 512:
            continue
        for i in range(n):
            off = struct.unpack(">H", page[hdr + 8 + i * 2:hdr + 10 + i * 2])[0]
            if off <= 0 or off >= page_size:
                continue
            plen, n1 = varint(page, off)
            if plen <= 1024:
                continue
            _, n2 = varint(page, off + n1)
            start = off + n1 + n2
            # 第一页指针（局部末尾 4 字节）→ 跟着链走，记录链上数据字节
            for loc in range(4, min(page_size - start, plen) + 1):
                at = start + loc - 4
                if at + 4 > page_size:
                    break
                first = struct.unpack(">I", page[at:at + 4])[0]
                if not (2 <= first <= total_pages):
                    continue
                chain_bytes, cur, ok, pages = 0, first, True, 0
                while chain_bytes < plen - loc:
                    if not (1 <= cur <= total_pages) or pages > 8192:
                        ok = False
                        break
                    p = page_bytes(data, page_size, cur)
                    nxt = struct.unpack(">I", p[0:4])[0]
                    pages += 1
                    take = min(per, plen - loc - chain_bytes)
                    chain_bytes += take
                    if chain_bytes >= plen - loc:
                        # 链正好用完：末页指针必须为 0
                        ok = nxt == 0
                        break
                    cur = nxt
                if not ok or chain_bytes != plen - loc:
                    continue
                local_data = loc
                # 记录头必须在页内（局部数据里），用它自证
                payload = bytes(page[start:start + loc])
                hlen = header_len(payload)
                if hlen <= 0:
                    continue
                return plen, chain_bytes, local_data, usable, pages, hlen
    return None


def main():
    tmpdir = tempfile.mkdtemp(prefix="sqlite_fit_")
    path = os.path.join(tmpdir, "fit.db")
    page_sizes = [1024, 2048, 4096, 8192]
    blob_lens = [1500, 3000, 5000, 8100, 12000, 20000, 30000, 45005, 60000]
    rows = []
    print(f"{'页':>6}{'usable':>8}{'maxL':>6}{'载荷':>8}{'链字节':>8}{'局部':>7}"
          f"{'记录头':>7}{'局部-头':>8}{'rest':>8}{'rest%P':>8}{'rest%(P-8)':>11}{'页数':>5}")
    for ps in page_sizes:
        for bl in blob_lens:
            try:
                data = build(path, ps, bl)
            except sqlite3.Error:
                continue
            m = measure(data, ps)
            if not m:
                continue
            plen, chain, local, usable, pages, hlen = m
            max_local = min((usable - 35) * 64 // 255, 1024)
            rest = plen - max_local
            print(f"{ps:>6}{usable:>8}{max_local:>6}{plen:>8}{chain:>8}{local:>7}"
                  f"{hlen:>7}{local-hlen:>8}{rest:>8}{rest % ps:>8}{rest % (ps - 8):>11}{pages:>5}")
            rows.append(dict(ps=ps, usable=usable, max_local=max_local, plen=plen,
                             chain=chain, local=local, hlen=hlen, rest=rest,
                             local_data=local - hlen))

    print("\n候选公式命中（比较『局部数据字节』= 局部 - 记录头）：")
    cands = [
        ("maxL + rest % (P-8) + 8", lambda r: r["max_local"] + r["rest"] % (r["ps"] - 8) + 8),
        ("maxL + rest % (P-8)", lambda r: r["max_local"] + r["rest"] % (r["ps"] - 8)),
        ("maxL + rest % (P-4) + 4", lambda r: r["max_local"] + r["rest"] % (r["ps"] - 4) + 4),
        ("maxL + rest % (P-4)", lambda r: r["max_local"] + r["rest"] % (r["ps"] - 4)),
        ("maxL + rest % P", lambda r: r["max_local"] + r["rest"] % r["ps"]),
        ("maxL + rest % (usable-8) + 8", lambda r: r["max_local"] + r["rest"] % (r["usable"] - 8) + 8),
        ("maxL + rest % (usable-8)", lambda r: r["max_local"] + r["rest"] % (r["usable"] - 8)),
        ("maxL + rest % (usable-4)", lambda r: r["max_local"] + r["rest"] % (r["usable"] - 4)),
    ]
    for name, fn in cands:
        hits = sum(1 for r in rows if fn(r) == r["local"])
        print(f"  {name:32s} {hits}/{len(rows)}")


if __name__ == "__main__":
    sys.exit(main())

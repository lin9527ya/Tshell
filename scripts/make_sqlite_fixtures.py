#!/usr/bin/env python3
"""生成植入体 SQLite 读取器的测试样本（implant/core/sqlite/testdata/）。

为什么要脚本而不是"随便塞几个 .db 进去"：样本是二进制，但**构造它的代码**必须留在
仓库里 —— 否则以后没人知道这些文件里到底有什么、期望值从哪来。样本很小（几十 KB），
随仓库提供，克隆即可跑测试（`.gitignore` 对 testdata 目录做了 *.db 例外）。

样本清单（与 core/sqlite/reader_test.go 的断言一一对应）：

| 文件 | 用途 |
|---|---|
| logins_cookies.db | 常规路径：Chrome 的 logins / cookies 表结构 + 多字节 UTF-8 |
| overflow.db       | 跨页大 BLOB：走溢出页链 |
| walonly.db(+wal)  | 主库还是旧快照、最新数据只在 WAL 里 |
| truncated.db      | 截断的库：必须显式报错，不能返回半截数据 |
| not_a_db.db       | 非 SQLite 文件：必须被拒 |

用法：python scripts/make_sqlite_fixtures.py
"""
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.normpath(os.path.join(
    HERE, "..", "Tshell-client", "implant", "core", "sqlite", "testdata"))

# 与 Go 侧测试共享的期望值（改这里必须同步改 reader_test.go）
# logins: (id, origin_url, username_value, password_value, date_created, signon_realm)
LOGINS = [
    (1, "https://portal.corp.local/login", "alice",
     b"v10\x00encrypted-password-1", 13300000000000000, "https://portal.corp.local/"),
    (2, "https://mail.corp.local/", "张三",
     b"v20\x00encrypted-password-2", 13300000001000000, "https://mail.corp.local/"),
    (3, "https://vpn.example.com/", "bob",
     b"plain-old-format", 13300000002000000, "https://vpn.example.com/"),
]
# cookies: (creation_utc, host_key, name, encrypted_value, path,
#           expires_utc, is_secure, is_httponly)
COOKIES = [
    (1, ".corp.local", "SESSIONID", b"v10\x00cookie-value-1", "/", 13399999999000000, 1, 1),
    (2, ".example.com", "csrftoken", b"v20\x00cookie-value-2", "/app", 13399999999000001, 0, 0),
]
BIG_BLOB_LEN = 30000
BIG_BLOB_SEED = 7  # 生成式 (i*7+11)%256，Go 侧逐字节对拍同一个式子

# 与 Chrome/Edge 真实 schema 一致（列名 + NOT NULL/DEFAULT 都要对得上，
# 否则建表语句解析与记录编码的形态就与真实库不同，测了等于没测）
SCHEMA_LOGINS = """
CREATE TABLE logins (
    origin_url VARCHAR NOT NULL,
    action_url VARCHAR,
    username_element VARCHAR,
    username_value VARCHAR,
    password_value BLOB,
    submit_element VARCHAR,
    signon_realm VARCHAR NOT NULL,
    date_created INTEGER NOT NULL,
    blacklisted_by_user INTEGER NOT NULL DEFAULT 0,
    scheme INTEGER NOT NULL DEFAULT 0,
    password_type INTEGER,
    possible_username_pairs BLOB,
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date_last_used INTEGER,
    moving_blocked_for BLOB,
    date_password_modified INTEGER
)
"""
SCHEMA_COOKIES = """
CREATE TABLE cookies (
    creation_utc INTEGER NOT NULL,
    host_key TEXT NOT NULL,
    name TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL,
    expires_utc INTEGER NOT NULL,
    is_secure INTEGER NOT NULL,
    is_httponly INTEGER NOT NULL,
    last_access_utc INTEGER NOT NULL,
    has_expires INTEGER NOT NULL DEFAULT 1,
    is_persistent INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 1,
    encrypted_value BLOB DEFAULT '',
    samesite INTEGER NOT NULL DEFAULT -1,
    source_scheme INTEGER NOT NULL DEFAULT 0,
    last_update_utc INTEGER NOT NULL DEFAULT 0
)
"""


def connect(path, page_size=4096, wal=True):
    for suffix in ("", "-wal", "-shm"):
        if os.path.exists(path + suffix):
            os.remove(path + suffix)
    con = sqlite3.connect(path)
    con.execute("PRAGMA page_size=%d" % page_size)
    con.execute("PRAGMA journal_mode=%s" % ("WAL" if wal else "DELETE"))
    con.execute(SCHEMA_LOGINS)
    con.execute(SCHEMA_COOKIES)
    return con


def insert_rows(con):
    con.executemany(
        "INSERT INTO logins (id, origin_url, username_value, password_value,"
        " date_created, signon_realm) VALUES (?,?,?,?,?,?)", LOGINS)
    for row in COOKIES:
        con.execute(
            "INSERT INTO cookies (creation_utc, host_key, name, encrypted_value,"
            " path, expires_utc, is_secure, is_httponly, last_access_utc)"
            " VALUES (?,?,?,?,?,?,?,?,?)",
            (row[0], row[1], row[2], row[3], row[4], row[5], row[6], row[7], row[5]))


def copy_out(con, dest_dir, name, *, with_wal=False, checkpoint=True):
    """落盘：必须在 close() 之前复制 —— SQLite 关闭最后一个连接时会自动
    checkpoint 并删除 -wal，关掉再找 -wal 只会得到"文件不存在"。"""
    os.makedirs(dest_dir, exist_ok=True)
    src = con.execute("PRAGMA database_list").fetchone()[2]
    con.commit()
    if checkpoint:
        con.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    shutil.copyfile(src, os.path.join(dest_dir, name))
    for suffix in ("-wal",):
        target = os.path.join(dest_dir, name + suffix)
        if with_wal and os.path.exists(src + suffix):
            shutil.copyfile(src + suffix, target)
        elif os.path.exists(target):
            os.remove(target)
    # -shm 是共享内存索引，读取器不解析，不入库
    stale = os.path.join(dest_dir, name + "-shm")
    if os.path.exists(stale):
        os.remove(stale)
    con.close()
    for suffix in ("", "-wal", "-shm"):
        if os.path.exists(src + suffix):
            os.remove(src + suffix)


def make_main(dest_dir):
    tmp = os.path.join(dest_dir, "_tmp_main.db")
    con = connect(tmp)
    insert_rows(con)
    copy_out(con, dest_dir, "logins_cookies.db")


def make_overflow(dest_dir):
    tmp = os.path.join(dest_dir, "_tmp_overflow.db")
    con = connect(tmp)
    insert_rows(con)
    big = b"v10\x00" + bytes((i * BIG_BLOB_SEED + 11) % 256 for i in range(BIG_BLOB_LEN))
    con.execute(
        "INSERT INTO cookies (creation_utc, host_key, name, encrypted_value, path,"
        " expires_utc, is_secure, is_httponly, last_access_utc)"
        " VALUES (?,?,?,?,?,?,?,?,?)",
        (999, ".big.example", "BIGCOOKIE", big, "/",
         13400000000000000, 1, 1, 13400000000000000))
    copy_out(con, dest_dir, "overflow.db")
    return len(big)


def make_wal_only(dest_dir):
    """内容只在 WAL 里：主库保持 1 页（旧快照）"""
    tmp = os.path.join(dest_dir, "_tmp_walonly.db")
    con = connect(tmp)
    con.execute("PRAGMA wal_autocheckpoint=0")  # 别让 SQLite 自己把 WAL 并回主库
    insert_rows(con)
    copy_out(con, dest_dir, "walonly.db", with_wal=True, checkpoint=False)


def make_broken(dest_dir):
    src = os.path.join(dest_dir, "logins_cookies.db")
    with open(src, "rb") as f:
        raw = f.read()
    with open(os.path.join(dest_dir, "truncated.db"), "wb") as f:
        f.write(raw[: len(raw) // 3])
    with open(os.path.join(dest_dir, "not_a_db.db"), "wb") as f:
        f.write(b"PK\x03\x04 this is a zip, not sqlite")


def main():
    os.makedirs(OUT, exist_ok=True)
    make_main(OUT)
    big_len = make_overflow(OUT)
    make_wal_only(OUT)
    make_broken(OUT)

    print("fixtures 已生成到 %s（大 BLOB 载荷 %d 字节）" % (OUT, big_len))
    for name in sorted(os.listdir(OUT)):
        p = os.path.join(OUT, name)
        if os.path.isfile(p):
            print("  %-24s %8d 字节" % (name, os.path.getsize(p)))
    return 0


if __name__ == "__main__":
    sys.exit(main())

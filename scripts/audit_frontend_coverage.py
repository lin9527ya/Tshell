#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Tshell 后端 → 前端覆盖审计脚本（只读，不修改任何文件）。

用法:
    python3 scripts/audit_frontend_coverage.py

输出:
    1. 后端 main.go 中重建出的全部注册路由数量；
    2. 无前端 API 封装匹配的路由（并给出已知等价实现/公开路由/已下线路由说明）；
    3. 前端 API 封装中零调用的方法（并给出等价实现/兼容别名说明）。
"""
import glob
import os
import re

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..'))
SERVER_MAIN = os.path.join(ROOT, 'Tshell-server', 'main.go')
WEB_SRC = os.path.join(ROOT, 'Tshell-web', 'src')


# --------------------------------------------------------------------------- #
# 1. 解析后端 main.go：重建 group 前缀 + 全部方法路由
# --------------------------------------------------------------------------- #
def parse_backend_routes(path):
    lines = open(path, encoding='utf-8', errors='ignore').read().splitlines()
    groups = {}
    routes = []

    for line in lines:
        stripped = line.lstrip()
        if stripped.startswith('//'):
            continue

        # var := r.Group("/api") / var := parent.Group("")
        m = re.search(r'(\w+)\s*:=\s*r\.Group\("([^"]*)"\)', line)
        if m:
            groups[m.group(1)] = m.group(2)
            continue
        m = re.search(r'(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"\)', line)
        if m:
            name, parent, sub = m.groups()
            groups[name] = groups.get(parent, '') + sub
            continue

        # r.Any("/d/*filepath", handler) 这类公开投递路由
        m = re.search(r'\br\.Any\("([^"]+)"', line)
        if m:
            routes.append(('Any', m.group(1)))
            continue

        # identifier.METHOD("path", handler)
        m = re.search(r'\b([A-Za-z_]\w*)\.(GET|POST|PUT|DELETE|PATCH|Any)\("([^"]+)"', line)
        if m:
            var, method, path = m.groups()
            if var == 'r':
                full = path
            else:
                full = groups.get(var, '') + path
            routes.append((method, full))

    # 去重，保持发现顺序
    return list(dict.fromkeys(routes))


def canonical_route(route):
    """把动态段统一为 *，便于和前端字符串比对。"""
    if route.startswith('/api'):
        route = route[4:]
    route = re.sub(r':\w+', '*', route)
    route = re.sub(r'\$\{[^}]*\}', '*', route)
    route = re.sub(r'encodeURIComponent\([^)]*\)', '*', route)
    route = route.rstrip('/')
    return route or '/'


def route_skeleton(route):
    """去掉动态段后的静态骨架片段（用于模板字符串拆分比对）。"""
    r = canonical_route(route)
    r = re.sub(r'\*+', '||', r)
    return [seg.strip('/') for seg in r.split('||') if seg.strip('/')]


# --------------------------------------------------------------------------- #
# 2. 解析前端 API 封装：方法名 -> URL 字符串
# --------------------------------------------------------------------------- #
METHOD_RE = re.compile(
    r'^\s*'
    r'(?:export\s+)?'
    r'(?:(?:async\s+)?function\s+([A-Za-z_]\w*)\s*\(|'
    r'(?:const|let|var)\s+([A-Za-z_]\w*)\s*=\s*(?:async\s*)?\([^)]*\)\s*=>|'
    r'(?:async\s+)?([A-Za-z_]\w*)\s*\()'
)
URL_RE = re.compile(r"['\"`](/[^'\"`]*)['\"`]")
KEYWORDS = {'if', 'for', 'while', 'switch', 'catch', 'return', 'function'}


def parse_frontend_api():
    wrappers = []  # (api_file, method, canonical_url)
    for f in glob.glob(os.path.join(WEB_SRC, 'api', '**', '*.ts'), recursive=True):
        rel = os.path.relpath(f, WEB_SRC)
        current = '(top-level)'
        for line in open(f, encoding='utf-8', errors='ignore').read().splitlines():
            m = METHOD_RE.match(line)
            if m:
                name = next(g for g in m.groups() if g)
                if name not in KEYWORDS:
                    current = name
                continue
            for u in URL_RE.findall(line):
                wrappers.append((rel, current, canonical_route(u)))
    return wrappers


def collect_non_api_text():
    text_by_file = {}
    for f in glob.glob(os.path.join(WEB_SRC, '**', '*.vue'), recursive=True):
        text_by_file[os.path.relpath(f, WEB_SRC)] = open(f, encoding='utf-8', errors='ignore').read()
    for f in glob.glob(os.path.join(WEB_SRC, '**', '*.ts'), recursive=True):
        rel = os.path.relpath(f, WEB_SRC)
        if rel.startswith('api'):
            continue
        text_by_file[rel] = open(f, encoding='utf-8', errors='ignore').read()
    return text_by_file


def collect_all_frontend_text():
    """全部前端源码（含 api 目录），用于识别 raw fetch/WebSocket 直连消费。"""
    parts = []
    for f in glob.glob(os.path.join(WEB_SRC, '**', '*.vue'), recursive=True):
        parts.append(open(f, encoding='utf-8', errors='ignore').read())
    for f in glob.glob(os.path.join(WEB_SRC, '**', '*.ts'), recursive=True):
        parts.append(open(f, encoding='utf-8', errors='ignore').read())
    return '\n'.join(parts)


# --------------------------------------------------------------------------- #
# 3. 判定与输出
# --------------------------------------------------------------------------- #
KNOWN_NO_WRAPPER = {
    ('Any', '/d/*filepath'): '公开 Delivery Hub 路由，URL 由后端返回、前端展示（WebDelivery/WebLog）',
    ('GET', '/api/terminal/connect'): '服务器本机终端已下线（后端固定返回 410 Gone），前端无需对接',
    ('GET', '/api/terminal/list'): '服务器本机终端已下线（后端固定返回 410 Gone），前端无需对接',
    ('DELETE', '/api/terminal/close/:uid'): '服务器本机终端已下线（后端固定返回 410 Gone），前端无需对接',
    ('GET', '/api/shell/:uid'): '客户端 PTY 终端等价能力由 /api/xterm/connect 承接（LinuxInteractiveTerminal）',
    ('GET', '/tencent/mcp/pc/pcsearch'): 'main.go 中整段被注释，未真正注册',
    ('POST', '/tencent/sensearch/collection/item/check'): 'main.go 中整段被注释，未真正注册',
}

KNOWN_ZERO_USAGE = {
    'getListenerKeyConfig': 'getAllListenerKeyConfigs 已覆盖同一面板需求（等价实现）',
    'deleteClient': 'batch_delete_clients 单元素兼容别名',
    'exit_client': 'do_exit 兼容别名',
    'update_note': 'add_uid_note 兼容别名',
    'update_sleep': 'edit_sleep 兼容别名',
    # 以下封装在非 api 目录零调用，但按 docs/UI_SHELL_CONTRACT.md，
    # src/api 是 main/vshell 两分支共享的内核、对外方法不得删除或改名，
    # 因此保留封装并在此登记原因（而不是当死代码删掉）。
    'do_exit': 'exit_client 内部调用（别名），并非真正零调用',
    'select_color': '会话行颜色功能：CS 外壳使用，vshell 外壳暂未接入；main 分支仍需要',
    'setClientColor': '同 select_color（指向同一后端 /client/color）',
    'batchDeleteListeners': '监听器页暂无多选批量删除 UI；后端 /listener/batch-delete 保留',
    'get_graph_data': '拓扑图尚无 vshell 页面；后端 /graph/data 能力保留',
}


def main():
    routes = parse_backend_routes(SERVER_MAIN)
    wrappers = parse_frontend_api()
    non_api = collect_non_api_text()
    all_text = collect_all_frontend_text()

    print('=' * 78)
    print('Tshell 后端 → 前端覆盖审计')
    print('=' * 78)
    print(f'后端注册路由（去重后）: {len(routes)}')
    print(f'前端 API 封装 URL 字符串: {len(wrappers)}')

    # --- 路由层 ---
    missing_routes = []
    direct_routes = []
    for method, route in routes:
        if (method, route) in KNOWN_NO_WRAPPER:
            continue
        skeleton = route_skeleton(route)
        if not skeleton:
            continue
        matched = any(all(seg in url for seg in skeleton) for _, _, url in wrappers)
        if matched:
            continue
        # 允许 stores/utils 中 raw request 或 WebSocket 直连（如 /users/refresh、/events/ws）
        if all(seg in all_text for seg in skeleton):
            direct_routes.append((method, route))
            continue
        missing_routes.append((method, route))

    print()
    print('--- 路由层缺口 ---')
    if missing_routes:
        print(f'发现 {len(missing_routes)} 条后端路由既无 API 封装也无任何前端源码引用:')
        for method, route in missing_routes:
            print(f'  [{method}] {route}')
    else:
        print('无。全部已注册 API 路由均有前端消费点（API 封装或 raw request/WebSocket 直连）。')

    if direct_routes:
        print(f'其中 {len(direct_routes)} 条路由不经 api 目录封装、由 store/util/视图直连:')
        for method, route in direct_routes:
            print(f'  [{method}] {route}')

    # --- 方法调用层 ---
    zero_usage = []
    for f, method, url in wrappers:
        hit = sum(1 for text in non_api.values() if re.search(r'\b' + re.escape(method) + r'\b', text))
        if hit == 0:
            zero_usage.append((f, method, url))

    print()
    print('--- 方法调用层缺口 ---')
    unknown = [(f, m, u) for f, m, u in zero_usage if m not in KNOWN_ZERO_USAGE]
    if unknown:
        print(f'发现 {len(unknown)} 个 API 方法在非 api 目录零调用:')
        for f, m, u in unknown:
            print(f'  {f}::{m}  ->  {u}')
    else:
        print('无真正缺口。零调用方法均为文档说明中的等价实现/兼容别名:')
        for f, m, u in zero_usage:
            print(f'  {f}::{m}  ({KNOWN_ZERO_USAGE.get(m, "")})')
        if not zero_usage:
            print('  （全部 API 方法均被视图/布局调用）')

    print()
    print('=' * 78)


if __name__ == '__main__':
    main()

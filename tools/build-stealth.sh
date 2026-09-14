#!/bin/bash

# Tshell 隐匿构建脚本 (Linux/macOS)
# 用于去除二进制文件中的调试信息、路径信息和敏感字符串

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# 配置变量
SERVER_DIR="Tshell-server"
CLIENT_DIR="Tshell-client"
OUTPUT_DIR="release"

# 默认参数
TARGET="all"
PLATFORM=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH="amd64"
CLEAN=false
VERIFY=false

# 日志函数
log_info() {
    echo -e "${CYAN}[信息] $(date '+%H:%M:%S') - $1${NC}"
}

log_success() {
    echo -e "${GREEN}[成功] $(date '+%H:%M:%S') - $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}[警告] $(date '+%H:%M:%S') - $1${NC}"
}

log_error() {
    echo -e "${RED}[错误] $(date '+%H:%M:%S') - $1${NC}"
}

# 显示帮助信息
show_help() {
    cat << EOF
Tshell 隐匿构建脚本 (Linux/macOS)

用法: $0 [选项]

选项:
  -t, --target      构建目标 (all, server, clients) [默认: all]
  -p, --platform    目标平台 (linux, darwin, windows) [默认: 当前系统]
  -a, --arch        目标架构 (amd64, 386, arm64) [默认: amd64]
  -c, --clean       构建前清理
  -v, --verify      验证构建结果
  -h, --help        显示此帮助信息

功能:
  • 去除构建信息和路径
  • 去除调试符号
  • 混淆字符串常量
  • 压缩二进制文件 (需要UPX)
  • 验证清理效果

示例:
  $0                                                # 构建全部
  $0 -t server -p linux                           # 构建Linux服务器
  $0 -t clients -c                                # 清理后构建客户端
  $0 -t all -p windows -a amd64 -v               # 构建Windows版本并验证
EOF
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -t|--target)
                TARGET="$2"
                shift 2
                ;;
            -p|--platform)
                PLATFORM="$2"
                shift 2
                ;;
            -a|--arch)
                ARCH="$2"
                shift 2
                ;;
            -c|--clean)
                CLEAN=true
                shift
                ;;
            -v|--verify)
                VERIFY=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# 检查工具
check_tools() {
    log_info "检查构建环境..."
    
    # 检查 Go
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装"
        exit 1
    fi
    
    local go_version=$(go version)
    log_info "Go 版本: $go_version"
    
    # 检查 UPX (可选)
    if command -v upx &> /dev/null; then
        log_info "UPX 可用，将进行二进制压缩"
        UPX_AVAILABLE=true
    else
        log_warning "UPX 未安装，跳过压缩步骤"
        UPX_AVAILABLE=false
    fi
    
    # 检查 strings 命令
    if command -v strings &> /dev/null; then
        STRINGS_AVAILABLE=true
        log_info "strings 命令可用，将进行验证"
    else
        STRINGS_AVAILABLE=false
        log_warning "strings 命令不可用，无法验证隐匿效果"
    fi
    
    log_success "环境检查完成"
}

# 清理环境
clean_environment() {
    log_info "清理构建环境..."
    
    if [[ -d "$OUTPUT_DIR" ]]; then
        rm -rf "$OUTPUT_DIR"
    fi
    mkdir -p "$OUTPUT_DIR"
    
    # 清理模块缓存
    go clean -cache -modcache
    
    log_success "环境清理完成"
}

# 生成混淆的构建参数
get_stealth_flags() {
    local timestamp=$(date +%s)
    local random_build=$(cat /dev/urandom | tr -dc 'A-Za-z0-9' | head -c 8)
    
    local ldflags=(
        "-s"  # 去除符号表
        "-w"  # 去除DWARF调试信息
        "-X 'main.version=1.0.0'"  # 设置版本信息
        "-X 'main.buildTime=$timestamp'"  # 设置构建时间
        "-X 'main.buildID=$random_build'"  # 设置随机构建ID
        "-buildid="  # 去除构建ID
    )
    
    echo "-ldflags=\"${ldflags[*]}\""
}

# 构建单个目标
build_target() {
    local source_dir="$1"
    local output_name="$2"
    local target_os="$3"
    local target_arch="$4"
    
    if [[ ! -d "$source_dir" ]]; then
        log_warning "源目录不存在: $source_dir"
        return 1
    fi
    
    log_info "构建 $output_name ($target_os/$target_arch)..."
    
    pushd "$source_dir" > /dev/null
    
    # 设置环境变量
    export GOOS="$target_os"
    export GOARCH="$target_arch"
    export CGO_ENABLED="0"
    
    # 去除模块信息
    export GOPROXY="direct"
    export GONOSUMDB="*"
    
    # 生成输出路径
    local extension=""
    if [[ "$target_os" == "windows" ]]; then
        extension=".exe"
    fi
    local output_path="../$OUTPUT_DIR/$output_name$extension"
    
    # 构建参数
    local stealth_flags=$(get_stealth_flags)
    
    log_info "执行: go build -a -trimpath -buildmode=exe $stealth_flags -o $output_path ."
    
    if go build -a -trimpath -buildmode=exe $stealth_flags -o "$output_path" .; then
        if [[ -f "$output_path" ]]; then
            local file_size=$(du -h "$output_path" | cut -f1)
            log_success "构建成功: $output_name ($file_size)"
            echo "$output_path"
        else
            log_error "构建文件不存在: $output_path"
            popd > /dev/null
            return 1
        fi
    else
        log_error "构建失败"
        popd > /dev/null
        return 1
    fi
    
    popd > /dev/null
    
    # 清理环境变量
    unset GOOS GOARCH CGO_ENABLED GOPROXY GONOSUMDB
}

# 压缩二进制文件
compress_binary() {
    local file_path="$1"
    
    if [[ "$UPX_AVAILABLE" != true ]] || [[ ! -f "$file_path" ]]; then
        return
    fi
    
    log_info "压缩二进制文件: $(basename "$file_path")"
    
    if upx --best --lzma "$file_path" >/dev/null 2>&1; then
        log_success "压缩完成"
    else
        log_warning "压缩失败"
    fi
}

# 验证清理效果
verify_stealth_build() {
    local file_path="$1"
    
    if [[ ! -f "$file_path" ]]; then
        log_error "文件不存在: $file_path"
        return
    fi
    
    if [[ "$STRINGS_AVAILABLE" != true ]]; then
        log_warning "strings 命令不可用，无法验证"
        return
    fi
    
    log_info "验证隐匿效果: $(basename "$file_path")"
    
    # 检查敏感信息
    local sensitive_patterns=(
        "go build"
        "go1\\."
        "golang\\.org"
        "github\\.com"
        "\\.go:"
        "runtime\\."
        "/Users/"
        "/home/"
        "C:\\\\Users\\\\"
        "src/runtime/"
    )
    
    local found_sensitive=()
    for pattern in "${sensitive_patterns[@]}"; do
        if strings "$file_path" | grep -q "$pattern"; then
            found_sensitive+=("$pattern")
        fi
    done
    
    if [[ ${#found_sensitive[@]} -eq 0 ]]; then
        log_success "隐匿效果良好，未发现敏感信息"
    else
        log_warning "发现以下敏感信息模式:"
        for pattern in "${found_sensitive[@]}"; do
            log_warning "  - $pattern"
        done
    fi
}

# 构建服务器
build_server() {
    local target_os="$1"
    local target_arch="$2"
    
    log_info "开始构建服务器..."
    
    local output_name="tshell-server-$target_os-$target_arch"
    local built_file
    
    if built_file=$(build_target "$SERVER_DIR" "$output_name" "$target_os" "$target_arch"); then
        compress_binary "$built_file"
        if [[ "$VERIFY" == true ]]; then
            verify_stealth_build "$built_file"
        fi
    fi
}

# 构建客户端
build_clients() {
    local target_os="$1"
    local target_arch="$2"
    
    log_info "开始构建客户端..."
    
    local clients=(
        "Reacon_tcp"
        "Reacon_kcp"
        "Reacon_http"
        "Reacon_oss"
        "Reacon_websocket"
    )
    
    for client in "${clients[@]}"; do
        local client_dir="$CLIENT_DIR/$client"
        local output_name="$client-$target_os-$target_arch"
        local built_file
        
        if built_file=$(build_target "$client_dir" "$output_name" "$target_os" "$target_arch"); then
            compress_binary "$built_file"
            if [[ "$VERIFY" == true ]]; then
                verify_stealth_build "$built_file"
            fi
        fi
    done
}

# 主函数
main() {
    log_info "Tshell 隐匿构建开始..."
    log_info "目标: $TARGET, 平台: $PLATFORM, 架构: $ARCH"
    
    check_tools
    
    if [[ "$CLEAN" == true ]]; then
        clean_environment
    elif [[ ! -d "$OUTPUT_DIR" ]]; then
        mkdir -p "$OUTPUT_DIR"
    fi
    
    case "$TARGET" in
        "server")
            build_server "$PLATFORM" "$ARCH"
            ;;
        "clients")
            build_clients "$PLATFORM" "$ARCH"
            ;;
        "all")
            build_server "$PLATFORM" "$ARCH"
            build_clients "$PLATFORM" "$ARCH"
            ;;
        *)
            log_error "未知目标: $TARGET"
            show_help
            exit 1
            ;;
    esac
    
    log_success "构建完成！文件输出到: $OUTPUT_DIR"
    
    if [[ -d "$OUTPUT_DIR" ]]; then
        log_info "构建产物:"
        for file in "$OUTPUT_DIR"/*; do
            if [[ -f "$file" ]]; then
                local size=$(du -h "$file" | cut -f1)
                log_info "  $(basename "$file") ($size)"
            fi
        done
    fi
}

# 解析参数并运行
parse_args "$@"
main
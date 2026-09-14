#!/bin/bash

# Tshell 服务端 CGO 构建脚本
# 启用 CGO 以支持 SQLite 数据库

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置变量
SERVER_DIR="Tshell-server"
OUTPUT_NAME="tshell-server"

# 日志函数
log_info() {
    echo -e "${BLUE}[信息]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[成功]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[警告]${NC} $1"
}

log_error() {
    echo -e "${RED}[错误]${NC} $1"
}

# 检查依赖
check_dependencies() {
    log_info "检查构建环境..."
    
    # 检查 Go
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装，请先安装 Go 1.19+"
        exit 1
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "Go 版本: $go_version"
    
    # 检查 C 编译器
    if ! command -v gcc &> /dev/null; then
        if ! command -v clang &> /dev/null; then
            log_error "未找到 C 编译器 (gcc 或 clang)"
            log_error "请安装开发工具:"
            if [[ "$OSTYPE" == "linux-gnu"* ]]; then
                echo "  Ubuntu/Debian: sudo apt-get install build-essential"
                echo "  CentOS/RHEL:   sudo yum groupinstall \"Development Tools\""
            elif [[ "$OSTYPE" == "darwin"* ]]; then
                echo "  macOS: xcode-select --install"
            fi
            exit 1
        else
            log_info "找到 Clang 编译器"
        fi
    else
        log_info "找到 GCC 编译器"
    fi
    
    # 检查 SQLite 开发库
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if ! pkg-config --exists sqlite3; then
            log_warning "未找到 SQLite3 开发库，可能影响编译"
            log_info "安装命令: sudo apt-get install libsqlite3-dev"
        else
            log_info "SQLite3 开发库已安装"
        fi
    fi
    
    # 检查服务端目录
    if [[ ! -d "$SERVER_DIR" ]]; then
        log_error "服务端目录不存在: $SERVER_DIR"
        exit 1
    fi
    
    log_success "构建环境检查通过"
}

# 构建服务端
build_server() {
    log_info "进入服务端目录: $SERVER_DIR"
    cd "$SERVER_DIR"
    
    # 清理旧构建
    if [[ -f "$OUTPUT_NAME" ]]; then
        log_info "清理旧的构建文件..."
        rm "$OUTPUT_NAME"
    fi
    
    # 设置 CGO 环境变量
    export CGO_ENABLED=1
    export GOOS=$(go env GOOS)
    export GOARCH=$(go env GOARCH)
    
    log_info "构建环境:"
    echo "  CGO_ENABLED=$CGO_ENABLED"
    echo "  GOOS=$GOOS"
    echo "  GOARCH=$GOARCH"
    echo
    
    # 整理依赖
    log_info "整理 Go 模块依赖..."
    go mod tidy
    
    # 下载依赖
    log_info "下载依赖包..."
    go mod download
    
    # 构建服务端 (启用 CGO)
    log_info "开始构建服务端 (启用 CGO)..."
    log_info "构建命令: go build -o $OUTPUT_NAME -ldflags=\"-s -w\" ."
    
    go build -o "$OUTPUT_NAME" -ldflags="-s -w" .
    
    # 检查构建结果
    if [[ ! -f "$OUTPUT_NAME" ]]; then
        log_error "构建文件不存在: $OUTPUT_NAME"
        exit 1
    fi
    
    # 设置执行权限
    chmod +x "$OUTPUT_NAME"
    
    # 显示构建信息
    local file_size=$(stat -f%z "$OUTPUT_NAME" 2>/dev/null || stat -c%s "$OUTPUT_NAME" 2>/dev/null || echo "未知")
    log_success "服务端构建完成！"
    echo "  文件: $OUTPUT_NAME"
    echo "  大小: $file_size 字节"
    
    cd - > /dev/null
}

# 验证构建
verify_build() {
    log_info "验证 CGO 编译..."
    
    cd "$SERVER_DIR"
    
    # 检查是否包含 SQLite 相关符号
    if command -v strings &> /dev/null; then
        if strings "$OUTPUT_NAME" | grep -q "sqlite"; then
            log_success "SQLite 支持已正确编译"
        else
            log_warning "未检测到 SQLite 相关字符串，可能编译有问题"
        fi
    fi
    
    # 尝试获取版本信息 (如果程序支持)
    if ./"$OUTPUT_NAME" --version &> /dev/null; then
        log_info "程序版本信息:"
        ./"$OUTPUT_NAME" --version || true
    elif ./"$OUTPUT_NAME" -v &> /dev/null; then
        log_info "程序版本信息:"
        ./"$OUTPUT_NAME" -v || true
    fi
    
    cd - > /dev/null
}

# 显示启动信息
show_startup_info() {
    echo
    echo "========================================"
    echo "           构建完成"
    echo "========================================"
    echo
    echo "启动命令: cd $SERVER_DIR && ./$OUTPUT_NAME -p 8089"
    echo "访问地址: http://localhost:8089"
    echo
    echo "注意事项:"
    echo "1. 确保数据库文件存在或程序有创建权限"
    echo "2. 如需修改端口，使用 -p 参数"
    echo "3. 配置文件: config.yaml"
    echo "4. 日志目录: logs/"
    echo
}

# 清理构建缓存
clean_cache() {
    log_info "清理构建缓存..."
    
    cd "$SERVER_DIR"
    go clean -cache -modcache
    cd - > /dev/null
    
    log_success "构建缓存清理完成"
}

# 显示帮助信息
show_help() {
    cat << EOF
Tshell 服务端 CGO 构建脚本

用法: $0 [选项]

选项:
  --clean         清理构建缓存后构建
  --verify-only   仅验证已有构建，不重新构建
  --help          显示此帮助信息

功能:
  - 启用 CGO 支持 SQLite 数据库
  - 自动检查编译环境
  - 优化二进制文件大小
  - 验证构建结果

示例:
  $0              # 标准构建
  $0 --clean      # 清理缓存后构建
EOF
}

# 主函数
main() {
    local clean_cache_flag=false
    local verify_only=false
    
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            --clean)
                clean_cache_flag=true
                shift
                ;;
            --verify-only)
                verify_only=true
                shift
                ;;
            --help)
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
    
    # 显示开始信息
    echo "========================================"
    echo "    Tshell 服务端 CGO 构建脚本"
    echo "========================================"
    echo
    
    if [[ "$verify_only" == true ]]; then
        verify_build
        exit 0
    fi
    
    # 执行构建流程
    check_dependencies
    
    if [[ "$clean_cache_flag" == true ]]; then
        clean_cache
    fi
    
    build_server
    verify_build
    show_startup_info
    
    log_success "🎉 CGO 构建完成！"
}

# 脚本入口点
main "$@"
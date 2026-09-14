#!/bin/bash

# Rshell 一键构建部署脚本
# 版本: 1.0.0
# 支持平台: Linux, macOS

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 配置
FRONTEND_DIR="Rshell-web"
BACKEND_DIR="Rshell---A-Cross-Platform-C2"
SERVER_NAME="rshell-server"

# 打印带颜色的消息
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${PURPLE}================================${NC}"
    echo -e "${PURPLE}$1${NC}"
    echo -e "${PURPLE}================================${NC}"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        print_error "$1 未安装或不在PATH中"
        return 1
    fi
    return 0
}

# 检查环境
check_environment() {
    print_header "环境检查"
    
    local errors=0
    
    # 检查Node.js
    if check_command "node"; then
        local node_version=$(node --version | cut -d'v' -f2)
        print_success "Node.js: v$node_version"
        
        # 检查版本是否>=16
        local major_version=$(echo $node_version | cut -d'.' -f1)
        if [ $major_version -lt 16 ]; then
            print_warning "建议使用Node.js 16或更高版本"
        fi
    else
        errors=$((errors + 1))
    fi
    
    # 检查npm
    if check_command "npm"; then
        local npm_version=$(npm --version)
        print_success "npm: v$npm_version"
    else
        errors=$((errors + 1))
    fi
    
    # 检查Go
    if check_command "go"; then
        local go_version=$(go version | cut -d' ' -f3)
        print_success "Go: $go_version"
    else
        errors=$((errors + 1))
    fi
    
    if [ $errors -gt 0 ]; then
        print_error "环境检查失败，请安装缺失的依赖"
        exit 1
    fi
    
    print_success "环境检查通过"
}

# 构建前端
build_frontend() {
    print_header "构建前端应用"
    
    if [ ! -d "$FRONTEND_DIR" ]; then
        print_error "前端目录不存在: $FRONTEND_DIR"
        exit 1
    fi
    
    cd "$FRONTEND_DIR"
    
    print_status "检查package.json..."
    if [ ! -f "package.json" ]; then
        print_error "package.json不存在"
        exit 1
    fi
    
    print_status "安装依赖..."
    if npm ci --silent 2>/dev/null || npm install --silent; then
        print_success "依赖安装完成"
    else
        print_error "依赖安装失败"
        exit 1
    fi
    
    print_status "构建前端应用..."
    if npm run build; then
        print_success "前端构建完成"
    else
        print_error "前端构建失败"
        exit 1
    fi
    
    cd ..
}

# 复制到后端
copy_to_backend() {
    print_header "复制构建产物到后端"
    
    if [ ! -d "$BACKEND_DIR" ]; then
        print_error "后端目录不存在: $BACKEND_DIR"
        exit 1
    fi
    
    cd "$FRONTEND_DIR"
    
    if [ -f "scripts/copy-to-backend.js" ]; then
        print_status "使用Node.js脚本复制文件..."
        if node scripts/copy-to-backend.js; then
            print_success "文件复制完成"
        else
            print_error "文件复制失败"
            exit 1
        fi
    else
        # 手动复制
        print_status "手动复制文件..."
        
        if [ ! -d "dist" ]; then
            print_error "构建输出目录不存在"
            exit 1
        fi
        
        # 创建目标目录
        mkdir -p "../$BACKEND_DIR/dist"
        
        # 复制文件
        cp -r dist/* "../$BACKEND_DIR/dist/"
        print_success "文件复制完成"
    fi
    
    cd ..
}

# 构建后端
build_backend() {
    print_header "编译Go后端服务器"
    
    cd "$BACKEND_DIR"
    
    print_status "检查Go模块..."
    if [ ! -f "go.mod" ]; then
        print_error "go.mod不存在"
        exit 1
    fi
    
    print_status "下载Go依赖..."
    if go mod download; then
        print_success "Go依赖下载完成"
    else
        print_warning "Go依赖下载失败，继续尝试编译..."
    fi
    
    print_status "编译服务器..."
    local build_flags="-ldflags='-w -s' -trimpath"
    
    if go build $build_flags -o "$SERVER_NAME" .; then
        print_success "服务器编译完成: $SERVER_NAME"
        
        # 显示文件信息
        if [ -f "$SERVER_NAME" ]; then
            local file_size=$(du -h "$SERVER_NAME" | cut -f1)
            print_status "服务器文件大小: $file_size"
        fi
    else
        print_error "服务器编译失败"
        exit 1
    fi
    
    cd ..
}

# 验证构建结果
verify_build() {
    print_header "验证构建结果"
    
    # 检查后端可执行文件
    if [ -f "$BACKEND_DIR/$SERVER_NAME" ]; then
        print_success "后端服务器存在"
    else
        print_error "后端服务器不存在"
        return 1
    fi
    
    # 检查前端文件
    if [ -f "$BACKEND_DIR/dist/index.html" ]; then
        print_success "前端主页存在"
    else
        print_error "前端主页不存在"
        return 1
    fi
    
    if [ -d "$BACKEND_DIR/dist/static" ]; then
        local static_count=$(find "$BACKEND_DIR/dist/static" -type f | wc -l)
        print_success "静态资源: $static_count 个文件"
    else
        print_warning "静态资源目录不存在"
    fi
    
    print_success "构建验证通过"
}

# 显示使用说明
show_usage() {
    print_header "构建完成 - 使用说明"
    
    echo -e "服务器位置: ${CYAN}$BACKEND_DIR/$SERVER_NAME${NC}"
    echo ""
    echo "启动服务器:"
    echo -e "  ${CYAN}cd $BACKEND_DIR${NC}"
    echo -e "  ${CYAN}./$SERVER_NAME${NC}"
    echo ""
    echo "或指定端口:"
    echo -e "  ${CYAN}./$SERVER_NAME -p 8080${NC}"
    echo ""
    echo "或指定配置文件:"
    echo -e "  ${CYAN}./$SERVER_NAME -config config.yaml${NC}"
    echo ""
    echo "构建脚本使用:"
    echo -e "  ${CYAN}./quick-build.sh${NC}                  # 完整构建"
    echo -e "  ${CYAN}./quick-build.sh --frontend-only${NC}  # 仅构建前端"
    echo -e "  ${CYAN}./quick-build.sh --backend-only${NC}   # 仅编译后端"
    echo -e "  ${CYAN}./quick-build.sh --clean${NC}          # 清理后构建"
    echo ""
}

# 清理函数
cleanup() {
    print_status "清理临时文件..."
    
    # 清理前端缓存
    if [ -d "$FRONTEND_DIR/node_modules/.vite" ]; then
        rm -rf "$FRONTEND_DIR/node_modules/.vite"
    fi
    
    # 清理前端构建产物
    if [ -d "$FRONTEND_DIR/dist" ]; then
        rm -rf "$FRONTEND_DIR/dist"
    fi
}

# 主函数
main() {
    print_header "Rshell 自动构建部署工具 v1.0.0"
    
    # 解析参数
    local frontend_only=false
    local backend_only=false
    local clean_build=false
    local show_help=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --frontend-only)
                frontend_only=true
                shift
                ;;
            --backend-only)
                backend_only=true
                shift
                ;;
            --clean)
                clean_build=true
                shift
                ;;
            --help|-h)
                show_help=true
                shift
                ;;
            *)
                print_warning "未知参数: $1"
                shift
                ;;
        esac
    done
    
    if [ "$show_help" = true ]; then
        echo "用法: $0 [选项]"
        echo ""
        echo "选项:"
        echo "  --frontend-only    仅构建前端"
        echo "  --backend-only     仅编译后端"
        echo "  --clean           清理后构建"
        echo "  --help, -h        显示帮助信息"
        echo ""
        exit 0
    fi
    
    # 记录开始时间
    local start_time=$(date +%s)
    
    # 清理构建
    if [ "$clean_build" = true ]; then
        cleanup
    fi
    
    # 执行构建流程
    if [ "$backend_only" != true ]; then
        check_environment
        build_frontend
        copy_to_backend
    fi
    
    if [ "$frontend_only" != true ]; then
        build_backend
    fi
    
    if [ "$frontend_only" != true ] && [ "$backend_only" != true ]; then
        verify_build
    fi
    
    # 计算耗时
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    print_success "构建完成! 耗时: ${duration}s"
    
    if [ "$frontend_only" != true ] && [ "$backend_only" != true ]; then
        show_usage
    fi
}

# 错误处理
trap 'print_error "构建过程中发生错误"; exit 1' ERR

# 运行主函数
main "$@"
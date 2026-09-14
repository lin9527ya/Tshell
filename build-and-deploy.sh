#!/bin/bash

# Rshell 前端自动构建和部署脚本
# 用于将 Vue 3 + Vite 前端构建产物自动集成到 Go 后端

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置变量
FRONTEND_DIR="./Rshell-web"
BACKEND_DIR="./Rshell---A-Cross-Platform-C2"
DIST_DIR="$FRONTEND_DIR/dist"
EMBED_DIR="$BACKEND_DIR/dist"
GO_EMBED_FILE="$BACKEND_DIR/main.go"

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查依赖
check_dependencies() {
    log_info "检查构建依赖..."
    
    # 检查 Node.js 和 npm
    if ! command -v node &> /dev/null; then
        log_error "Node.js 未安装，请先安装 Node.js"
        exit 1
    fi
    
    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装，请先安装 npm"
        exit 1
    fi
    
    # 检查 Go
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装，请先安装 Go"
        exit 1
    fi
    
    # 检查目录结构
    if [[ ! -d "$FRONTEND_DIR" ]]; then
        log_error "前端目录不存在: $FRONTEND_DIR"
        exit 1
    fi
    
    if [[ ! -d "$BACKEND_DIR" ]]; then
        log_error "后端目录不存在: $BACKEND_DIR"
        exit 1
    fi
    
    log_success "依赖检查通过"
}

# 安装前端依赖
install_frontend_deps() {
    log_info "安装前端依赖..."
    
    cd "$FRONTEND_DIR"
    
    if [[ -f "package-lock.json" ]]; then
        npm ci --silent
    else
        npm install --silent
    fi
    
    cd - > /dev/null
    log_success "前端依赖安装完成"
}

# 构建前端
build_frontend() {
    log_info "开始构建前端应用 (Vue 3 + Vite)..."
    
    cd "$FRONTEND_DIR"
    
    # 清理旧的构建文件
    if [[ -d "dist" ]]; then
        rm -rf dist
        log_info "清理旧的构建文件"
    fi
    
    # 执行构建
    npm run build
    
    # 检查构建结果
    if [[ ! -d "dist" ]]; then
        log_error "前端构建失败，dist 目录不存在"
        cd - > /dev/null
        exit 1
    fi
    
    # 显示构建文件大小
    log_info "构建产物大小:"
    du -sh dist/*
    
    cd - > /dev/null
    log_success "前端构建完成"
}

# 优化构建产物
optimize_build() {
    log_info "优化构建产物..."
    
    cd "$DIST_DIR"
    
    # 压缩静态资源 (如果有 gzip 命令)
    if command -v gzip &> /dev/null; then
        find . -type f \( -name "*.js" -o -name "*.css" -o -name "*.html" \) -exec gzip -k {} \;
        log_info "已生成 gzip 压缩文件"
    fi
    
    # 生成资源清单
    find . -type f -name "*.js" -o -name "*.css" -o -name "*.html" > ../manifest.txt
    log_info "已生成资源清单"
    
    cd - > /dev/null
    log_success "构建产物优化完成"
}

# 更新后端嵌入文件
update_backend_embed() {
    log_info "更新后端嵌入文件..."
    
    # 清理旧的嵌入目录
    if [[ -d "$EMBED_DIR" ]]; then
        rm -rf "$EMBED_DIR"
        log_info "清理旧的嵌入目录"
    fi
    
    # 复制前端构建产物到后端
    cp -r "$DIST_DIR" "$EMBED_DIR"
    log_info "复制前端构建产物到后端目录"
    
    # 确保 Go embed 指令正确
    if ! grep -q "//go:embed dist" "$GO_EMBED_FILE"; then
        log_warning "请确保 main.go 文件包含 //go:embed dist 指令"
    fi
    
    log_success "后端嵌入文件更新完成"
}

# 构建后端
build_backend() {
    log_info "构建后端应用 (Go + Gin)..."
    
    cd "$BACKEND_DIR"
    
    # 整理依赖
    go mod tidy
    
    # 构建二进制文件
    if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
        # Windows 环境
        go build -ldflags="-s -w" -o rshell-server.exe .
        BINARY_NAME="rshell-server.exe"
    else
        # Unix-like 环境
        go build -ldflags="-s -w" -o rshell-server .
        BINARY_NAME="rshell-server"
    fi
    
    # 检查构建结果
    if [[ ! -f "$BINARY_NAME" ]]; then
        log_error "后端构建失败"
        cd - > /dev/null
        exit 1
    fi
    
    # 显示二进制文件大小
    log_info "后端二进制文件大小:"
    ls -lh "$BINARY_NAME"
    
    cd - > /dev/null
    log_success "后端构建完成"
}

# 验证构建结果
verify_build() {
    log_info "验证构建结果..."
    
    cd "$BACKEND_DIR"
    
    # 检查嵌入的静态文件
    if command -v strings &> /dev/null; then
        if strings rshell-server* | grep -q "index.html"; then
            log_success "静态文件已正确嵌入到二进制文件中"
        else
            log_warning "未检测到嵌入的静态文件，请检查 go:embed 指令"
        fi
    fi
    
    cd - > /dev/null
}

# 生成版本信息
generate_version_info() {
    local version_file="$BACKEND_DIR/version.txt"
    local build_time=$(date '+%Y-%m-%d %H:%M:%S')
    local git_commit=""
    
    # 获取 Git 提交信息
    if command -v git &> /dev/null && git rev-parse --git-dir > /dev/null 2>&1; then
        git_commit=$(git rev-parse --short HEAD)
    fi
    
    cat > "$version_file" << EOF
构建时间: $build_time
Git提交: $git_commit
前端技术栈: Vue 3 + TypeScript + Element Plus + Vite
后端技术栈: Go + Gin + SQLite + JWT
EOF
    
    log_info "版本信息已生成: $version_file"
}

# 清理构建缓存
clean_cache() {
    log_info "清理构建缓存..."
    
    # 清理前端缓存
    cd "$FRONTEND_DIR"
    if [[ -d "node_modules/.cache" ]]; then
        rm -rf node_modules/.cache
    fi
    if [[ -d ".vite" ]]; then
        rm -rf .vite
    fi
    cd - > /dev/null
    
    # 清理 Go 缓存
    cd "$BACKEND_DIR"
    go clean -cache
    cd - > /dev/null
    
    log_success "构建缓存清理完成"
}

# 显示帮助信息
show_help() {
    cat << EOF
Rshell 自动构建脚本

用法: $0 [选项]

选项:
  --clean         清理构建缓存后构建
  --no-deps       跳过依赖安装
  --no-backend    仅构建前端，不构建后端
  --no-optimize   跳过构建产物优化
  --help          显示此帮助信息

示例:
  $0              # 完整构建流程
  $0 --clean      # 清理缓存后构建
  $0 --no-backend # 仅构建前端
EOF
}

# 主函数
main() {
    local clean_cache_flag=false
    local skip_deps=false
    local skip_backend=false
    local skip_optimize=false
    
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            --clean)
                clean_cache_flag=true
                shift
                ;;
            --no-deps)
                skip_deps=true
                shift
                ;;
            --no-backend)
                skip_backend=true
                shift
                ;;
            --no-optimize)
                skip_optimize=true
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
    log_info "开始 Rshell 自动构建流程..."
    log_info "前端: Vue 3 + TypeScript + Element Plus + Vite"
    log_info "后端: Go + Gin + SQLite + JWT"
    echo
    
    # 执行构建流程
    check_dependencies
    
    if [[ "$clean_cache_flag" == true ]]; then
        clean_cache
    fi
    
    if [[ "$skip_deps" != true ]]; then
        install_frontend_deps
    fi
    
    build_frontend
    
    if [[ "$skip_optimize" != true ]]; then
        optimize_build
    fi
    
    update_backend_embed
    
    if [[ "$skip_backend" != true ]]; then
        build_backend
        verify_build
    fi
    
    generate_version_info
    
    echo
    log_success "🎉 Rshell 自动构建完成！"
    
    if [[ "$skip_backend" != true ]]; then
        echo
        log_info "启动命令:"
        if [[ -f "$BACKEND_DIR/rshell-server.exe" ]]; then
            echo "  cd $BACKEND_DIR && ./rshell-server.exe -p 8089"
        else
            echo "  cd $BACKEND_DIR && ./rshell-server -p 8089"
        fi
        echo "  然后访问: http://localhost:8089"
    fi
}

# 脚本入口点
main "$@"
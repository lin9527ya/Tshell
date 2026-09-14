#!/bin/bash

# Rshell 完整工作流管理脚本
# 版本: 1.0.0
# 功能: 统一管理开发、构建、部署的完整生命周期

set -e

# 颜色定义
RED='\\033[0;31m'
GREEN='\\033[0;32m'
YELLOW='\\033[1;33m'
BLUE='\\033[0;34m'
PURPLE='\\033[0;35m'
CYAN='\\033[0;36m'
NC='\\033[0m' # No Color

# 配置
FRONTEND_DIR=\"Rshell-web\"
BACKEND_DIR=\"Rshell---A-Cross-Platform-C2\"
SERVER_NAME=\"rshell-server\"
DEFAULT_PORT=\"8089\"
PID_FILE=\".rshell-server.pid\"

# 打印函数
print_header() {
    echo -e \"${PURPLE}====================================================${NC}\"
    echo -e \"${PURPLE}$1${NC}\"
    echo -e \"${PURPLE}====================================================${NC}\"
}

print_info() {
    echo -e \"${BLUE}[INFO]${NC} $1\"
}

print_success() {
    echo -e \"${GREEN}[SUCCESS]${NC} $1\"
}

print_warning() {
    echo -e \"${YELLOW}[WARNING]${NC} $1\"
}

print_error() {
    echo -e \"${RED}[ERROR]${NC} $1\"
}

# 显示菜单
show_menu() {
    clear
    print_header \"Rshell 工作流管理工具 v1.0.0\"
    echo \"\"
    echo -e \"${CYAN}开发模式:${NC}\"
    echo \"  1) 启动前端开发服务器 (Vue 3 + Vite)\"
    echo \"  2) 启动后端开发服务器 (Go + Gin)\"
    echo \"  3) 同时启动前后端开发环境\"
    echo \"\"
    echo -e \"${CYAN}构建部署:${NC}\"
    echo \"  4) 快速构建部署 (推荐)\"
    echo \"  5) 仅构建前端\"
    echo \"  6) 仅编译后端\"
    echo \"  7) 完整构建 + 验证\"
    echo \"\"
    echo -e \"${CYAN}服务器管理:${NC}\"
    echo \"  8) 启动生产服务器\"
    echo \"  9) 停止服务器\"
    echo \"  10) 重启服务器\"
    echo \"  11) 查看服务器状态\"
    echo \"\"
    echo -e \"${CYAN}维护工具:${NC}\"
    echo \"  12) 清理构建缓存\"
    echo \"  13) 重置开发环境\"
    echo \"  14) 查看构建日志\"
    echo \"  15) 运行验证测试\"
    echo \"\"
    echo \"  0) 退出\"
    echo \"\"
    echo -n -e \"${BLUE}请选择操作 (0-15): ${NC}\"
}

# 检查环境
check_environment() {
    local errors=0
    
    # 检查必要的命令
    for cmd in node npm go; do
        if ! command -v $cmd &> /dev/null; then
            print_error \"$cmd 未安装或不在PATH中\"
            errors=$((errors + 1))
        fi
    done
    
    # 检查目录
    if [ ! -d \"$FRONTEND_DIR\" ]; then
        print_error \"前端目录不存在: $FRONTEND_DIR\"
        errors=$((errors + 1))
    fi
    
    if [ ! -d \"$BACKEND_DIR\" ]; then
        print_error \"后端目录不存在: $BACKEND_DIR\"
        errors=$((errors + 1))
    fi
    
    return $errors
}

# 启动前端开发服务器
start_frontend_dev() {
    print_info \"启动前端开发服务器...\"
    
    cd \"$FRONTEND_DIR\"
    
    # 检查依赖
    if [ ! -d \"node_modules\" ]; then
        print_info \"安装前端依赖...\"
        npm install
    fi
    
    print_success \"前端开发服务器启动中，请访问 http://localhost:3333\"
    npm run dev
}

# 启动后端开发服务器
start_backend_dev() {
    print_info \"启动后端开发服务器...\"
    
    cd \"$BACKEND_DIR\"
    
    # 检查Go依赖
    if [ -f \"go.mod\" ]; then
        go mod download
    fi
    
    print_success \"后端开发服务器启动中，请访问 http://localhost:$DEFAULT_PORT\"
    go run . -p $DEFAULT_PORT
}

# 同时启动前后端
start_full_dev() {
    print_info \"启动完整开发环境...\"
    
    # 启动后端（后台）
    cd \"$BACKEND_DIR\"
    if [ -f \"go.mod\" ]; then
        go mod download
    fi
    
    print_info \"启动后端服务器 (端口 $DEFAULT_PORT)...\"
    go run . -p $DEFAULT_PORT &
    BACKEND_PID=$!
    
    # 等待后端启动
    sleep 3
    
    # 启动前端
    cd \"../$FRONTEND_DIR\"
    
    if [ ! -d \"node_modules\" ]; then
        print_info \"安装前端依赖...\"
        npm install
    fi
    
    print_success \"前端开发服务器: http://localhost:3333\"
    print_success \"后端API服务器: http://localhost:$DEFAULT_PORT\"
    print_info \"按 Ctrl+C 停止所有服务\"
    
    # 设置陷阱，确保退出时杀死后端进程
    trap \"kill $BACKEND_PID 2>/dev/null; exit\" INT TERM
    
    npm run dev
    
    # 清理
    kill $BACKEND_PID 2>/dev/null
}

# 快速构建部署
quick_build_deploy() {
    print_info \"执行快速构建部署...\"
    
    if [ -f \"quick-build.sh\" ]; then
        chmod +x quick-build.sh
        ./quick-build.sh
    else
        print_warning \"quick-build.sh 不存在，使用备用方法\"
        build_frontend_only
        build_backend_only
    fi
}

# 仅构建前端
build_frontend_only() {
    print_info \"构建前端应用...\"
    
    cd \"$FRONTEND_DIR\"
    
    if [ ! -d \"node_modules\" ]; then
        print_info \"安装依赖...\"
        npm install
    fi
    
    npm run build:embed
    cd ..
    
    print_success \"前端构建完成\"
}

# 仅编译后端
build_backend_only() {
    print_info \"编译后端服务器...\"
    
    cd \"$BACKEND_DIR\"
    
    go mod download
    go build -ldflags=\"-w -s\" -trimpath -o \"$SERVER_NAME\" .
    
    cd ..
    
    print_success \"后端编译完成\"
}

# 完整构建和验证
full_build_and_validate() {
    print_info \"执行完整构建和验证...\"
    
    # 构建
    build_frontend_only
    build_backend_only
    
    # 验证
    if [ -f \"validate-build.js\" ]; then
        print_info \"运行构建验证...\"
        node validate-build.js
    else
        print_warning \"validate-build.js 不存在，跳过验证\"
    fi
}

# 启动生产服务器
start_production_server() {
    print_info \"启动生产服务器...\"
    
    cd \"$BACKEND_DIR\"
    
    if [ ! -f \"$SERVER_NAME\" ]; then
        print_error \"服务器可执行文件不存在，请先构建\"
        return 1
    fi
    
    # 检查端口是否被占用
    if lsof -Pi :$DEFAULT_PORT -sTCP:LISTEN -t >/dev/null ; then
        print_warning \"端口 $DEFAULT_PORT 已被占用\"
        return 1
    fi
    
    # 启动服务器（后台）
    nohup \"./$SERVER_NAME\" -p $DEFAULT_PORT > server.log 2>&1 &
    SERVER_PID=$!
    
    # 保存PID
    echo $SERVER_PID > \"$PID_FILE\"
    
    # 等待服务器启动
    sleep 2
    
    if kill -0 $SERVER_PID 2>/dev/null; then
        print_success \"服务器已启动 (PID: $SERVER_PID)\"
        print_success \"访问地址: http://localhost:$DEFAULT_PORT\"
        print_info \"日志文件: $BACKEND_DIR/server.log\"
    else
        print_error \"服务器启动失败\"
        return 1
    fi
    
    cd ..
}

# 停止服务器
stop_server() {
    print_info \"停止服务器...\"
    
    if [ -f \"$BACKEND_DIR/$PID_FILE\" ]; then
        local pid=$(cat \"$BACKEND_DIR/$PID_FILE\")
        if kill -0 $pid 2>/dev/null; then
            kill $pid
            print_success \"服务器已停止 (PID: $pid)\"
        else
            print_warning \"服务器进程不存在\"
        fi
        rm -f \"$BACKEND_DIR/$PID_FILE\"
    else
        print_warning \"PID文件不存在，尝试通过端口查找...\"
        local pid=$(lsof -ti:$DEFAULT_PORT)
        if [ -n \"$pid\" ]; then
            kill $pid
            print_success \"服务器已停止 (PID: $pid)\"
        else
            print_warning \"未找到运行中的服务器\"
        fi
    fi
}

# 重启服务器
restart_server() {
    print_info \"重启服务器...\"
    stop_server
    sleep 1
    start_production_server
}

# 查看服务器状态
check_server_status() {
    print_info \"检查服务器状态...\"
    
    if [ -f \"$BACKEND_DIR/$PID_FILE\" ]; then
        local pid=$(cat \"$BACKEND_DIR/$PID_FILE\")
        if kill -0 $pid 2>/dev/null; then
            print_success \"服务器运行中 (PID: $pid)\"
            
            # 检查HTTP响应
            if curl -s \"http://localhost:$DEFAULT_PORT\" > /dev/null; then
                print_success \"HTTP服务正常\"
            else
                print_warning \"HTTP服务无响应\"
            fi
        else
            print_warning \"PID文件存在但进程不存在\"
        fi
    else
        local pid=$(lsof -ti:$DEFAULT_PORT)
        if [ -n \"$pid\" ]; then
            print_warning \"端口被占用但PID文件不存在 (PID: $pid)\"
        else
            print_info \"服务器未运行\"
        fi
    fi
}

# 清理构建缓存
clean_build_cache() {
    print_info \"清理构建缓存...\"
    
    # 清理前端缓存
    if [ -d \"$FRONTEND_DIR/node_modules/.vite\" ]; then
        rm -rf \"$FRONTEND_DIR/node_modules/.vite\"
        print_success \"前端Vite缓存已清理\"
    fi
    
    if [ -d \"$FRONTEND_DIR/dist\" ]; then
        rm -rf \"$FRONTEND_DIR/dist\"
        print_success \"前端构建输出已清理\"
    fi
    
    # 清理后端缓存
    if [ -d \"$BACKEND_DIR/dist\" ]; then
        rm -rf \"$BACKEND_DIR/dist\"
        print_success \"后端嵌入目录已清理\"
    fi
    
    if [ -f \"$BACKEND_DIR/$SERVER_NAME\" ]; then
        rm -f \"$BACKEND_DIR/$SERVER_NAME\"
        print_success \"后端可执行文件已清理\"
    fi
    
    # 清理Go缓存
    cd \"$BACKEND_DIR\"
    go clean -cache
    go clean -modcache
    cd ..
    
    print_success \"所有构建缓存已清理\"
}

# 重置开发环境
reset_dev_environment() {
    print_info \"重置开发环境...\"
    
    # 停止服务器
    stop_server
    
    # 清理缓存
    clean_build_cache
    
    # 重新安装前端依赖
    cd \"$FRONTEND_DIR\"
    if [ -d \"node_modules\" ]; then
        rm -rf node_modules package-lock.json
    fi
    npm install
    cd ..
    
    # 重新下载Go依赖
    cd \"$BACKEND_DIR\"
    go mod download
    cd ..
    
    print_success \"开发环境已重置\"
}

# 查看构建日志
view_build_logs() {
    print_info \"查看构建和运行日志...\"
    
    echo -e \"\n${CYAN}=== 服务器日志 ===${NC}\"
    if [ -f \"$BACKEND_DIR/server.log\" ]; then
        tail -20 \"$BACKEND_DIR/server.log\"
    else
        print_warning \"服务器日志文件不存在\"
    fi
    
    echo -e \"\n${CYAN}=== 构建报告 ===${NC}\"
    if [ -f \"validation-report.json\" ]; then
        cat validation-report.json | head -20
    else
        print_warning \"验证报告不存在\"
    fi
    
    echo -e \"\n${CYAN}=== 前端构建报告 ===${NC}\"
    if [ -f \"$FRONTEND_DIR/build-report.json\" ]; then
        cat \"$FRONTEND_DIR/build-report.json\"
    else
        print_warning \"前端构建报告不存在\"
    fi
}

# 运行验证测试
run_validation_test() {
    if [ -f \"validate-build.js\" ]; then
        print_info \"运行构建验证测试...\"
        node validate-build.js
    else
        print_error \"validate-build.js 不存在\"
        return 1
    fi
}

# 主循环
main() {
    # 检查环境
    if ! check_environment; then
        print_error \"环境检查失败，请先安装必需的依赖\"
        exit 1
    fi
    
    while true; do
        show_menu
        read -r choice
        
        case $choice in
            1)
                start_frontend_dev
                ;;
            2)
                start_backend_dev
                ;;
            3)
                start_full_dev
                ;;
            4)
                quick_build_deploy
                ;;
            5)
                build_frontend_only
                ;;
            6)
                build_backend_only
                ;;
            7)
                full_build_and_validate
                ;;
            8)
                start_production_server
                ;;
            9)
                stop_server
                ;;
            10)
                restart_server
                ;;
            11)
                check_server_status
                ;;
            12)
                clean_build_cache
                ;;
            13)
                reset_dev_environment
                ;;
            14)
                view_build_logs
                ;;
            15)
                run_validation_test
                ;;
            0)
                print_info \"退出工作流管理工具\"
                exit 0
                ;;
            *)
                print_error \"无效选择，请输入 0-15\"
                ;;
        esac
        
        echo \"\"
        echo -n -e \"${BLUE}按回车键继续...${NC}\"
        read
    done
}

# 处理命令行参数
if [ \"$1\" = \"--help\" ] || [ \"$1\" = \"-h\" ]; then
    echo \"Rshell 工作流管理工具\"
    echo \"\"
    echo \"用法: $0 [选项]\"
    echo \"\"
    echo \"选项:\"
    echo \"  --help, -h    显示帮助信息\"
    echo \"\"
    echo \"功能:\"
    echo \"  - 开发环境管理（前端/后端/全栈）\"
    echo \"  - 构建部署流程（快速/分离/完整）\"
    echo \"  - 生产服务器管理（启动/停止/重启/状态）\"
    echo \"  - 维护工具（清理/重置/日志/验证）\"
    echo \"\"
    exit 0
fi

# 运行主函数
main
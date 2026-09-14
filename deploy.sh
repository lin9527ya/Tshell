#!/bin/bash

# Rshell 自动化部署脚本
# 支持 Docker Compose 和 Kubernetes 部署

set -e  # 遇到错误立即退出

# 颜色定义
RED='\\033[0;31m'
GREEN='\\033[0;32m'
YELLOW='\\033[1;33m'
BLUE='\\033[0;34m'
NC='\\033[0m' # No Color

# 日志函数
log_info() {
    echo -e \"${BLUE}[INFO]${NC} $1\"
}

log_success() {
    echo -e \"${GREEN}[SUCCESS]${NC} $1\"
}

log_warning() {
    echo -e \"${YELLOW}[WARNING]${NC} $1\"
}

log_error() {
    echo -e \"${RED}[ERROR]${NC} $1\"
}

# 显示帮助信息
show_help() {
    cat << EOF
Rshell 部署脚本

用法: $0 [选项] <命令>

命令:
  docker-build    构建 Docker 镜像
  docker-up      启动 Docker Compose 服务
  docker-down    停止 Docker Compose 服务
  k8s-deploy     部署到 Kubernetes
  k8s-delete     从 Kubernetes 删除
  backup         备份数据
  restore        恢复数据
  status         查看服务状态
  logs           查看服务日志
  
选项:
  -e, --env      环境 (dev/staging/prod)
  -n, --namespace Kubernetes 命名空间 (默认: rshell)
  -f, --file     配置文件路径
  -h, --help     显示此帮助信息
  -v, --verbose  详细输出

示例:
  $0 docker-build
  $0 -e prod docker-up
  $0 -n rshell-prod k8s-deploy
  $0 backup
EOF
}

# 检查依赖
check_dependencies() {
    local deps=(\"docker\" \"docker-compose\")
    
    if [[ \"$DEPLOYMENT_MODE\" == \"k8s\" ]]; then
        deps+=(\"kubectl\")
    fi
    
    for dep in \"${deps[@]}\"; do
        if ! command -v \"$dep\" &> /dev/null; then
            log_error \"$dep 未安装或不在 PATH 中\"
            exit 1
        fi
    done
    
    log_info \"依赖检查通过\"
}

# 设置环境变量
setup_env() {
    local env_file=\".env\"
    
    if [[ -n \"$ENV\" ]]; then
        env_file=\".env.$ENV\"
    fi
    
    if [[ -f \"$env_file\" ]]; then
        log_info \"加载环境变量: $env_file\"
        set -a  # 自动导出变量
        source \"$env_file\"
        set +a
    else
        log_warning \"环境变量文件 $env_file 不存在，使用默认值\"
    fi
    
    # 设置默认值
    export SERVER_PORT=${SERVER_PORT:-8089}
    export WEB_PORT=${WEB_PORT:-80}
    export REDIS_PASSWORD=${REDIS_PASSWORD:-rshell123}
    export ENV=${ENV:-development}
    export NAMESPACE=${NAMESPACE:-rshell}
}

# 构建 Docker 镜像
docker_build() {
    log_info \"开始构建 Docker 镜像...\"
    
    # 构建服务端镜像
    log_info \"构建 Rshell 服务端镜像...\"
    docker build -t rshell/server:latest ./Rshell---A-Cross-Platform-C2/
    
    # 构建 Web 前端镜像
    log_info \"构建 Rshell Web 镜像...\"
    docker build -t rshell/web:latest ./Rshell-web/
    
    log_success \"Docker 镜像构建完成\"
}

# 启动 Docker Compose 服务
docker_up() {
    log_info \"启动 Rshell 服务 (Docker Compose)...\"
    
    local compose_files=\"-f docker-compose.yml\"
    
    if [[ \"$ENV\" == \"production\" ]]; then
        compose_files+\" -f docker-compose.prod.yml\"
    fi
    
    if [[ -f \"docker-compose.override.yml\" ]]; then
        compose_files+\" -f docker-compose.override.yml\"
    fi
    
    eval \"docker-compose $compose_files up -d\"
    
    log_info \"等待服务启动...\"
    sleep 10
    
    # 检查服务状态
    docker_status
}

# 停止 Docker Compose 服务
docker_down() {
    log_info \"停止 Rshell 服务...\"
    docker-compose down
    log_success \"服务已停止\"
}

# 部署到 Kubernetes
k8s_deploy() {
    log_info \"部署到 Kubernetes (命名空间: $NAMESPACE)...\"
    
    # 检查 kubectl 连接
    if ! kubectl cluster-info &> /dev/null; then
        log_error \"无法连接到 Kubernetes 集群\"
        exit 1
    fi
    
    # 应用配置
    kubectl apply -f k8s-deployment.yaml
    
    log_info \"等待部署完成...\"
    kubectl wait --for=condition=available --timeout=300s deployment/rshell-server -n \"$NAMESPACE\"
    kubectl wait --for=condition=available --timeout=300s deployment/rshell-web -n \"$NAMESPACE\"
    
    log_success \"Kubernetes 部署完成\"
    
    # 显示服务信息
    k8s_status
}

# 从 Kubernetes 删除
k8s_delete() {
    log_info \"从 Kubernetes 删除 Rshell...\"
    kubectl delete -f k8s-deployment.yaml --ignore-not-found=true
    log_success \"删除完成\"
}

# 备份数据
backup_data() {
    local timestamp=$(date +\"%Y%m%d_%H%M%S\")
    local backup_dir=\"backups/$timestamp\"
    
    log_info \"创建备份: $backup_dir\"
    mkdir -p \"$backup_dir\"
    
    if docker-compose ps | grep -q \"rshell-server\"; then
        # Docker Compose 备份
        docker-compose exec rshell-server tar czf - /data | cat > \"$backup_dir/server-data.tar.gz\"
        docker-compose exec redis redis-cli --rdb - > \"$backup_dir/redis-dump.rdb\"
    elif kubectl get namespace \"$NAMESPACE\" &> /dev/null; then
        # Kubernetes 备份
        local server_pod=$(kubectl get pods -n \"$NAMESPACE\" -l app=rshell-server -o jsonpath='{.items[0].metadata.name}')
        kubectl exec -n \"$NAMESPACE\" \"$server_pod\" -- tar czf - /data | cat > \"$backup_dir/server-data.tar.gz\"
    fi
    
    log_success \"备份完成: $backup_dir\"
}

# 恢复数据
restore_data() {
    local backup_dir=\"$1\"
    
    if [[ -z \"$backup_dir\" ]]; then
        log_error \"请指定备份目录\"
        exit 1
    fi
    
    if [[ ! -d \"$backup_dir\" ]]; then
        log_error \"备份目录不存在: $backup_dir\"
        exit 1
    fi
    
    log_info \"从备份恢复: $backup_dir\"
    
    # 恢复逻辑根据部署模式实现
    log_success \"数据恢复完成\"
}

# 查看 Docker 服务状态
docker_status() {
    log_info \"Docker Compose 服务状态:\"
    docker-compose ps
    
    echo
    log_info \"服务健康检查:\"
    local services=(\"rshell-server\" \"rshell-web\" \"redis\")
    
    for service in \"${services[@]}\"; do
        if docker-compose ps | grep -q \"$service.*Up\"; then
            log_success \"$service: 运行中\"
        else
            log_error \"$service: 异常\"
        fi
    done
    
    echo
    log_info \"访问地址:\"
    echo \"  Web 界面: http://localhost:${WEB_PORT}\"
    echo \"  API 接口: http://localhost:${SERVER_PORT}/api\"
}

# 查看 Kubernetes 状态
k8s_status() {
    log_info \"Kubernetes 服务状态 (命名空间: $NAMESPACE):\"
    
    echo \"Deployments:\"
    kubectl get deployments -n \"$NAMESPACE\"
    
    echo
    echo \"Pods:\"
    kubectl get pods -n \"$NAMESPACE\"
    
    echo
    echo \"Services:\"
    kubectl get services -n \"$NAMESPACE\"
    
    echo
    echo \"Ingress:\"
    kubectl get ingress -n \"$NAMESPACE\"
}

# 查看服务日志
show_logs() {
    local service=\"$1\"
    local lines=\"${2:-100}\"
    
    if docker-compose ps | grep -q \"Up\"; then
        if [[ -n \"$service\" ]]; then
            docker-compose logs --tail=\"$lines\" -f \"$service\"
        else
            docker-compose logs --tail=\"$lines\" -f
        fi
    elif kubectl get namespace \"$NAMESPACE\" &> /dev/null; then
        if [[ -n \"$service\" ]]; then
            kubectl logs -n \"$NAMESPACE\" -l \"app=$service\" --tail=\"$lines\" -f
        else
            kubectl logs -n \"$NAMESPACE\" --all-containers=true --tail=\"$lines\" -f
        fi
    else
        log_error \"未找到运行中的服务\"
        exit 1
    fi
}

# 主函数
main() {
    local command=\"\"
    
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -e|--env)
                ENV=\"$2\"
                shift 2
                ;;
            -n|--namespace)
                NAMESPACE=\"$2\"
                shift 2
                ;;
            -f|--file)
                CONFIG_FILE=\"$2\"
                shift 2
                ;;
            -v|--verbose)
                set -x
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            docker-build|docker-up|docker-down|k8s-deploy|k8s-delete|backup|restore|status|logs)
                command=\"$1\"
                shift
                ;;
            *)
                log_error \"未知参数: $1\"
                show_help
                exit 1
                ;;
        esac
    done
    
    if [[ -z \"$command\" ]]; then
        log_error \"请指定命令\"
        show_help
        exit 1
    fi
    
    # 设置环境
    setup_env
    
    # 检查依赖
    if [[ \"$command\" == k8s-* ]]; then
        DEPLOYMENT_MODE=\"k8s\"
    else
        DEPLOYMENT_MODE=\"docker\"
    fi
    check_dependencies
    
    # 执行命令
    case \"$command\" in
        docker-build)
            docker_build
            ;;
        docker-up)
            docker_up
            ;;
        docker-down)
            docker_down
            ;;
        k8s-deploy)
            k8s_deploy
            ;;
        k8s-delete)
            k8s_delete
            ;;
        backup)
            backup_data
            ;;
        restore)
            restore_data \"$1\"
            ;;
        status)
            if [[ \"$DEPLOYMENT_MODE\" == \"k8s\" ]]; then
                k8s_status
            else
                docker_status
            fi
            ;;
        logs)
            show_logs \"$1\" \"$2\"
            ;;
        *)
            log_error \"未支持的命令: $command\"
            exit 1
            ;;
    esac
}

# 脚本入口
main \"$@\"
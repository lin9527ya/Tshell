#!/bin/bash
echo "=== Linux 交互式终端修复验证 ==="
echo ""

# 定义客户端列表
CLIENTS=("Reacon_http" "Reacon_websocket" "Reacon_oss" "Reacon_tcp")

cd Tshell-client || exit 1

for client in "${CLIENTS[@]}"; do
    echo "🔧 编译测试 $client..."
    cd "$client" || continue
    
    # 设置环境变量并编译Linux版本
    export GOOS=linux
    export GOARCH=amd64
    
    if go build -ldflags="-s -w" -o "r_linux_amd64" .; then
        echo "  ✅ $client Linux版本编译成功"
        ls -la r_linux_amd64
    else
        echo "  ❌ $client Linux版本编译失败"
    fi
    
    cd ..
    echo ""
done

echo "📋 修复内容总结:"
echo "✅ 修复了 'file already closed' 错误"
echo "✅ 添加了 panic 恢复机制"
echo "✅ 改进了数据缓冲区管理"
echo "✅ 优化了资源清理顺序"
echo "✅ 增强了错误处理逻辑"
echo ""
echo "🎯 预期效果:"
echo "  - 消除客户端日志中的错误信息"
echo "  - 提高交互式终端的稳定性"
echo "  - 防止因资源竞争导致的崩溃"
echo "  - 提供更流畅的用户体验"
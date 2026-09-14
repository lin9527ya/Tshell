#!/bin/bash

echo "🧪 HTTP隧道测试脚本"
echo "========================"

# 检查HTTP隧道是否正确启动
echo "1. 检查HTTP隧道状态..."

# 测试端口1090是否监听
echo "2. 检查服务端端口监听..."
netstat -an | grep :1090 || echo "❌ 端口1090未监听"

# 测试HTTP代理功能
echo "3. 测试HTTP代理功能..."
echo "   尝试通过HTTP隧道访问网站..."

# 使用curl测试HTTP代理
curl -x http://127.0.0.1:1090 -v http://httpbin.org/ip --connect-timeout 10

echo ""
echo "4. 测试结果分析："
echo "   - 如果看到连接成功：HTTP隧道工作正常"
echo "   - 如果连接拒绝：检查隧道是否正确启动"
echo "   - 如果超时：检查客户端是否正确处理HTTP请求"

echo ""
echo "📋 HTTP隧道工作原理："
echo "   1. 用户 -> HTTP代理端口(1090)"
echo "   2. 服务端 -> 解析HTTP请求"
echo "   3. 服务端 -> 发送隧道命令给客户端"
echo "   4. 客户端 -> 连接目标网站"
echo "   5. 客户端 -> 转发数据"
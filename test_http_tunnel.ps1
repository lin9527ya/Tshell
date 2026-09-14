Write-Host "🧪 HTTP隧道测试脚本" -ForegroundColor Yellow
Write-Host "========================" -ForegroundColor Yellow

# 检查HTTP隧道是否正确启动
Write-Host "1. 检查HTTP隧道状态..." -ForegroundColor Cyan

# 测试端口1090是否监听
Write-Host "2. 检查服务端端口监听..." -ForegroundColor Cyan
$listening = netstat -an | Select-String ":1090"
if ($listening) {
    Write-Host "✅ 端口1090正在监听" -ForegroundColor Green
    Write-Host $listening -ForegroundColor Green
} else {
    Write-Host "❌ 端口1090未监听" -ForegroundColor Red
}

# 测试HTTP代理功能
Write-Host "3. 测试HTTP代理功能..." -ForegroundColor Cyan
Write-Host "   尝试通过HTTP隧道访问网站..." -ForegroundColor White

# 使用curl测试HTTP代理（如果有curl）
if (Get-Command curl -ErrorAction SilentlyContinue) {
    Write-Host "   使用curl测试HTTP代理..." -ForegroundColor White
    try {
        $result = curl -x http://127.0.0.1:1090 http://httpbin.org/ip --connect-timeout 10 2>&1
        Write-Host "✅ HTTP代理测试结果:" -ForegroundColor Green
        Write-Host $result -ForegroundColor White
    } catch {
        Write-Host "❌ HTTP代理测试失败: $_" -ForegroundColor Red
    }
} else {
    Write-Host "   curl未安装，使用PowerShell测试..." -ForegroundColor White
    try {
        # 使用PowerShell的Web请求通过代理
        $proxy = [System.Net.WebProxy]::new("http://127.0.0.1:1090")
        $client = [System.Net.WebClient]::new()
        $client.Proxy = $proxy
        $result = $client.DownloadString("http://httpbin.org/ip")
        Write-Host "✅ HTTP代理测试成功:" -ForegroundColor Green
        Write-Host $result -ForegroundColor White
    } catch {
        Write-Host "❌ HTTP代理测试失败: $_" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "4. 测试结果分析：" -ForegroundColor Cyan
Write-Host "   - 如果看到连接成功：HTTP隧道工作正常" -ForegroundColor White
Write-Host "   - 如果连接拒绝：检查隧道是否正确启动" -ForegroundColor White
Write-Host "   - 如果超时：检查客户端是否正确处理HTTP请求" -ForegroundColor White

Write-Host ""
Write-Host "📋 HTTP隧道工作原理：" -ForegroundColor Magenta
Write-Host "   1. 用户 -> HTTP代理端口(1090)" -ForegroundColor White
Write-Host "   2. 服务端 -> 解析HTTP请求" -ForegroundColor White
Write-Host "   3. 服务端 -> 发送隧道命令给客户端" -ForegroundColor White
Write-Host "   4. 客户端 -> 连接目标网站" -ForegroundColor White
Write-Host "   5. 客户端 -> 转发数据" -ForegroundColor White

Write-Host ""
Write-Host "💡 使用提示：" -ForegroundColor Yellow
Write-Host "   - 浏览器代理设置: HTTP代理 127.0.0.1:1090" -ForegroundColor White
Write-Host "   - curl命令: curl -x http://127.0.0.1:1090 http://example.com" -ForegroundColor White
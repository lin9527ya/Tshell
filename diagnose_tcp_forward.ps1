# TCP正向客户端诊断脚本
# 用于排查TCP正向客户端的连接和通信问题

Write-Host "🔍 TCP正向客户端诊断脚本" -ForegroundColor Green
Write-Host "=================================" -ForegroundColor Green

# 检查常用端口
$commonPorts = @(8080, 8000, 9090, 8443, 443, 80)

Write-Host "`n📋 检查常用端口状态:" -ForegroundColor Yellow
foreach ($port in $commonPorts) {
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Any, $port)
        $listener.Start()
        Write-Host "   ✅ 端口 $port 可用" -ForegroundColor Green
        $listener.Stop()
    }
    catch {
        Write-Host "   ❌ 端口 $port 被占用" -ForegroundColor Red
    }
}

# 检查防火墙状态
Write-Host "`n🔥 检查Windows防火墙状态:" -ForegroundColor Yellow
try {
    $firewallStatus = Get-NetFirewallProfile | Select-Object Name, Enabled
    foreach ($profile in $firewallStatus) {
        $status = if ($profile.Enabled) { "启用" } else { "禁用" }
        $color = if ($profile.Enabled) { "Red" } else { "Green" }
        Write-Host "   $($profile.Name): $status" -ForegroundColor $color
    }
}
catch {
    Write-Host "   ⚠️ 无法获取防火墙状态" -ForegroundColor Yellow
}

# 检查网络连接
Write-Host "`n🌐 检查网络连接:" -ForegroundColor Yellow
$testHosts = @("127.0.0.1", "localhost", "0.0.0.0")
foreach ($host in $testHosts) {
    try {
        $ping = Test-Connection -ComputerName $host -Count 1 -Quiet
        if ($ping) {
            Write-Host "   ✅ $host 可达" -ForegroundColor Green
        } else {
            Write-Host "   ❌ $host 不可达" -ForegroundColor Red
        }
    }
    catch {
        Write-Host "   ⚠️ $host 测试失败" -ForegroundColor Yellow
    }
}

# 检查TCP正向客户端进程
Write-Host "`n🔍 查找TCP正向客户端进程:" -ForegroundColor Yellow
$processNames = @("tcp_windows_amd64", "Reacon_tcp_forward", "tcp_forward")
$foundProcesses = @()

foreach ($name in $processNames) {
    $processes = Get-Process -Name $name -ErrorAction SilentlyContinue
    if ($processes) {
        foreach ($proc in $processes) {
            $foundProcesses += $proc
            Write-Host "   ✅ 发现进程: $($proc.Name) (PID: $($proc.Id))" -ForegroundColor Green
        }
    }
}

if ($foundProcesses.Count -eq 0) {
    Write-Host "   ❌ 未发现TCP正向客户端进程" -ForegroundColor Red
}

# 检查监听端口
Write-Host "`n👂 检查监听端口:" -ForegroundColor Yellow
try {
    $listeners = Get-NetTCPConnection | Where-Object { $_.State -eq "Listen" } | 
                 Select-Object LocalAddress, LocalPort, @{Name="Process";Expression={(Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).Name}} |
                 Sort-Object LocalPort
    
    $count = 0
    foreach ($listener in $listeners) {
        if ($listener.Process -in $processNames -or $listener.LocalPort -in $commonPorts) {
            Write-Host "   📡 $($listener.LocalAddress):$($listener.LocalPort) - $($listener.Process)" -ForegroundColor Cyan
            $count++
        }
    }
    
    if ($count -eq 0) {
        Write-Host "   ⚠️ 未发现相关监听端口" -ForegroundColor Yellow
    }
}
catch {
    Write-Host "   ❌ 无法获取监听端口信息" -ForegroundColor Red
}

# 提供修复建议
Write-Host "`n💡 修复建议:" -ForegroundColor Yellow
Write-Host "   1. 确保TCP正向客户端正在运行" -ForegroundColor White
Write-Host "   2. 检查客户端是否监听在正确的端口上" -ForegroundColor White
Write-Host "   3. 如果防火墙启用，添加客户端端口到白名单" -ForegroundColor White
Write-Host "   4. 确保服务端能够访问客户端的IP和端口" -ForegroundColor White
Write-Host "   5. 检查密钥配置是否与服务端一致" -ForegroundColor White

# 简单的端口测试服务器
Write-Host "`n🧪 启动测试服务器 (端口8080):" -ForegroundColor Yellow
try {
    $testPort = 8080
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Any, $testPort)
    $listener.Start()
    Write-Host "   ✅ 测试服务器启动成功，监听端口 $testPort" -ForegroundColor Green
    Write-Host "   📞 请从服务端尝试连接: telnet $env:COMPUTERNAME $testPort" -ForegroundColor Cyan
    Write-Host "   ⏰ 等待连接 (10秒后自动关闭)..." -ForegroundColor Yellow
    
    $timeout = [System.DateTime]::Now.AddSeconds(10)
    while ([System.DateTime]::Now -lt $timeout) {
        if ($listener.Pending()) {
            $client = $listener.AcceptTcpClient()
            Write-Host "   🎉 收到连接: $($client.Client.RemoteEndPoint)" -ForegroundColor Green
            $client.Close()
            break
        }
        Start-Sleep -Milliseconds 100
    }
    
    $listener.Stop()
    Write-Host "   🛑 测试服务器已关闭" -ForegroundColor Yellow
}
catch {
    Write-Host "   ❌ 测试服务器启动失败: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "`n✅ 诊断完成" -ForegroundColor Green
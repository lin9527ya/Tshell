# 批量更新所有客户端main.go文件中的命令类型常量
# PowerShell脚本

$clients = @(
    "Reacon_websocket",
    "Reacon_http", 
    "Reacon_kcp",
    "Reacon_oss"
)

Write-Host "开始批量更新客户端main.go文件..." -ForegroundColor Green

foreach ($client in $clients) {
    Write-Host "处理客户端: $client" -ForegroundColor Yellow
    
    $mainFile = "c:\Users\admin\Desktop\Tshell\Tshell-client\$client\main.go"
    
    if (Test-Path $mainFile) {
        Write-Host "  更新 main.go..." -ForegroundColor Cyan
        
        # 读取文件内容
        $content = Get-Content $mainFile -Raw
        
        # 替换命令类型常量
        $content = $content -replace 'services\.INTERACTIVE_SHELL_START_CMD', 'services.CMD_MODE_START'
        $content = $content -replace 'services\.INTERACTIVE_SHELL_INPUT_CMD', 'services.CMD_MODE_INPUT'
        $content = $content -replace 'services\.INTERACTIVE_SHELL_STOP_CMD', 'services.CMD_MODE_STOP'
        $content = $content -replace 'services\.INTERACTIVE_SHELL_INTERRUPT', 'services.WMI_MODE_START'
        
        # 替换注释和函数调用
        $content = $content -replace 'CMD模式交互式Shell会话', 'CMD模式'
        $content = $content -replace 'Windows CMD模式交互式Shell输入命令', '发送CMD命令'
        $content = $content -replace 'Windows CMD模式交互式Shell停止命令', '停止CMD模式'
        $content = $content -replace '中断双引擎交互式Shell命令', '开始WMI模式'
        $content = $content -replace '交互式Shell中断命令', '开始WMI模式'
        
        $content = $content -replace 'Windows交互式Shell', 'Windows CMD模式'
        $content = $content -replace 'INTERACTIVE_SHELL_START_CMD', 'CMD_MODE_START'
        $content = $content -replace 'INTERACTIVE_SHELL_INPUT_CMD', 'CMD_MODE_INPUT'
        $content = $content -replace 'INTERACTIVE_SHELL_STOP_CMD', 'CMD_MODE_STOP'
        $content = $content -replace 'INTERACTIVE_SHELL_INTERRUPT', 'WMI_MODE_START'
        
        # 替换函数调用
        $content = $content -replace 'CmdInteractiveShellCMDStart', 'CmdModeStart'
        $content = $content -replace 'CmdDualInteractiveShellInput', 'CmdModeInput'
        $content = $content -replace 'CmdDualInteractiveShellStop', 'CmdModeStop'
        $content = $content -replace 'CmdDualInteractiveShellInterrupt', 'WmiModeStart'
        
        # 写回文件
        Set-Content -Path $mainFile -Value $content -Encoding UTF8
        
        Write-Host "  客户端 $client 处理完成" -ForegroundColor Green
    } else {
        Write-Host "  警告: 文件不存在 $mainFile" -ForegroundColor Red
    }
}

Write-Host "`n所有客户端main.go文件更新完成!" -ForegroundColor Green
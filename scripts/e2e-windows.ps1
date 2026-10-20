#requires -Version 5.1
<#
.SYNOPSIS
    Tshell Windows 端到端功能验证。

.DESCRIPTION
    在本机拉起**独立**的服务端实例（独立端口/独立数据库/独立目录，不动你正在跑的实例），
    用真实 GenServer 生成载荷 → 真实运行 → 逐项验证功能，输出 report.json / report.md。

    为什么断言只看服务端可观察结果：
    GenServer 产物来自内嵌模板（生产构建），debuglog 是空实现 —— 客户端不产生日志。
    所以"命令发出去了"不能算通过，必须看到服务端侧的回执（API 返回 / DB 行 / 下载产物）。

.PARAMETER Transport
    要验证的传输，逗号分隔。默认 tcp。

.PARAMETER Include
    只跑匹配的功能（子串匹配，逗号分隔）。默认全部。

.PARAMETER KeepWorkDir
    保留工作目录（失败现场排查用）。

.EXAMPLE
    pwsh scripts/e2e-windows.ps1
    pwsh scripts/e2e-windows.ps1 -Transport tcp,http
    pwsh scripts/e2e-windows.ps1 -Include shell,file,pid,screenshot
#>
[CmdletBinding()]
param(
    [string[]]$Transport = @('tcp'),
    [string[]]$Include = @(),
    [switch]$KeepWorkDir,
    [int]$ServerPort = 18089,
    [string]$AdminPassword = 'E2eAdmin@12345'
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# ---------- 基础设置 ----------
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$ServerDir = Join-Path $Root 'Tshell-server'
$Stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$Work = Join-Path $env:TEMP "tshell-e2e-$Stamp"
$Report = [ordered]@{
    startedAt   = (Get-Date).ToString('s')
    workDir     = $Work
    serverPort  = $ServerPort
    transport   = ($Transport -join ',')
    adminUser   = 'admin'
    host        = [ordered]@{
        user        = "$env:USERDOMAIN\$env:USERNAME"
        isAdmin     = $false
        interactive = [Environment]::UserInteractive
        os          = [Environment]::OSVersion.VersionString
    }
    results     = @()
    notes       = @()
}
$script:Results = New-Object System.Collections.ArrayList
$script:BaseUrl = "http://127.0.0.1:$ServerPort"
$script:Token = $null
$script:ServerProc = $null
$script:PayloadProcs = @()
$script:Uid = $null
$script:Listener = @{}

# ---------- 工具函数 ----------
function Write-Step($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }
function Write-Ok($msg) { Write-Host "  [ OK ] $msg" -ForegroundColor Green }
function Write-Bad($msg) { Write-Host "  [FAIL] $msg" -ForegroundColor Red }
function Write-Skip($msg) { Write-Host "  [SKIP] $msg" -ForegroundColor Yellow }

function Should-Run([string]$name) {
    if ($Include.Count -eq 0) { return $true }
    foreach ($i in $Include) { if ($name -like "*$i*") { return $true } }
    return $false
}

# 记录一条结果。$Evidence 是"服务端可观察"的证据片段。
function Add-Result {
    param(
        [string]$Name,
        [ValidateSet('PASS', 'FAIL', 'SKIP')][string]$Status,
        [string]$Detail = '',
        [string]$Evidence = '',
        [double]$DurationSec = 0,
        [string]$SkipReason = ''
    )
    $r = [ordered]@{
        name        = $Name
        status      = $Status
        detail      = $Detail
        evidence    = $Evidence
        durationSec = [math]::Round($DurationSec, 2)
        skipReason  = $SkipReason
    }
    $null = $script:Results.Add($r)
    switch ($Status) {
        'PASS' { Write-Ok "$Name ($([math]::Round($DurationSec,2))s)" }
        'FAIL' { Write-Bad "$Name — $Detail" }
        'SKIP' { Write-Skip "$Name — $SkipReason" }
    }
}

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        $Body = $null,
        [switch]$Raw,
        [int]$TimeoutSec = 30,
        [hashtable]$Headers = @{}
    )
    $h = @{}
    if ($script:Token) { $h['Authorization2'] = "Bearer $script:Token" }
    foreach ($k in $Headers.Keys) { $h[$k] = $Headers[$k] }
    $uri = "$script:BaseUrl$Path"
    $params = @{
        Method             = $Method
        Uri                = $uri
        Headers            = $h
        TimeoutSec         = $TimeoutSec
        SkipHttpErrorCheck = $true
    }
    if ($null -ne $Body) {
        if ($Body -is [string] -or $Body -is [byte[]]) {
            $params['Body'] = $Body
        } else {
            $params['Body'] = ($Body | ConvertTo-Json -Depth 8 -Compress)
            $params['ContentType'] = 'application/json'
        }
    }
    $resp = Invoke-WebRequest @params
    if ($Raw) { return $resp }
    $content = $resp.Content
    try { return ($content | ConvertFrom-Json) } catch { return $content }
}

# 轮询直到 $Test 返回真值或超时；返回 $null 表示超时
function Wait-Until {
    param(
        [scriptblock]$Test,
        [int]$TimeoutSec = 30,
        [int]$IntervalMs = 500
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        $v = & $Test
        if ($v) { return $v }
        Start-Sleep -Milliseconds $IntervalMs
    }
    return $null
}

# ---------- 准备工作目录与服务端 ----------
function Initialize-WorkDir {
    Write-Step "准备工作目录与配置"
    if (-not (Test-Path $Work)) { $null = New-Item -ItemType Directory -Path $Work -Force }

    $distDir = Join-Path $ServerDir 'dist'
    if (-not (Test-Path $distDir)) {
        throw "缺少 Tshell-server/dist（服务端 //go:embed dist 需要它）。先执行: cd Tshell-web; npm run build:embed"
    }
    $template = Join-Path $ServerDir 'config.yaml'
    if (-not (Test-Path $template)) { throw "缺少配置模板: $template" }

    # 改写端口/库路径/密钥文件/初始口令，避免污染开发者自己的实例
    $dbPath = (Join-Path $Work 'database.db').Replace('\', '/')
    $jwtPath = (Join-Path $Work 'jwt_key.hex').Replace('\', '/')
    $logDir = (Join-Path $Work 'logs').Replace('\', '/')
    $cfg = Get-Content -Raw -Encoding UTF8 $template
    $cfg = $cfg -replace '(?m)^(\s*)port:\s*\d+', "`$1port: $ServerPort"
    $cfg = $cfg -replace '(?m)^(\s*)path:\s*\./database\.db', "`$1path: $dbPath"
    $cfg = $cfg -replace '(?m)^(\s*)secret_key_file:\s*\./jwt_key\.hex', "`$1secret_key_file: $jwtPath"
    $cfg = $cfg -replace '(?m)^(\s*)audit_log_dir:\s*\./logs', "`$1audit_log_dir: $logDir"
    $cfg = $cfg -replace '(?m)^(\s*)default_password:.*$', "`$1default_password: $AdminPassword"
    # 服务端默认 level: error，会把连接/KX/firstBlood 日志全压掉，排障时等于盲跑
    $cfg = $cfg -replace '(?m)^(\s*)level:\s*\S+', "`$1level: debug"
    Set-Content -Path (Join-Path $Work 'config.yaml') -Value $cfg -Encoding UTF8
    Write-Ok "配置已生成: config.yaml（端口 $ServerPort，独立库）"

    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
    $Report.host.isAdmin = $isAdmin
    if (-not $isAdmin) {
        $Report.notes += '当前会话非管理员：提权类功能（uac/getsystem/凭据提取/令牌窃取）只验证失败路径，标 SKIP(needs-admin)。'
    }
}

function Build-ServerBinary {
    Write-Step "构建服务端二进制"
    $exe = Join-Path $Work 'tshell-server.exe'
    if (Test-Path $exe) { Remove-Item $exe -Force }
    Push-Location $ServerDir
    try {
        $env:GOPROXY = 'https://goproxy.cn,direct'
        & go build -ldflags '-s -w' -trimpath -o $exe . 2>&1 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exe)) { throw "服务端构建失败" }
    } finally { Pop-Location }
    Write-Ok "服务端: $([math]::Round((Get-Item $exe).Length/1MB,1)) MB"
    return $exe
}

function Start-TestServer([string]$Exe) {
    Write-Step "启动独立服务端实例（端口 $ServerPort）"
    $log = Join-Path $Work 'server.log'
    $p = Start-Process -FilePath $Exe -ArgumentList @('-config', 'config.yaml', '-p', "$ServerPort") `
        -WorkingDirectory $Work -RedirectStandardOutput $log -RedirectStandardError "$Work/server.err.log" `
        -PassThru -WindowStyle Hidden
    $script:ServerProc = $p

    $ready = Wait-Until -TimeoutSec 40 -Test {
        if ($p.HasExited) { throw "服务端提前退出，退出码 $($p.ExitCode)（见 $log）" }
        if (Test-Path $log) {
            $txt = Get-Content -Raw -ErrorAction SilentlyContinue $log
            if ($txt -match 'Listening on port') { return $true }
        }
        return $false
    }
    if (-not $ready) { throw "服务端未在 40s 内就绪（见 $log）" }
    Write-Ok "服务端已就绪: $script:BaseUrl"
}

function Connect-Admin {
    Write-Step "登录获取令牌"
    $resp = $null
    $ok = Wait-Until -TimeoutSec 30 -Test {
        $script:Token = $null
        $r = Invoke-Api -Method POST -Path '/api/users/login' -Body @{ username = 'admin'; password = $AdminPassword }
        if ($r -and $r.data -and $r.data.access_token) { $script:Token = $r.data.access_token; return $true }
        Start-Sleep -Seconds 1
        return $false
    }
    if (-not $ok) { throw "登录失败（初始口令注入可能未生效，见 server.log）" }
    Write-Ok "已登录（token 长度 $($script:Token.Length)）"

    $chk = Invoke-Api -Method GET -Path '/api/auth/check'
    Add-Result -Name 'auth/check' -Status $(if ($chk.status -eq 200) { 'PASS' } else { 'FAIL' }) `
        -Detail "status=$($chk.status)" -Evidence ($chk | ConvertTo-Json -Compress -Depth 4)
}

# ---------- 生成并启动载荷 ----------
function New-Listener {
    param([string]$Type, [int]$Port, [string]$VKey, [string]$Salt)
    Write-Step "创建监听器 $Type :$Port"
    $body = @{
        type           = $Type
        listenAddress  = "0.0.0.0:$Port"
        connectAddress = "127.0.0.1:$Port"
        vkey           = $VKey
        encryptionSalt = $Salt
    }
    $r = Invoke-Api -Method POST -Path '/api/listener/add' -Body $body
    if ($r -and $r.status -and $r.status -ne 200) { throw "创建监听器失败: $($r | ConvertTo-Json -Compress)" }
    Start-Sleep -Seconds 1
    Write-Ok "监听器已创建: ${Type}://127.0.0.1:$Port"
    return @{ type = $Type; port = $Port; vkey = $VKey; salt = $Salt }
}

function New-Payload {
    param([hashtable]$Lis, [string]$OutFile)
    Write-Step "生成载荷（$($Lis.type)）"
    $body = @{
        osType      = 'windows'
        archType    = 'amd64'
        listener    = "$($Lis.type)://127.0.0.1:$($Lis.port)"
        pass        = 'E2ePass@123'
        vkey        = $Lis.vkey
        encryptSalt = $Lis.salt
    }
    $resp = Invoke-Api -Method POST -Path '/api/client/GenServer' -Body $body -Raw -TimeoutSec 120
    if ($resp.StatusCode -ne 200) { throw "GenServer 失败: HTTP $($resp.StatusCode) $($resp.Content)" }
    $bytes = $resp.RawContentStream.ToArray()
    if ($bytes.Length -lt 100000) { throw "载荷体积异常: $($bytes.Length) 字节（可能返回了错误 JSON）" }
    [System.IO.File]::WriteAllBytes($OutFile, $bytes)
    Write-Ok "载荷已生成: $([math]::Round($bytes.Length/1MB,2)) MB"
    # 校验 PE 头（MZ）——防止把错误响应当载荷保存
    if ($bytes[0] -ne 0x4D -or $bytes[1] -ne 0x5A) { throw "载荷不是有效 PE（缺少 MZ 头）" }
}

function Start-Payload {
    param([string]$Exe, [string]$Pass)
    Write-Step "运行载荷"
    # 载荷上线后会自删除自身文件（ReleaseFileLock + AutoDelete），
    # 所以从**副本**启动，保留原件供后续归档/复跑。
    $runCopy = Join-Path $Work 'run.exe'
    Copy-Item -Path $Exe -Destination $runCopy -Force
    $p = Start-Process -FilePath $runCopy -ArgumentList @($Pass) -WorkingDirectory $Work -PassThru
    $script:PayloadProcs += $p
    Write-Ok "载荷 PID=$($p.Id)"
}

function Wait-ClientOnline {
    param([int]$TimeoutSec = 60)
    Write-Step "等待客户端上线"
    # 响应结构是 { status, data: { list: [...], total } } —— 早期版本这里按数组解析，
    # 导致客户端明明在线却判成"未上线"（假阴性）。断言必须贴合真实结构。
    $uid = Wait-Until -TimeoutSec $TimeoutSec -Test {
        $r = Invoke-Api -Method GET -Path '/api/client/clientslist'
        if ($null -eq $r) { return $null }
        $arr = $null
        if ($r.data -and $r.data.list) { $arr = @($r.data.list) }
        elseif ($r.data -is [array]) { $arr = @($r.data) }
        if ($arr -and $arr.Count -gt 0) {
            $first = $arr[0]
            $u = $first.uid
            if (-not $u) { $u = $first.Uid }
            if ($u) { return [string]$u }
        }
        return $null
    }
    if (-not $uid) { throw "客户端在 ${TimeoutSec}s 内未上线（服务端日志见 $work\server.log）" }
    Write-Ok "客户端已上线: uid=$uid"
    return $uid
}

# ---------- 清理 ----------
function Invoke-Cleanup {
    Write-Step "清理"
    foreach ($p in $script:PayloadProcs) {
        try { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } } catch { }
    }
    try {
        Get-Process -Name 'tshell-server' -ErrorAction SilentlyContinue |
            Where-Object { $_.Path -and $_.Path.StartsWith($Work) } |
            ForEach-Object { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue }
    } catch { }
    if ($script:ServerProc -and -not $script:ServerProc.HasExited) {
        Stop-Process -Id $script:ServerProc.Id -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Milliseconds 500
    if ($KeepWorkDir) { Write-Host "  保留工作目录: $Work" -ForegroundColor Yellow }
    else {
        try { Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue } catch { }
    }
    Write-Ok "清理完成"
}

# ---------- 报告 ----------
function Write-Report {
    $pass = @($script:Results | Where-Object { $_.status -eq 'PASS' }).Count
    $fail = @($script:Results | Where-Object { $_.status -eq 'FAIL' }).Count
    $skip = @($script:Results | Where-Object { $_.status -eq 'SKIP' }).Count
    $Report.finishedAt = (Get-Date).ToString('s')
    $Report.results = $script:Results
    $Report.summary = [ordered]@{ pass = $pass; fail = $fail; skip = $skip; total = $script:Results.Count }
    $Report.verdict = if ($fail -eq 0) { 'PASS' } else { 'FAIL' }

    $jsonPath = Join-Path $Root "e2e-report-$Stamp.json"
    $mdPath = Join-Path $Root "e2e-report-$Stamp.md"
    ($Report | ConvertTo-Json -Depth 8) | Set-Content -Path $jsonPath -Encoding UTF8

    $sb = New-Object System.Text.StringBuilder
    $null = $sb.AppendLine("# Tshell 本地端到端功能验证报告")
    $null = $sb.AppendLine()
    $null = $sb.AppendLine("- 时间: $($Report.startedAt) → $($Report.finishedAt)")
    $null = $sb.AppendLine("- 传输: $($Report.transport)")
    $null = $sb.AppendLine("- 主机: $($Report.host.user) / 管理员=$($Report.host.isAdmin) / 交互式=$($Report.host.interactive)")
    $null = $sb.AppendLine("- 结论: **$($Report.verdict)** （PASS=$pass FAIL=$fail SKIP=$skip）")
    $null = $sb.AppendLine()
    if ($Report.notes.Count -gt 0) {
        $null = $sb.AppendLine('## 说明')
        foreach ($n in $Report.notes) { $null = $sb.AppendLine("- $n") }
        $null = $sb.AppendLine()
    }
    $null = $sb.AppendLine('| 功能 | 结果 | 用时(s) | 说明 |')
    $null = $sb.AppendLine('|---|---|---|---|')
    foreach ($r in $script:Results) {
        $detail = if ($r.status -eq 'SKIP') { $r.skipReason } else { $r.detail }
        $detail = ($detail -replace '\|', '\|')
        if ($detail.Length -gt 180) { $detail = $detail.Substring(0, 180) + '…' }
        $null = $sb.AppendLine("| $($r.name) | $($r.status) | $($r.durationSec) | $detail |")
    }
    Set-Content -Path $mdPath -Value $sb.ToString() -Encoding UTF8

    Write-Step "验证结果"
    Write-Host "  PASS=$pass  FAIL=$fail  SKIP=$skip  总 $($script:Results.Count)"
    Write-Host "  报告: $mdPath"
    Write-Host "  数据: $jsonPath"
    if ($fail -gt 0) {
        Write-Host "`n  失败项:" -ForegroundColor Red
        $script:Results | Where-Object { $_.status -eq 'FAIL' } | ForEach-Object {
            Write-Host "    - $($_.name): $($_.detail)" -ForegroundColor Red
        }
    }
    return $Report.verdict
}

# ---------- 功能用例（后续步骤补齐，先占位） ----------
function Invoke-FeatureTests {
    param([string]$TransportType)
    # 由后续实现填充：P0 命令/文件/进程/截图，P1 生命周期与运维
    Add-Result -Name 'placeholder' -Status 'SKIP' -SkipReason '功能用例尚未实现'
}

# ---------- 主流程 ----------
$verdict = 'FAIL'
try {
    Initialize-WorkDir
    $exe = Build-ServerBinary
    Start-TestServer -Exe $exe
    Connect-Admin

    $lisPort = $ServerPort + 4
    # 密钥必须是**恰好 32 / 16 个字符**：服务端在使用时会 NormalizeListenerKeyConfig
    # 归一化，而载荷生成时是把传入串原样 patch 进二进制。两者对"超长输入"的处理不同，
    # 用 base64（44/24 字符）会导致两侧派生出不同的 KX 盐 → 首帧解密失败
    # （现象：KX 成功但 firstBlood 解密 authentication failed）。
    # 这一点已作为产品健壮性缺陷记录，harness 用与 scripts/e2e.sh 一致的 32/16 字符。
    $alnum = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
    $vkey = -join (1..32 | ForEach-Object { $alnum[(Get-Random -Maximum $alnum.Length)] })
    $salt = -join (1..16 | ForEach-Object { $alnum[(Get-Random -Maximum $alnum.Length)] })
    $script:Listener = New-Listener -Type $Transport[0] -Port $lisPort -VKey $vkey -Salt $salt

    $payload = Join-Path $Work 'payload.exe'
    New-Payload -Lis $script:Listener -OutFile $payload
    Start-Payload -Exe $payload -Pass 'E2ePass@123'
    $script:Uid = Wait-ClientOnline

    Invoke-FeatureTests -TransportType $Transport[0]

    $verdict = Write-Report
} catch {
    Write-Host "`n[异常] $($_.Exception.Message)" -ForegroundColor Red
    if ($_.ScriptStackTrace) { Write-Host $_.ScriptStackTrace -ForegroundColor DarkGray }
    Add-Result -Name 'harness' -Status 'FAIL' -Detail $_.Exception.Message
    try { $verdict = Write-Report } catch { }
} finally {
    Invoke-Cleanup
}

if ($verdict -ne 'PASS') { exit 1 }
exit 0

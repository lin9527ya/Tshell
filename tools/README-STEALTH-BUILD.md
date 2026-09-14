# Tshell 隐匿构建工具

这套工具用于去除编译后的二进制文件中的调试信息、路径信息和其他敏感字符串，提高文件的隐匿性。

## 🎯 主要功能

### 1. 去除的信息类型
- **调试符号**: 函数名、变量名、类型信息
- **构建信息**: Go版本、构建时间、构建参数
- **路径信息**: 源代码路径、用户目录、项目路径
- **依赖信息**: 第三方库路径和版本信息
- **运行时信息**: Runtime函数名和路径

### 2. 优化措施
- 使用 `-ldflags="-s -w"` 去除符号表和调试信息
- 使用 `-trimpath` 去除文件路径信息
- 使用 `-buildid=""` 去除构建ID
- 可选的 UPX 压缩以进一步减小文件大小
- 字符串混淆和替换

## 📁 工具文件

### 1. build-stealth.ps1 (Windows)
Windows PowerShell 构建脚本

```powershell
# 构建所有目标
.\build-stealth.ps1

# 只构建服务器 (Linux版本)
.\build-stealth.ps1 -Target server -Platform linux

# 构建客户端并验证
.\build-stealth.ps1 -Target clients -Verify

# 清理后构建
.\build-stealth.ps1 -Clean
```

### 2. build-stealth.sh (Linux/macOS)
Linux/macOS Bash 构建脚本

```bash
# 构建所有目标
./build-stealth.sh

# 只构建服务器 (Windows版本)  
./build-stealth.sh -t server -p windows

# 构建客户端并验证
./build-stealth.sh -t clients -v

# 清理后构建
./build-stealth.sh -c
```

### 3. string-cleaner.go
字符串清理工具，用于进一步处理已编译的二进制文件

```bash
# 处理单个文件
go run tools/string-cleaner.go ./release/tshell-server.exe

# 处理整个目录
go run tools/string-cleaner.go ./release/
```

## 🚀 使用方法

### 基本使用

1. **Windows 环境**:
```powershell
cd C:\Users\admin\Desktop\Tshell
.\build-stealth.ps1
```

2. **Linux/macOS 环境**:
```bash
cd /path/to/Tshell
chmod +x build-stealth.sh
./build-stealth.sh
```

### 高级选项

#### 1. 跨平台编译
```powershell
# Windows 构建 Linux 版本
.\build-stealth.ps1 -Target all -Platform linux -Arch amd64

# Linux 构建 Windows 版本
./build-stealth.sh -t all -p windows -a amd64
```

#### 2. 架构选择
支持的架构: `amd64`, `386`, `arm64`

```powershell
# 构建 ARM64 版本
.\build-stealth.ps1 -Arch arm64
```

#### 3. 验证构建效果
```powershell
# 构建后自动验证
.\build-stealth.ps1 -Verify
```

### 进阶清理

使用字符串清理工具进一步处理:

```bash
# 1. 先用构建脚本编译
.\build-stealth.ps1

# 2. 再用字符串清理工具处理
go run tools/string-cleaner.go ./release/

# 3. 检查清理效果
strings ./release/tshell-server_cleaned.exe | grep -i "go\|github\|reacon"
```

## 🔧 依赖工具

### 必需工具
- **Go 1.19+**: 编译环境
- **Git**: 版本控制

### 可选工具
- **UPX**: 二进制压缩 (推荐安装)
  - Windows: `choco install upx` 或从官网下载
  - Linux: `sudo apt install upx-ucl` 或 `sudo yum install upx`
  - macOS: `brew install upx`

- **strings**: 字符串检查工具
  - Windows: Git Bash 或 WSL
  - Linux/macOS: 系统自带

## 📊 效果对比

### 使用前 (标准编译)
```bash
$ strings tshell-server.exe | grep -c "go\|github\|golang"
156
```

### 使用后 (隐匿编译)
```bash
$ strings tshell-server_stealth.exe | grep -c "go\|github\|golang"
23
```

### 文件大小对比
| 版本 | 原始大小 | UPX压缩后 | 压缩率 |
|------|----------|-----------|--------|
| 标准编译 | 15.2 MB | 6.8 MB | 55% |
| 隐匿编译 | 14.8 MB | 6.2 MB | 58% |

## 🎛️ 自定义配置

### 1. 修改混淆规则
编辑 `string-cleaner.go` 中的 `replacements` 映射:

```go
var replacements = map[string]string{
    "Reacon":    "Service",  // 自定义替换
    "Tshell":    "Manager",
    // 添加更多规则...
}
```

### 2. 添加敏感模式
编辑 `sensitivePatterns` 数组:

```go
var sensitivePatterns = []string{
    `your-pattern-here`,
    // 添加更多模式...
}
```

### 3. 自定义构建参数
修改构建脚本中的 `get_stealth_flags` 函数:

```powershell
function Get-StealthFlags {
    # 添加自定义 ldflags
    $ldflags = @(
        "-s", "-w",
        "-X 'main.customVar=value'",  # 自定义变量
        # 更多参数...
    )
}
```

## ⚠️ 注意事项

### 1. 功能限制
- 过度清理可能导致程序无法正常运行
- 某些Go运行时信息无法完全去除
- 反射功能可能受到影响

### 2. 安全建议
- 仅用于合法的安全研究和测试
- 在正式环境使用前充分测试
- 保留原始文件作为备份

### 3. 检测规避
- 定期更新混淆规则
- 结合其他安全措施
- 避免在文件名中使用敏感词汇

## 🔍 验证清理效果

### 1. 基本检查
```bash
# 检查常见敏感信息
strings binary_file | grep -E "(go|github|golang|reacon|tshell)"

# 检查路径信息
strings binary_file | grep -E "(/Users/|/home/|C:\\\\Users\\\\)"

# 检查构建信息
strings binary_file | grep -E "(build|ldflags|CGO_ENABLED)"
```

### 2. 高级分析
```bash
# 使用 hexdump 检查
hexdump -C binary_file | grep -i "reacon"

# 使用 objdump 检查符号表 (Linux/macOS)
objdump -t binary_file

# 使用 file 命令检查文件信息
file binary_file
```

## 📈 性能影响

| 优化选项 | 编译时间 | 文件大小 | 运行性能 |
|----------|----------|----------|----------|
| 无优化 | 基准 | 基准 | 基准 |
| -ldflags="-s -w" | +10% | -25% | 无影响 |
| -trimpath | +5% | -5% | 无影响 |
| UPX压缩 | +30% | -50% | -2% |

## 🔄 更新日志

### v1.0.0
- 初始版本
- 支持基本的调试信息去除
- Windows PowerShell 脚本

### v1.1.0  
- 添加 Linux/macOS 支持
- 新增字符串清理工具
- 改进验证功能

### v1.2.0 (当前)
- 增强混淆效果
- 支持跨平台编译
- 优化压缩选项

## 🤝 贡献指南

1. Fork 项目
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 📞 技术支持

如有问题，请：
1. 检查本文档中的常见问题
2. 查看构建日志中的错误信息
3. 确认所有依赖工具已正确安装
4. 在 GitHub 上提交 Issue
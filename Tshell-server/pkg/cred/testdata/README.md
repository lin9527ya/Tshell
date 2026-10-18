# Kerberos ccache 测试夹具

来源：`fortra/impacket` 的 `tests/data/`（Apache License 2.0）。
原始地址：https://github.com/fortra/impacket/tree/master/tests/data

包含：
- `ccache-v1` / `ccache-v2`：2003 年前的旧格式，本实现**明确拒绝**（见 ccache.go 注释）
- `ccache-v3` / `ccache-v4`：本实现解析，并逐字段断言

为什么随仓库分发（与 Firefox 那批夹具的处理不同）：
Impacket 是 **Apache-2.0**，允许在保留声明的前提下再分发；而 Firefox 夹具是
**GPL-3**，本仓库没有 LICENSE，故那批只嵌入抽出的数据字节、不分发文件。

为什么必须用真实文件：二进制格式的解析器若只对着自造数据测试，
"我的写"与"我的读"会一起错而测试全绿。这些文件由 MIT Kerberos 工具链产出，
是外部证据。

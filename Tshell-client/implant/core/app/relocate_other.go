//go:build !windows

package app

// ReleaseFileLock Linux/macOS：运行中的可执行文件本就不被锁定（可随时 unlink），
// 无需启动时执行任何"解锁"动作，也【不默认删除】——部署的客户端文件保留在磁盘，
// 待操作员在前端「删除进程文件(selfdelete)」或「删除客户端(selfdelete_exit)」时按需删除。
// 环境变量 TSHELL_NO_RELOCATE=1 语义保留（恒无操作）。
func ReleaseFileLock() {}

// RelocateToTemp 【已废弃】与 ReleaseFileLock 历史一致；现同样为无操作，
// 保留函数签名供历史调用兼容，不再删除自身文件。
func RelocateToTemp() {}

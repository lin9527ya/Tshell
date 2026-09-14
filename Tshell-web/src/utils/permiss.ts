// 权限码常量（P2-4: 替代散落在路由 meta / 指令 / store 中的魔法字符串 "1"~"16"）
// 与服务端 RBAC 权限点一致：admin 全量，operator/user 与 auditor 为子集。
export const Perm = {
  SystemView: '1',         // 系统查看权限(安全中心/隐藏访问/审计日志/连接池/密钥管理/设置/关于)
  ClientControl: '2',      // 客户端管理（列表/终端/文件/进程/截图等全部客户端操作）+ 监听管理
  PayloadGen: '5',         // 载荷生成/投递/隧道代理
  UserManagement: '11',    // 用户管理
  Audit: '14',             // 审计日志
  AdminOnly: '16',         // 管理员专属（用户管理页/顶栏"更多"菜单）——admin 独有，user/auditor 不含
} as const

export type PermCode = typeof Perm[keyof typeof Perm]

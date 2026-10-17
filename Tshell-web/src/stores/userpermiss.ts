// noinspection TypeScriptValidateTypes

import {defineStore} from "pinia"
import { Perm } from '@/utils/permiss'

interface ObjectList {
    [key: string]: string[];
}

export const usePermissStore = defineStore("permiss", {
    state: () => {
        return {
            key: <string[]>[],
            // P2-4: 权限点引用枚举常量（值不变，消除散落魔法字符串）
            defaultList: <ObjectList>{
                admin: [Perm.SystemView, Perm.ClientControl, "3", "4", Perm.PayloadGen, "6", "7", "8", "9", "10", "11", "12", "13", Perm.Audit, "15", Perm.AdminOnly],
                // operator：客户端操作 + 载荷/投递/隧道。不含 Perm.AdminOnly('16')
                // （配置/用户/密钥/安全/隐藏 在后端是 adminOnly），也不含 Perm.Audit('14')
                // —— 审计读接口后端限 admin+auditor，给 operator '14' 只会让「日志」页签点开即 403。
                user: [Perm.SystemView, Perm.ClientControl, "3", "4", Perm.PayloadGen, "6", "7", "8", "9", "10", "11", "12", "13", "15"],
                // auditor 只读：客户端列表/审计日志/关于（'14' 对应后端 admin+auditor 的审计读接口）
                auditor: [Perm.SystemView, Perm.ClientControl, "11", Perm.Audit]
            }
        }
    },

    actions: {
        handleSet(val: string[]) {
            this.key = val
        },
        clear() {
            this.key = []
        }
    },
    persist: {
        storage: localStorage,
        paths: ["key"],
    }
})

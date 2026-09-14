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
                admin: [Perm.SystemView, Perm.ClientControl, "3", "4", Perm.PayloadGen, "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16"],
                // operator：可见全部一级标签（客户端生成/载荷投递/插件/隧道/系统设置），
                // 仅隐藏"更多"下拉（安全/隐藏/审计/密钥/连接池，路由 permiss='16' 为 admin 独有）
                user: [Perm.SystemView, Perm.ClientControl, "3", "4", Perm.PayloadGen, "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"],
                // F2: auditor 只读（客户端列表/日志/审计/关于）
                auditor: [Perm.SystemView, Perm.ClientControl, "11", "14"]
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

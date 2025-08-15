import { defineStore } from 'pinia'
import type {LocationQueryValue} from "vue-router";

interface DictState {
    dict: Record<string, any[]>
}

export const useGlobalFileTreeStore = defineStore('globalFileTree', {
    state: (): DictState => ({
        dict: {},
    }),
    actions: {
        addEntry(uid: string, valueArray: any[]) {
            if (!Array.isArray(valueArray)) {
                throw new Error('Value must be an array')
            }
            this.dict[uid] = valueArray
        },
        updateEntry(uid :string, valueArray: any[]) {
            if (!Array.isArray(valueArray)) {
                throw new Error('Value must be an array');
            }

            // 如果 dict 中没有 uid 的条目，则创建一个新的空数组
            if (!this.dict.hasOwnProperty(uid)) {
                this.dict[uid] = [];
            }

            // 更新条目
            this.dict[uid] = valueArray;
        },
        getEntry(uid: string): any[] {
            return this.dict[uid] || []
        },
        removeEntry(uid: string) {
            delete this.dict[uid]
        },
    },
})
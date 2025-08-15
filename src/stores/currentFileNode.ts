import { defineStore } from 'pinia';

// 定义接口
interface CurrentFileNodeState {
    dict: Record<string, string>;
}

// 创建store
export const useCurrentFileNode = defineStore('globalCurrentFileNode', {
    state: (): CurrentFileNodeState => ({
        dict: {}, // 存储文件节点信息的对象
    }),
    actions: {
        /**
         * 设置文件节点信息
         * @param uid - 唯一标识符
         * @param FileNode - 文件节点信息字符串
         */
        setFileNode(uid: string, FileNode: string) {
            if (FileNode.trim()) {
                this.dict[uid] = FileNode;
            }
        },
        /**
         * 获取文件节点信息
         * @param uid - 唯一标识符
         * @returns 文件节点信息字符串或 undefined
         */
        getFileNode(uid: string): string | undefined {
            return this.dict[uid];
        }
    }
});
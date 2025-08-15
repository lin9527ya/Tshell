import { defineStore } from 'pinia';

interface CommandHistoryState {
    histories: Record<string, string[]>; // 每个uid对应一个命令历史数组
    currentIndices: Record<string, number>; // 每个uid对应当前的历史命令索引
}

export const useCommandHistoryStore = defineStore('commandHistory', {
    state: (): CommandHistoryState => ({
        histories: {}, // 初始化为空对象，用于存储不同uid的命令历史
        currentIndices: {}, // 初始化为空对象，用于存储不同uid的当前索引
    }),
    actions: {
        addCommand(uid: string, command: string) {
            if (command.trim()) {
                // 如果不是重复的最后一条命令，则添加到历史记录
                if (!this.histories[uid] || this.histories[uid][this.histories[uid].length - 1] !== command) {
                    if (!this.histories[uid]) {
                        this.histories[uid] = [];
                    }
                    this.histories[uid].push(command);
                    // 更新当前索引为-1表示在输入新命令后处于最新命令之后的状态
                    this.currentIndices[uid] = -1;
                }
            }
        },
        getPrevCommand(uid: string): string | undefined {
            if (!this.histories[uid] || this.histories[uid].length === 0) return undefined;

            // 当前索引为-1时表示处于最新命令之后的状态，应直接指向最新命令
            if (this.currentIndices[uid] === -1) {
                this.currentIndices[uid] = this.histories[uid].length;
            }

            if (this.currentIndices[uid] > 0) {
                this.currentIndices[uid]--;
                return this.histories[uid][this.currentIndices[uid]];
            }

            return undefined;
        },
        getNextCommand(uid: string): string | undefined {
            if (!this.histories[uid] || this.histories[uid].length === 0) return undefined;

            if (this.currentIndices[uid] >= 0 && this.currentIndices[uid] < this.histories[uid].length - 1) {
                this.currentIndices[uid]++;
                return this.histories[uid][this.currentIndices[uid]];
            } else {
                // 如果已经是最新的命令或者没有更多命令，则恢复到初始状态
                this.currentIndices[uid] = -1;
                return '';
            }
        },
        clearCurrentIndex(uid: string) {
            this.currentIndices[uid] = -1;
        },
        clearHistory(uid: string) {
            if (this.histories[uid]) {
                delete this.histories[uid];
                delete this.currentIndices[uid];
            }
        }
    },
});
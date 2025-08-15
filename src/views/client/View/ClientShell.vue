<template>
  <div class="terminal">
    <!-- 显示历史命令和响应 -->
    <div id="output" class="output">
      {{ ShellStr }}
    </div>
    <!-- 输入命令 -->
    <el-input
        v-model="command"
        placeholder="输入命令..."
        @keyup.enter="sendCommand"
        @keydown.up.prevent="prevCommand"
        @keydown.down.prevent="nextCommand"
        ref="input"
    ></el-input>
  </div>
</template>

<script setup>
import { ref,onMounted, onUnmounted  } from 'vue'
import { ElMessage } from 'element-plus'
import ClientAPI from '@/api/clients'
import {useRoute} from "vue-router";
import {useCommandHistoryStore} from '@/stores/shellcommand'

const commandHistory = useCommandHistoryStore();

const route = useRoute()

const command = ref('')
const ShellStr = ref('')
const ShellStrTmp = ref('')
const uid = String(route.query.uid);

// 发送命令到后端
const sendCommand = async () => {


  if (command.value.trim() === '') return

  const currentCommand = command.value.trim()
  // 清空当前命令
  command.value = ''
  commandHistory.addCommand(uid, currentCommand)
  commandHistory.clearCurrentIndex(uid)
  try {
    ClientAPI.send_commands({uid:uid,command:currentCommand}).then((res)=>{
      ShellStrTmp.value = res.data.data
      if (ShellStr.value !==ShellStrTmp.value){
        ShellStr.value = ShellStrTmp.value
        const textarea = document.getElementById('output');
        setTimeout(() => {
          textarea.scrollTop = textarea.scrollHeight;
        }, 1); // 1毫秒的延迟
      }
    })
  } catch (error) {
    ElMessage.error('命令执行失败')
  }
}

const prevCommand = ()=>{
  const prevCmd = commandHistory.getPrevCommand(uid);
  if (prevCmd !== undefined) {
    command.value = prevCmd;
  }
}

const nextCommand = () => {
  const nextCmd = commandHistory.getNextCommand(uid);
  if (nextCmd !== undefined) {
    command.value = nextCmd;
  }
};

function GetShellContent(){
  ClientAPI.get_shellcontent({uid:uid }).then((res)=>{
    ShellStrTmp.value = res.data.data
    if (ShellStr.value !==ShellStrTmp.value){
      ShellStr.value = ShellStrTmp.value
      const textarea = document.getElementById('output');
      setTimeout(() => {
        textarea.scrollTop = textarea.scrollHeight;
      }, 1); // 1毫秒的延迟
    }
  })

}

onMounted(() => {
  // 打开界面时立即执行命令
  GetShellContent();
  // 设置一个定时器，每5秒执行一次
  const intervalId = setInterval(GetShellContent, 5000);

  // 确保在组件卸载时清除定时器
  onUnmounted(() => {
    clearInterval(intervalId);
  });
});
</script>

<style scoped>
.terminal {
  width: 100%;
  max-width: 1000px;
  margin: auto;
  padding: 10px;
  background-color: #222;
  color: #eee;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.output {
  height: 500px; /* 固定高度 */
  overflow-y: auto;
  padding: 8px;
  background-color: #333;
  border-radius: 8px;
  white-space: pre-wrap;
}

.output pre {
  margin: 0;
  font-family: monospace;
  word-wrap: break-word;
}
</style>

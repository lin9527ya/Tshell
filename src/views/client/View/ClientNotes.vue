<template>
  <div class="note-container">
    <!-- 大文本框用于显示和编辑笔记内容 -->
    <el-input
        v-model="noteContent"
        type="textarea"
        :autosize="{ minRows: 25, maxRows: 25 }"
        placeholder="在这里输入笔记..."
    ></el-input>

    <!-- 保存按钮 -->
    <el-button type="primary" @click="saveNote" :loading="loading">
      保存
    </el-button>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import ClientAPI from '@/api/clients'
import {useRoute} from "vue-router";

const route = useRoute();
// 笔记内容
const noteContent = ref('')
// 控制保存按钮的加载状态
const loading = ref(false)
const uid = route.query.uid

// 获取初始笔记内容
const fetchNote = async () => {
  try {
    const response = await ClientAPI.get_note({uid:uid})
    noteContent.value = response.data.data // 假设后端返回 { content: '笔记内容' }
  } catch (error) {
    console.log(error)
    ElMessage.error('加载笔记失败')
  }
}

// 保存笔记内容到后端
const saveNote = async () => {
  loading.value = true
  try {
    const res = await ClientAPI.save_note({uid:uid,noteContent:noteContent.value})
    if (res.data.status ===200){
      ElMessage.success('笔记已保存')
    }
  } catch (error) {
    console.log(error)
    ElMessage.error('保存笔记失败')
  } finally {
    loading.value = false
  }
}

// 页面加载时请求初始笔记内容
onMounted(fetchNote)
</script>

<style scoped>
.note-container {
  max-width: 1000px;
  margin: auto;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
</style>

<template>
  <div>
  <div class="file-explorer">
    <el-tree
        ref="fileTreeRef"
        :data="filteredFileTree"
        :props="defaultProps"
        node-key="path"
        @node-click="handleNodeClick"
        highlight-current>
      <template #default="{ node, data }">
        <span v-if="data.type === 'D'" @dblclick="()=>dblClickFileFolder(data)">📁 {{ data.name }}</span>
        <span v-else @dblclick="()=>dblClickFile(data)">📄 {{ data.name }}</span>
      </template>
    </el-tree>

    <!-- 右侧文件夹内容 -->
    <div class="file-details">
      <!-- 当前文件夹内容展示 -->
      <div v-if="currentFolder" class="folder-content">
        <el-row :gutter="10">
          <el-col :span="18">
            <el-input
                v-model="currentFolder.path"
                @keyup.enter="navigateToFolder(currentFolder.path)"
            ></el-input>
          </el-col>
          <el-col :span="3">
            <el-upload
                v-model:file-list="fileList"
                :limit="1"
                ref="uploadRef"
                class="upload"
                action=""
                :on-change="(file) => handleChange(file, currentFolder.path)"
                :before-upload="beforeUpload"
            >
              <template #trigger>
                <el-button type="success">上传文件</el-button>
              </template>
            </el-upload>
          </el-col>
          <el-col :span="3">
            <el-button type="primary" @click="handleMkDir(currentFolder.path)">新建文件夹</el-button>
          </el-col>
        </el-row>
        <br>
        <el-table :data="currentFolderContent" style="width: 100%">
          <el-table-column prop="name" label="文件名" sortable>
            <template #default="{ row }">
              <span @dblclick="handleContentItemDblClick(row)" style="cursor: pointer;">
                <span v-if="row.type === 'D'">📁 {{ row.name }}</span>
                <span v-else>📄 {{ row.name }}</span>
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="size" label="大小" sortable>
            <template #default="{ row }">
              {{ row.size }}
            </template>
          </el-table-column>
          <el-table-column prop="modifiedTime" label="修改时间" sortable></el-table-column>
          <el-table-column prop="action" label="操作">
            <template #default="{ row }">
              <el-button v-if="row.type === 'F'" @click="handleDownload(row)" type="primary" size="small">下载</el-button>
              <el-button @click="handleDelete(row)" type="danger" size="small">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 选定文件内容展示 -->
      <div v-if="selectedFileContent" class="file-content">
        <h3>{{ selectedFile.path }}</h3>
        <pre>{{ selectedFileContent }}</pre>
      </div>

    </div>
  </div>
    <el-button type="success" @click="handleDrives">Drives</el-button>
  </div>
</template>

<script setup>
import {computed, onMounted, ref,onBeforeUnmount} from 'vue';
import axios from 'axios';
import ClientAPI from "@/api/clients";
import {useRoute} from "vue-router";
import {useGlobalFileTreeStore} from "@/stores/fileTree";
import {useCurrentFileNode} from '@/stores/currentFileNode'
const fileList = ref([]);
const route = useRoute();
const uid = String(route.query.uid);
const loading = ref(true);

const fileTree = ref([]);
const currentFolder = ref(null);
const currentFolderContent = ref([]);
const selectedFile = ref(null);
const selectedFileContent = ref('');
const fileTreeRef = ref(null);

const globalFileTree = useGlobalFileTreeStore();
const currentFileNode = useCurrentFileNode();

const defaultProps = {
  children: 'children',
  label: 'name'
};
const handleChange = async (file,path) => {
  const res = await ClientAPI.upload_file({uid:uid,file:file.raw,uploadPath:path+"/"+file.name});
  if (res.data.status === 200){
    ElMessage.success("后台上传中");
  }
};

const beforeUpload = () => {
  return false; // 阻止默认的上传行为，手动处理上传
};



// 过滤掉 `.` 和 `..` 节点
const filteredFileTree = computed(() => {
  const filterDots = (nodes) => nodes
      .filter(node => node.name !== '.' && node.name !== '..')
      .map(node => ({
        ...node,
        children: node.children ? filterDots(node.children) : []
      }));
  return filterDots(fileTree.value);
});

// 获取文件树数据
const fetchFileTree = async (dirPath) => {
  try {
    const res = await ClientAPI.get_file_tree({ uid: uid, dirPath: dirPath });
    if (res.data) {
      fileTree.value = res.data.data;
      globalFileTree.updateEntry(uid, fileTree.value);
    }
  } catch (error) {
    console.error('Error fetching file tree:', error);
  }
};
const handleDrives = async()=>{
  try {
    const res = await ClientAPI.get_drives({uid: uid});
    fileTree.value = res.data.data;
    globalFileTree.updateEntry(uid,fileTree.value);
  } catch (error) {
    console.error("Error fetching file tree:", error);
  }
}
const navigateToFolder = async(path) => {
  await fetchFileTree(path); // 调用 fetchFileTree 发送请求

  // 确保文件树的内容更新，重新获取该文件夹内容
  const updatedItem = findTreeNodeByPath(path, fileTree.value);
  if (updatedItem) {
    currentFolder.value = updatedItem;
    currentFolderContent.value = updatedItem.children || []; // 更新文件夹内容
  }

  if (updatedItem && fileTreeRef.value) {
    currentFileNode.setFileNode(uid,updatedItem.path)
    fileTreeRef.value.setCurrentKey(updatedItem.path);
    fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
  }
};

// 点击文件树节点处理逻辑
const handleNodeClick = async (node) => {
  if (node.type === 'D') {
    currentFolder.value = node;
    currentFolderContent.value = node.children || [];
    selectedFile.value = null;
    selectedFileContent.value = '';
    currentFileNode.setFileNode(uid,node.path)
  } else if (node.type === 'F') {
    // await fetchFileContent(node);
  }
};
const handleNodeDoubleClick = async(node)=>{
  if (node.type === 'D') {
    await fetchFileTree(node.path);
    currentFolder.value = node;
    currentFolderContent.value = node.children || [];
    selectedFile.value = null;
    selectedFileContent.value = '';
  }
}
// 双击右侧文件夹内容中的文件夹
const handleContentItemDblClick = async (item) => {
  if (item.type === 'D') {
    await fetchFileTree(item.path); // 调用 fetchFileTree 发送请求

    // 确保文件树的内容更新，重新获取该文件夹内容
    const updatedItem = findTreeNodeByPath(item.path, fileTree.value);
    if (updatedItem) {
      currentFolder.value = updatedItem;
      currentFolderContent.value = updatedItem.children || []; // 更新文件夹内容
    }

    if (updatedItem && fileTreeRef.value) {
      currentFileNode.setFileNode(uid,updatedItem.path)
      fileTreeRef.value.setCurrentKey(updatedItem.path);
      fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
    }
  } else if (item.type === 'F') {
    await fetchFileContent(item);
  }
};
const dblClickFileFolder = async(item) =>{
  await fetchFileTree(item.path); // 调用 fetchFileTree 发送请求

  // 确保文件树的内容更新，重新获取该文件夹内容
  const updatedItem = findTreeNodeByPath(item.path, fileTree.value);
  if (updatedItem) {
    currentFolder.value = updatedItem;
    currentFolderContent.value = updatedItem.children || []; // 更新文件夹内容
  }

  if (updatedItem && fileTreeRef.value) {
    currentFileNode.setFileNode(uid,updatedItem.path)
    fileTreeRef.value.setCurrentKey(updatedItem.path);
    // fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
    fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true

  }
}
const dblClickFile = async(item) =>{
  await fetchFileContent(item);
}

// 根据路径查找节点
const findTreeNodeByPath = (path, root) => {
  // 如果 root 是一个数组，遍历数组中的每个元素
  if (Array.isArray(root)) {
    for (const node of root) {
      const result = findTreeNodeByPath(path, node);
      if (result) return result;
    }
    return null;
  }

  // 如果当前节点的路径匹配，返回当前节点
  if (root.path === path) return root;

  // 如果当前节点有子节点，递归查找子节点
  if (root.children) {
    for (const child of root.children) {
      const result = findTreeNodeByPath(path, child);
      if (result) return result;
    }
  }

  // 如果没有找到匹配的节点，返回 null
  return null;
};
// 加载文件内容
const fetchFileContent = async (file) => {
  selectedFile.value = file;
  try {
    const res = await ClientAPI.fetch_file_content({uid:uid,path:file.path});
    selectedFileContent.value = res.data.content;
  } catch (error) {
    console.error("Error fetching file content:", error);
    selectedFileContent.value = "Failed to load content.";
  }
};

const deleteFile = async (filePath) => {
  try {
    const res = await ClientAPI.delete_file({uid:uid,filePath:filePath});
    fileTree.value = res.data.data;
    globalFileTree.updateEntry(uid,fileTree.value);
  } catch (error) {
    console.error("Error fetching file tree:", error);
  }
};

const makeDir = async(dirPath) => {
  try {
    const res = await ClientAPI.make_dir({uid:uid,dirPath:dirPath});
    fileTree.value = res.data.data;
    globalFileTree.updateEntry(uid,fileTree.value);
  } catch (error) {
    console.error("Error fetching file tree:", error);
  }
};

const handleMkDir = async(path) =>{
  const { value } = await ElMessageBox.prompt(
      '请输入新文件夹的名称',
      '新建文件夹',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputPlaceholder: '文件夹名称',
      }
  )
  // 检查输入值
  if (!value) {
    ElMessage.warning('文件夹名称不能为空')
    return
  }
  await makeDir(path+"/"+value);
  ElMessage.success('新建成功');
  const updatedItem = findTreeNodeByPath(path, fileTree.value);
  if (updatedItem) {
    currentFolder.value = updatedItem;
    currentFolderContent.value = updatedItem.children || []; // 更新文件夹内容
  }
  if (updatedItem && fileTreeRef.value) {
    currentFileNode.setFileNode(uid,updatedItem.path)
    fileTreeRef.value.setCurrentKey(updatedItem.path);
    fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
  }


}
const handleDelete = (row) => {
  const itemType = row.type === 'F' ? '文件' : '文件夹'
  ElMessageBox.confirm(`是否删除${itemType}：${row.name}?`, '删除确认', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  })
      .then(async () => {
        await deleteFile(row.path);
        ElMessage.success('删除'+row.name+'成功');
        let beforeLastSlash = "";
        // 查找最后一个斜杠的位置
        let lastSlashIndex = row.path.lastIndexOf('/');

        // 如果找到了斜杠
        if (lastSlashIndex !== -1) {
          // 获取最后一个斜杠之前的部分
          beforeLastSlash = row.path.substring(0, lastSlashIndex);
        }

        const updatedItem = findTreeNodeByPath(beforeLastSlash, fileTree.value);
        if (updatedItem) {
          currentFolder.value = updatedItem;
          currentFolderContent.value = updatedItem.children || []; // 更新文件夹内容
        }
        if (updatedItem && fileTreeRef.value) {
          currentFileNode.setFileNode(uid,updatedItem.path)
          fileTreeRef.value.setCurrentKey(updatedItem.path);
          fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
        }

      })
};

const handleDownload = (row) => {
  ElMessageBox.confirm(`是否下载文件：${row.name}?`, '下载确认', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  })
      .then(async ({ value }) => {
        try {
          const res = await ClientAPI.download_file({uid:uid,filePath:row.path});
          if(res.data.status === 200){
            ElMessage.success('后台下载'+row.path+'中');
          }else{
            ElMessage.error('下载'+row.path+'失败');
          }
        } catch (error) {
          ElMessage.error('下载'+row.path+'失败');
        }
      })
      .catch(() => {
        // 取消下载操作
        ElMessage.info('已取消下载');
      });
};

// 组件挂载时获取文件树
onMounted(
    () => {
      const tmp = globalFileTree.getEntry(uid);
      if (Array.isArray(tmp) && tmp.length === 0) {
        fetchFileTree("./");
      } else{
        fileTree.value = globalFileTree.getEntry(uid);
        const currentNodePath = currentFileNode.getFileNode(uid);

        // 确保文件树的内容更新，重新获取该文件夹内容
        const updatedItem = findTreeNodeByPath(currentNodePath, fileTree.value);
        if (updatedItem) {
          currentFolder.value = updatedItem;
          currentFolderContent.value = updatedItem.children || []; // 更新文件夹内容
        }
        setTimeout(() => {
          if (updatedItem && fileTreeRef.value) {
            fileTreeRef.value.setCurrentKey(updatedItem.path);
            fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
          }
        }, 100); // 等待 500 毫秒
        // if (updatedItem && fileTreeRef.value) {
        //   fileTreeRef.value.setCurrentKey(updatedItem.path);
        //   fileTreeRef.value.store.nodesMap[updatedItem.path].expanded = true
        // }
      }
    }
);
onBeforeUnmount(() => {
  // 清理文件树等状态
  fileTree.value = [];
  currentFolder.value = null;
  currentFolderContent.value = [];
  selectedFile.value = null;
  selectedFileContent.value = '';
});
</script>

<style scoped>
.file-explorer {
  display: flex;
}

.el-tree {
  width: 30%;
  border-right: 1px solid #ddd;
  padding-right: 10px;
}

.file-details {
  width: 70%;
  padding-left: 10px;
}
</style>

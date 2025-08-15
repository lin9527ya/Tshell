<template>
  <div>
    <div>
      <el-dialog
          v-model="dialogVisible"
          width="30%"
      >
        <el-button type="primary" @click="handleAddNote(uid)">Note</el-button>
        <el-button type="success" @click="handleMarkColor(uid)">Color</el-button>
        <el-button type="warning" @click="handleEditSleep(uid)">Sleep</el-button>
        <el-button type="danger" @click="handleExit(uid)">Exit</el-button>
      </el-dialog>

      <!-- 颜色选择对话框 -->
      <el-dialog
          v-model="colorDialogVisible"
          title="选择颜色"
          width="30%"
          @close="handleClose"
      >
        <div class="color-picker">
          <el-radio-group v-model="selectedColor">
            <el-radio
                v-for="(color, index) in colors"
                :key="index"
                :label="color"
                style="display: block; margin: 10px 0;"
            >
              <span :style="{ background: color, display: 'inline-block', width: '20px', height: '20px', borderRadius: '50%' }"></span>
            </el-radio>
          </el-radio-group>
        </div>
        <template #footer>
        <span class="dialog-footer">
          <el-button @click="cancelSelection">取 消</el-button>
          <el-button type="primary" @click="confirmSelection">确 定</el-button>
        </span>
        </template>
      </el-dialog>
      <div>
        <TablesView
            :columns="Clients_tableColumn"
            :table-data="tableData"
            :options="options"
            @pagination-change="handlePaginationChange"
        >
          <template #FirstStart="{ row }">
            <span :style="{ color: row.Color }">{{ row.FirstStart }}
            </span>
          </template>
          <template #ExternalIP="{ row }">
            <span>{{ row.ExternalIP }}
            </span>
          </template>
<!--          <template #InternalIP="{ row }">-->
<!--            <span>{{ row.InternalIP }}-->
<!--            </span>-->
<!--          </template>-->
          <template #Username="{ row }">
            <span>{{ row.Username }}
            </span>
          </template>
          <template #Computer="{ row }">
            <span>{{ row.Computer }}
            </span>
          </template>
<!--          <template #Process="{ row }">-->
<!--            <span>{{ row.Process }}-->
<!--            </span>-->
<!--          </template>-->
<!--          <template #Pid="{ row }">-->
<!--            <span>{{ row.Pid }}-->
<!--            </span>-->
<!--          </template>-->
          <template #Address="{ row }">
            <span>{{ row.Address }}
            </span>
          </template>
          <template #Arch="{ row }">
            <span>{{ row.Arch }}
            </span>
          </template>
          <template #Note="{ row }">
            <span>{{ row.Note }}
            </span>
          </template>
          <template #Sleep="{ row }">
            <span>{{ row.Sleep }}
            </span>
          </template>
          <template #onlineStatus="{ row }">
            <el-tag :type="row.Online === '1' ? 'success' : 'danger'" size="small">
              {{ row.Online === '1' ? '在线' : '离线' }}
            </el-tag>
          </template>
          <!-- 如果不传入按钮组的数据就使用自定义插槽的方式， 自定义插槽需在配置项里配置slot -->
          <template #action="{ row }">
            <div>
              <el-button type="primary" @click="handleViewClick(row)" size="small">查看</el-button>
              <el-button type="success" @click="handleMoreOptions(row)" size="small">更多操作</el-button>
            </div>
          </template>
        </TablesView>
      </div>
    </div>
  </div>


</template>

<script setup lang="ts">

import TablesView from "@/components/Common/Tables/TablesView.vue";
import {inject, onMounted, onUnmounted, reactive, ref, toRefs, watch} from "vue";
import {useRoute, useRouter} from "vue-router";
import {ElMessage,ElMessageBox} from "element-plus";
// import {useAxiosConfigStore} from "@/stores/server";
// const axiosConfig = useAxiosConfigStore();
const route = useRoute()
const router = useRouter()
const input_search = ref()
const dialogVisible = ref(false);
const colorDialogVisible = ref(false);
const selectedColor = ref('')
const colors = [
    '#f31616',
  '#09e609',
  '#0b0be6',
  '#e8e807',
  '#e10fe1',
  '#12dada'];
let uid :string='';
const handleClose = () => {
  selectedColor.value = '';
};

// 取消选择
const cancelSelection = () => {
  selectedColor.value = '';
  colorDialogVisible.value = false;
};

// 确定选择
const confirmSelection = async() => {
  if (selectedColor.value) {
    const res = await ClientAPI.select_color({uid:uid,color:selectedColor.value})
    if(res.data.status === 200){
      ElMessage.success("修改颜色成功")
    }
  }
  colorDialogVisible.value = false;
  dialogVisible.value = false;
};


interface TableDataType {
  Uid: string
  FirstStart: string
  ExternalIP: string
  InternalIP: string
  Listener: string
  Username: string
  Computer: string
  Process: string
  Pid: string
  Address: string
  Arch: string
  Note: string
  Sleep: string
  Online:string
  Color: string
}
const tableData = ref<TableDataType[]>([])
const params = {
  page: Number(route.query.page) || 1,
  pageSize: Number(route.query.pageSize) || 10,
}
import ClientAPI from '@/api/clients'
// import {ReadLastLine} from "../../../wailsjs/go/main/App";
const get_clients_list = (page: number = 1, pageSize: number = 10) => {
  /**
   * 获取任务列表数据
   */
  return ClientAPI.get_clients_list({ page: page, page_size: pageSize })
}
watch(
    () => route.query,
    async (val: any) => {
      // const value = await ReadLastLine()
      // axiosConfig.updateBaseURL(value);
      const { page, pageSize } = val
      params.page = Number(page) || params.page
      params.pageSize = Number(pageSize) || params.pageSize
      console.log('send requests', params.page, params.pageSize)
      get_clients_list(params.page, params.pageSize).then((res) => {
        tableData.value = res.data.data.list
        state.options.paginationConfig = {
          total: res.data.data.total,
          currentPage: params.page,
          pageSize: params.pageSize,
          pageSizes: [10, 50, 100, 1000],
          layout: 'total,prev, pager, next,sizes'
        }
      })
    },
    { immediate: true }
)
function getList(){
  get_clients_list(params.page, params.pageSize).then((res) => {
    tableData.value = res.data.data.list
    state.options.paginationConfig = {
      total: res.data.data.total,
      currentPage: params.page,
      pageSize: params.pageSize,
      pageSizes: [10, 50, 100, 1000],
      layout: 'total,prev, pager, next,sizes'
    }
  })
}
onMounted(async() => {
  // const value = await ReadLastLine()
  // axiosConfig.updateBaseURL(value);

  // 设置一个定时器，每5秒执行一次
  const intervalId = setInterval(getList, 2000);

  // 确保在组件卸载时清除定时器
  onUnmounted(() => {
    clearInterval(intervalId);
  });
});

const Clients_tableColumn :Table.Column[]= [
  // {prop: 'uid', label: 'uid', showOverflowTooltip: true},
  {prop: 'FirstStart', label: 'FirstStart', showOverflowTooltip: true, Color: 'Color',width:"130"},
  {prop: 'ExternalIP', label: '外网IP', showOverflowTooltip: true, Color: 'Color',width:"130"},
  // {prop: 'InternalIP', label: '内网IP', showOverflowTooltip: true, Color: 'Color'},
  {prop: 'Username', label: 'User', showOverflowTooltip: true,width:"170", Color: 'Color'},
  {prop: 'Computer', label: 'Computer', showOverflowTooltip: true,width:"170", Color: 'Color'},
  {prop: 'Process', label: 'Process', showOverflowTooltip: true, Color: 'Color'},
  {prop: 'Pid', label: 'Pid', showOverflowTooltip: true, Color: 'Color'},
  {prop: 'Address', label: 'Address', showOverflowTooltip: true, Color: 'Color',width:"200"},
  {prop: 'Arch', label: 'Arch', showOverflowTooltip: true,width:"60", Color: 'Color'},
  {prop: 'Note', label: 'Note',  showOverflowTooltip: true, Color: 'Color',width:"120"},
  {prop: 'Sleep', label: 'Sleep',  showOverflowTooltip: true,width:"65", Color: 'Color'},
  {
    label: 'Online',
    slot: 'onlineStatus',  // 使用插槽方式
    width: "80",
    align: 'center',  // 居中显示
    Color: 'Color'
  },
  {
    // width: '200',
    label: '操作',
    buttons: [],
    slot: 'action', Color: 'Color'
  }
]
const handleMoreOptions = (val:any) =>{
  dialogVisible.value = true;
  uid = val.Uid;
}
const handleAddNote = async(uid :string) =>{
  const { value } = await ElMessageBox.prompt(
      '输入Note',
      'Note',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputPlaceholder: 'Note',
      }
  )
  if (!value) {
    ElMessage.warning('Note不能为空')
    return
  }
  const res = await ClientAPI.add_uid_note({uid:uid,note:value})
  if (res.data.status === 200){
    ElMessage.success("Note成功")
  }
  dialogVisible.value = false

}
const handleEditSleep = async(uid :string) =>{
  const { value } = await ElMessageBox.prompt(
      'Sleep',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputPlaceholder: 'Sleep',
      }
  )
  if (!value) {
    ElMessage.warning('Sleep不能为空')
    return
  }
  const res = await ClientAPI.edit_sleep({uid:uid,sleep:value})
  if (res.data.status === 200){
    ElMessage.success("修改Sleep成功")
  }
  dialogVisible.value = false

}
const handleMarkColor = async(uid :string) =>{
  colorDialogVisible.value = true;
}
const handleExit = (uid :string) =>{
  ElMessageBox.confirm(`是否退出客户端?`, '退出确认', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  })
      .then(async () => {
        const res = await ClientAPI.do_exit({uid:uid})
        if (res.data.status === 200){
          ElMessage.success("退出成功")
        }
        dialogVisible.value = false
      })
}
const handleViewClick = (val: any) => {
  router.push({
    path: '/client/shell',
    query: {
      uid: val.Uid
    }
  })
}
interface State {
  options: Table.Options
}
const state = reactive<State>({
  options: { showPagination: true }
})
const { options } = toRefs(state)
// 局部刷新方法
const reload: any = inject('reload')
const ReloadClick = async () => {
  input_search.value = ''
  await router.push({
    path: route.path,
    query: {
      page: 1,
      pageSize: 10,
      search: ''
    }
  })
  reload()
}
const handlePaginationChange = async (
    page: number,
    pageSize: number,
) => {
  await router.push({
    path: route.path,
    query: {
      page,
      pageSize,
    }
  })
  reload()
}
</script>


<style scoped>

</style>
<template>
  <div>
    <TablesView
        :columns="PID_tableColumn"
        :table-data="tableData"
        :options="options"
    >
      <template #PID="{ row }">
            <span>{{ row.PID }}
            </span>
      </template>
      <template #PPID="{ row }">
            <span>{{ row.PPID }}
            </span>
      </template>
      <template #Name="{ row }">
            <span>{{ row.Name }}
            </span>
      </template>
      <template #Arch="{ row }">
            <span>{{ row.Arch }}
            </span>
      </template>
      <template #User="{ row }">
            <span>{{ row.User }}
            </span>
      </template>

      <!-- 如果不传入按钮组的数据就使用自定义插槽的方式， 自定义插槽需在配置项里配置slot -->
      <template #action="{ row }">
        <div>
          <!--              <el-button @click="handleStop(row)">停止</el-button>-->
          <el-button type="danger" @click="handleKill(row)">Kill</el-button>
        </div>
      </template>
    </TablesView>
  </div>
</template>

<script setup lang="ts">

import TablesView from "@/components/Common/Tables/TablesView.vue";
import {inject, onMounted, reactive, ref, toRefs, watch} from "vue";
import {useRoute, useRouter} from "vue-router";
import ClientAPI from "@/api/clients"
import {ElMessageBox} from "element-plus"
const route = useRoute()
const router = useRouter()

const PID_tableColumn :Table.Column[]= [
  {prop: 'PID', label: 'PID', showOverflowTooltip: true,Color:"black"},
  {prop: 'PPID', label: 'PPID', sortable: true, showOverflowTooltip: true,Color:"black"},
  {prop: 'Name', label: 'Name', sortable: true, showOverflowTooltip: true,Color:"black"},
  {prop: 'Arch', label: 'Arch', sortable: true, showOverflowTooltip: true,Color:"black"},
  {prop: 'User', label: 'User', sortable: true, showOverflowTooltip: true,Color:"black"},
  {
    // width: '240',
    label: '操作',
    buttons: [],
    slot: 'action'
    ,Color:"black"
  }
]

interface TableDataType {
  PID: string
  PPID: string
  Name: string
  Arch: string
  User: string
}
const tableData = ref<TableDataType[]>([])

interface State {
  options: Table.Options
}
const state = reactive<State>({
  options: { showPagination: false }
})
const { options } = toRefs(state)

const uid = route.query.uid;
// onMounted(()=>{
//   ClientAPI.get_pid_list({uid:uid}).then((res)=>{
//     tableData.value = res.data.data
//   })
// })

const handleKill = (val: any) => {
  console.log('delete 任务', val.desc)
  ElMessageBox.confirm(`是否删除此任务？`, '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning'
  })
      .then(() => {
        ClientAPI.kill_pid({uid: uid,pid: val.PID}).then((res)=>{
          ClientAPI.get_pid_list({uid:uid}).then((res)=>{
            tableData.value = res.data.data
          })
        })
      })
}
watch(
    () => route.query,
    async (val: any) => {
      ClientAPI.get_pid_list({uid:uid}).then((res)=>{
        tableData.value = res.data.data
      })
    },
    { immediate: true }
)
</script>

<style scoped>

</style>
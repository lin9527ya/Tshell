import request from '@/utils/request'

export default {
    addListener(data:{type:string,listenAddress:string,connectAddress:string,retries?:string,interval?:string,note?:string,vkey?:string,encryptionSalt?:string,profile?:string,beaconSleepSeconds?:number,beaconJitterPercent?:number}){
        return request({
            url:"/listener/add",
            method:"POST",
            data:data
        })
    },
    updateListener(data:{id:number,type:string,listenAddress:string,connectAddress:string,retries?:string,interval?:string,note?:string,vkey?:string,encryptionSalt?:string,profile?:string,beaconSleepSeconds?:number,beaconJitterPercent?:number}){
        return request({
            url:"/listener/update",
            method:"POST",
            data:data
        })
    },
    // 列表内联编辑备注：只写 note 列，不会覆盖地址/心跳等其它配置
    updateListenerNote(data:{id?:number,listenAddress?:string,note:string}){
        return request({
            url:"/listener/note",
            method:"POST",
            data:data
        })
    },
    getListenerList(){
        return request({
            url:"/listener/list",
            method:"GET"
        })
    },
    openListener(data:{listenAddress:string}){
        return request({
            url:"/listener/open",
            method:"POST",
            data:data
        })
    },
    closeListener(data:{listenAddress:string}){
        return request({
            url:"/listener/close",
            method:"POST",
            data:data
        })
    },
    // 重启监听器(断开后重启,会断开该监听器下全部客户端连接,调用方需确认)
    restartListener(data:{listenAddress:string}){
        return request({
            url:"/listener/restart",
            method:"POST",
            data:data
        })
    },
    deleteListener(data:{listenAddress:string}){
        return request({
            url:"/listener/delete",
            method:"POST",
            data:data
        })
    },
    batchDeleteListeners(data:{listen_addresses:string[]}){
        return request({
            url:"/listener/batch-delete",
            method:"POST",
            data:data
        })
    }
}
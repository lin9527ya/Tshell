import request from '@/utils/request'

export default {
    addListener(data:{type:string,listenAddress:string,connectAddress:string}){
        return request({
            url:"/listener/add",
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
    deleteListener(data:{listenAddress:string}){
        return request({
            url:"/listener/delete",
            method:"POST",
            data:data
        })
    }
}
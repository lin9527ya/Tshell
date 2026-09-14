import request from '@/utils/request'
import pinia from '@/stores/index';
// import {useAxiosConfigStore} from '@/stores/server';
// const axiosConfig = useAxiosConfigStore(pinia);

export default {
  login(data: { username: string; password: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/login',
      method: 'post',
      data: data
    })
  },
  logout(data: { refresh: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/logout',
      // 后端 ShouldBindJSON 绑定 refresh_token（users.go:271），必须发 JSON，不能 urlencoded
      method: 'post',
      data: { refresh_token: data.refresh }
    })
  },
  rest_password(data: { new_password: string; old_password: string }) {
    const form = new URLSearchParams()
    form.append('old_password', data.old_password)
    form.append('new_password', data.new_password)
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/user_setting/ChangePassword',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      method: 'post',
      data: form
    })
  },
  // rest_api_key() {
  //   return request({
  //     url: '/users/user_setting/reset_api_key',
  //     method: 'get'
  //   })
  // },
  getUserList(data: { page?: number; page_size?: number; search?: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/user_setting/get_user_list',
      method: 'post',
      params: data
    })
  },
  create_user(data: {
    username: string
    password: string
    role?: string
  }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/user_setting/create_user',
      method: 'post',
      data: data
    })
  },
  delete_user(data: { username: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/user_setting/delete_user',
      method: 'post',
      data: data
    })
  },
  admin_reset_password(data: { username: string, new_password: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/user_setting/admin_reset_password',
      method: 'post',
      data: data
    })
  }
}

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
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      method: 'post',
      data: data
    })
  },
  rest_password(data: { new_password: string; old_password: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/user_setting/ChangePassword',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      method: 'post',
      data: data
    })
  },
  // rest_api_key() {
  //   return request({
  //     url: '/users/user_setting/reset_api_key',
  //     method: 'get'
  //   })
  // },
  getUserInfo() {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/getuserinfo',
      method: 'get'
    })
  },
  getUserList(data: { page: number; page_size: number; search: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/getuserinfo/get_user_list',
      method: 'get',
      params: data
    })
  },
  create_user(data: {
    username: string
    password: string
    password_again: string
    phone: string
    email: string
    permissions: number
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
  }
}

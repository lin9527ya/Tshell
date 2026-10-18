import request from '@/utils/request'
import pinia from '@/stores/index';
// import {useAxiosConfigStore} from '@/stores/server';
// const axiosConfig = useAxiosConfigStore(pinia);

export default {
  /**
   * 登录。开启 MFA 的账号在**不带 totp_code** 时会返回
   * `{ mfa_required: true, pending_token }` 而不是凭据 —— 调用方需进入第二步。
   */
  login(data: { username: string; password: string; totp_code?: string }) {
    return request({
//      baseURL: axiosConfig.baseURL,
      url: '/users/login',
      method: 'post',
      data: data
    })
  },
  /** 两步登录的第二步：pending_token + 动态码（或恢复码） */
  loginMfa(data: { pending_token: string; totp_code?: string; recovery_code?: string }) {
    return request({
      url: '/users/login/mfa',
      method: 'post',
      data: data
    })
  },
  /** 当前账号的 MFA 状态（只回状态，不含密钥） */
  mfaStatus() {
    return request({ url: '/users/mfa', method: 'get' })
  },
  /** 开始绑定：返回密钥 + otpauth 链接 + 恢复码（只在这一次返回） */
  mfaStart(data: { password: string }) {
    return request({ url: '/users/mfa/start', method: 'post', data })
  },
  /** 确认绑定：口令 + 密钥 + 动态码 + 恢复码 */
  mfaConfirm(data: {
    password: string
    secret: string
    totp_code: string
    recovery_codes: string[]
  }) {
    return request({ url: '/users/mfa/confirm', method: 'post', data })
  },
  /** 关闭 MFA：口令 + 动态码（或一个恢复码） */
  mfaDisable(data: { password: string; totp_code?: string; recovery_code?: string }) {
    return request({ url: '/users/mfa/disable', method: 'post', data })
  },
  /** 管理员重置他人 MFA（丢手机的救援路径，需带管理员自己的口令） */
  mfaReset(data: { username: string; password: string }) {
    return request({ url: '/users/mfa/reset', method: 'post', data })
  },
  /**
   * 二维码图片 URL（服务端只接受 otpauth:// 前缀）。
   * 走 <img src> 而不是 axios：让浏览器直接取 SVG，省一层 base64 转换。
   */
  mfaQrUrl(text: string) {
    return `/api/users/mfa/qr?text=${encodeURIComponent(text)}`
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

import request from '@/utils/request'

// C2 Profile（Malleable C2 Profile YAML）API
const c2profileApi = {
  // 已加载 profile 列表
  listProfiles() {
    return request({
      url: '/c2profile/list',
      method: 'get',
    })
  },

  // 预览 profile YAML 原文
  previewProfile(name: string) {
    return request({
      url: '/c2profile/preview',
      method: 'get',
      params: { name },
    })
  },

  // 保存 profile（写 profiles/<name>.yaml，3 秒内热加载）
  saveProfile(name: string, content: string) {
    return request({
      url: '/c2profile/save',
      method: 'post',
      data: { name, content },
    })
  },
}

export default c2profileApi

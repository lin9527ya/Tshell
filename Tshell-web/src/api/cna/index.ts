import request from '@/utils/request'

// CNA Aggressor Script 脚本引擎 API（CS 插件兼容）
const cnaApi = {
  // 已加载脚本列表
  listScripts() {
    return request({
      url: '/cna/scripts',
      method: 'get',
    })
  },

  // scripts 目录下的 .cna 文件
  listFiles() {
    return request({
      url: '/cna/files',
      method: 'get',
    })
  },

  // 加载脚本
  loadScript(name: string) {
    return request({
      url: '/cna/load',
      method: 'post',
      data: { name },
    })
  },

  // 卸载脚本
  unloadScript(name: string) {
    return request({
      url: '/cna/unload',
      method: 'post',
      data: { name },
    })
  },

  // 重载脚本
  reloadScript(name: string) {
    return request({
      url: '/cna/reload',
      method: 'post',
      data: { name },
    })
  },

  // 语法检查
  testScript(content: string) {
    return request({
      url: '/cna/test',
      method: 'post',
      data: { content },
    })
  },

  // 上传 .cna 脚本（multipart 字段名 script）
  uploadScript(file: File) {
    const formData = new FormData()
    formData.append('script', file)
    return request({
      url: '/cna/upload',
      method: 'post',
      data: formData,
    })
  },

  // 已注册菜单（Beacon 右键菜单插件）
  listMenus() {
    return request({
      url: '/cna/menus',
      method: 'get',
    })
  },

  // 执行菜单项
  execMenuItem(data: { uid: string; kind: string; path: string[] }) {
    return request({
      url: '/cna/menu-exec',
      method: 'post',
      data,
    })
  },

  // 执行 alias：脚本注册的命令会拦截控制台输入（服务端 ExecAlias，未命中返回 hit=false）。
  // 原实现缺这个封装，而 /cna/alias-exec 是唯一一条"后端有路由、前端零消费"的接口。
  execAlias(data: { uid: string; name: string; args: string[] }) {
    return request({
      url: '/cna/alias-exec',
      method: 'post',
      data,
    })
  },

  // 交互式求值脚本片段（Aggressor 控制台）
  evalScript(content: string, uid: string) {
    return request({
      url: '/cna/eval',
      method: 'post',
      data: { content, uid },
    })
  },
}

export default cnaApi

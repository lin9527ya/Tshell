import request from '@/utils/request';

// 插件相关API
const pluginApi = {
  // 获取在线客户端列表
  get_online_clients() {
    return request({
      url: '/client/online-clients',
      method: 'get'
    });
  },

  // 获取插件列表
  get_plugins() {
    return request({
      url: '/plugin/list',
      method: 'get'
    });
  },

  // 获取插件详细信息
  get_plugin_details(pluginName: string) {
    return request({
      url: `/plugin/details/${encodeURIComponent(pluginName)}`,
      method: 'get'
    });
  },

  // 上传插件（后端 c.FormFile("plugin") 要求 multipart/form-data 字段名 plugin；
  // 不手写 Content-Type，交给 Axios 自动追加 boundary）
  upload_plugin(file: File) {
    const formData = new FormData();
    formData.append('plugin', file);
    return request({
      url: '/plugin/upload',
      method: 'post',
      data: formData
    });
  },

  // 下载插件文件（带完整性校验与防缓存响应，返回 Blob）
  download_plugin(pluginName: string) {
    return request({
      url: `/plugin/download/${encodeURIComponent(pluginName)}`,
      method: 'get',
      responseType: 'blob'
    });
  },

  // 删除插件文件（F4: 原文件被 `\n 污染导致 delete_plugin 丢失 → 插件删除运行时 TypeError）
  delete_plugin(pluginName: string) {
    return request({
      url: `/plugin/delete/${encodeURIComponent(pluginName)}`,
      method: 'delete'
    });
  },

  // 获取Shell历史记录
  get_shell_history(uid: string) {
    return request({
      url: `/client/shell/history/${uid}`,
      method: 'get'
    });
  }
};

export default pluginApi;

// 类型定义
export interface ClientInfo {
  Uid?: string;
  uid?: string;           // 后端 json tag 为小写
  Computer?: string;
  computer?: string;
  Username?: string;
  username?: string;
  Online?: string;
  online?: string;
  FirstStart?: string;
  first_start?: string;
  ExternalIP?: string;
  external_ip?: string;
  Listener?: string;
  listener?: string;
  Os?: string;
  os?: string;
  Arch?: string;
  arch?: string;
}

export interface PluginInfo {
  name: string;        // 服务端 json tag 为小写 name（此前前端用 Name 导致下拉空白）
  Name?: string;       // 兼容旧字段
  Size?: number;
  size?: number;
  LastModified?: string;
  last_modified?: string;
  Type?: string;
  type?: string;
  Description?: string;
  description?: string;
}
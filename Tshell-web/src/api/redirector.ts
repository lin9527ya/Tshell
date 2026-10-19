import request from '@/utils/request'

// 重定向器配置生成 API 封装（后端 pkg/api/redirector.go）
//
// 为什么要做成界面入口：手写重定向器最容易与 C2 画像漂移 ——
// 画像里的 http-get/http-post/ws 路径一旦改了而重定向器没跟上，beacon 会直接失联，
// 而且失联现象看起来像"网络问题"，很难第一时间联想到路径不匹配。
// 让运维从界面按**当前画像**生成，能把这条漂移堵住。

/** 生成器类型 */
export type RedirectorType = 'nginx' | 'apache'

export interface RedirectorRequest {
  type: RedirectorType
  profile: string
  /** 后端地址 host:port（不带协议前缀） */
  backend: string
  /** 对外域名，留空则用占位 cdn.example.com */
  domain?: string
  /** 诱饵落点，必须是 http(s) URL */
  decoy?: string
  /** 完整 nginx 配置时需要 TLS 证书路径（绝对路径） */
  cert?: string
  key?: string
  /** 1 = 生成完整 nginx.conf（自包含，含 TLS）；否则只生成 server 片段 */
  full?: boolean
}

export interface RedirectorResult {
  type: RedirectorType
  profile: string
  /** 该画像下会被转发的路径清单 */
  uris: string[]
  /** 生成出来的配置全文 */
  content: string
  lines: number
  full: boolean
  /** 建议的文件名 */
  filename: string
}

/** 生成重定向器配置 */
export function getRedirectorTemplate(params: RedirectorRequest) {
  return request.get('/redirector/template', {
    params: {
      type: params.type,
      profile: params.profile,
      backend: params.backend,
      domain: params.domain || undefined,
      decoy: params.decoy || undefined,
      cert: params.cert || undefined,
      key: params.key || undefined,
      full: params.full ? '1' : undefined,
    },
  })
}

export default { getRedirectorTemplate }

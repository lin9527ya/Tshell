/**
 * 文件查看/编辑器的内容解码（阶段三 §3.11：拆 2128 行的 ClientFiles.vue）。
 *
 * 抽它的理由：这段逻辑**形状兼容性极多**，而且每一支都是踩过坑才加的 ——
 * 后端不同接口、不同版本回的内容形态不一样（裸字符串 / {data} / {data:{content}} /
 * {content} / {data:{fileContent}} / 对象），再加上"看着像 base64 就解一次"的
 * 启发式。这些分支埋在 `editFile` 里时无法单独验证，出错的症状又只是
 * "编辑器里显示乱码"或"显示 [object Object]"。
 */

/** 后端返回的各种内容形态 → 待显示的字符串 */
export interface FileContentResponse {
  data?: unknown
}

/**
 * 从响应里取出原始内容字符串。
 *
 * 各分支按"越具体越先试"排列：`data.data.content` 比 `data.data` 具体，
 * 所以必须先判前者，否则对象整体会被 JSON.stringify 掉，
 * 编辑器里就会出现一整段 {"content":"真正的文件内容"}。
 */
export function extractRawContent(res: FileContentResponse | string | null | undefined): string {
  if (res === null || res === undefined) return ''
  // 极少数路径直接回字符串
  if (typeof res === 'string') return res

  const data = (res as FileContentResponse).data
  if (data === null || data === undefined) return ''
  if (typeof data === 'string') return data

  const d = data as Record<string, unknown>
  if (typeof d.data === 'string') return d.data
  if (d.data && typeof d.data === 'object') {
    const inner = d.data as Record<string, unknown>
    if (inner.content !== undefined) return String(inner.content)
    if (inner.fileContent !== undefined) return String(inner.fileContent)
    // 兜底：把对象美化输出，至少让人看到结构（而不是 [object Object]）
    return JSON.stringify(d.data, null, 2)
  }
  if (d.content !== undefined) return String(d.content)
  if (d.fileContent !== undefined) return String(d.fileContent)
  return ''
}

/**
 * 判断一段文本是否"看起来像 base64"。
 *
 * 这是个启发式，条件刻意偏保守（宁可漏解也不误判）：
 *   - 只含 base64 字母表与 '='
 *   - 长度是 4 的倍数（base64 的必要条件）
 *   - 不含换行（真实文本文件几乎都含换行；含换行的长串更可能是文本）
 *   - 长度 > 24（短串太容易巧合命中，比如 "test"、"abcd"）
 *
 * 误判的代价比漏判大：把普通文本当 base64 解，编辑器里会显示乱码，
 * 而用户很可能以为文件被损坏了。
 */
export function looksLikeBase64(raw: string): boolean {
  if (!raw) return false
  const trimmed = raw.trim()
  if (trimmed.length <= 24) return false
  if (trimmed.length % 4 !== 0) return false
  if (raw.includes('\n')) return false
  return /^[A-Za-z0-9+/=]+$/.test(trimmed)
}

/**
 * 尝试把 base64 解成 UTF-8 文本。
 *
 * 解码或 UTF-8 还原失败时**原样返回输入**（而不是返回半截乱码或抛错）：
 * 调用方拿到的一定是"能显示的东西"。
 *
 * 用 TextDecoder 而不是 `decodeURIComponent(escape(...))`：后者对
 * 非 Latin-1 可表示的字节会抛 URIError，且 escape 本身已废弃。
 */
export function tryDecodeBase64(raw: string): string {
  try {
    const binary = atob(raw.trim())
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i)
    }
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return raw
  }
}

/** 完整流程：从响应取出内容，必要时解一次 base64 */
export function decodeFileContent(res: FileContentResponse | string | null | undefined): string {
  const raw = extractRawContent(res)
  if (looksLikeBase64(raw)) {
    return tryDecodeBase64(raw)
  }
  return String(raw ?? '')
}

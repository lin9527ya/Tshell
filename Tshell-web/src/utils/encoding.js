/**
 * 编码转换工具类
 * 专门用于处理中文乱码问题
 *
 * 【KNOWN_LIMITATIONS #11 修复】原实现为硬编码乱码→中文映射表(数十条,只能覆盖
 * 固定词组,任意输出仍乱码)。现改为字节级 TextDecoder('gb18030') 解码为主:
 * 服务端回传的 shell 输出若为 GBK 系编码被误按 UTF-8/Latin-1 解释,前端将其
 * charCode 还原为字节后按 gb18030(GBK 超集,兼容 GB2312/GBK)重新解码,
 * 并以"解码后中文占比/可读性"置信度判定是否采纳。
 */

/**
 * 检测字符串是否包含中文乱码
 * @param {string} str
 * @returns {boolean}
 */
export function hasGarbledText(str) {
  if (!str) return false

  // GBK 误读形态: 双字节序列(B1: 0x81-0xFE, B2: 0x40-0xFE)被按 Latin-1 解释为
  // 连续高位拉丁/符号字符。B2 可低至 0x40('@')故不能只查 À-ÿ 区间
  // (原模式漏检 Ä¿Â¼ 之类 B2<0xC0 的形态)。
  const garbledPatterns = [
    /�/,                    // 替换字符 (replacement character)
    /[À-ÿ][@-ÿ]/, // GBK 双字节被 Latin-1 误读的典型形态
    /[-¿][-¿]/, // 连续高位符号
  ]

  return garbledPatterns.some(pattern => pattern.test(str))
}

/**
 * 统计字符串中 CJK 字符占比(解码质量置信度用)
 * @param {string} str
 * @returns {number} 0-1
 */
function cjkRatio(str) {
  if (!str) return 0
  let cjk = 0
  let total = 0
  for (const ch of str) {
    const c = ch.codePointAt(0)
    if (c < 0x20 && c !== 0x0a && c !== 0x0d && c !== 0x09) continue // 控制字符不计
    if (c <= 0x7f) { total++; continue }                              // ASCII
    total++
    if ((c >= 0x4e00 && c <= 0x9fff) ||   // CJK 统一表意
        (c >= 0x3000 && c <= 0x303f) ||   // CJK 标点
        (c >= 0xff00 && c <= 0xffef)) {   // 全角形式
      cjk++
    }
  }
  return total === 0 ? 0 : cjk / total
}

// gb18030 解码器缓存(解码器有状态,每次新建;TextDecoder 构造成本低,不做全局缓存)
let gb18030Supported = null

/**
 * 尝试按 gb18030 解码"被误读的字节"
 * @param {string} str
 * @returns {string|null} 解码成功且置信度达标返回新串,否则 null
 */
function tryDecodeGb18030(str) {
  if (typeof TextDecoder === 'undefined') return null
  if (gb18030Supported === null) {
    try {
      new TextDecoder('gb18030')
      gb18030Supported = true
    } catch {
      gb18030Supported = false
    }
  }
  if (!gb18030Supported) return null

  // charCode & 0xFF 还原单字节序列;>0xFF 的码点说明串中已有真正的 Unicode
  // 字符(非 GBK 误读形态),字节级还原无意义
  const bytes = new Uint8Array(str.length)
  for (let i = 0; i < str.length; i++) {
    const c = str.charCodeAt(i)
    if (c > 0xff) return null
    bytes[i] = c
  }

  try {
    const decoded = new TextDecoder('gb18030', { fatal: false }).decode(bytes)
    if (!decoded || decoded === str) return null
    // 置信度: 解码后应出现合理比例的 CJK(纯 ASCII 输入不含中文,解码后与原文
    // 相同或无 CJK,均不采纳);且不应引入新的替换字符
    if (/�/.test(decoded)) return null
    if (cjkRatio(decoded) < 0.05) return null
    return decoded
  } catch {
    return null
  }
}

/**
 * 尝试修复GBK编码的乱码
 * @param {string} str
 * @returns {string}
 */
export function fixGBKEncoding(str) {
  if (!str) return str

  try {
    // 方法1: 处理URL编码后的GBK
    if (str.includes('%')) {
      try {
        const decoded = decodeURIComponent(str)
        if (decoded !== str && !hasGarbledText(decoded)) {
          return decoded
        }
      } catch {
        // 继续尝试其他方法
      }
    }

    // 方法2: 字节级 gb18030 流式解码(GB2312/GBK 超集,替代原硬编码映射表)
    if (hasGarbledText(str)) {
      const decoded = tryDecodeGb18030(str)
      if (decoded !== null) {
        return decoded
      }
    }

    return str
  } catch (error) {
    console.warn('编码修复失败:', error)
    return str
  }
}

/**
 * 智能编码检测和修复
 * @param {string} content
 * @returns {string}
 */
export function smartDecodeContent(content) {
  if (!content) return content

  // 如果没有乱码，直接返回
  if (!hasGarbledText(content)) {
    return content
  }

  // 尝试修复GBK编码
  const fixed = fixGBKEncoding(content)

  // 如果修复成功，返回修复后的内容
  if (fixed !== content && !hasGarbledText(fixed)) {
    return fixed
  }

  return content
}

/**
 * 处理终端输出的编码问题
 * @param {string} output
 * @returns {string}
 */
export function processTerminalOutput(output) {
  if (!output) return output

  try {
    // 先进行智能编码修复
    let processed = smartDecodeContent(output)

    // 移除Linux交互式标识符（避免用户看到技术标记）
    processed = processed.replace(/\[LINUX_INTERACTIVE\]\s*/g, '')

    // 处理特殊的终端控制字符
    // 将ANSI颜色代码转换为HTML而不是移除它们
    processed = convertAnsiToHtml(processed)
    processed = processed.replace(/\r\n/g, '\n') // 标准化换行符
    processed = processed.replace(/\r/g, '\n') // 处理回车符

    return processed
  } catch (error) {
    console.warn('终端输出处理失败:', error)
    return output
  }
}

/**
 * 将ANSI转义序列转换为HTML颜色标签
 * @param {string} text
 * @returns {string}
 */
function convertAnsiToHtml(text) {
  if (!text) return text

  // 【安全修复】先转义 HTML 特殊字符，防止命令输出注入 v-html（XSS）。
  // 转义必须在 ANSI→span 转换之前执行，保证转换生成的标签不被转义。
  text = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')

  // ANSI颜色代码映射表
  const ansiColorMap = {
    // 前景色
    '30': 'color: #2e3436;',    // 黑色
    '31': 'color: #cc0000;',    // 红色
    '32': 'color: #4e9a06;',    // 绿色
    '33': 'color: #c4a000;',    // 黄色
    '34': 'color: #3465a4;',    // 蓝色
    '35': 'color: #75507b;',    // 紫色
    '36': 'color: #06989a;',    // 青色
    '37': 'color: #d3d7cf;',    // 白色
    '90': 'color: #555753;',    // 亮黑色（灰色）
    '91': 'color: #ef2929;',    // 亮红色
    '92': 'color: #8ae234;',    // 亮绿色
    '93': 'color: #fce94f;',    // 亮黄色
    '94': 'color: #729fcf;',    // 亮蓝色
    '95': 'color: #ad7fa8;',    // 亮紫色
    '96': 'color: #34e2e2;',    // 亮青色
    '97': 'color: #eeeeec;',    // 亮白色
    // 背景色
    '40': 'background-color: #2e3436;',
    '41': 'background-color: #cc0000;',
    '42': 'background-color: #4e9a06;',
    '43': 'background-color: #c4a000;',
    '44': 'background-color: #3465a4;',
    '45': 'background-color: #75507b;',
    '46': 'background-color: #06989a;',
    '47': 'background-color: #d3d7cf;',
    // 样式
    '1': 'font-weight: bold;',
    '2': 'opacity: 0.7;',
    '3': 'font-style: italic;',
    '4': 'text-decoration: underline;',
    '0': '' // 重置
  }

  // 处理ANSI转义序列（\x1b 是 ESC 控制字符，这里必须匹配）
  // eslint-disable-next-line no-control-regex
  return text.replace(/\x1b\[([0-9;]*)m/g, (match, codes) => {
    if (!codes) return '<span style="color: #d3d7cf;">'

    const codeList = codes.split(';')
    const styles = []

    for (const code of codeList) {
      if (code === '0' || code === '') {
        return '</span><span style="color: #d3d7cf;">'
      }
      if (ansiColorMap[code]) {
        styles.push(ansiColorMap[code])
      }
    }

    if (styles.length > 0) {
      return `</span><span style="${styles.join(' ')}">`
    }

    return '<span style="color: #d3d7cf;">'
  })
}

// 导出默认对象
export default {
  hasGarbledText,
  fixGBKEncoding,
  smartDecodeContent,
  processTerminalOutput
}

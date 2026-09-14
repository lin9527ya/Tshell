/**
 * 编码转换工具类
 * 专门用于处理中文乱码问题
 */

/**
 * 检测字符串是否包含中文乱码
 * @param {string} str 
 * @returns {boolean}
 */
export function hasGarbledText(str) {
  if (!str) return false
  
  // 检测常见的乱码特征
  const garbledPatterns = [
    /\ufffd/,                    // 替换字符 (replacement character)
    /[À-ÿ]{2,}/,                 // 连续的拉丁扩展字符
    /[Â¡Â¢Â£Â¤Â¥Â¦Â§Â¨Â©ÂªÂ«Â¬Â­Â®Â¯Â°Â±Â²Â³Â´ÂµÂ¶Â·Â¸Â¹ÂºÂ»Â¼Â½Â¾Â¿]/, // GBK转UTF-8常见乱码
    /æ[°±²³´µ¶·¸¹º»¼½¾¿][\x80-\xBF]/, // 中文转UTF-8的常见乱码模式
    /[èéêëìíîï][€‚ƒ„…†‡ˆ‰Š‹ŒŽ''""•–—˜™š›œžŸ][\x80-\xBF]/, // 另一种常见模式
  ]
  
  return garbledPatterns.some(pattern => pattern.test(str))
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
      } catch (e) {
        // 继续尝试其他方法
      }
    }
    
    // 方法2: 手动处理常见的GBK->UTF-8乱码
    let fixed = str
    
    // 常见的GBK乱码字符映射
    const gbkFixes = {
      'ä¸­æ–‡': '中文',
      'æµ‹è¯•': '测试',
      'ä½ å¥½': '你好',
      'ç³»ç»Ÿ': '系统',
      'æ–‡ä»¶': '文件',
      'ç›®å½•': '目录',
      'é"™è¯¯': '错误',
      'æˆåŠŸ': '成功',
      'å¤±è´¥': '失败',
      'å®Œæˆ': '完成',
      'æ­£åœ¨': '正在',
      'è¯·ç­‰å¾…': '请等待',
      'å·²å®Œæˆ': '已完成',
      // 新增更多常见乱码模式
      '扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁': '目录',
      '扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁扁': '存放的目录',
      'æ˜ä»¶': '文件',
      'ç®å½•': '目录',
      'é”™è¯¯': '错误',
      'æˆ’åž': '成功',
      'å¤±è´¥': '失败',
      'ç³»ç»Ÿ': '系统',
      'è¿žæ¥': '连接',
      'æˆ­æ­': '断开',
      'çˆæ¬': '版本',
      'è¾å‡º': '输出',
      'è¾å…¥': '输入',
      'ä¸»æº': '主机',
      'å®¢æˆ·ç«¯': '客户端',
      'æˆä»¤': '命令',
      'æˆ†è¡': '执行',
      'å®æ˜‚': '完成',
      'å¤„ç«¯': '终端',
      'å¨æ˜ƒ': '动态',
      'é…œç½®': '配置'
    }
    
    // 应用修复映射
    for (const [garbled, correct] of Object.entries(gbkFixes)) {
      fixed = fixed.replace(new RegExp(garbled, 'g'), correct)
    }
    
    // 方法3: 使用字节级别的转换
    if (hasGarbledText(fixed) && typeof TextDecoder !== 'undefined') {
      try {
        // 将字符串转换为字节数组
        const bytes = new Uint8Array(str.length)
        for (let i = 0; i < str.length; i++) {
          bytes[i] = str.charCodeAt(i) & 0xFF
        }
        
        // 尝试不同的编码解码
        const encodings = ['gbk', 'gb2312', 'gb18030', 'big5']
        
        for (const encoding of encodings) {
          try {
            const decoder = new TextDecoder(encoding, { fatal: false })
            const decoded = decoder.decode(bytes)
            
            // 检查解码结果是否包含中文且没有乱码
            if (decoded && 
                decoded !== str && 
                /[\u4e00-\u9fff]/.test(decoded) && 
                !hasGarbledText(decoded)) {
              console.log(`成功使用${encoding}编码修复乱码`)
              return decoded
            }
          } catch (e) {
            // 继续尝试下一种编码
          }
        }
      } catch (e) {
        console.warn('字节级编码转换失败:', e)
      }
    }
    
    return fixed
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
  
  console.log('检测到可能的编码问题，尝试修复...')
  
  // 尝试修复GBK编码
  const fixed = fixGBKEncoding(content)
  
  // 如果修复成功，返回修复后的内容
  if (fixed !== content && !hasGarbledText(fixed)) {
    console.log('编码修复成功')
    return fixed
  }
  
  console.warn('无法自动修复编码问题，返回原始内容')
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
  
  // 处理ANSI转义序列
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
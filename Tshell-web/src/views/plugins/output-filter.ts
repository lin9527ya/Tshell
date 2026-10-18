/**
 * 插件输出的**过滤/解析**逻辑（阶段三 §3.11：拆 1948 行的 PluginsView.vue）。
 *
 * 抽这批的理由：这是该视图里最长、最绕、也最容易出错的纯字符串处理
 * （两个函数合计约 170 行），而且此前**没有一行测试**。它的行为直接决定
 * 操作员在插件面板里看到什么 —— 过滤错了就是"执行成功但看不到结果"。
 *
 * 抽取原则与 ClientFiles 那次一致：**只搬运，不顺手改行为**。
 * 具体保留了两处"看起来该统一、但改了就是行为变更"的地方：
 *   1. 两个过滤函数用的**凭据关键字表不一样**（见下）
 *   2. 原来的 `console.log` 调试输出仍然会发生 —— 通过可注入的 logger 保留，
 *      而不是借抽取之机删掉。要不要去掉那些日志是独立决定。
 */

/** 会出现在 shell 历史里的执行模式命令名（用于判断"下一条命令"的边界） */
export const EXECUTION_MODES = [
  'execute-assembly',
  'inline-bin',
  'shellcode-inject',
  'script',
  'binary',
  'bof',
  'exec-file',
]

/**
 * filterShellContentByMode 用的凭据关键字（较宽）。
 * 与 mimikatz 版**刻意不同**：这个版本还认 ssp/credman/tspkg 等 provider 名。
 */
export const CREDENTIAL_KEYWORDS = [
  'Authentication Id',
  'Session',
  'Username',
  'Domain',
  'NTLM',
  'SHA1',
  'Password',
  'Kerberos',
  'Primary',
  'msv',
  'tspkg',
  'wdigest',
  'kerberos',
  'ssp',
  'credman',
  '=== CAPTURED OUTPUT ===',
  '=== FINAL OUTPUT DUMP ===',
]

/** filterShellContentForMimikatz 用的凭据关键字（较窄，但多认 mimikatz 字样） */
export const MIMIKATZ_CREDENTIAL_KEYWORDS = [
  'Authentication Id',
  'Session',
  'Username',
  'Domain',
  'NTLM',
  'SHA1',
  'mimikatz',
  '=== CAPTURED OUTPUT ===',
]

export interface FilterOptions {
  /** 找不到对应输出时返回的提示文案（本地化由调用方决定，模块保持与语言无关） */
  noOutputHint: string
  /** 调试日志出口；默认丢弃。组件传 console.log 即可保持原有行为 */
  log?: (...args: unknown[]) => void
}

/** 该行是否是"以 $ 开头的命令回显" */
function isCommandLine(line: string): boolean {
  return line.startsWith('$ ')
}

/** 从后往前找最后一次"当前模式 + 当前插件"的命令行下标，找不到返回 -1 */
function findLastCommandIndex(lines: string[], currentMode: string, pluginName: string): number {
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i]
    if (isCommandLine(line) && line.includes(currentMode) && line.includes(pluginName)) {
      return i
    }
  }
  return -1
}

/** 内容里是否出现任一凭据关键字（大小写不敏感） */
function hasCredentialKeyword(content: string, keywords: string[]): boolean {
  const lower = content.toLowerCase()
  return keywords.some((k) => lower.includes(k.toLowerCase()))
}

/**
 * filterShellContentByMode 从 shell 全量历史里切出"最近一次该模式执行"的输出。
 *
 * 切法：从最后一条匹配命令起，到下一条**任意执行模式**命令前为止。
 * 找不到匹配命令时返回提示文案（而不是整段历史 —— 那会让操作员以为看到了本次结果）。
 */
export function filterShellContentByMode(
  fullContent: string,
  currentMode: string,
  pluginName: string,
  opts: FilterOptions,
): string {
  if (!fullContent || !currentMode) return fullContent
  const log = opts.log ?? (() => {})
  const lines = fullContent.split('\n')

  const lastCommandIndex = findLastCommandIndex(lines, currentMode, pluginName)
  if (lastCommandIndex === -1) {
    return opts.noOutputHint
  }

  // 到下一条执行模式命令为止
  let endIndex = lines.length
  for (let i = lastCommandIndex + 1; i < lines.length; i++) {
    const line = lines[i]
    if (isCommandLine(line) && EXECUTION_MODES.some((m) => line.includes(m))) {
      endIndex = i
      break
    }
  }

  const latestExecutionLines = lines.slice(lastCommandIndex, endIndex)
  let finalContent = latestExecutionLines.join('\n')

  // 内容过少且是 mimikatz 时向前扩展：mimikatz 的关键输出常在其上方若干行
  if (finalContent.length < 500 && pluginName.toLowerCase().includes('mimikatz')) {
    log('🔍 检测到 mimikatz，扩展输出范围以包含更多内容')
    const expandedStartIndex = Math.max(0, lastCommandIndex - 50)
    const expandedLines = lines.slice(expandedStartIndex, endIndex)
    if (hasCredentialKeyword(expandedLines.join('\n'), CREDENTIAL_KEYWORDS)) {
      log('🎯 发现凭据相关内容，使用扩展范围')
      finalContent = expandedLines.join('\n')
    }
  }

  log('🔍 最新执行内容过滤结果:', {
    原始总行数: lines.length,
    最后命令位置: lastCommandIndex,
    结束位置: endIndex,
    提取行数: latestExecutionLines.length,
    最终内容长度: finalContent.length,
    执行模式: currentMode,
    插件名: pluginName,
  })

  return finalContent
}

/**
 * filterShellContentForMimikatz mimikatz 专用的宽松过滤。
 *
 * 与通用版的区别（都是有意的）：
 *   - 找不到"当前模式"的命令时，退而找**任何** mimikatz/sekur/同名插件命令
 *   - 仍找不到就返回最近 200 行里含凭据关键字的那段（宁可多给，别漏结果）
 *   - 结束边界更宽松：只在遇到"另一个插件的执行命令"时才停，
 *     且排除 `[CLIENT-PACKET]` / `完整数据包` 这类同一次执行的中间回显
 *   - 向前多取 20 行上下文
 */
export function filterShellContentForMimikatz(
  fullContent: string,
  currentMode: string,
  pluginName: string,
  opts: FilterOptions,
): string {
  if (!fullContent || !currentMode) return fullContent
  const log = opts.log ?? (() => {})
  const lines = fullContent.split('\n')

  let lastCommandIndex = findLastCommandIndex(lines, currentMode, pluginName)

  // 退路一：任何 mimikatz / sekur / 同名插件的命令
  if (lastCommandIndex === -1) {
    for (let i = lines.length - 1; i >= 0; i--) {
      const line = lines[i]
      const lower = line.toLowerCase()
      if (isCommandLine(line) && (lower.includes('mimikatz') || lower.includes('sekur') || line.includes(pluginName))) {
        lastCommandIndex = i
        break
      }
    }
  }

  // 退路二：最近 200 行里含凭据关键字就给出来
  if (lastCommandIndex === -1) {
    const recentLines = lines.slice(Math.max(0, lines.length - 200))
    const recentContent = recentLines.join('\n')
    if (hasCredentialKeyword(recentContent, MIMIKATZ_CREDENTIAL_KEYWORDS)) {
      log('🎯 在最近内容中发现可能的 mimikatz 输出')
      return recentContent
    }
    return opts.noOutputHint
  }

  // 宽松结束边界：只在遇到"明确属于另一个插件"的执行命令时停止
  let endIndex = lines.length
  for (let i = lastCommandIndex + 1; i < lines.length; i++) {
    const line = lines[i]
    if (
      isCommandLine(line) &&
      !line.includes('[CLIENT-PACKET]') &&
      !line.includes('完整数据包') &&
      EXECUTION_MODES.some((m) => line.includes(m)) &&
      !line.includes(pluginName)
    ) {
      endIndex = i
      break
    }
  }

  const startIndex = Math.max(0, lastCommandIndex - 20)
  const extractedLines = lines.slice(startIndex, endIndex)

  log('🔍 Mimikatz 专用过滤结果:', {
    原始总行数: lines.length,
    开始位置: startIndex,
    最后命令位置: lastCommandIndex,
    结束位置: endIndex,
    提取行数: extractedLines.length,
    执行模式: currentMode,
    插件名: pluginName,
  })

  return extractedLines.join('\n')
}

/** 执行模式下拉/标签的显示文案。多数是固定英文名（与后端模式名一致），少数需要本地化。 */
export function executionModeText(mode: string, isZh: boolean): string {
  switch (mode) {
    case 'execute-assembly':
      return 'Execute Assembly'
    case 'inline-bin':
      return 'Inline Bin'
    case 'shellcode-inject':
      return isZh ? 'Shellcode注入' : 'Shellcode'
    case 'script':
      return 'Linux Script'
    case 'binary':
      return 'Linux Binary'
    case 'bof':
      return 'BOF(COFF)'
    case 'exec-file':
      return 'Exec File'
    default:
      return isZh ? '请选择模式' : 'Select Mode'
  }
}

/**
 * 前端债棘轮（阶段三 §3.11）。
 *
 * 为什么用"棘轮"而不是"一次禁止"：
 *
 * 内联三元有 1600+ 处、`as any` 有 60+ 处，散在 60 多个文件里。要求一次清零
 * 只会让 CI 长期红着、然后被人关掉 —— 那还不如不设。棘轮记录**当前上限**，
 * 只许降不许升：既不让存量卡住 CI，又保证新写的代码不再增加，
 * 每次清理顺手把上限调小，进度只进不退。
 *
 * 计数口径（刻意排除掉"不该算"的部分）：
 *  - `isZh ?` 只统计**模板/表达式里的内联三元**；`isZh` 作为**函数参数**的文件
 *    （api/cred/index.ts、stores/screenshots.ts）不算 —— 那不是组件，
 *    把语言作为显式参数传入是正确设计，不该被逼着改成 store 调用。
 *  - 注释里的示例代码不算。
 */
import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, basename } from 'node:path'

// 扫描范围 = src/ 本身。此前写成 join(__dirname, '..')（= 前端根目录），
// 导致两件事：relative() 出来的路径带 src\ 前缀、与下面的排除集合对不上，
// 且会把 src/ 之外的东西（脚本、产物）也纳入计数。
const SRC = __dirname

/**
 * 语言以**函数参数**传入、而非从 store 取的文件（正确设计，不计入）。
 *
 * 这些都不是组件：把语言作为显式参数传入，模块才能不依赖 Pinia 直接单测。
 * 强行改成 store 调用反而破坏可测试性 —— 所以棘轮对它们豁免，
 * 而不是逼着后来者把好设计改掉。
 */
const PARAM_LOCALE_FILES = new Set([
  join('api', 'cred', 'index.ts'),
  join('stores', 'screenshots.ts'),
  join('views', 'plugins', 'output-filter.ts'),
])

function walk(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    const p = join(dir, e)
    if (statSync(p).isDirectory()) {
      // 跳过依赖/产物，以及**隐藏目录**：.claude/worktrees 下是其它 agent 的
      // 工作树副本（同源文件的另一份），把它们算进来会让计数凭空变多、
      // 而且那些副本不该被本仓库的棘轮约束。
      if (e === 'node_modules' || e === 'dist' || e.startsWith('.')) continue
      walk(p, out)
    } else if (/\.(vue|ts)$/.test(e) && !e.endsWith('.test.ts')) {
      out.push(p)
    }
  }
  return out
}

/** 去掉注释，避免把注释里的示例代码算进棘轮 */
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
}

function countMatches(re: RegExp, excludeParamFiles = false): { total: number; byFile: Map<string, number> } {
  let total = 0
  const byFile = new Map<string, number>()
  for (const file of walk(SRC)) {
    const rel = relative(SRC, file)
    if (basename(file) === 'i18n.ts') continue
    if (excludeParamFiles && PARAM_LOCALE_FILES.has(rel)) continue
    const src = stripComments(readFileSync(file, 'utf8'))
    const n = (src.match(re) || []).length
    if (n > 0) {
      total += n
      byFile.set(rel, n)
    }
  }
  return { total, byFile }
}

function report(byFile: Map<string, number>): string {
  return [...byFile.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 10)
    .map(([f, n]) => `    ${f}: ${n}`)
    .join('\n')
}

describe('前端债棘轮', () => {
  it('内联 i18n 三元不再增长（用 i18nStore.tl(zh, en)）', () => {
    // 上限 0：组件里**不允许**再出现内联语言三元。
    // 2026-10-19 从 1655 处一次性迁到 0（脚本见 scripts/migrate-i18n*.cjs）。
    const CAP = 0
    const { total, byFile } = countMatches(/isZh\s*\?/g, true)
    if (total > CAP) {
      throw new Error(
        `内联三元 ${total} 处，超过棘轮上限 ${CAP}。新代码请用 i18nStore.tl('中文','English')` +
          `（见 src/stores/i18n.ts 的 tl 说明）。当前分布：\n${report(byFile)}`,
      )
    }
    expect(total).toBeLessThanOrEqual(CAP)
  })

  it('`as any` 不再增长', () => {
    // 2026-10-19 从 63 降到 32（axios 配置扩展、performance.memory 接口、
    // 厂商前缀全屏 API 声明各消掉一批）。清理时同步调小。
    const CAP = 32
    const { total, byFile } = countMatches(/\bas\s+any\b/g)
    if (total > CAP) {
      throw new Error(
        `\`as any\` ${total} 处，超过棘轮上限 ${CAP}。` +
          `优先给真实类型；确实拿不到时用 unknown + 收窄。当前分布：\n${report(byFile)}`,
      )
    }
    expect(total).toBeLessThanOrEqual(CAP)
  })

  it('i18n store 暴露 tl，且按 locale 真的分支', async () => {
    // tl 若退化成"恒返回中文"，切换语言会静默失效 —— 而 1600 处调用点
    // 全是它，坏了不会有人立刻发现。这里直接钉住行为。
    const { createPinia, setActivePinia } = await import('pinia')
    setActivePinia(createPinia())
    const { useI18nStore } = await import('@/stores/i18n')
    const s = useI18nStore()

    s.setLocale('zh')
    expect(s.tl('中文', 'English')).toBe('中文')
    s.setLocale('en')
    expect(s.tl('中文', 'English')).toBe('English')
    // 回退：缺翻译时不应变成空串
    expect(s.t('no.such.key', '兜底')).toBe('兜底')
    expect(s.t('no.such.key')).toBe('no.such.key')
  })
})

// 一次性迁移脚本：`isZh ? '中文' : 'English'` → `i18nStore.tl('中文', 'English')`
//
// 只处理**形态统一**的单行单引号三元（1621/1655）。其余 34 处（模板字符串、
// 成员访问如 s.zh/s.en、字符串拼接、嵌套三元）留给人工 —— 机械替换它们会出错。
//
// 用法：node scripts/migrate-i18n.cjs [--write]
// 不带 --write 时只报告将改动多少处（dry-run）。
const fs = require('fs')
const path = require('path')

const WRITE = process.argv.includes('--write')
const SRC = path.join(__dirname, '..', 'src')

// 单行、单引号、两侧都是纯字面量（允许字面量里出现转义的单引号 \'）
const RE = /isZh\s*\?\s*'((?:[^'\\\r\n]|\\.)*)'\s*:\s*'((?:[^'\\\r\n]|\\.)*)'/g

// 这两个文件里的 isZh 是**函数参数**，不是 store 的 computed：
//   api/cred/index.ts      sourceLabel(source, isZh)
//   stores/screenshots.ts  capture(uid, quality, isZh)
// 它们不是组件，把语言作为显式参数传入是正确设计 —— 不能改写成 i18nStore.tl
// （那些作用域里根本没有 i18nStore，改了直接编译不过）。
const EXCLUDE = new Set([
  path.join('api', 'cred', 'index.ts'),
  path.join('stores', 'screenshots.ts'),
])

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (/\.(vue|ts)$/.test(e.name)) out.push(p)
  }
  return out
}

let filesChanged = 0
let sitesChanged = 0
const skipped = []

for (const file of walk(SRC)) {
  const rel = path.relative(SRC, file)
  if (path.basename(file) === 'i18n.ts' || EXCLUDE.has(rel)) continue
  const src = fs.readFileSync(file, 'utf8')
  if (!/isZh\s*\?/.test(src)) continue

  const before = (src.match(/isZh\s*\?/g) || []).length
  const next = src.replace(RE, (_m, zh, en) => `i18nStore.tl('${zh}', '${en}')`)
  const after = (next.match(/isZh\s*\?/g) || []).length
  const done = before - after

  if (done > 0) {
    filesChanged++
    sitesChanged += done
    if (WRITE) fs.writeFileSync(file, next, 'utf8')
  }
  if (after > 0) {
    skipped.push(`  ${path.relative(SRC, file)}: 剩 ${after} 处需人工`)
  }
}

console.log(`${WRITE ? '已写入' : 'dry-run'}：${filesChanged} 个文件，${sitesChanged} 处`)
if (skipped.length) {
  console.log(`\n仍需人工处理的文件（${skipped.length}）：`)
  for (const s of skipped) console.log(s)
}

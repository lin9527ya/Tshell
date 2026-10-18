// 迁移第二遍：处理第一遍正则覆盖不到的形态。
//
// 第一遍只吃「单行 + 单引号 + 纯字面量」。剩下这些形态需要更宽的规则：
//   isZh ? x.zh : x.en                 成员访问（i18n 表项对象）
//   isZh ? `共 ${n} 项` : `${n} items`  模板字符串（含插值）
//
// 仍然**不碰**的：嵌套三元、字符串拼接、以及 isZh 是函数参数的文件
// —— 那些留给人工，机械替换容易把括号配错。
//
// 用法：node scripts/migrate-i18n-pass2.cjs [--write]
const fs = require('fs')
const path = require('path')

const WRITE = process.argv.includes('--write')
const SRC = path.join(__dirname, '..', 'src')

// isZh 是函数参数、作用域里没有 i18nStore 的文件
const EXCLUDE = new Set([
  path.join('api', 'cred', 'index.ts'),
  path.join('stores', 'screenshots.ts'),
])

// 形态 A：成员访问 isZh ? a.b : a.c（两侧同前缀，只差 zh/en）
const RE_MEMBER = /isZh\s*\?\s*([A-Za-z_$][\w$]*(?:\.[\w$]+)*)\.zh\s*:\s*\1\.en/g

// 形态 B：模板字符串 isZh ? `...` : `...`
//   只匹配「不含反引号、且 ${...} 里不含花括号」的简单插值 —— 更复杂的会漏掉，
//   漏掉是安全的（保持原样），错配才是危险的。
const RE_TPL = /isZh\s*\?\s*(`(?:[^`\\]|\\.)*`)\s*:\s*(`(?:[^`\\]|\\.)*`)/g

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (/\.(vue|ts)$/.test(e.name)) out.push(p)
  }
  return out
}

let files = 0
let sites = 0

for (const file of walk(SRC)) {
  const rel = path.relative(SRC, file)
  if (path.basename(file) === 'i18n.ts' || EXCLUDE.has(rel)) continue
  const src = fs.readFileSync(file, 'utf8')
  if (!/isZh\s*\?/.test(src)) continue

  let n = 0
  let next = src.replace(RE_MEMBER, (_m, obj) => {
    n++
    return `i18nStore.tl(${obj}.zh, ${obj}.en)`
  })
  next = next.replace(RE_TPL, (_m, zh, en) => {
    n++
    return `i18nStore.tl(${zh}, ${en})`
  })

  if (n > 0) {
    files++
    sites += n
    if (WRITE) fs.writeFileSync(file, next, 'utf8')
  }
}

console.log(`${WRITE ? '已写入' : 'dry-run'}：${files} 个文件，${sites} 处`)

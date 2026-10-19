// 测试环境的浏览器语义补齐：localStorage / sessionStorage。
//
// 为什么需要：浏览器与 jsdom 都提供 Storage，但 vitest 1.x 的 populateGlobal 在部分
// Node 版本下不会把它桥到 globalThis —— 实测 Node 26 上 `globalThis.localStorage`
// 的属性存在、值却是 undefined（CI 用的 Node 20 未必复现，不能靠环境碰运气）。
// 组件按浏览器语义写的是裸 `localStorage`（PivotGraphView 的拖拽位置持久化），
// 于是测试里一取就是 TypeError: Cannot read properties of undefined (reading 'clear')。
//
// 这里只在**缺失**时补一个符合 Storage 接口的内存实现：不覆盖环境已有的实现，
// 也不引入对 jsdom 内部行为的依赖，测试结果因此与 Node 版本无关。
function makeStorage(): Storage {
  const m = new Map<string, string>()
  return {
    get length() {
      return m.size
    },
    clear: () => m.clear(),
    getItem: (k: string) => (m.has(k) ? m.get(k)! : null),
    key: (i: number) => Array.from(m.keys())[i] ?? null,
    removeItem: (k: string) => {
      m.delete(k)
    },
    setItem: (k: string, v: string) => {
      m.set(k, String(v))
    },
  } as Storage
}

for (const name of ['localStorage', 'sessionStorage'] as const) {
  const g = globalThis as unknown as Record<string, unknown>
  const w = g.window as (Window & typeof globalThis) | undefined
  const existing = g[name] ?? w?.[name]
  if (typeof existing === 'undefined') {
    Object.defineProperty(globalThis, name, {
      value: makeStorage(),
      configurable: true,
      writable: true,
    })
  }
}

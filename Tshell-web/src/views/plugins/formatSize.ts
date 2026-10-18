/**
 * 插件模块的文件大小格式化（阶段三 §3.11）。
 *
 * 为什么单独一个模块：原实现里同一个函数被**两处**用（插件详情条、上传对话框
 * 的文件列表）。拆组件时如果各自留一份，就变成了两份实现、两条演化路径 ——
 * 正是这次拆分要避免的事。
 *
 * 与 `views/client/files/format.ts` 的 `formatFileSize` **刻意不同**：
 *
 *   本模块（插件）      两位小数 + parseFloat 去尾零   1024 → "1 KB"
 *   文件管理器的那个    一位小数                      1024 → "1.0 KB"
 *
 * 这是原代码就存在的分歧，不是本次抽取引入的。统一会改变其中一处的显示，
 * 属可见变更，应该单独提一次由人确认，而不是夹在重构里悄悄发生。
 * 两边的测试各自钉住了当前行为，改动会在测试里暴露。
 */

/** 1024 进制单位表 */
const UNITS = ['B', 'KB', 'MB', 'GB']

/**
 * formatPluginFileSize 插件模块的文件大小显示。
 *
 * 负数/NaN 归零（而不是产出 "NaN GB" 这种字符串）：
 * 后端偶尔回 size 缺失，页面上出现 "NaN GB" 比 "0 B" 更难排查。
 */
export function formatPluginFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const k = 1024
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), UNITS.length - 1)
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + UNITS[i]
}

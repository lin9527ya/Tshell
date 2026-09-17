# 前端壳层契约（UI Shell Contract）

> 用途：`vshell-ui` 分支（VShell 风格改造）重写外壳时**必须遵守本契约**，保证
> 30 个功能面板、27 个对话框、全部 store/api 零改动即可挂到新壳上。
> 本文件描述「壳与内核之间」的全部接口；壳内部的布局/视觉不受约束。

## 1. 内核与壳的边界（一句话）

**壳 = 路由目标 + 布局容器 + 导航入口 + 面板/对话框的挂载方式。**
**内核 = panelRegistry 的 30 个面板、27 个对话框组件、stores、api、i18n。**
换壳只允许改壳；内核文件的 import 路径与对外 API 不得变化。

## 2. 路由契约（不可变）

路由仅 4 条，`/` 指向的唯一业务入口组件可以被 vshell 分支替换为 `VShellApp.vue`：

| 路径 | 说明 |
|---|---|
| `/` | 业务主入口（当前渲染 `views/HomeView.vue` → `CsShell`） |
| `/login` `/404` `/401` | 独立页面 |

守卫链不变：JWT 校验（内存 token + HttpOnly Cookie 兜底）→ RBAC `meta.permiss`。

## 3. panelRegistry 契约（面板挂载的唯一真相源）

`layout/Workspace/panelRegistry.ts`：`Record<string, Component>`，共 30 个有效键。
新壳打开面板的方式与现壳一致：

```ts
workspace.openTab({ id, title, component: '<key>', params?: { uid?: string } })
```

- 键不存在时壳需渲染占位（现壳渲染 `panel.component [x] not registered`）
- 面板内通过 `inject('realUid')` 获取目标会话 uid —— **壳必须 provide**
  （现实现在 `CsTabContainer.vue` 的 `TabContentHost`；新壳的标签内容宿主同样要 provide）
- 面板键清单（16 个核心 + 14 个会话类）见 panelRegistry.ts 内注释分组

## 4. workspace store 契约（壳的操作入口）

`stores/workspace.ts` 对壳暴露（新壳应保持这些调用不变）：

| API | 语义 |
|---|---|
| `openTab({id,title,component,params})` / `closeTab(id)` | 标签页打开/关闭（同 id 幂等聚焦） |
| `openDialog(key, params?)` / `closeDialog(key)` | 对话框开关（key = DialogState 27 键之一） |
| `setTopViewMode('table'\|'graph'\|'targets')` | 顶部主视图切换（**vshell 壳可忽略此概念，改由导航/页面承担**） |
| `isTopCollapsed` | 顶部主视图折叠状态（同上，可忽略） |
| `splitRatio` | 上下分屏比例（同上，可忽略） |
| `tabs` / `activeTabId` / `dialogs` | 状态读取 |

> 说明：`setTopViewMode/isTopCollapsed/splitRatio` 是 CS 壳的分屏概念，
> **vshell 壳不需要实现**，但保留字段以兼容仍在调用它们的旧入口（菜单等）。

## 5. 对话框契约

27 个 `DialogState` 键在 `stores/workspace.ts`；每个键对应的组件在 `CsShell.vue`
中平铺挂载（`v-model="workspace.dialogs.<key>"`）。
**新壳必须在自己的根组件里同样平铺挂载这 27 个组件**（直接照抄 CsShell 的挂载块即可），
否则 `workspace.openDialog(...)` 静默无效。

嵌套子对话框（不在 DialogState 中，由其父组件自行挂载，壳无需关心）：
`MailServerDialog`（←SpearPhish）、`ChoosePayloadDialog`（←多个载荷对话框）、
`EditListenerDialog`（←listener/ForwardListeners）。

## 6. 主题令牌契约

- `assets/cs-theme.css`：24 个 `--cs-*` 令牌（明暗两套 `:root[data-theme]`），
  是全站唯一设计令牌源；**vshell 壳可直接复用令牌层**（保持观感一致）或另建
  `--vs-*` 体系（但面板内 3236 处 `cs-*` 类名 + `var(--cs-*)` 引用不会改，
  因此最省力路径仍是复用 `--cs-*` 令牌 + `.cs-btn/.cs-dialog/.cs-form-*` 基础件）。
- 主色链路：`theme store.primaryColor → --theme-primary → --cs-active`（已打通）。
- 明暗切换：`html[data-theme="dark"]`；Element Plus 令牌已在 `.cs-shell` 层级映射
  （注意：el-dialog 等 teleport 到 body 的弹层不在此作用域，靠 `.cs-dialog`
  自带 `!important` 覆盖 + main.css 全局深色段——新壳若换根类名需保留等效处理）。

## 7. i18n 契约（不可变）

- `stores/i18n.ts`：`t(key, defaultText?)` + `isZh`；`menu.*` 键名是 shell 与
  MenuBar 的约定键，**新壳若重建菜单需复用这些键或新增而非改名**。
- 除字典外，全站有 ~1558 处内联 `isZh ? '中文' : 'English'` 三元——**必须保留
  `isZh` computed 的可用性**（组件内自行 useI18nStore，不依赖壳 provide）。
- `App.vue` 的 `<el-config-provider :locale>` 联动 zh-cn/en 不可移除。

## 8. 事件流与状态契约

- 全局事件 WS 单例在 `stores/events.ts`（客户端上线/离线/新会话 → 节流刷新 clients store），
  壳不参与。
- 会话列表数据源 `stores/clients.ts`（SessionTable 与各面板共享）。
- StatusBar 展示的数据（在线数/lag 等）来自 clients/events store，新壳自定义状态栏时
  直接读这些 store。

## 9. vshell 分支改造清单（照此执行）

1. `git checkout -b vshell-ui`（从优化后的 main 切出）
2. 新建 `layout-vshell/`：`VShellApp.vue`（根容器）+ `Sidebar.vue` + `Header.vue` + `TagsBar.vue`
   - 标签/对话框系统：**复用** workspace store 与 CsTabContainer 的内容宿主模式
     （可参考/改造 CsTabContainer.vue 的 `TabContentHost`，确保 `provide('realUid')`）
   - 27 个对话框挂载块从 CsShell.vue 原样搬运
3. `views/HomeView.vue` 改为渲染 `VShellApp`（CsShell 保留在分支上以备对照）
4. 导航映射：MenuBar 8 顶级菜单 → 侧边栏分区树；所有 `openTab/openDialog` action 原样保留
5. 主题：深色侧栏 + 浅色内容 + `--theme-primary` 默认 #0960bd（Vben 风）；
   暗色模式保留（`data-theme` 机制不动）
6. 验证：type-check 0 错误 / vitest / build:embed / 手工走查面板与对话框全通

## 10. 按需编译

分支切换即风格切换：`git checkout main → build`（CS 风）/ `git checkout vshell-ui → build`（VShell 风）。
两分支共享内核演进：vshell 分支定期 `git rebase main`。
未来若要单 checkout 双风格，引入 `VITE_UI_STYLE=cs|vshell` 构建变量，
切换点即 `HomeView.vue` 的根组件选择（其余契约均不变）。

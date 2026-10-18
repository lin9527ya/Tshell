import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export type LocaleType = 'zh' | 'en'

export const useI18nStore = defineStore(
  'cs-i18n',
  () => {
    // 默认中文 'zh'
    const currentLocale = ref<LocaleType>('zh')

    const isZh = computed(() => currentLocale.value === 'zh')

    function setLocale(lang: LocaleType) {
      currentLocale.value = lang
    }

    function toggleLocale() {
      currentLocale.value = currentLocale.value === 'zh' ? 'en' : 'zh'
    }

    const dict: Record<LocaleType, Record<string, string>> = {
      zh: {
        // 8 大主菜单
        'menu.cs': 'Tshell',
        'menu.view': '视图(V)',
        'menu.payload': '有效载荷(P)',
        'menu.attacks': '攻击(A)',
        'menu.site': '站点管理(M)',
        'menu.sysconfig': '系统配置(S)',
        'menu.reporting': '报告(R)',
        'menu.help': '帮助(H)',

        // Tshell 菜单项
        'menu.cs.new_conn': '新建连接(N)',
        'menu.cs.preferences': '偏好设置(P)',
        'menu.cs.visualization': '可视化(V)',
        'menu.cs.pivot_graph': '拓扑网络图',
        'menu.cs.session_table': '会话列表',
        'menu.cs.target_table': '目标列表',
        'menu.cs.listeners': '监听器(L)',
        'menu.cs.vpn_interface': 'VPN网卡(I)',
        'menu.cs.c2_profile': 'C2扩展配置(M)',
        'menu.cs.script_manager': '脚本管理器(S)',
        'menu.cs.script_console': '脚本控制台(C)',
        'menu.cs.disconnect': '断开连接(C)',

        // 视图(V) 菜单项 (截图 1, 2)
        'menu.view.sessions': '会话列表(B)',
        'menu.view.applications': '应用程序(A)',
        'menu.view.credentials': '密码凭证(C)',
        'menu.view.downloads': '下载列表(D)',
        'menu.view.event_log': '事件日志(E)',
        'menu.view.keystrokes': '键盘记录(K)',
        'menu.view.proxy_pivots': '代理转发(P)',
        'menu.view.screenshots': '屏幕截图(S)',
        'menu.view.targets': '目标列表(T)',
        'menu.view.web_log': 'Web日志(W)',
        'menu.view.beacon_log': 'Beacon日志',

        // 有效载荷(P) 菜单项
        'menu.payload.gen': '分阶段载荷生成器(P)',
        'menu.payload.stageless_gen': '无阶段载荷生成器(S)',
        'menu.payload.win_exe': 'Windows分阶段可执行程序(E)',
        'menu.payload.win_exe_stageless': 'Windows无阶段可执行程序(W)',
        'menu.payload.win_stageless_all': '生成所有格式有效载荷(A)',

        // 攻击(A) 菜单项
        'menu.attacks.web_delivery': 'Web投递(无阶段)(S)',
        'menu.attacks.forward_connect': '正向连接(F)',
        'menu.attacks.signed_applet': '签名Applet攻击(S)',
        'menu.attacks.smart_applet': '智能Applet攻击(T)',
        'menu.attacks.sys_profiler': '信息收集(P)',
        'menu.attacks.spear_phishing': '邮件钓鱼(S)',

        // 站点管理(M) 菜单项
        'menu.site.manage': '站点管理(M)',
        'menu.site.clone': '网站克隆(C)',
        'menu.site.host_file': '文件托管(H)',

        // 系统配置(S) 菜单项
        'menu.sysconfig.security': '安全中心(S)',
        'menu.sysconfig.stealth': '隐藏访问(H)',
        'menu.sysconfig.audit': '审计日志(A)',
        'menu.sysconfig.keys': '密钥管理(K)',
        'menu.sysconfig.session_keys': '会话密钥(E)',
        'menu.sysconfig.users': '用户管理(U)',
        'menu.sysconfig.config': '服务配置(C)',
        'menu.sysconfig.status': '服务器状态(H)',
        'menu.sysconfig.pool': '连接池(P)',

        // 报告(R) 菜单项
        'menu.reporting.activity': '0. 活动报告',
        'menu.reporting.hosts': '1. 主机资产报告',
        'menu.reporting.ioc': '2. 威胁指标报告',
        'menu.reporting.sessions': '3. 会话活动报告',
        'menu.reporting.social_eng': '4. 社会工程报告',
        'menu.reporting.tactics': '5. 战术技术过程报告',
        'menu.reporting.clear_data': '清空数据(R)',
        'menu.reporting.export_data': '导出数据(X)',

        // 帮助(H) 菜单项
        'menu.help.home': '官方主页(H)',
        'menu.help.support': '技术支持(S)',
        'menu.help.arsenal': '武器库(A)',
        'menu.help.community': '社区工具包(C)',
        'menu.help.sleep_manual': '睡眠手册',
        'menu.help.keybindings': '快捷键说明',
        'menu.help.sysinfo': '系统信息(S)',
        'menu.help.about': '关于(A)',

        // Toolbar
        'tb.connect': '新建连接',
        'tb.disconnect': '断开连接',
        'tb.listeners': '配置监听器',
        'tb.pivot_graph': '拓扑网络图',
        'tb.session_table': '会话列表',
        'tb.targets': '目标列表',
        'tb.web_delivery': 'Web投递',
        'tb.payload_gen': '分阶段载荷生成器',
        'tb.tunnel': '代理转发',
        'tb.keystrokes': '键盘记录',
        'tb.screenshots': '屏幕截图',

        // Session Table Headers (中文模式)
        'table.external': '外网IP',
        'table.internal': '内网IP',
        'table.address': '地址',
        'table.listener': '监听器',
        'table.user': '用户',
        'table.computer': '计算机',
        'table.note': '备注',
        'table.process': '进程',
        'table.pid': 'PID',
        'table.arch': '架构',
        'table.last': '最后心跳',
        'table.sleep': '间隔',

        // Session Context Menu
        'ctx.interact': '会话交互(I)',
        'ctx.sleep': '回连间隔(S)...',
        'ctx.note': '备注(N)...',
        
        'ctx.access': '凭证提权(A)',
        'ctx.persistence': '权限维持(P)',
        'ctx.persistence.registry': '注册表 Run 键',
        'ctx.persistence.schtasks': '计划任务',
        'ctx.persistence.startup': '启动文件夹',
        'ctx.persistence.wmi': 'WMI 持久化...',
        'ctx.persistence.cron': '定时任务 (Crontab)',
        'ctx.persistence.bashrc': '环境脚本 (.bashrc)',
        'ctx.persistence.hide_proc': '隐藏进程...',
        'ctx.persistence.unhide_proc': '恢复进程名',
        'ctx.persistence.hide_net': '隐藏网络连接...',
        'ctx.persistence.unhide_net': '恢复网络连接',
        'ctx.access.hashdump': '抓取Hash(D)',
        'ctx.access.elevate': '权限提升(E)...',
        'ctx.access.make_token': '创建令牌(T)...',
        'ctx.access.steal_token': '窃取令牌(S)...',
        'ctx.access.rev2self': '恢复令牌(R)',
        'ctx.access.oneliner': 'PowerShell一句话(O)',
        'ctx.access.mimikatz': '抓取明文密码(M)',
        'ctx.access.spawn': '新建会话(S)...',
        'ctx.access.spawnas': '以其他用户上线(S)...',

        'ctx.explore': '浏览探测(E)',
        'ctx.explore.browser_pivot': '浏览器代理(B)',
        'ctx.explore.desktop': '远程桌面(D)',
        'ctx.explore.files': '文件浏览(F)',
        'ctx.explore.netview': '网络探测(N)',
        'ctx.explore.portscan': '端口扫描(S)...',
        'ctx.explore.pids': '进程列表(P)',
        'ctx.explore.screenshots': '屏幕截图(C)',
        'ctx.explore.terminal': 'Linux终端交互(T)',
        'ctx.explore.tools': '客户端工具(K)',
        'ctx.explore.downloads': '已下载文件(L)',
        'ctx.explore.startup': '开机启动(P)...',

        'ctx.pivoting': '代理转发(P)',
        'ctx.pivoting.socks': '代理转发(S)...',
        'ctx.pivoting.listener': '转发上线(L)...',

        'ctx.session': '会话操作(e)',
        'ctx.session.color': '颜色标记(C)',
        'ctx.session.group': '设置分组(G)...',
        'ctx.session.check_alive': '智能存活检测(C)...',
        'ctx.session.migrate': '进程注入(I)...',
        'ctx.session.remove': '删除进程文件(保持在线)(R)',
        'ctx.session.disconnect': '断开并保留记录(D)',
        'ctx.session.exit': '删除文件并退出(X)',
        'ctx.session.cleanup_offline': '清理离线客户端记录',

        'ctx.postexp': '内存执行',
        'ctx.postexp.assembly': 'Execute Assembly (.NET内存执行)',
        'ctx.postexp.inline': 'Inline Bin (PE内存执行)',
        'ctx.postexp.shellcode': 'Shellcode注入',
        'ctx.postexp.bof': 'BOF / COFF执行',
        'ctx.postexp.script': 'Linux脚本执行',
        'ctx.postexp.binary': 'Linux二进制加载',
        'ctx.postexp.manager': '插件中心...',

        // Bottom Action Buttons
        'btn.interact': '交互',
        'btn.explore': '探测 ▾',
        'btn.note': '备注...',
        'btn.color': '颜色...',
        'btn.sleep': '心跳...',
        'btn.disconnect': '断开',
        'btn.remove': '删除',
        'btn.help': '帮助',
        'btn.add': '添加...',
        'btn.edit': '编辑...',
        'btn.copy_pwd': '复制密码',
        'btn.copy_hash': '复制哈希',
        'btn.export': '导出',
        'btn.clear_all': '清空全部',
        'btn.delete': '删除',
        'btn.refresh': '刷新',
        'btn.sync_download': '同步下载',
        'btn.view_full': '全屏查看',
        'btn.capture_screen': '屏幕抓取',
        'btn.clear_gallery': '清空画廊',
        'btn.start_keylog': '启动键盘记录',
        'btn.get_keylog': '获取击键记录',
        'btn.copy_all': '复制全部',

        // Status Bar
        'sb.connected': '已连接',
        'sb.disconnected': '未连接',
        'sb.teamserver_ip': '主控服务器:',
        'sb.beacons': '会话数:',
        'sb.lag': '延迟:',

        // Empty states
        'empty.no_sessions': '暂无活动的 Beacon 会话',
        'empty.no_creds': '暂无抓取到的凭据',
        'empty.no_downloads': '暂无下载的文件',
        'empty.no_screenshots': '暂无屏幕截图',
        'empty.no_keystrokes': '暂无记录的击键',
      },

      en: {
        // 8 Main Menus
        'menu.cs': 'Tshell',
        'menu.view': 'View(V)',
        'menu.payload': 'Payloads(P)',
        'menu.attacks': 'Attacks(A)',
        'menu.site': 'Site Manage(M)',
        'menu.sysconfig': 'System(S)',
        'menu.reporting': 'Reporting(R)',
        'menu.help': 'Help(H)',

        // Tshell Menu
        'menu.cs.new_conn': 'New Connection(N)',
        'menu.cs.preferences': 'Preferences(P)',
        'menu.cs.visualization': 'Visualization(V)',
        'menu.cs.pivot_graph': 'Pivot Graph',
        'menu.cs.session_table': 'Session Table',
        'menu.cs.target_table': 'Target Table',
        'menu.cs.listeners': 'Listeners(L)',
        'menu.cs.vpn_interface': 'VPN Interfaces(I)',
        'menu.cs.c2_profile': 'Malleable C2 Profile(M)',
        'menu.cs.script_manager': 'Script Manager(S)',
        'menu.cs.script_console': 'Script Console(C)',
        'menu.cs.disconnect': 'Disconnect(C)',

        // View Menu
        'menu.view.sessions': 'Beacons(B)',
        'menu.view.applications': 'Applications(A)',
        'menu.view.credentials': 'Credentials(C)',
        'menu.view.downloads': 'Downloaded Files(D)',
        'menu.view.event_log': 'Event Log(E)',
        'menu.view.keystrokes': 'Keystrokes(K)',
        'menu.view.proxy_pivots': 'Proxy Pivots(P)',
        'menu.view.screenshots': 'Screenshots(S)',
        'menu.view.targets': 'Targets(T)',
        'menu.view.web_log': 'Web Log(W)',
        'menu.view.beacon_log': 'Beacon Log',

        // Payloads Menu
        'menu.payload.gen': 'Payload Generator (Staged)(P)',
        'menu.payload.stageless_gen': 'Payload Generator (Stageless)',
        'menu.payload.win_exe': 'Windows Executable (Staged)(E)',
        'menu.payload.win_exe_stageless': 'Windows Executable (Stageless)',
        'menu.payload.win_stageless_all': 'Windows Stageless (All Formats)',

        // Attacks Menu
        'menu.attacks.web_delivery': 'Web Delivery (Stageless)(S)',
        'menu.attacks.forward_connect': 'Forward Connect(F)',
        'menu.attacks.signed_applet': 'Signed Applet Attack(S)',
        'menu.attacks.smart_applet': 'Smart Applet Attack(T)',
        'menu.attacks.sys_profiler': 'System Profiler(P)',
        'menu.attacks.spear_phishing': 'Spear Phishing(S)',

        // Site Manage Menu
        'menu.site.manage': 'Manage Sites(M)',
        'menu.site.clone': 'Clone Site(C)',
        'menu.site.host_file': 'Host File(H)',

        // System Config Menu
        'menu.sysconfig.security': 'Security Center(S)',
        'menu.sysconfig.stealth': 'Stealth Access(H)',
        'menu.sysconfig.audit': 'Audit Logs(A)',
        'menu.sysconfig.keys': 'Key Management(K)',
        'menu.sysconfig.session_keys': 'Session Keys(E)',
        'menu.sysconfig.users': 'User Management(U)',
        'menu.sysconfig.config': 'Server Config(C)',
        'menu.sysconfig.status': 'Server Status(H)',
        'menu.sysconfig.pool': 'Connection Pool(P)',

        // Reporting Menu
        'menu.reporting.activity': '0. Activity Report',
        'menu.reporting.hosts': '1. Hosts Report',
        'menu.reporting.ioc': '2. Indicators of Compromise',
        'menu.reporting.sessions': '3. Sessions Report',
        'menu.reporting.social_eng': '4. Social Engineering Report',
        'menu.reporting.tactics': '5. Tactics, Techniques, and Procedures',
        'menu.reporting.clear_data': 'Reset Data(R)',
        'menu.reporting.export_data': 'Export Data(X)',

        // Help Menu
        'menu.help.home': 'Home(H)',
        'menu.help.support': 'Support(S)',
        'menu.help.arsenal': 'Arsenal(A)',
        'menu.help.community': 'Community Kit(C)',
        'menu.help.sleep_manual': 'Sleep Manual',
        'menu.help.sysinfo': 'System Information(S)',
        'menu.help.about': 'About(A)',

        // Toolbar 11 Icons
        'tb.connect': 'New Connection',
        'tb.disconnect': 'Disconnect',
        'tb.listeners': 'Configure Listeners',
        'tb.pivot_graph': 'Pivot Graph',
        'tb.session_table': 'Session Table',
        'tb.targets': 'Targets Table',
        'tb.web_delivery': 'Scripted Web Delivery',
        'tb.payload_gen': 'Payload Generator (Staged)',
        'tb.tunnel': 'Proxy Pivots (SOCKS)',
        'tb.keystrokes': 'Keystrokes Browser',
        'tb.screenshots': 'Screenshots Gallery',

        // Session Table Headers
        'table.external': 'external',
        'table.internal': 'internal',
        'table.address': 'address',
        'table.listener': 'listener',
        'table.user': 'user',
        'table.computer': 'computer',
        'table.note': 'note',
        'table.process': 'process',
        'table.pid': 'pid',
        'table.arch': 'arch',
        'table.last': 'last',
        'table.sleep': 'sleep',

        // Session Context Menu
        'ctx.interact': 'Interact(I)',
        'ctx.sleep': 'Sleep(S)...',
        'ctx.note': 'Note(N)...',
        
        'ctx.access': 'Access(A)',
        'ctx.persistence': 'Persistence(P)',
        'ctx.persistence.registry': 'Registry Run Key',
        'ctx.persistence.schtasks': 'Scheduled Task',
        'ctx.persistence.startup': 'Startup Folder',
        'ctx.persistence.wmi': 'WMI Persistence...',
        'ctx.persistence.cron': 'Crontab',
        'ctx.persistence.bashrc': 'Bashrc Profile',
        'ctx.persistence.hide_proc': 'Hide Process...',
        'ctx.persistence.unhide_proc': 'Restore Process Name',
        'ctx.persistence.hide_net': 'Hide Network...',
        'ctx.persistence.unhide_net': 'Restore Network Visibility',
        'ctx.access.hashdump': 'Dump Hashes(D)',
        'ctx.access.elevate': 'Elevate(E)...',
        'ctx.access.make_token': 'Make Token(T)...',
        'ctx.access.steal_token': 'Steal Token(S)...',
        'ctx.access.rev2self': 'Rev2Self(R)',
        'ctx.access.oneliner': 'One-liner Payload(O)',
        'ctx.access.mimikatz': 'Run Mimikatz(M)',
        'ctx.access.spawn': 'Spawn(S)...',
        'ctx.access.spawnas': 'Spawn As(S)...',

        'ctx.explore': 'Explore(E)',
        'ctx.explore.browser_pivot': 'Browser Pivot(B)',
        'ctx.explore.desktop': 'Remote Desktop(D)',
        'ctx.explore.files': 'File Browser(F)',
        'ctx.explore.netview': 'Net View(N)',
        'ctx.explore.portscan': 'Port Scan(S)...',
        'ctx.explore.pids': 'Process List(P)',
        'ctx.explore.screenshots': 'Screenshot(C)',
        'ctx.explore.terminal': 'Interactive Terminal(T)',
        'ctx.explore.tools': 'Client Tools(K)',
        'ctx.explore.downloads': 'Downloaded Files(L)',
        'ctx.explore.startup': 'Persistence (P)...',

        'ctx.pivoting': 'Pivoting(P)',
        'ctx.pivoting.socks': 'Proxy Pivoting(S)...',
        'ctx.pivoting.listener': 'Forward Listener(L)...',

        'ctx.session': 'Session(e)',
        'ctx.session.color': 'Color(C)',
        'ctx.session.group': 'Set Group(G)...',
        'ctx.session.check_alive': 'Smart Alive Check(C)...',
        'ctx.session.migrate': 'Process Inject(I)...',
        'ctx.session.remove': 'Delete Process File (Stay Online)(R)',
        'ctx.session.disconnect': 'Disconnect (Keep Record)(D)',
        'ctx.session.exit': 'Delete File & Exit(X)',
        'ctx.session.cleanup_offline': 'Clean Up Offline Sessions',

        'ctx.postexp': 'Memory Execution (PostExpKit)',
        'ctx.postexp.assembly': 'Execute Assembly',
        'ctx.postexp.inline': 'Inline Bin',
        'ctx.postexp.shellcode': 'Shellcode Inject',
        'ctx.postexp.bof': 'BOF Execute',
        'ctx.postexp.script': 'Linux Script',
        'ctx.postexp.binary': 'Linux Binary',
        'ctx.postexp.manager': 'Plugins Center...',

        // Bottom Action Buttons
        'btn.interact': 'Interact',
        'btn.explore': 'Explore ▾',
        'btn.note': 'Note...',
        'btn.color': 'Color...',
        'btn.sleep': 'Sleep...',
        'btn.disconnect': 'Disconnect',
        'btn.remove': 'Remove',
        'btn.help': 'Help',
        'btn.add': 'Add...',
        'btn.edit': 'Edit...',
        'btn.copy_pwd': 'Copy Password',
        'btn.copy_hash': 'Copy Hash',
        'btn.export': 'Export',
        'btn.clear_all': 'Clear All',
        'btn.delete': 'Delete',
        'btn.refresh': 'Refresh',
        'btn.sync_download': 'Sync Download',
        'btn.view_full': 'View Full',
        'btn.capture_screen': 'Capture Screen',
        'btn.clear_gallery': 'Clear Gallery',
        'btn.start_keylog': 'Start Keylogger',
        'btn.get_keylog': 'Get Keystrokes',
        'btn.copy_all': 'Copy All',

        // Status Bar
        'sb.connected': 'Connected',
        'sb.disconnected': 'Disconnected',
        'sb.teamserver_ip': 'TeamServer IP:',
        'sb.beacons': 'Beacons:',
        'sb.lag': 'lag:',

        // Empty states
        'empty.no_sessions': 'No Active Beacon Sessions',
        'empty.no_creds': 'No Harvested Credentials',
        'empty.no_downloads': 'No Downloaded Files',
        'empty.no_screenshots': 'No Screenshots Captured',
        'empty.no_keystrokes': 'No Keystrokes Recorded',
      },
    }

    function t(key: string, defaultText?: string): string {
      return dict[currentLocale.value]?.[key] || defaultText || key
    }

    /**
     * tl（translate literal）按当前语言返回两段字面量之一。
     *
     * 为什么要有它，而不是继续写 `isZh ? '中' : 'En'`：
     *
     *  1. **可机械迁移**：内联三元在 59 个组件文件里有 1600+ 处，形态统一但
     *     grep 不出"哪些是待翻译文案"；`tl(` 把每一处都标出来，后续可用脚本抽取。
     *  2. **给字典化留一个升级点**：以后把 tl 改成"先查 dict、查不到再回退字面量"，
     *     调用点一处都不用动。这是把 1600 处迁移从"一次大爆炸"变成"可增量"的关键。
     *  3. 组件不再需要 `const isZh = computed(...)` 这一层，少一个响应式依赖。
     *
     * 注意：它读的是 `currentLocale.value`，在模板里调用时会被渲染函数追踪，
     * 切换语言仍会正常重渲染（与 isZh 行为一致）。
     */
    function tl(zhText: string, enText: string): string {
      return currentLocale.value === 'zh' ? zhText : enText
    }

    return {
      currentLocale,
      isZh,
      setLocale,
      toggleLocale,
      t,
      tl,
    }
  },
  {
    persist: {
      paths: ['currentLocale'],
    },
  },
)

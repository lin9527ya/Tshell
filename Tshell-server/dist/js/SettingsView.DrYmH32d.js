const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["js/ServerConfigView.BvoufHd0.js","js/index.BJ0GMplj.js","js/vue.Z7l_21JU.js","js/element.we09KMSP.js","js/utils.NIGUFBhG.js","css/index.Ct-JeeV1.css","js/EmptyState.D-1OFt06.js","css/EmptyState.coVD2JeJ.css","css/ServerConfigView.DJnRvDk_.css","js/ServerStatusView.BvN349Hf.js","js/dashboard.CEJOMFOM.js","css/ServerStatusView.BfrTENxm.css","js/UserManagementView.BNF7bKuv.js","js/index.CbLyuxJW.js","css/UserManagementView.DLJ6I36M.css","css/el-select.DYD98qvO.css","js/KeyManagementView.CFIFcA6s.js","css/KeyManagementView.DnLXEHUs.css","js/SecurityCenterView.DFu1Ut_0.js","js/index.B6TIENyq.js","css/SecurityCenterView.C1zImBCC.css","js/HiddenAccessView.BA5b5KR5.js","css/HiddenAccessView.DqQesSrU.css","js/LogsView.Dr8bQF3n.js","css/LogsView.BG09T77P.css","js/ConnectionPoolView.Dv6cHUPa.js","css/ConnectionPoolView.MHWPXCqB.css"])))=>i.map(i=>d[i]);
import{s as T,u as I,_ as x,S as B,h as p}from"./index.BJ0GMplj.js";import{y as R,e as g,a as k,C as P,D as L,i as s,L as $,J as v,f as C,h as f,K as z,ac as O,E as N,ad as U,aj as K,ak as G,am as H,al as W,af as Z,b as j,G as D,H as F,an as Y,F as q,aq as J}from"./vue.Z7l_21JU.js";/* empty css                  */import{r as X,t as Q,k as ee,i as _}from"./element.we09KMSP.js";import"./utils.NIGUFBhG.js";const A={listProfiles(){return T({url:"/c2profile/list",method:"get"})},previewProfile(n){return T({url:"/c2profile/preview",method:"get",params:{name:n}})},saveProfile(n,m){return T({url:"/c2profile/save",method:"post",data:{name:n,content:m}})}},te=n=>(K("data-v-755f52a4"),n=n(),G(),n),ae={class:"cs-profile-container cs-font-ui"},oe={class:"profile-toolbar"},se={class:"profile-title"},le=te(()=>s("div",{class:"profile-spacer"},null,-1)),ie={class:"profile-editor-wrap"},ne=["placeholder"],re={class:"cs-dialog-footer"},ce=`# ============================================================
# Tshell Malleable C2 Profile（YAML 格式，仿 CS profile 概念）
# 修改保存后 3 秒内自动热加载（仅影响新生成的客户端与新建监听器）
# ============================================================

name: default
description: "默认画像：保留腾讯系接口伪装路径"

# 网络请求 User-Agent（消除 Go-http-client/1.1 特征）
useragent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

# 心跳基准（毫秒，1-60000）
sleeptime_ms: 1000

# 心跳抖动百分比（0-50）
jitter_percent: 20

# HTTP 传输伪装
http:
  get:
    uri: "/tencent/mcp/pc/pcsearch"
    metadata_cookie_prepend: "BDUSS="
  post:
    uri: "/tencent/sensearch/collection/item/check"

# WebSocket 路径
ws:
  path: "/ws"

# TLS 指纹（chrome_120 / firefox_121 / edge_120 / safari_17）
tls:
  ja3: "chrome_120"

# DNS 传输特征
dns:
  beacon_prefix: "update"
  query_type: "TXT"
`,ue=R({__name:"C2ProfileDialog",props:{modelValue:{type:Boolean}},emits:["update:modelValue"],setup(n,{emit:m}){const b=n,S=I(),o=g(()=>S.isZh),r=g({get:()=>b.modelValue,set:e=>m("update:modelValue",e)}),u=k(ce);function V(){if(!u.value.trim()){_.warning(o.value?"画像内容为空":"Profile content is empty");return}_.info(o.value?"语法在保存时由后端校验（YAML）":"Syntax is validated on save (YAML)")}const l=k("default"),h=k(["default"]);async function y(e){var t;if(e!==!1)try{const a=await A.listProfiles();h.value=(((t=a==null?void 0:a.data)==null?void 0:t.data)||[]).map(E=>E.name)}catch{}}async function w(){var e;try{const t=await A.previewProfile(l.value);((e=t==null?void 0:t.data)==null?void 0:e.status)===200&&typeof t.data.data=="string"&&(u.value=t.data.data,_.success(o.value?`已加载画像 ${l.value}`:`Loaded profile ${l.value}`))}catch{_.error(o.value?"加载画像失败":"Failed to load profile")}}async function i(){var e,t;if(!u.value.trim()){_.warning(o.value?"画像内容为空":"Profile content is empty");return}try{const a=await A.saveProfile(l.value,u.value);((e=a==null?void 0:a.data)==null?void 0:e.status)===200?_.success(o.value?`画像 ${l.value} 已保存（3 秒内热加载）`:`Profile ${l.value} saved (hot-reload in 3s)`):_.error(((t=a==null?void 0:a.data)==null?void 0:t.message)||"保存失败")}catch(a){_.error(o.value?"保存失败: "+((a==null?void 0:a.message)||""):"Save failed: "+((a==null?void 0:a.message)||""))}}function d(){r.value=!1}return(e,t)=>{const a=Q,E=X,M=ee;return f(),P(M,{modelValue:r.value,"onUpdate:modelValue":t[2]||(t[2]=c=>r.value=c),title:o.value?"C2 扩展配置画像编辑器":"Malleable C2 Profile Editor",width:"720px","close-on-click-modal":!1,"append-to-body":"",class:"cs-dialog"},{footer:L(()=>[s("div",re,[s("button",{class:"cs-btn cs-btn-primary",onClick:i},v(o.value?"保存画像":"Save Profile"),1),s("button",{class:"cs-btn",onClick:d},v(o.value?"关闭":"Close"),1)])]),default:L(()=>[s("div",ae,[s("div",oe,[s("span",se,v(o.value?"画像: ":"Profile: "),1),$(E,{modelValue:l.value,"onUpdate:modelValue":t[0]||(t[0]=c=>l.value=c),size:"small",style:{width:"160px"},onVisibleChange:y},{default:L(()=>[(f(!0),C(z,null,O(h.value,c=>(f(),P(a,{key:c,label:c,value:c},null,8,["label","value"]))),128))]),_:1},8,["modelValue"]),le,s("button",{class:"cs-btn",onClick:w},v(o.value?"加载":"Load"),1),s("button",{class:"cs-btn",onClick:V},v(o.value?"语法校验":"Lint / Verify"),1)]),s("div",ie,[N(s("textarea",{"onUpdate:modelValue":t[1]||(t[1]=c=>u.value=c),class:"cs-profile-textarea cs-font-mono",spellcheck:"false",placeholder:o.value?"# Tshell C2 扩展通信画像配置...":"# C2 profile definition..."},null,8,ne),[[U,u.value]])])])]),_:1},8,["modelValue","title"])}}}),de=x(ue,[["__scopeId","data-v-755f52a4"]]),pe={class:"st-page"},_e={class:"st-tabs"},fe=["onClick"],ve={class:"st-body"},me={key:0,class:"st-subbar"},he=R({name:"Settings",__name:"SettingsView",setup(n){const m=H(),b=W(),S=I(),o=g(()=>S.isZh),r=g(()=>{const i=String(m.params.tab||"config");return B.includes(i)?i:"config"}),u=[{key:"config",zh:"服务配置",en:"Config"},{key:"status",zh:"服务器状态",en:"Status"},{key:"users",zh:"用户管理",en:"Users"},{key:"keys",zh:"密钥管理",en:"Keys"},{key:"security",zh:"安全中心",en:"Security"},{key:"hidden",zh:"隐蔽访问",en:"Hidden Access"},{key:"logs",zh:"日志",en:"Logs"},{key:"pool",zh:"连接池",en:"Connections"}],V={config:()=>p(()=>import("./ServerConfigView.BvoufHd0.js"),__vite__mapDeps([0,1,2,3,4,5,6,7,8])),status:()=>p(()=>import("./ServerStatusView.BvN349Hf.js"),__vite__mapDeps([9,1,2,3,4,5,10,11])),users:()=>p(()=>import("./UserManagementView.BNF7bKuv.js"),__vite__mapDeps([12,1,2,3,4,5,13,6,7,14,15])),keys:()=>p(()=>import("./KeyManagementView.CFIFcA6s.js"),__vite__mapDeps([16,1,2,3,4,5,17])),security:()=>p(()=>import("./SecurityCenterView.DFu1Ut_0.js"),__vite__mapDeps([18,1,2,3,4,5,19,20])),hidden:()=>p(()=>import("./HiddenAccessView.BA5b5KR5.js"),__vite__mapDeps([21,1,2,3,4,5,19,22])),logs:()=>p(()=>import("./LogsView.Dr8bQF3n.js"),__vite__mapDeps([23,1,2,3,4,5,24])),pool:()=>p(()=>import("./ConnectionPoolView.Dv6cHUPa.js"),__vite__mapDeps([25,1,2,3,4,5,26,15]))},l=Z({}),h=g(()=>l[r.value]??null);j(r,async i=>{if(l[i])return;const d=await V[i]();l[i]=J(d.default)},{immediate:!0});const y=k(!1);function w(i){b.replace(`/setting/${i}`)}return(i,d)=>(f(),C("div",pe,[s("div",_e,[(f(),C(z,null,O(u,e=>s("div",{key:e.key,class:q(["st-tab",{active:e.key===r.value}]),onClick:t=>w(e.key)},v(o.value?e.zh:e.en),11,fe)),64))]),s("div",ve,[r.value==="config"?(f(),C("div",me,[s("button",{class:"cs-btn",onClick:d[0]||(d[0]=e=>y.value=!0)},v(o.value?"C2 扩展配置":"C2 Profile"),1)])):D("",!0),(f(),P(Y,null,[h.value?(f(),P(F(h.value),{key:r.value})):D("",!0)],1024)),$(de,{modelValue:y.value,"onUpdate:modelValue":d[1]||(d[1]=e=>y.value=e)},null,8,["modelValue"])])]))}}),be=x(he,[["__scopeId","data-v-391541c9"]]);export{be as default};

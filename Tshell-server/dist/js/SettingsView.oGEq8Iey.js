const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["js/ServerConfigView.CT9Mn0fP.js","js/index.CT4p0nPn.js","js/vue.CUqvj3p8.js","js/element.Be0ueEdW.js","js/utils.NIGUFBhG.js","css/index.Cs8VT6fr.css","js/EmptyState.BoyGbGoK.js","css/EmptyState.coVD2JeJ.css","css/ServerConfigView.DJnRvDk_.css","js/ServerStatusView.CIxAhpQ2.js","js/dashboard.Dvabyi_R.js","css/ServerStatusView.BfrTENxm.css","js/UserManagementView.BqG6Wh74.js","js/index.Cq_9BjY8.js","css/UserManagementView.DLJ6I36M.css","js/KeyManagementView.DUXrlO6W.js","css/KeyManagementView.DC8qqqn_.css","css/el-switch.BaA37b49.css","js/SecurityCenterView.DcXNK9Ch.js","js/index.DVSPI-mN.js","css/SecurityCenterView.C1zImBCC.css","js/HiddenAccessView.DrCPiIL_.js","css/HiddenAccessView.ExI7M2oG.css","js/LogsView.qjXtj2H5.js","css/LogsView.BG09T77P.css","js/ConnectionPoolView.DHyHstW6.js","css/ConnectionPoolView.MHWPXCqB.css"])))=>i.map(i=>d[i]);
import{s as T,u as I,_ as x,S as M,g as p}from"./index.CT4p0nPn.js";import{y as R,e as g,a as k,C,D as L,i as s,L as $,J as v,f as b,h as f,K as z,ac as O,E as N,ad as U,ao as K,ap as j,ak as G,aj as H,af as W,b as Z,G as D,H as F,al as Y,F as q,aq as J}from"./vue.CUqvj3p8.js";import{f as X,g as Q,b as ee,j as _}from"./element.Be0ueEdW.js";import"./utils.NIGUFBhG.js";const A={listProfiles(){return T({url:"/c2profile/list",method:"get"})},previewProfile(n){return T({url:"/c2profile/preview",method:"get",params:{name:n}})},saveProfile(n,m){return T({url:"/c2profile/save",method:"post",data:{name:n,content:m}})}},te=n=>(K("data-v-d0d66175"),n=n(),j(),n),oe={class:"cs-profile-container cs-font-ui"},ae={class:"profile-toolbar"},se={class:"profile-title"},le=te(()=>s("div",{class:"profile-spacer"},null,-1)),ie={class:"profile-editor-wrap"},ne=["placeholder"],re={class:"cs-dialog-footer"},ce=`# ============================================================
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
`,ue=R({__name:"C2ProfileDialog",props:{modelValue:{type:Boolean}},emits:["update:modelValue"],setup(n,{emit:m}){const P=n,S=I(),a=g(()=>S.isZh),r=g({get:()=>P.modelValue,set:e=>m("update:modelValue",e)}),u=k(ce);function V(){if(!u.value.trim()){_.warning(a.value?"画像内容为空":"Profile content is empty");return}_.info(a.value?"语法在保存时由后端校验（YAML）":"Syntax is validated on save (YAML)")}const l=k("default"),h=k(["default"]);async function y(e){var t;if(e!==!1)try{const o=await A.listProfiles();h.value=(((t=o==null?void 0:o.data)==null?void 0:t.data)||[]).map(E=>E.name)}catch{}}async function w(){var e;try{const t=await A.previewProfile(l.value);((e=t==null?void 0:t.data)==null?void 0:e.status)===200&&typeof t.data.data=="string"&&(u.value=t.data.data,_.success(a.value?`已加载画像 ${l.value}`:`Loaded profile ${l.value}`))}catch{_.error(a.value?"加载画像失败":"Failed to load profile")}}async function i(){var e,t;if(!u.value.trim()){_.warning(a.value?"画像内容为空":"Profile content is empty");return}try{const o=await A.saveProfile(l.value,u.value);((e=o==null?void 0:o.data)==null?void 0:e.status)===200?_.success(a.value?`画像 ${l.value} 已保存（3 秒内热加载）`:`Profile ${l.value} saved (hot-reload in 3s)`):_.error(((t=o==null?void 0:o.data)==null?void 0:t.message)||"保存失败")}catch(o){_.error(a.value?"保存失败: "+((o==null?void 0:o.message)||""):"Save failed: "+((o==null?void 0:o.message)||""))}}function d(){r.value=!1}return(e,t)=>{const o=Q,E=X,B=ee;return f(),C(B,{modelValue:r.value,"onUpdate:modelValue":t[2]||(t[2]=c=>r.value=c),title:a.value?"C2 扩展配置画像编辑器":"Malleable C2 Profile Editor",width:"720px","close-on-click-modal":!1,"append-to-body":"",class:"cs-dialog"},{footer:L(()=>[s("div",re,[s("button",{class:"cs-btn cs-btn-primary",onClick:i},v(a.value?"保存画像":"Save Profile"),1),s("button",{class:"cs-btn",onClick:d},v(a.value?"关闭":"Close"),1)])]),default:L(()=>[s("div",oe,[s("div",ae,[s("span",se,v(a.value?"画像: ":"Profile: "),1),$(E,{modelValue:l.value,"onUpdate:modelValue":t[0]||(t[0]=c=>l.value=c),size:"small",style:{width:"160px"},onVisibleChange:y},{default:L(()=>[(f(!0),b(z,null,O(h.value,c=>(f(),C(o,{key:c,label:c,value:c},null,8,["label","value"]))),128))]),_:1},8,["modelValue"]),le,s("button",{class:"cs-btn",onClick:w},v(a.value?"加载":"Load"),1),s("button",{class:"cs-btn",onClick:V},v(a.value?"语法校验":"Lint / Verify"),1)]),s("div",ie,[N(s("textarea",{"onUpdate:modelValue":t[1]||(t[1]=c=>u.value=c),class:"cs-profile-textarea cs-font-mono",spellcheck:"false",placeholder:a.value?"# BladeShell C2 扩展通信画像配置...":"# C2 profile definition..."},null,8,ne),[[U,u.value]])])])]),_:1},8,["modelValue","title"])}}}),de=x(ue,[["__scopeId","data-v-d0d66175"]]),pe={class:"st-page"},_e={class:"st-tabs"},fe=["onClick"],ve={class:"st-body"},me={key:0,class:"st-subbar"},he=R({name:"Settings",__name:"SettingsView",setup(n){const m=G(),P=H(),S=I(),a=g(()=>S.isZh),r=g(()=>{const i=String(m.params.tab||"config");return M.includes(i)?i:"config"}),u=[{key:"config",zh:"服务配置",en:"Config"},{key:"status",zh:"服务器状态",en:"Status"},{key:"users",zh:"用户管理",en:"Users"},{key:"keys",zh:"密钥管理",en:"Keys"},{key:"security",zh:"安全中心",en:"Security"},{key:"hidden",zh:"隐蔽访问",en:"Hidden Access"},{key:"logs",zh:"日志",en:"Logs"},{key:"pool",zh:"连接池",en:"Connections"}],V={config:()=>p(()=>import("./ServerConfigView.CT9Mn0fP.js"),__vite__mapDeps([0,1,2,3,4,5,6,7,8])),status:()=>p(()=>import("./ServerStatusView.CIxAhpQ2.js"),__vite__mapDeps([9,1,2,3,4,5,10,11])),users:()=>p(()=>import("./UserManagementView.BqG6Wh74.js"),__vite__mapDeps([12,1,2,3,4,5,13,6,7,14])),keys:()=>p(()=>import("./KeyManagementView.DUXrlO6W.js"),__vite__mapDeps([15,1,2,3,4,5,16,17])),security:()=>p(()=>import("./SecurityCenterView.DcXNK9Ch.js"),__vite__mapDeps([18,1,2,3,4,5,19,20])),hidden:()=>p(()=>import("./HiddenAccessView.DrCPiIL_.js"),__vite__mapDeps([21,1,2,3,4,5,19,22,17])),logs:()=>p(()=>import("./LogsView.qjXtj2H5.js"),__vite__mapDeps([23,1,2,3,4,5,24])),pool:()=>p(()=>import("./ConnectionPoolView.DHyHstW6.js"),__vite__mapDeps([25,1,2,3,4,5,26]))},l=W({}),h=g(()=>l[r.value]??null);Z(r,async i=>{if(l[i])return;const d=await V[i]();l[i]=J(d.default)},{immediate:!0});const y=k(!1);function w(i){P.replace(`/setting/${i}`)}return(i,d)=>(f(),b("div",pe,[s("div",_e,[(f(),b(z,null,O(u,e=>s("div",{key:e.key,class:q(["st-tab",{active:e.key===r.value}]),onClick:t=>w(e.key)},v(a.value?e.zh:e.en),11,fe)),64))]),s("div",ve,[r.value==="config"?(f(),b("div",me,[s("button",{class:"cs-btn",onClick:d[0]||(d[0]=e=>y.value=!0)},v(a.value?"C2 扩展配置":"C2 Profile"),1)])):D("",!0),(f(),C(Y,null,[h.value?(f(),C(F(h.value),{key:r.value})):D("",!0)],1024)),$(de,{modelValue:y.value,"onUpdate:modelValue":d[1]||(d[1]=e=>y.value=e)},null,8,["modelValue"])])]))}}),Ce=x(he,[["__scopeId","data-v-391541c9"]]);export{Ce as default};

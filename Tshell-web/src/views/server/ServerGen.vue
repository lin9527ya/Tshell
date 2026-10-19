<template>
  <div class="servergen-page cs-font-ui">
    <PageHeader
      :title="i18nStore.tl('客户端生成', 'Payloads')"
      :subtitle="i18nStore.tl('载荷生成器', 'Payload Generator')"
    >
      <span class="sg-tip">{{ i18nStore.tl('根据监听器自动注入 VKey 与加密盐', 'Automatic VKey & Salt injection based on Listener') }}</span>
      <!-- 高级格式：HTA / Office 宏 / Windows EXE（原全局对话框收编到页内，下拉右对齐） -->
      <el-dropdown trigger="click" placement="bottom-end" class="sg-dropdown" @command="openAdvDialog">
        <button type="button" class="cs-btn">
          {{ i18nStore.tl('高级格式', 'Advanced Formats') }}
          <span class="sg-caret">▾</span>
        </button>
        <template #dropdown>
          <el-dropdown-menu class="cs-menu-popper">
            <el-dropdown-item command="hta">{{ i18nStore.tl('HTA 攻击', 'HTML Application (HTA)') }}</el-dropdown-item>
            <el-dropdown-item command="macro">{{ i18nStore.tl('Office 宏', 'MS Office Macro') }}</el-dropdown-item>
            <el-dropdown-item command="exe">{{ i18nStore.tl('Windows EXE', 'Windows Executable') }}</el-dropdown-item>
            <el-dropdown-item command="exeStageless">{{ i18nStore.tl('Windows EXE 无阶段', 'Windows Executable (Stageless)') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </PageHeader>

    <!-- 页面级页签（vshell 骨架：页签在页面、内容卡片在下方） -->
    <div class="sg-tabs">
      <div
        v-for="t in TABS"
        :key="t.name"
        class="sg-tab"
        :class="{ active: activeTab === t.name }"
        @click="switchTab(t.name)"
      >
        {{ t.label }}
      </div>
    </div>

    <div class="gen-card">
      <div class="payload-generator">
        <!-- 当前页签的说明（页签本身在页面级，见 .sg-tabs） -->
        <div class="tab-content">
          <el-icon class="tab-icon"><component :is="activeTabDef.icon" /></el-icon>
          <span class="tab-description-large">{{ activeTabDef.desc }}</span>
        </div>


        <!-- 配置表单 -->
        <el-form :model="formData" label-width="140px" class="config-form">

          <!-- stageless -->
          <div v-if="activeTab === 'stageless'">
            <el-form-item :label="i18nStore.tl('监听器', 'Listener')">
              <el-select
                v-model="formData.listener"
                :placeholder="i18nStore.tl('选择监听器', 'Select a listener')"
                @visible-change="handleDropdown"
                class="listener-select"
              >
                <el-option
                  v-for="item in options"
                  :key="item"
                  :label="getListenerDetails(item).fullText"
                  :value="item"
                >
                  <div class="listener-option-enhanced">
                    <div class="listener-details-enhanced">{{ getListenerDetails(item).fullText }}</div>
                  </div>
                </el-option>
              </el-select>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('上线密码', 'Password')">
              <el-input
                v-model="formData.pass"
                :placeholder="i18nStore.tl('输入上线密码', 'Enter password')"
                class="password-input"
                show-password
              />
            </el-form-item>

            <el-form-item :label="i18nStore.tl('客户端类型', 'Client Type')">
              <div class="client-types-compact">
                <el-tag
                  v-for="client in clientTypes"
                  :key="client.value"
                  :type="formData.clientType === client.value ? 'primary' : 'info'"
                  :effect="formData.clientType === client.value ? 'dark' : 'plain'"
                  class="client-tag"
                  @click="selectClientType(client.value)"
                  size="large"
                >
                  {{ client.label }}
                </el-tag>
              </div>
            </el-form-item>
          </div>

          <!-- dll stageless -->
          <div v-else-if="activeTab === 'dll'">
            <el-alert
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 16px"
              :title="i18nStore.tl('DLL 无阶段载荷（仅 Windows）：rundll32.exe client.dll,Run 或 regsvr32.exe client.dll 加载，导出 Run / DllRegisterServer / DllInstall 三个入口', 'DLL stageless payload (Windows only): load with rundll32.exe client.dll,Run or regsvr32.exe client.dll; exports Run / DllRegisterServer / DllInstall')"
            />

            <el-form-item :label="i18nStore.tl('监听器', 'Listener')">
              <el-select
                v-model="formData.listener"
                :placeholder="i18nStore.tl('选择监听器', 'Select a listener')"
                @visible-change="handleDropdown"
                class="listener-select"
              >
                <el-option
                  v-for="item in options"
                  :key="item"
                  :label="getListenerDetails(item).fullText"
                  :value="item"
                >
                  <div class="listener-option-enhanced">
                    <div class="listener-details-enhanced">{{ getListenerDetails(item).fullText }}</div>
                  </div>
                </el-option>
              </el-select>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('上线密码', 'Password')">
              <el-input
                v-model="formData.pass"
                :placeholder="i18nStore.tl('输入上线密码', 'Enter password')"
                class="password-input"
                show-password
              />
            </el-form-item>

            <el-form-item :label="i18nStore.tl('客户端类型', 'Client Type')">
              <div class="client-types-compact">
                <el-tag
                  v-for="client in dllClientTypes"
                  :key="client.value"
                  :type="formData.clientType === client.value ? 'primary' : 'info'"
                  :effect="formData.clientType === client.value ? 'dark' : 'plain'"
                  class="client-tag"
                  @click="selectClientType(client.value)"
                  size="large"
                >
                  {{ client.label }}
                </el-tag>
              </div>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('加载方式', 'Load Method')">
              <div class="hint-text">
                <div>rundll32: <code>rundll32.exe client.dll,Run</code></div>
                <div>regsvr32: <code>regsvr32.exe client.dll</code></div>
                <div>regsvr32 /i: <code>regsvr32.exe /s /i client.dll</code></div>
              </div>
            </el-form-item>
          </div>

          <!-- shellcode：基于 WebDelivery -->
          <div v-else-if="activeTab === 'shellcode'">
            <el-alert
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 16px"
              :title="i18nStore.tl('请先在「WebDelivery」页面创建并开启托管，再选择对应 WebDelivery 生成 shellcode', 'Create and start a host on the WebDelivery page first, then pick it here to build shellcode')"
            />

            <el-form-item label="WebDelivery">
              <el-select
                v-model="formData.webDeliveryPort"
                :placeholder="i18nStore.tl('选择 WebDelivery（按端口）', 'Select a WebDelivery (by port)')"
                @visible-change="handleWebDeliveryDropdown"
                class="listener-select"
              >
                <el-option
                  v-for="item in webDeliveryOptions"
                  :key="item.ListeningPort"
                  :label="formatWebDeliveryLabel(item)"
                  :value="item.ListeningPort"
                />
              </el-select>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('目标信息', 'Target Info')" v-if="selectedWebDelivery">
              <el-tag type="success">{{ selectedWebDelivery.OS }}_{{ selectedWebDelivery.Arch }}</el-tag>
              <el-tag style="margin-left: 8px">{{ selectedWebDelivery.ListenerConfig }}</el-tag>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('格式', 'Format')">
              <div class="format-options">
                <el-tag
                  v-for="format in shellcodeFormats"
                  :key="format.value"
                  :type="formData.shellcodeFormat === format.value ? 'primary' : 'info'"
                  :effect="formData.shellcodeFormat === format.value ? 'dark' : 'plain'"
                  class="format-tag"
                  @click="formData.shellcodeFormat = format.value"
                  size="large"
                >
                  {{ format.label }}
                </el-tag>
              </div>
            </el-form-item>
          </div>

          <!-- stage：基于 WebDelivery 的动态 stager -->
          <div v-else-if="activeTab === 'stage'">
            <el-alert
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 16px"
              :title="i18nStore.tl('请先在「WebDelivery」页面创建并开启托管。动态 Stager 需要服务端安装 Go 工具链', 'Create and start a host on the WebDelivery page first. Dynamic Stager needs Go installed on the server')"
            />

            <el-form-item label="WebDelivery">
              <el-select
                v-model="formData.webDeliveryPort"
                :placeholder="i18nStore.tl('选择 WebDelivery（按端口）', 'Select a WebDelivery (by port)')"
                @visible-change="handleWebDeliveryDropdown"
                class="listener-select"
              >
                <el-option
                  v-for="item in webDeliveryOptions"
                  :key="item.ListeningPort"
                  :label="formatWebDeliveryLabel(item)"
                  :value="item.ListeningPort"
                />
              </el-select>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('目标信息', 'Target Info')" v-if="selectedWebDelivery">
              <el-tag type="success">{{ selectedWebDelivery.OS }}_{{ selectedWebDelivery.Arch }}</el-tag>
              <el-tag style="margin-left: 8px">{{ selectedWebDelivery.ListenerConfig }}</el-tag>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('构建信息', 'Build Info')" v-if="selectedWebDelivery">
              <div class="stager-info-box">
                <div v-if="stagerInfoLoading" class="si-loading">{{ i18nStore.tl('加载中...', 'Loading...') }}</div>
                <template v-else-if="stagerInfo">
                  <div><b>URL:</b> {{ stagerInfo.server_url || '-' }}</div>
                  <div><b>{{ i18nStore.tl('最后构建', 'Last Build') }}:</b> {{ stagerInfo.last_build || '-' }}</div>
                  <div><b>{{ i18nStore.tl('构建次数', 'Build Count') }}:</b> {{ stagerInfo.build_count || 0 }}</div>
                </template>
                <div v-else class="si-empty">{{ i18nStore.tl('点击查询获取该投递的 Stager 构建统计', 'Query build statistics for this delivery') }}</div>
              </div>
              <el-button size="small" :loading="stagerInfoLoading" @click="fetchStagerBuildInfo">
                {{ i18nStore.tl('查询构建信息', 'Query Build Info') }}
              </el-button>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('输出格式', 'Output Format')">
              <div class="format-options">
                <el-tag
                  v-for="format in stageFormats"
                  :key="format.value"
                  :type="formData.stageFormat === format.value ? 'primary' : 'info'"
                  :effect="formData.stageFormat === format.value ? 'dark' : 'plain'"
                  class="format-tag"
                  :disabled="!isStageFormatAllowed(format.value)"
                  @click="isStageFormatAllowed(format.value) && (formData.stageFormat = format.value)"
                  size="large"
                >
                  {{ format.label }}
                </el-tag>
              </div>
              <div class="hint-text" v-if="selectedWebDelivery">
                {{ selectedWebDelivery.OS === 'linux' ? (i18nStore.tl('Linux 推荐 elf / exe（可执行文件）', 'Linux recommends elf / exe')) : (i18nStore.tl('Windows 支持 exe / bin / hex / c', 'Windows supports exe / bin / hex / c')) }}
              </div>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('反调试', 'Anti-Debug')">
              <el-switch v-model="formData.antiDebug" />
            </el-form-item>
            <el-form-item :label="i18nStore.tl('代码混淆', 'Obfuscation')">
              <el-switch v-model="formData.obfuscation" />
            </el-form-item>
            <el-form-item label="User-Agent">
              <el-input
                v-model="formData.userAgent"
                :placeholder="i18nStore.tl('留空使用随机 UA', 'Empty = random UA')"
                class="password-input"
              />
            </el-form-item>
          </div>

          <!-- listen 正向客户端 -->
          <div v-else-if="activeTab === 'listen'">
            <el-form-item :label="i18nStore.tl('协议', 'Protocol')">
              <el-radio-group v-model="formData.listenProtocol">
                <el-radio label="tcp_forward">TCP</el-radio>
                <el-radio label="kcp_forward">KCP</el-radio>
                <el-radio label="ws_forward">WebSocket</el-radio>
                <el-radio label="http_forward">HTTP</el-radio>
                <el-radio label="https_forward">HTTPS</el-radio>
              </el-radio-group>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('客户端类型', 'Client Type')">
              <div class="client-types-compact">
                <el-tag
                  v-for="client in listenClientTypes"
                  :key="client.value"
                  :type="formData.clientType === client.value ? 'primary' : 'info'"
                  :effect="formData.clientType === client.value ? 'dark' : 'plain'"
                  class="client-tag"
                  @click="selectClientType(client.value)"
                  size="large"
                >
                  {{ client.label }}
                </el-tag>
              </div>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('监听地址', 'Listen Address')">
              <el-input v-model="formData.listenAddress" placeholder="0.0.0.0" class="address-input">
                <template #append>
                  <el-button :icon="Refresh" @click="refreshAddress" />
                </template>
              </el-input>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('监听端口', 'Listen Port')">
              <el-input v-model="formData.listenPort" placeholder="80" class="port-input" type="number" />
            </el-form-item>

            <el-form-item :label="i18nStore.tl('上线密码', 'Password')">
              <el-input v-model="formData.pass" :placeholder="i18nStore.tl('输入上线密码', 'Enter password')" class="password-input" show-password />
            </el-form-item>

            <el-form-item label="Vkey">
              <el-input v-model="formData.vkey" :placeholder="i18nStore.tl('留空使用默认值', 'Empty = default')" class="vkey-input">
                <template #append>
                  <el-button :icon="RefreshRight" @click="generateVkey" />
                </template>
              </el-input>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('流量加密盐', 'Traffic Salt')">
              <el-input v-model="formData.encryptSalt" :placeholder="i18nStore.tl('留空使用默认值', 'Empty = default')" class="salt-input">
                <template #append>
                  <el-button :icon="RefreshRight" @click="generateSalt" />
                </template>
              </el-input>
            </el-form-item>
          </div>

          <!-- dll listen 正向 DLL 客户端 -->
          <div v-else-if="activeTab === 'dlllisten'">
            <el-alert
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 16px"
              :title="i18nStore.tl('DLL 正向客户端（仅 Windows）：rundll32.exe client.dll,Run 或 regsvr32.exe client.dll 加载，导出 Run / DllRegisterServer / DllInstall 三个入口', 'DLL forward client (Windows only): load with rundll32.exe client.dll,Run or regsvr32.exe client.dll; exports Run / DllRegisterServer / DllInstall')"
            />

            <el-form-item :label="i18nStore.tl('协议', 'Protocol')">
              <el-radio-group v-model="formData.listenProtocol">
                <el-radio label="tcp_forward">TCP</el-radio>
                <el-radio label="kcp_forward">KCP</el-radio>
                <el-radio label="ws_forward">WebSocket</el-radio>
                <el-radio label="http_forward">HTTP</el-radio>
                <el-radio label="https_forward">HTTPS</el-radio>
              </el-radio-group>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('客户端类型', 'Client Type')">
              <div class="client-types-compact">
                <el-tag
                  v-for="client in dllClientTypes"
                  :key="client.value"
                  :type="formData.clientType === client.value ? 'primary' : 'info'"
                  :effect="formData.clientType === client.value ? 'dark' : 'plain'"
                  class="client-tag"
                  @click="selectClientType(client.value)"
                  size="large"
                >
                  {{ client.label }}
                </el-tag>
              </div>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('监听地址', 'Listen Address')">
              <el-input v-model="formData.listenAddress" placeholder="0.0.0.0" class="address-input">
                <template #append>
                  <el-button :icon="Refresh" @click="refreshAddress" />
                </template>
              </el-input>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('监听端口', 'Listen Port')">
              <el-input v-model="formData.listenPort" placeholder="80" class="port-input" type="number" />
            </el-form-item>

            <el-form-item :label="i18nStore.tl('上线密码', 'Password')">
              <el-input v-model="formData.pass" :placeholder="i18nStore.tl('输入上线密码', 'Enter password')" class="password-input" show-password />
            </el-form-item>

            <el-form-item label="Vkey">
              <el-input v-model="formData.vkey" :placeholder="i18nStore.tl('留空使用默认值', 'Empty = default')" class="vkey-input">
                <template #append>
                  <el-button :icon="RefreshRight" @click="generateVkey" />
                </template>
              </el-input>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('流量加密盐', 'Traffic Salt')">
              <el-input v-model="formData.encryptSalt" :placeholder="i18nStore.tl('留空使用默认值', 'Empty = default')" class="salt-input">
                <template #append>
                  <el-button :icon="RefreshRight" @click="generateSalt" />
                </template>
              </el-input>
            </el-form-item>

            <el-form-item :label="i18nStore.tl('加载方式', 'Load Method')">
              <div class="hint-text">
                <div>rundll32: <code>rundll32.exe client.dll,Run</code></div>
                <div>regsvr32: <code>regsvr32.exe client.dll</code></div>
                <div>regsvr32 /i: <code>regsvr32.exe /s /i client.dll</code></div>
              </div>
            </el-form-item>
          </div>
        </el-form>

        <!-- 投递（合并原 站点管理 / 站点克隆 / 文件托管 / 投递日志） -->
        <div v-if="activeTab === 'delivery'" class="delivery-pane">
          <WebDelivery />
        </div>

        <!-- 表单页脚操作（vshell：表单卡内左对齐） -->
        <div v-if="activeTab !== 'delivery'" class="sg-bottom-bar">
          <button
            class="cs-btn cs-btn-primary"
            :disabled="!isFormValid || generating"
            @click="handleGenerate"
          >
            {{ generating ? (i18nStore.tl('生成中...', 'Generating...')) : (i18nStore.tl('生成载荷', 'Generate Payload')) }}
          </button>
          <button class="cs-btn" @click="ElMessage.info(i18nStore.tl('载荷生成器说明', 'Payload Generator Manual'))">
            {{ i18nStore.tl('帮助', 'Help') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 高级格式对话框（页内常驻局部 ref；原全局 store 挂载已移除） -->
    <HtaAttackDialog v-model="htaVisible" />
    <OfficeMacroDialog v-model="macroVisible" />
    <WindowsExeDialog v-model="exeVisible" />
    <WindowsExeStagelessDialog v-model="exeStagelessVisible" :key="exeStagelessKey" />
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, nextTick, markRaw } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18nStore } from '@/stores/i18n';
import {
  Document,
  DocumentAdd,
  Files,
  Connection,
  Download,
  Link,
  Refresh,
  RefreshRight,
} from '@element-plus/icons-vue';
import { defineAsyncComponent } from 'vue';
const WebDelivery = defineAsyncComponent(() => import('@/views/server/WebDelivery.vue'));
import ClientAPI from '@/api/clients';
import PageHeader from '@/components/Common/PageHeader.vue';
import HtaAttackDialog from '@/layout/Dialogs/Payloads/HtaAttackDialog.vue';
import OfficeMacroDialog from '@/layout/Dialogs/Payloads/OfficeMacroDialog.vue';
import WindowsExeDialog from '@/layout/Dialogs/Payloads/WindowsExeDialog.vue';
import WindowsExeStagelessDialog from '@/layout/Dialogs/Payloads/WindowsExeStagelessDialog.vue';

const i18nStore = useI18nStore();
const isZh = computed(() => i18nStore.isZh);

const options = ref<string[]>([]);
const listenerDetails = ref<any[]>([]);
const webDeliveryOptions = ref<any[]>([]);
const activeTab = ref('stageless');

// 页签定义（图标 markRaw：组件对象不该被做成响应式代理）
const TABS = computed(() => [
  { name: 'stage', label: i18nStore.tl('分阶段(stage)', 'stage'), icon: markRaw(Document), desc: i18nStore.tl('分阶段载荷：依赖已配置的 WebDelivery，体积小，运行时二次拉取完整客户端', 'Staged payload: needs a configured WebDelivery, small size, fetches the full client at run time') },
  { name: 'shellcode', label: i18nStore.tl('Shellcode', 'shellcode'), icon: markRaw(DocumentAdd), desc: i18nStore.tl('Stage Shellcode：基于 WebDelivery 生成可注入 shellcode（Windows）', 'Stage Shellcode: injectable shellcode built from a WebDelivery (Windows)') },
  { name: 'stageless', label: i18nStore.tl('无阶段(stageless)', 'stageless'), icon: markRaw(Files), desc: i18nStore.tl('无阶段载荷：完整功能客户端，直接上线', 'Stageless payload: full-featured client, connects directly') },
  { name: 'dll', label: i18nStore.tl('DLL 无阶段', 'dll stageless'), icon: markRaw(Files), desc: i18nStore.tl('DLL 无阶段载荷：rundll32 / regsvr32 加载（仅 Windows）', 'DLL stageless payload: loaded via rundll32 / regsvr32 (Windows only)') },
  { name: 'listen', label: i18nStore.tl('正向监听(listen)', 'listen'), icon: markRaw(Connection), desc: i18nStore.tl('正向客户端：监听模式，由服务端主动连出上线', 'Forward client: listen mode, the server dials out to it') },
  { name: 'dlllisten', label: i18nStore.tl('DLL 正向监听', 'dll listen'), icon: markRaw(Connection), desc: i18nStore.tl('DLL 正向客户端：rundll32 / regsvr32 加载（仅 Windows）', 'DLL forward client: loaded via rundll32 / regsvr32 (Windows only)') },
  { name: 'delivery', label: i18nStore.tl('投递', 'delivery'), icon: markRaw(Link), desc: i18nStore.tl('Web 投递：Stager 托管 / 站点克隆 / 文件托管 / 投递日志', 'Web delivery: stager hosting / site clone / file hosting / delivery logs') },
])

const activeTabDef = computed(() => TABS.value.find((t) => t.name === activeTab.value) || TABS.value[0])

function switchTab(name: string) {
  if (activeTab.value === name) return
  activeTab.value = name
  handleTabChange(name)
}
const generating = ref(false);

// 「高级格式 ▾」对话框（原全局 store 挂载 → 页内局部 ref）
const htaVisible = ref(false);
const macroVisible = ref(false);
const exeVisible = ref(false);
const exeStagelessVisible = ref(false);
// 无阶段 EXE 弹窗自身没有「打开时初始化」逻辑：打开前换 key 强制重挂，避免上次的监听器/OPSEC 残留
const exeStagelessKey = ref(0);

const openAdvDialog = async (cmd: string) => {
  if (cmd === 'hta') {
    htaVisible.value = true;
  } else if (cmd === 'macro') {
    macroVisible.value = true;
  } else if (cmd === 'exe') {
    exeVisible.value = true;
  } else if (cmd === 'exeStageless') {
    // 先重挂空实例，再打开（保留 el-dialog 的打开动画）
    exeStagelessKey.value += 1;
    await nextTick();
    exeStagelessVisible.value = true;
  }
};

const formData = reactive({
  listener: '',
  clientType: '',
  pass: '',
  payloadType: 'stageless',
  shellcodeFormat: 'bin',
  stageFormat: 'exe',
  listenProtocol: 'tcp_forward',
  webDeliveryPort: '',
  listenAddress: '0.0.0.0',
  listenPort: '80',
  vkey: '',
  encryptSalt: '',
  antiDebug: false,
  obfuscation: false,
  userAgent: '',
});

const DEFAULT_VKEY = 'HlTKzBWsfWTEFaFkewvNeFaFK6R6sY6Q';
const DEFAULT_ENCRYPT_SALT = 'f8uulfgNYpnpAhwd';

const clientTypes = [
  { value: 'windows_amd64', label: 'windows_amd64', os: 'windows', arch: 'amd64' },
  { value: 'windows_i386', label: 'windows_i386', os: 'windows', arch: '386' },
  { value: 'linux_amd64', label: 'linux_amd64', os: 'linux', arch: 'amd64' },
  { value: 'linux_386', label: 'linux_386', os: 'linux', arch: '386' },
  { value: 'linux_arm64', label: 'linux_arm64', os: 'linux', arch: 'arm64' },
  { value: 'linux_arm', label: 'linux_arm', os: 'linux', arch: 'arm' },
  { value: 'darwin_amd64', label: 'darwin_amd64', os: 'darwin', arch: 'amd64' },
  { value: 'darwin_arm64', label: 'darwin_arm64', os: 'darwin', arch: 'arm64' },
];

const listenClientTypes = [
  { value: 'windows_amd64', label: 'windows_amd64', os: 'windows', arch: 'amd64' },
  { value: 'windows_i386', label: 'windows_i386', os: 'windows', arch: '386' },
  { value: 'linux_amd64', label: 'linux_amd64', os: 'linux', arch: 'amd64' },
  { value: 'linux_386', label: 'linux_386', os: 'linux', arch: '386' },
  { value: 'linux_arm64', label: 'linux_arm64', os: 'linux', arch: 'arm64' },
  { value: 'linux_arm', label: 'linux_arm', os: 'linux', arch: 'arm' },
];

// dll stageless 仅支持 windows 目标
const dllClientTypes = [
  { value: 'windows_amd64', label: 'windows_amd64', os: 'windows', arch: 'amd64' },
  { value: 'windows_i386', label: 'windows_i386', os: 'windows', arch: '386' },
];

// 与后端 StageShellCodeGen / DynamicStageShellCodeGen 对齐：exe/bin/hex/c/raw
const shellcodeFormats = [
  { value: 'exe', label: 'exe' },
  { value: 'raw', label: 'raw' },
  { value: 'bin', label: 'bin' },
  { value: 'hex', label: 'hex' },
  { value: 'c', label: 'c' },
];

const stageFormats = [
  { value: 'exe', label: 'exe' },
  { value: 'elf', label: 'elf' },
  { value: 'raw', label: 'raw' },
  { value: 'bin', label: 'bin' },
  { value: 'hex', label: 'hex' },
  { value: 'c', label: 'c' },
];

const selectedWebDelivery = computed(() => {
  if (!formData.webDeliveryPort) return null;
  return webDeliveryOptions.value.find((w: any) => w.ListeningPort === formData.webDeliveryPort) || null;
});

// Stager 构建信息（/shellcode/stager-info）
const stagerInfo = ref<any>(null);
const stagerInfoLoading = ref(false);

const fetchStagerBuildInfo = async () => {
  const wd = selectedWebDelivery.value;
  if (!wd) {
    ElMessage.warning(isZh.value ? '请先选择 WebDelivery' : 'Select a WebDelivery first');
    return;
  }
  stagerInfoLoading.value = true;
  try {
    const res = await ClientAPI.GetStagerBuildInfo(wd.ListeningPort);
    if (res.data?.status === 200) {
      stagerInfo.value = res.data.data || null;
    } else {
      stagerInfo.value = null;
      ElMessage.warning(res.data?.data || (isZh.value ? '暂无构建信息' : 'No build info available'));
    }
  } catch (e: any) {
    stagerInfo.value = null;
    ElMessage.error(e?.message || (isZh.value ? '获取构建信息失败' : 'Failed to load build info'));
  } finally {
    stagerInfoLoading.value = false;
  }
};

const isFormValid = computed(() => {
  if (activeTab.value === 'dll') {
    return !!(formData.clientType && formData.listener);
  }
  if (activeTab.value === 'listen' || activeTab.value === 'dlllisten') {
    return !!(formData.clientType && formData.listenAddress && formData.listenPort && formData.pass);
  }
  if (activeTab.value === 'shellcode') {
    return !!(formData.webDeliveryPort && formData.shellcodeFormat);
  }
  if (activeTab.value === 'stage') {
    return !!(formData.webDeliveryPort && formData.stageFormat && isStageFormatAllowed(formData.stageFormat));
  }
  // stageless
  return !!(formData.clientType && formData.listener);
});

const selectClientType = (clientType: string) => {
  formData.clientType = clientType;
};

const formatWebDeliveryLabel = (item: any) => {
  const status = item.Status === 1 ? (isZh.value ? '运行中' : 'running') : (isZh.value ? '已关闭' : 'stopped');
  return isZh.value
    ? `端口 ${item.ListeningPort} | ${item.OS}_${item.Arch} | ${item.ListenerConfig || '-'} | ${status}`
    : `Port ${item.ListeningPort} | ${item.OS}_${item.Arch} | ${item.ListenerConfig || '-'} | ${status}`;
};

const isStageFormatAllowed = (fmt: string) => {
  const wd = selectedWebDelivery.value;
  if (!wd) return true;
  if (wd.OS === 'linux') {
    return fmt === 'elf' || fmt === 'exe' || fmt === 'raw';
  }
  // windows
  return fmt === 'exe' || fmt === 'raw' || fmt === 'bin' || fmt === 'hex' || fmt === 'c';
};

const handleDropdown = async (visible?: boolean) => {
  if (visible === false) return;
  try {
    const listenerAPI = await import('@/api/listener');
    const fullListRes = await listenerAPI.default.getListenerList();
    if (fullListRes.status === 200 && fullListRes.data?.status === 200 && Array.isArray(fullListRes.data.data)) {
      // 仅保留运行中的监听器 (Status === 1)
      const runningListeners = fullListRes.data.data.filter((l: any) => Number(l.Status) === 1);
      listenerDetails.value = runningListeners.map((l: any) => ({
        id: l.Id,
        protocol: l.Type,
        listenAddress: l.ListenAddress,
        connectAddress: l.ConnectAddress,
        url: `${l.Type}://${l.ConnectAddress}`,
        vkey: l.VKey,
        encryptSalt: l.EncryptionSalt,
        status: l.Status
      }));
      options.value = listenerDetails.value.map((item: any) => item.url);
      if (!options.value.length) {
        ElMessage.warning(isZh.value ? '暂无运行中的监听器，请先启动监听器' : 'No running listener, start one first');
      }
      return;
    }
  } catch (error: any) {
    /* fallback */
  }

  try {
    const detailRes = await ClientAPI.ShowListenerDetails();
    if (detailRes.status === 200 && detailRes.data.status === 200) {
      const running = (detailRes.data.data || []).filter((item: any) => item.status === undefined || Number(item.status) === 1 || Number(item.Status) === 1);
      listenerDetails.value = running;
      options.value = running.map((item: any) => {
        return item.url || `${item.protocol}://${item.connectAddress}`;
      });
      if (!options.value.length) {
        ElMessage.warning(isZh.value ? '暂无运行中的监听器，请先启动监听器' : 'No running listener, start one first');
      }
      return;
    }
  } catch (error: any) {
    if (!(error.response && error.response.status === 404)) {
      console.log('无法获取详细信息，使用原有API:', error?.message);
    }
  }

  const res = await ClientAPI.ShowListener();
  if (res.status === 200) {
    if (res.data.status === 200) {
      options.value = res.data.data;
      listenerDetails.value = [];
    } else {
      ElMessage.error(res.data.data);
    }
  } else {
    ElMessage.error(isZh.value ? '获取监听器列表失败' : 'Failed to load listener list');
  }
};

const handleWebDeliveryDropdown = async (visible?: boolean) => {
  if (visible === false) return;
  try {
    const res = await ClientAPI.GetWebDeliveryList();
    if (res.status === 200 && res.data.status === 200) {
      webDeliveryOptions.value = res.data.data || [];
      if (!webDeliveryOptions.value.length) {
        ElMessage.warning(isZh.value ? '暂无 WebDelivery，请先到 WebDelivery 页面创建' : 'No WebDelivery yet, create one on the WebDelivery page first');
      }
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '获取 WebDelivery 列表失败' : 'Failed to load WebDelivery list'));
    }
  } catch (e: any) {
    ElMessage.error(isZh.value ? `获取 WebDelivery 失败: ${e?.message || e}` : `Failed to load WebDelivery: ${e?.message || e}`);
  }
};

const getListenerDetails = (listener: string) => {
  if (listenerDetails.value && listenerDetails.value.length > 0) {
    const detail = listenerDetails.value.find((item: any) => {
      const itemUrl = item.url || `${item.protocol}://${item.connectAddress}`;
      if (itemUrl === listener) return true;
      const listenerParts = listener.split('://');
      if (listenerParts.length === 2) {
        const [protocol, address] = listenerParts;
        return (
          item.protocol &&
          item.connectAddress &&
          item.protocol.toLowerCase() === protocol.toLowerCase() &&
          item.connectAddress === address
        );
      }
      return false;
    }) as any;

    if (detail && (detail.id != null || detail.ID != null)) {
      const parts = listener.split('://');
      if (parts.length === 2) {
        const protocol = parts[0].toUpperCase();
        const listenAddress = detail.listenAddress || `0.0.0.0:${parts[1].split(':')[1] || '8080'}`;
        const connectAddress = detail.connectAddress || parts[1];
        const actualId = detail.ID || detail.id || detail.Id;
        return {
          id: actualId.toString(),
          protocol,
          listenAddress,
          connectAddress,
          fullText: isZh.value
            ? `ID：${actualId} ${protocol} 监听地址: ${listenAddress} 连接地址: ${connectAddress}`
            : `ID: ${actualId} ${protocol} listen ${listenAddress} connect ${connectAddress}`,
        };
      }
    }
  }

  const parts = listener.split('://');
  if (parts.length === 2) {
    const protocol = parts[0].toUpperCase();
    const address = parts[1];
    return {
      id: 'N/A',
      protocol,
      listenAddress: address,
      connectAddress: address,
      fullText: isZh.value ? `${protocol} 连接地址: ${address}` : `${protocol} connect ${address}`,
    };
  }
  return {
    id: 'N/A',
    protocol: 'UNKNOWN',
    listenAddress: listener,
    connectAddress: listener,
    fullText: listener,
  };
};

/** 解析 blob 错误响应中的 JSON 错误信息 */
const extractBlobError = async (error: any): Promise<string> => {
  try {
    const data = error?.response?.data;
    if (data instanceof Blob) {
      const text = await data.text();
      try {
        const json = JSON.parse(text);
        return json.data || json.error || json.message || text;
      } catch {
        return text || error.message;
      }
    }
    if (typeof data === 'object' && data) {
      return data.data || data.error || data.message || error.message;
    }
    return error?.message || String(error);
  } catch {
    return error?.message || String(error);
  }
};

const downloadBlob = (res: any, fallbackName: string) => {
  let fileName = fallbackName;
  const contentDisposition = res.headers?.['content-disposition'] || '';
  const matches = /filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/.exec(contentDisposition);
  if (matches && matches.length > 1) {
    fileName = decodeURIComponent(matches[1].replace(/['"]/g, ''));
  }

  // 若后端错误返回 JSON blob（content-type json），不要当文件下载
  const ctype = (res.headers?.['content-type'] || '').toLowerCase();
  if (ctype.includes('application/json') || ctype.includes('text/json')) {
    return false;
  }

  const blob = new Blob([res.data], { type: res.headers?.['content-type'] || 'application/octet-stream' });
  const downloadElement = document.createElement('a');
  const href = window.URL.createObjectURL(blob);
  downloadElement.href = href;
  downloadElement.download = fileName;
  document.body.appendChild(downloadElement);
  downloadElement.click();
  document.body.removeChild(downloadElement);
  window.URL.revokeObjectURL(href);
  return true;
};

const handleGenerate = async () => {
  if (!isFormValid.value) {
    ElMessage.warning(isZh.value ? '请完整填写必填项' : 'Fill in all required fields');
    return;
  }

  generating.value = true;
  try {
    let res: any;
    let fileName = `${activeTab.value}_${Date.now()}`;

    if (activeTab.value === 'stageless' || activeTab.value === 'listen' || activeTab.value === 'dlllisten') {
      const isListen = activeTab.value === 'listen' || activeTab.value === 'dlllisten';
      let currentClientTypes = activeTab.value === 'dlllisten' ? dllClientTypes : isListen ? listenClientTypes : clientTypes;
      const selectedClient = currentClientTypes.find((c) => c.value === formData.clientType);
      if (!selectedClient) {
        ElMessage.error(isZh.value ? '请选择客户端类型' : 'Select a client type');
        return;
      }

      let finalVkey = formData.vkey || DEFAULT_VKEY;
      let finalEncryptSalt = formData.encryptSalt || DEFAULT_ENCRYPT_SALT;

      if (formData.listener && listenerDetails.value.length > 0) {
        const selectedListenerDetail = listenerDetails.value.find((detail: any) => {
          const itemUrl = detail.url || `${detail.protocol}://${detail.connectAddress}`;
          return itemUrl === formData.listener;
        }) as any;
        if (selectedListenerDetail) {
          try {
            const listenerAPI = await import('@/api/listener');
            const fullListRes = await listenerAPI.default.getListenerList();
            if (fullListRes.status === 200 && fullListRes.data.status === 200) {
              const matchedListener = fullListRes.data.data.find(
                (l: any) =>
                  l.ConnectAddress === selectedListenerDetail.connectAddress &&
                  l.Type.toLowerCase() === selectedListenerDetail.protocol.toLowerCase()
              );
              if (matchedListener) {
                if (matchedListener.VKey) finalVkey = matchedListener.VKey;
                if (matchedListener.EncryptionSalt) finalEncryptSalt = matchedListener.EncryptionSalt;
              }
            }
          } catch {
            /* 使用默认值 */
          }
        }
      }

      const requestParams: any = {
        osType: selectedClient.os,
        archType: selectedClient.arch,
        pass: formData.pass,
        vkey: finalVkey,
        encryptSalt: finalEncryptSalt,
      };
      if (isListen) {
        requestParams.listener = `${formData.listenProtocol || 'tcp_forward'}://${formData.listenAddress || '0.0.0.0'}:${formData.listenPort || '80'}`;
        if (activeTab.value === 'dlllisten') {
          requestParams.format = 'dll';
        }
      } else {
        requestParams.listener = formData.listener;
      }

      res = await ClientAPI.GenServer(requestParams);
      fileName = `${activeTab.value}_${selectedClient.label}`;
      if (selectedClient.os === 'windows') {
        fileName += activeTab.value === 'dlllisten' ? '.dll' : '.exe';
      }
    } else if (activeTab.value === 'dll') {
      const selectedClient = dllClientTypes.find((c) => c.value === formData.clientType);
      if (!selectedClient) {
        ElMessage.error(isZh.value ? '请选择客户端类型' : 'Select a client type');
        return;
      }
      res = await ClientAPI.GenServer({
        osType: selectedClient.os,
        archType: selectedClient.arch,
        listener: formData.listener,
        pass: formData.pass,
        format: 'dll',
      });
      fileName = `dll_stageless_${selectedClient.label}.dll`;
    } else if (activeTab.value === 'stage') {
      const wd = selectedWebDelivery.value;
      if (!wd) {
        ElMessage.error(isZh.value ? '请选择 WebDelivery' : 'Select a WebDelivery');
        return;
      }
      if (!isStageFormatAllowed(formData.stageFormat)) {
        ElMessage.error(isZh.value ? `当前 WebDelivery 系统(${wd.OS})不支持格式 ${formData.stageFormat}` : `WebDelivery OS (${wd.OS}) does not support format ${formData.stageFormat}`);
        return;
      }

      // port 必须是 WebDelivery 的 ListeningPort
      res = await ClientAPI.DynamicStageShellCodeGen({
        listener: wd.ListenerConfig || '',
        port: wd.ListeningPort,
        format: formData.stageFormat,
        anti_debug: formData.antiDebug,
        obfuscation: formData.obfuscation,
        user_agent: formData.userAgent || '',
        lightweight: false,
      });

      const ext =
        formData.stageFormat === 'exe'
          ? '.exe'
          : formData.stageFormat === 'elf'
            ? ''
            : formData.stageFormat === 'raw'
              ? '.bin'
              : formData.stageFormat === 'bin'
                ? '.bin'
                : formData.stageFormat === 'hex'
                  ? '.txt'
                  : '.c';
      fileName = `dynamic_stager_${wd.ListeningPort}${ext || ''}`;
    } else if (activeTab.value === 'shellcode') {
      const wd = selectedWebDelivery.value;
      if (!wd) {
        ElMessage.error(isZh.value ? '请选择 WebDelivery' : 'Select a WebDelivery');
        return;
      }
      if (wd.OS !== 'windows' && formData.shellcodeFormat !== 'exe') {
        // 传统 StageShellCodeGen 主要面向 Windows stager + donut
        ElMessage.warning(isZh.value ? '传统 Stage Shellcode 主要为 Windows 设计；非 Windows 建议使用 stage 动态 stager' : 'Legacy Stage Shellcode targets Windows; use the stage dynamic stager for other systems');
      }

      res = await ClientAPI.StageShellCodeGen({
        listener: wd.ListenerConfig || '',
        port: wd.ListeningPort,
        format: formData.shellcodeFormat || 'bin',
      });

      const fmt = formData.shellcodeFormat || 'bin';
      const ext = fmt === 'exe' ? '.exe' : fmt === 'raw' || fmt === 'bin' ? '.bin' : fmt === 'hex' ? '.txt' : '.c';
      fileName = `stage_shellcode_${wd.ListeningPort}${ext}`;
    }

    // blob 接口：HTTP 200 也可能是 JSON 错误（部分后端用 200 + status 字段）
    if (res && res.status === 200) {
      const ctype = (res.headers?.['content-type'] || '').toLowerCase();
      if (ctype.includes('json')) {
        try {
          const text = await (res.data as Blob).text();
          const json = JSON.parse(text);
          ElMessage.error(json.data || json.error || (isZh.value ? '生成失败' : 'Generation failed'));
          return;
        } catch {
          ElMessage.error(isZh.value ? '生成失败：后端返回了无法解析的错误' : 'Generation failed: unparsable error from server');
          return;
        }
      }
      if (downloadBlob(res, fileName)) {
        ElMessage.success(isZh.value ? '生成成功' : 'Generated');
      } else {
        ElMessage.error(isZh.value ? '生成失败：响应不是可下载文件' : 'Generation failed: response is not a downloadable file');
      }
    } else {
      ElMessage.error(isZh.value ? '生成失败，请检查后端日志' : 'Generation failed, check server logs');
    }
  } catch (error: any) {
    const msg = await extractBlobError(error);
    ElMessage.error(isZh.value ? `生成失败: ${msg}` : `Generation failed: ${msg}`);
  } finally {
    generating.value = false;
  }
};

const handleTabChange = (tabName: string | number) => {
  const name = String(tabName);
  formData.payloadType = name;
  formData.clientType = '';
  formData.webDeliveryPort = '';
  stagerInfo.value = null;
  // stage 默认格式
  if (name === 'stage') {
    formData.stageFormat = 'exe';
  }
  if (name === 'shellcode') {
    formData.shellcodeFormat = 'bin';
  }
  if (name === 'stage' || name === 'shellcode') {
    handleWebDeliveryDropdown(true);
  }
};

const generateVkey = () => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < 32; i++) result += chars.charAt(Math.floor(Math.random() * chars.length));
  formData.vkey = result;
};

const generateSalt = () => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < 16; i++) result += chars.charAt(Math.floor(Math.random() * chars.length));
  formData.encryptSalt = result;
};

const refreshAddress = () => {
  formData.listenAddress = '0.0.0.0';
};
</script>

<style scoped>
.delivery-pane {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.delivery-pane > * {
  flex: 1;
  min-height: 0;
}

.servergen-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow-y: auto;
}

.sg-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.sg-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sg-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.sg-desc {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.sg-spacer { flex: 1; }

.sg-tip {
  font-size: 14px;
  color: var(--cs-text-dim);
}

/* 「高级格式 ▾」下拉：与工具条按钮同高同圆角 */
.sg-dropdown {
  display: inline-flex;
  align-items: center;
}

.sg-caret {
  font-size: 10px;
  line-height: 1;
  color: var(--cs-text-dim);
}

.gen-card {
  /* vshell：表单卡贴合内容，不随视口拉伸（避免大屏下大片空白） */
  flex: 0 0 auto;
  margin: 10px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  padding: 16px 20px;
  box-sizing: border-box;
}

.tab-description-large {
  font-size: 13px;
}

.sg-bottom-bar {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--cs-border);
}

.listener-select {
  width: 100%;
}

.listener-select :deep(.el-input__inner) {
  font-size: 12px;
  line-height: 1.2;
  height: auto;
  min-height: 32px;
  padding: 8px 12px;
  white-space: normal;
  word-wrap: break-word;
}

.listener-select :deep(.el-input__wrapper) {
  min-height: 32px;
  height: auto;
}

.listener-option-enhanced {
  padding: 8px 0;
  line-height: 1.4;
}

.listener-details-enhanced {
  font-size: 11px;
  color: var(--text-tertiary, #999999);
  line-height: 1.2;
  margin-top: 2px;
  max-width: 500px;
  word-wrap: break-word;
  white-space: normal;
}

.password-input,
.address-input,
.port-input,
.vkey-input,
.salt-input {
  width: 320px;
}

.client-types-compact {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}

.client-tag {
  cursor: pointer;
  transition: all 0.15s ease;
  padding: 6px 14px;
  font-size: 13px;
  font-family: var(--cs-font-mono);
  border-radius: 4px;
  white-space: nowrap;
  user-select: none;
}

.client-tag:hover {
  transform: translateY(-1px);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--cs-active, #1677ff) 25%, transparent);
}

.generate-section {
  text-align: center;
  padding-top: 20px;
  border-top: 1px solid var(--border-color, #e5e5e5);
}

.format-options {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.format-tag {
  cursor: pointer;
  transition: all 0.15s ease;
}

.format-tag:hover {
  transform: translateY(-1px);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--theme-primary, #1677ff) 15%, transparent);
}

.hint-text {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-tertiary, #999999);
}

.stager-info-box {
  background: var(--cs-bg, #0d1117);
  border: 1px solid var(--cs-border, #30363d);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--cs-text, #c9d1d9);
  margin-bottom: 8px;
  max-width: 460px;
}

.stager-info-box b {
  color: var(--cs-text-bright, #e6edf3);
}

.si-loading,
.si-empty {
  font-size: 11px;
  color: var(--cs-text-dim, #8b949e);
}

.generate-btn {
  padding: 12px 32px;
  font-size: 16px;
  border-radius: 8px;
}

.config-form {
  /* 表单行限宽：宽屏下控件不拉满整卡（vshell 同款：字段贴左、右侧留白） */
  max-width: 920px;
}

/* 页面级页签（vshell 下划线页签） */
.sg-tabs {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 20px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  overflow-x: auto;
}

.sg-tab {
  height: 40px;
  display: inline-flex;
  align-items: center;
  padding: 0 14px;
  font-size: 14px;
  color: var(--cs-text);
  cursor: pointer;
  white-space: nowrap;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}

.sg-tab:hover {
  color: var(--cs-active);
}

.sg-tab.active {
  color: var(--cs-active);
  border-bottom-color: var(--cs-active);
}
</style>

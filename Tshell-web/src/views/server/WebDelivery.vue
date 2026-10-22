<template>
  <div class="delivery-page cs-font-ui">
    <!-- 工具条：左侧标题/计数，右侧操作 -->
    <div class="wd-toolbar">
      <div class="wd-title-group">
        <span class="wd-title">{{ i18nStore.tl('站点管理与 Web 投递', 'Hosted Sites & Payloads') }}</span>
        <span class="wd-count">({{ tableData.length }} {{ i18nStore.tl('个活跃站点', 'sites active') }})</span>
      </div>

      <div class="wd-spacer" />

      <button class="cs-btn cs-btn-primary" @click="openCreate">
        {{ i18nStore.tl('新增投递 / 站点', 'New Delivery / Site') }}
      </button>
      <button class="cs-btn" @click="openCloneSite">
        {{ i18nStore.tl('克隆站点', 'Clone Site') }}
      </button>
      <button class="cs-btn" @click="openHostFile">
        {{ i18nStore.tl('文件托管', 'Host File') }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="selectedRow && copyUrl(selectedRow)">
        {{ i18nStore.tl('复制 URL', 'Copy URL') }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow || statusOf(selectedRow) === 1" @click="selectedRow && handleOpen(selectedRow)">
        {{ i18nStore.tl('启动', 'Launch') }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow || statusOf(selectedRow) === 2" @click="selectedRow && handleClose(selectedRow)">
        {{ i18nStore.tl('停止', 'Kill / Stop') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="selectedRow && handleDelete(selectedRow)">
        {{ i18nStore.tl('删除', 'Remove') }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>
      <button class="cs-btn" @click="getWebDeliveryList">
        {{ i18nStore.tl('刷新', 'Refresh') }}
      </button>
    </div>

    <div class="wd-table-wrap">
    <el-table :data="tableData" row-key="id" height="100%" @selection-change="handleSelectionChange">
      <template #empty>
        <EmptyState icon="🌐" variant="inline" :title="i18nStore.tl('暂无 Web 投递记录', 'No Web Deliveries')" />
      </template>
      <el-table-column type="selection" width="40" :reserve-selection="true" />
      <el-table-column type="expand" width="34">
        <template #default="{ row }">
          <div class="wd-expand-detail cs-font-ui">
            <div class="wd-detail-grid">
              <div class="wd-detail-item"><b>{{ i18nStore.tl('绑定模式', 'Bind Mode') }}:</b> {{ modeOf(row) }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('公网主机', 'Public Host') }}:</b> {{ field(row, 'PublicHost', 'public_host') || '-' }}</div>
              <div class="wd-detail-item"><b>Scheme:</b> {{ field(row, 'PublicScheme', 'public_scheme') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('路径前缀', 'Base Path') }}:</b> {{ field(row, 'BasePath', 'base_path') || '-' }}</div>
              <div class="wd-detail-item"><b>URI:</b> {{ field(row, 'URIPath', 'uri_path') || '-' }}</div>
              <div class="wd-detail-item"><b>Content-Type:</b> {{ field(row, 'ContentType', 'content_type') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('Stager 内容类型', 'Stage CT') }}:</b> {{ field(row, 'StageContentType', 'stage_content_type') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('访问令牌', 'Access Token') }}:</b> {{ field(row, 'AccessToken', 'access_token') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('UA 白名单', 'UA Allow') }}:</b> {{ field(row, 'UAAllow', 'ua_allow') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('最大下载次数', 'Max Downloads') }}:</b> {{ field(row, 'MaxDownloads', 'max_downloads') || 0 }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('一次性链接', 'One Time') }}:</b> {{ boolOf(row, 'OneTime', 'one_time') ? (i18nStore.tl('是', 'Yes')) : (i18nStore.tl('否', 'No')) }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('命中次数', 'Hit Count') }}:</b> {{ field(row, 'HitCount', 'hit_count') || 0 }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('Stager 构建次数', 'Stager Build Count') }}:</b> {{ field(row, 'BuildCount', 'build_count') || 0 }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('最后命中 IP', 'Last Hit IP') }}:</b> {{ field(row, 'LastHitIP', 'last_hit_ip') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('最后命中时间', 'Last Hit At') }}:</b> {{ field(row, 'LastHitAt', 'last_hit_at') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ i18nStore.tl('备注', 'Note') }}:</b> {{ field(row, 'Note', 'note') || '-' }}</div>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('模式', 'Mode')" width="100">
        <template #default="{ row }">
          <el-tag :type="modeOf(row) === 'dedicated' ? 'warning' : 'success'" size="small">
            {{ modeOf(row) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('密钥 / 端口', 'Key / Port')" min-width="140">
        <template #default="{ row }">
          <div class="mono">{{ rowKey(row) || '-' }}</div>
          <div class="sub" v-if="rowPort(row)">{{ i18nStore.tl('端口', 'port') }}: {{ rowPort(row) }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('监听器', 'Listener')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ field(row, 'ListenerConfig', 'listener_config') }}</template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('目标', 'Target')" width="130">
        <template #default="{ row }">
          {{ field(row, 'OS', 'os') }}_{{ field(row, 'Arch', 'arch') }}
        </template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('命中', 'Hits')" width="90">
        <template #default="{ row }">
          {{ field(row, 'HitCount', 'hit_count') || 0 }}
          <div class="sub" v-if="field(row, 'LastHitIP', 'last_hit_ip')">
            {{ field(row, 'LastHitIP', 'last_hit_ip') }}
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('状态', 'Status')" width="90">
        <template #default="{ row }">
          <el-tag :type="statusOf(row) === 1 ? 'success' : 'danger'" size="small">
            {{ statusOf(row) === 1 ? (i18nStore.tl('开启', 'On')) : (i18nStore.tl('关闭', 'Off')) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="220" fixed="right" align="center">
        <template #default="{ row }">
          <div class="wd-action-row">
            <button
              class="cs-btn cs-btn-xs cs-btn-primary"
              :disabled="statusOf(row) === 2"
              :title="i18nStore.tl('查看上线命令', 'Show Commands')"
              @click="showCommands(row)"
            >
              {{ i18nStore.tl('上线命令', 'Commands') }}
            </button>
            <button
              class="cs-btn cs-btn-xs"
              :title="i18nStore.tl('复制投递 URL', 'Copy Delivery URL')"
              @click="copyUrl(row)"
            >
              {{ i18nStore.tl('复制URL', 'Copy') }}
            </button>
            <el-dropdown trigger="click" @command="(cmd: string) => handleMoreAction(cmd, row)">
              <button class="cs-btn cs-btn-xs" :title="i18nStore.tl('更多操作', 'More Actions')">
                ⋮
              </button>
              <template #dropdown>
                <el-dropdown-menu class="cs-menu-popper">
                  <el-dropdown-item v-if="statusOf(row) !== 1" command="open">{{ i18nStore.tl('开启服务', 'Start') }}</el-dropdown-item>
                  <el-dropdown-item v-if="statusOf(row) === 1" command="close">{{ i18nStore.tl('关闭服务', 'Stop') }}</el-dropdown-item>
                  <el-dropdown-item command="tokens">{{ i18nStore.tl('门禁令牌', 'Access Tokens') }}</el-dropdown-item>
                  <el-dropdown-item command="logs">{{ i18nStore.tl('访问日志', 'Logs') }}</el-dropdown-item>
                  <el-dropdown-item v-if="field(row, 'OS', 'os') === 'windows' && statusOf(row) === 1" command="shellcode">Shellcode</el-dropdown-item>
                  <el-dropdown-item v-if="statusOf(row) === 1" command="dynamic">{{ i18nStore.tl('动态 Stager', 'Dynamic Stager') }}</el-dropdown-item>
                  <el-dropdown-item v-if="statusOf(row) === 1" command="script">{{ i18nStore.tl('脚本载荷', 'Script Payload') }}</el-dropdown-item>
                  <el-dropdown-item command="delete" divided class="text-danger">{{ i18nStore.tl('删除', 'Delete') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <!-- 创建 Delivery Hub -->
    <el-dialog v-model="dialogVisible" :title="i18nStore.tl('配置 Delivery Hub', 'Configure Delivery Hub')" width="720px" destroy-on-close>
      <el-form :model="formData" label-width="130px">
        <el-form-item :label="i18nStore.tl('绑定模式', 'Bind Mode')">
          <el-radio-group v-model="formData.bindMode">
            <el-radio label="shared">{{ i18nStore.tl('shared（主端口 /d/key，推荐）', 'shared (main port /d/key, recommended)') }}</el-radio>
            <el-radio label="dedicated">{{ i18nStore.tl('dedicated（独立端口）', 'dedicated (separate port)') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('监听器', 'Listener')" required>
          <el-select
            v-model="formData.listener"
            :placeholder="i18nStore.tl('选择监听器', 'Select a listener')"
            style="width:100%"
            @visible-change="handleDropdown"
          >
            <el-option v-for="item in options" :key="item" :label="item" :value="item" />
          </el-select>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('操作系统', 'OS')" required>
          <el-radio-group v-model="formData.os" @change="onOSChange">
            <el-radio label="windows">windows</el-radio>
            <el-radio label="linux">linux</el-radio>
            <el-radio label="darwin">darwin</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('架构', 'Arch')" required>
          <el-radio-group v-model="formData.arch">
            <el-radio label="amd64">amd64</el-radio>
            <el-radio label="386">386</el-radio>
            <el-radio label="arm64">arm64</el-radio>
            <el-radio label="arm">arm</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('上线口令', 'Exec Key')">
          <el-input v-model="formData.pass" show-password :placeholder="i18nStore.tl('反沙箱启动口令 / Exec Key', 'Anti-sandbox exec key')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('文件名', 'File Name')">
          <el-input v-model="formData.filename" :placeholder="i18nStore.tl('托管文件名，如 svchost.exe', 'Hosted file name, e.g. svchost.exe')" />
        </el-form-item>
        <el-form-item v-if="formData.os === 'windows'" :label="i18nStore.tl('载荷格式', 'Payload Format')">
          <el-radio-group v-model="formData.payloadFormat" @change="onFormatChange">
            <el-radio label="exe">{{ i18nStore.tl('exe（可执行文件）', 'exe (executable)') }}</el-radio>
            <el-radio label="dll">{{ i18nStore.tl('dll（rundll32 / regsvr32 加载）', 'dll (loaded via rundll32 / regsvr32)') }}</el-radio>
          </el-radio-group>
          <span class="hint" v-if="formData.payloadFormat === 'dll'">
            {{ i18nStore.tl('rundll32.exe client.dll,Run 或 regsvr32.exe client.dll 加载', 'Load with rundll32.exe client.dll,Run or regsvr32.exe client.dll') }}
          </span>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('保存文件名', 'Save As')">
          <el-input v-model="formData.saveFilename" :placeholder="i18nStore.tl('目标机落地文件名', 'File name written on target')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('下载路径', 'Download Path')">
          <el-input v-model="formData.downloadPath" />
        </el-form-item>
        <el-form-item v-if="formData.bindMode === 'dedicated'" :label="i18nStore.tl('独立端口', 'Dedicated Port')" required>
          <el-input v-model="formData.port" :placeholder="i18nStore.tl('如 8000', 'e.g. 8000')" />
        </el-form-item>

        <el-divider content-position="left">{{ i18nStore.tl('伪装与公开地址', 'Masquerade & Public Address') }}</el-divider>
        <el-form-item :label="i18nStore.tl('公网主机', 'Public Host')">
          <el-input v-model="formData.publicHost" :placeholder="i18nStore.tl('留空自动推断，如 1.2.3.4:8089', 'Leave empty to auto-detect, e.g. 1.2.3.4:8089')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('协议', 'Scheme')">
          <el-radio-group v-model="formData.publicScheme">
            <el-radio label="">{{ i18nStore.tl('默认', 'Default') }}</el-radio>
            <el-radio label="http">http</el-radio>
            <el-radio label="https">https</el-radio>
          </el-radio-group>
          <span class="hint">{{ i18nStore.tl('接 CDN 时选 https；空则用 config delivery.default_scheme', 'Use https behind a CDN; empty falls back to config delivery.default_scheme') }}</span>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('路径前缀', 'Base Path')">
          <el-input v-model="formData.basePath" :placeholder="i18nStore.tl('CDN 路径前缀，如 /static 或留空', 'CDN path prefix, e.g. /static, or leave empty')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('URL 预览', 'URL Preview')">
          <div class="url-box" v-if="previewUrls">
            <div><b>{{ i18nStore.tl('完整', 'Full') }}:</b> <code>{{ previewUrls.full }}</code></div>
            <div v-if="previewUrls.stager"><b>{{ i18nStore.tl('分阶段', 'Stager') }}:</b> <code>{{ previewUrls.stager }}</code></div>
            <div v-if="previewUrls.shellcode"><b>{{ i18nStore.tl('Shellcode', 'SC') }}:</b> <code>{{ previewUrls.shellcode }}</code></div>
          </div>
          <el-button @click="doPreviewUrls" style="margin-top:6px">{{ i18nStore.tl('刷新预览', 'Refresh Preview') }}</el-button>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('URI 路径', 'URI Path')">
          <el-input v-model="formData.uriPath" :placeholder="i18nStore.tl('可选伪装路径；空则用随机 key', 'Optional decoy path; random key if empty')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('内容类型', 'Content-Type')">
          <el-select v-model="formData.contentType" filterable allow-create style="width:100%">
            <el-option label="application/octet-stream" value="application/octet-stream" />
            <el-option label="application/javascript" value="application/javascript" />
            <el-option label="font/woff" value="font/woff" />
            <el-option label="image/png" value="image/png" />
            <el-option label="text/css" value="text/css" />
          </el-select>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('Stager 内容类型', 'Stage CT')">
          <el-select v-model="formData.stageContentType" filterable allow-create style="width:100%">
            <el-option label="font/woff" value="font/woff" />
            <el-option label="font/woff2" value="font/woff2" />
            <el-option label="application/octet-stream" value="application/octet-stream" />
          </el-select>
        </el-form-item>

        <el-divider content-position="left">{{ i18nStore.tl('防护', 'Protection') }}</el-divider>
        <el-form-item :label="i18nStore.tl('访问令牌', 'Access Token')">
          <el-input
            v-model="formData.accessToken"
            :placeholder="i18nStore.tl('可选；请求需 ?t=token 或 Header X-Delivery-Token', 'Optional; request needs ?t=token or X-Delivery-Token header')"
          />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('UA 白名单', 'UA Allowlist')">
          <el-input v-model="formData.uaAllow" :placeholder="i18nStore.tl('逗号分隔子串，如 curl,Wget,PowerShell', 'Comma-separated substrings, e.g. curl,Wget,PowerShell')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('最大下载次数', 'Max Downloads')">
          <el-input-number v-model="formData.maxDownloads" :min="0" />
          <span class="hint">{{ i18nStore.tl('0 = 不限制', '0 = unlimited') }}</span>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('一次性链接', 'One-Time Link')">
          <el-switch v-model="formData.oneTime" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('构建 Stager', 'Build Stager')">
          <el-switch v-model="formData.buildStager" />
          <span class="hint">{{ i18nStore.tl('需要服务端 Go 工具链', 'Requires Go toolchain on server') }}</span>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('备注', 'Note')">
          <el-input v-model="formData.note" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ i18nStore.tl('取消', 'Cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="handleCreate">{{ i18nStore.tl('创建并启动', 'Create & Start') }}</el-button>
      </template>
    </el-dialog>

    <!-- 上线命令矩阵 -->
    <el-dialog v-model="commandDialogVisible" :title="i18nStore.tl('上线命令矩阵', 'Payload Command Matrix')" width="1100px">
      <div v-if="commandUrls" class="url-box mb-12">
        <div>
          <b>{{ i18nStore.tl('完整', 'Full') }}:</b> <code>{{ commandUrls.full }}</code>
          <el-button link type="primary" @click="copyText(commandUrls.full)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
        </div>
        <div v-if="commandUrls.stager">
          <b>{{ i18nStore.tl('分阶段', 'Stager') }}:</b> <code>{{ commandUrls.stager }}</code>
          <el-button link type="primary" @click="copyText(commandUrls.stager)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
        </div>
        <div v-if="commandUrls.shellcode">
          <b>{{ i18nStore.tl('Shellcode/WOFF', 'SC/WOFF') }}:</b> <code>{{ commandUrls.shellcode }}</code>
          <el-button link type="primary" @click="copyText(commandUrls.shellcode)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
        </div>
      </div>
      <el-table :data="commandList" max-height="560">
        <el-table-column prop="name" :label="i18nStore.tl('名称', 'Name')" width="180" />
        <el-table-column prop="os" label="OS" width="90" />
        <el-table-column prop="description" :label="i18nStore.tl('说明', 'Description')" width="180" show-overflow-tooltip />
        <el-table-column prop="code" :label="i18nStore.tl('命令', 'Command')" min-width="480">
          <template #default="{ row }">
            <code class="cmd">{{ row.code }}</code>
          </template>
        </el-table-column>
        <el-table-column label="" width="80">
          <template #default="{ row }">
            <el-button type="primary" @click="copyText(row.code)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 访问日志 -->
    <el-dialog v-model="logsDialogVisible" :title="i18nStore.tl('投递访问日志', 'Delivery Access Logs')" width="900px">
      <el-table :data="accessLogs" max-height="500">
        <el-table-column prop="created_at" :label="i18nStore.tl('时间', 'Time')" width="170" />
        <el-table-column prop="resource" :label="i18nStore.tl('资源', 'Resource')" width="90" />
        <el-table-column prop="client_ip" label="IP" width="130" />
        <el-table-column prop="status_code" :label="i18nStore.tl('状态', 'Status')" width="80" />
        <el-table-column prop="bytes_sent" :label="i18nStore.tl('字节', 'Bytes')" width="100" />
        <el-table-column prop="user_agent" label="UA" min-width="240" show-overflow-tooltip />
      </el-table>
    </el-dialog>

    <!-- StageShellcode（兼容旧接口，port 使用 ListeningPort） -->
    <el-dialog v-model="dialogVisible2" title="StageShellcode">
      <el-form :model="selectedFormat" label-width="120px">
        <el-form-item :label="i18nStore.tl('输出格式', 'Output Format')">
          <el-select v-model="selectedFormat.format">
            <el-option :label="i18nStore.tl('十六进制 (hex)', 'Hex')" value="hex" />
            <el-option :label="i18nStore.tl('C 数组 (c)', 'C Array')" value="c" />
            <el-option :label="i18nStore.tl('二进制 (bin)', 'Binary')" value="bin" />
            <el-option :label="i18nStore.tl('原始二进制 (raw)', 'Raw Binary')" value="raw" />
            <el-option :label="i18nStore.tl('可执行 (exe)', 'Executable')" value="exe" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleStageShellcode">{{ i18nStore.tl('生成', 'Generate') }}</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>

    <!-- 动态 Stager（兼容旧接口，port 使用 ListeningPort） -->
    <el-dialog v-model="dialogVisible3" :title="i18nStore.tl('动态 Stager 生成', 'Dynamic Stager Generation')" width="640px">
      <el-form :model="dynamicStagerForm" label-width="120px">
        <el-form-item :label="i18nStore.tl('格式', 'Format')">
          <el-select v-model="dynamicStagerForm.format">
            <el-option label="exe" value="exe" />
            <el-option label="elf" value="elf" />
            <el-option label="raw" value="raw" />
            <el-option label="bin" value="bin" />
            <el-option label="hex" value="hex" />
            <el-option label="c" value="c" />
          </el-select>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('反调试', 'Anti-Debug')">
          <el-switch v-model="dynamicStagerForm.anti_debug" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('混淆', 'Obfuscation')">
          <el-switch v-model="dynamicStagerForm.obfuscation" />
        </el-form-item>
        <el-form-item label="User-Agent">
          <el-input v-model="dynamicStagerForm.user_agent" :placeholder="i18nStore.tl('留空随机', 'Empty = random')" />
        </el-form-item>
        <el-form-item :label="i18nStore.tl('轻量模式', 'Lightweight')">
          <el-switch v-model="dynamicStagerForm.lightweight" />
        </el-form-item>
        <!-- Stager 构建信息（后端 /shellcode/stager-info?port=...） -->
        <el-form-item :label="i18nStore.tl('构建信息', 'Build Info')">
          <div class="stager-info-box">
            <div v-if="stagerInfoLoading" class="si-loading">{{ i18nStore.tl('加载中...', 'Loading...') }}</div>
            <template v-else-if="stagerInfo">
              <div><b>{{ i18nStore.tl('端口', 'Port') }}:</b> {{ stagerInfo.port || port }}</div>
              <div><b>{{ i18nStore.tl('目标', 'Target') }}:</b> {{ stagerInfo.os }}_{{ stagerInfo.arch }}</div>
              <div><b>URL:</b> {{ stagerInfo.server_url || '-' }}</div>
              <div><b>{{ i18nStore.tl('最后构建', 'Last Build') }}:</b> {{ stagerInfo.last_build || '-' }}</div>
              <div><b>{{ i18nStore.tl('构建次数', 'Build Count') }}:</b> {{ stagerInfo.build_count || 0 }}</div>
            </template>
            <div v-else class="si-empty">{{ i18nStore.tl('选择 WebDelivery 后点击【查询构建信息】', 'Select a WebDelivery and query build info') }}</div>
          </div>
          <el-button :loading="stagerInfoLoading" @click="fetchStagerBuildInfo">{{ i18nStore.tl('查询构建信息', 'Query Build Info') }}</el-button>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="dynamicGenerating" @click="handleDynamicStager">{{ i18nStore.tl('生成', 'Generate') }}</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>

    <!-- 脚本载荷（按客户端 OS：Windows=ps1/vbs/bat/hta；Linux=sh） -->
    <el-dialog v-model="scriptDialogVisible" :title="i18nStore.tl('脚本载荷', 'Script Payload')" width="720px">
      <el-form label-width="130px">
        <el-form-item :label="i18nStore.tl('形态', 'Mode')">
          <el-radio-group v-model="scriptMode">
            <el-radio label="url">{{ i18nStore.tl('引用 URL（下载 /full 后执行）', 'Reference URL (download /full then run)') }}</el-radio>
            <el-radio label="embed">{{ i18nStore.tl('内嵌 base64（ps1 / bat / sh 支持）', 'Embedded base64 (ps1 / bat / sh)') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <div v-if="scriptRows.length" class="url-box">
          <div v-for="s in scriptRows" :key="s.kind" style="margin-bottom: 8px">
            <b>{{ s.kind.toUpperCase() }}:</b> <code>{{ s.url }}</code>
            <el-button link type="primary" @click="copyText(s.url)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
          </div>
        </div>
        <el-alert
          type="info"
          :closable="false"
          :title="i18nStore.tl('脚本运行时自动下载完整载荷并执行；embed 形态内嵌载荷字节，无需外部请求。', 'The script downloads the full payload at run time; the embed mode inlines the payload bytes with no external request.')"
        />
      </el-form>
    </el-dialog>

    <!-- 门禁令牌（防盗门）：StagerToken 配额管理 -->
    <el-dialog v-model="tokenDialogVisible" :title="i18nStore.tl('门禁令牌', 'Access Tokens')" width="720px">
      <div style="margin-bottom: 12px">
        <el-alert
          type="warning"
          :closable="false"
          show-icon
          :title="i18nStore.tl('为每个 Stager 生成独立令牌（带使用配额，一次一密）。Stager 下载时携带 X-Delivery-Token 头，配额用完即返回 404。', 'Issues a unique token per Stager with a usage quota. The Stager sends it in the X-Delivery-Token header; once the quota is used up the server returns 404.')"
        />
      </div>
      <el-table :data="tokenRows">
        <el-table-column prop="token" :label="i18nStore.tl('令牌', 'Token')" min-width="160">
          <template #default="{ row }"><code>{{ row.token }}</code></template>
        </el-table-column>
        <el-table-column :label="i18nStore.tl('配额', 'Quota')" width="110">
          <template #default="{ row }">{{ row.used }}/{{ row.quota }}</template>
        </el-table-column>
        <el-table-column prop="note" :label="i18nStore.tl('备注', 'Note')" min-width="120">
          <template #default="{ row }">{{ row.note || '-' }}</template>
        </el-table-column>
        <el-table-column prop="last_used_ip" :label="i18nStore.tl('最后使用 IP', 'Last Used IP')" min-width="120">
          <template #default="{ row }">{{ row.last_used_ip || '-' }}</template>
        </el-table-column>
        <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="90">
          <template #default="{ row }">
            <el-button type="danger" link @click="deleteToken(row)">{{ i18nStore.tl('删除', 'Delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="openTokenCreate">{{ i18nStore.tl('生成新令牌', 'New Token') }}</el-button>
        <el-button type="primary" @click="tokenDialogVisible = false">{{ i18nStore.tl('关闭', 'Close') }}</el-button>
      </template>
    </el-dialog>

    <!-- 生成令牌 -->
    <el-dialog v-model="tokenCreateVisible" :title="i18nStore.tl('生成门禁令牌', 'Generate Access Token')" width="460px">
      <el-form label-width="90px">
        <el-form-item :label="i18nStore.tl('配额', 'Quota')">
          <el-input-number v-model="tokenQuota" :min="1" :max="100" />
          <span class="hint">{{ i18nStore.tl('允许下载次数（默认 3）', 'Allowed downloads (default 3)') }}</span>
        </el-form-item>
        <el-form-item :label="i18nStore.tl('备注', 'Note')">
          <el-input v-model="tokenNote" :placeholder="i18nStore.tl('如：Stager-Win-x64-钓鱼', 'e.g. Stager-Win-x64-phish')" />
        </el-form-item>
        <el-form-item v-if="tokenCreated" :label="i18nStore.tl('生成结果', 'Result')">
          <div style="width: 100%">
            <div class="url-box" style="word-break: break-all">
              <code>{{ tokenCreated.token }}</code>
            </div>
            <div class="url-box" style="margin-top: 6px; word-break: break-all">
              <code>X-Delivery-Token: {{ tokenCreated.token }}</code>
              <el-button link type="primary" @click="copyText('X-Delivery-Token: ' + tokenCreated.token)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
            </div>
            <div class="url-box" style="margin-top: 6px; word-break: break-all">
              <code>{{ tokenCreated.curl }}</code>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tokenCreateVisible = false">{{ i18nStore.tl('关闭', 'Close') }}</el-button>
        <el-button type="primary" :loading="tokenCreating" @click="createToken">{{ i18nStore.tl('生成', 'Generate') }}</el-button>
      </template>
    </el-dialog>

    <!-- 页内对话框（原全局 store 挂载收编到本页；局部 ref + v-model） -->
    <CloneSiteDialog v-model="cloneSiteVisible" :key="cloneSiteKey" />
    <HostFileDialog v-model="hostFileVisible" :key="hostFileKey" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { Plus, Refresh } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import EmptyState from '@/components/Common/EmptyState.vue'
import CloneSiteDialog from '@/layout/Dialogs/CloneSiteDialog.vue'
import HostFileDialog from '@/layout/Dialogs/HostFileDialog.vue'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const tableData = ref<any[]>([])
const dialogVisible = ref(false)
const commandDialogVisible = ref(false)
const logsDialogVisible = ref(false)
const dialogVisible2 = ref(false)
const dialogVisible3 = ref(false)
const scriptDialogVisible = ref(false)
const scriptMode = ref('url')
const scriptBaseUrl = ref('')
const scriptKinds = ref<string[]>([])
// 脚本类型按客户端 OS 区分：Windows=ps1/vbs/bat/hta；Linux=sh
const scriptRows = computed(() =>
  scriptKinds.value.map((kind) => ({
    kind,
    url: `${scriptBaseUrl.value}/${kind}?mode=${scriptMode.value}`,
  }))
)
const creating = ref(false)
const dynamicGenerating = ref(false)
// Stager 构建信息（/shellcode/stager-info）
const stagerInfo = ref<any>(null)
const stagerInfoLoading = ref(false)
const options = ref<string[]>([])
const commandList = ref<any[]>([])
const commandUrls = ref<any>(null)
const accessLogs = ref<any[]>([])
const previewUrls = ref<any>(null)
const deliveryDefaults = ref<any>(null)
const lis = ref('')
const port = ref('')
const selectedRow = ref<any>(null)
const handleHelp = () => {
  ElMessage.info('Web Log / Hosted Sites Manual')
}

const formData = reactive({
  bindMode: 'shared' as 'shared' | 'dedicated',
  listener: '',
  os: 'windows',
  arch: 'amd64',
  payloadFormat: 'exe',
  pass: '',
  filename: 'svchost.exe',
  saveFilename: 'svchost.exe',
  downloadPath: 'C:\\Windows\\Temp',
  port: '',
  publicHost: '',
  publicScheme: '',
  basePath: '',
  uriPath: '',
  contentType: 'application/octet-stream',
  stageContentType: 'font/woff',
  accessToken: '',
  uaAllow: '',
  maxDownloads: 0,
  oneTime: false,
  buildStager: true,
  note: '',
})

const selectedFormat = reactive({ format: 'bin' })
const dynamicStagerForm = reactive({
  format: 'exe',
  anti_debug: false,
  obfuscation: false,
  user_agent: '',
  lightweight: true,
})

/** 兼容 xorm 导出的 PascalCase / snake_case 字段 */
const field = (row: any, ...keys: string[]) => {
  if (!row) return ''
  for (const k of keys) {
    if (row[k] !== undefined && row[k] !== null && row[k] !== '') return row[k]
  }
  return ''
}

const modeOf = (row: any) => field(row, 'BindMode', 'bind_mode') || 'shared'
const statusOf = (row: any) => {
  const s = row?.Status ?? row?.status
  return s === undefined || s === null ? 2 : s
}
const rowId = (row: any) => field(row, 'ID', 'id')
const rowKey = (row: any) => field(row, 'PublicKey', 'public_key')
const rowPort = (row: any) => field(row, 'ListeningPort', 'listening_port')
const boolOf = (row: any, ...keys: string[]) => {
  const v = field(row, ...keys)
  return v === 1 || v === true || v === '1' || v === 'true'
}

const getWebDeliveryList = async () => {
  try {
    const res = await ClientAPI.GetWebDeliveryList()
    if (res.status === 200 && res.data.status === 200) {
      tableData.value = res.data.data || []
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '获取列表失败' : 'Failed to load list'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '获取列表失败' : 'Failed to load list'))
  }
}

const handleDropdown = async (visible?: boolean) => {
  if (visible === false) return
  try {
    const listenerAPI = await import('@/api/listener')
    const fullListRes = await listenerAPI.default.getListenerList()
    if (fullListRes.status === 200 && fullListRes.data?.status === 200 && Array.isArray(fullListRes.data.data)) {
      // 仅保留运行中的监听器 (Status === 1)
      const runningListeners = fullListRes.data.data.filter((l: any) => Number(l.Status) === 1)
      options.value = runningListeners.map(
        (item: any) => `${item.Type}://${item.ConnectAddress}`
      )
      if (!options.value.length) {
        ElMessage.warning(isZh.value ? '暂无运行中的监听器，请先启动监听器' : 'No running listener, start one first')
      }
      return
    }
  } catch {
    /* fallback */
  }
  try {
    const detailRes = await ClientAPI.ShowListenerDetails()
    if (detailRes.status === 200 && detailRes.data?.status === 200) {
      const running = (detailRes.data.data || []).filter(
        (item: any) => item.status === undefined || Number(item.status) === 1 || Number(item.Status) === 1
      )
      options.value = running.map(
        (item: any) => item.url || `${item.protocol}://${item.connectAddress}`
      )
      if (!options.value.length) {
        ElMessage.warning(isZh.value ? '暂无运行中的监听器，请先启动监听器' : 'No running listener, start one first')
      }
      return
    }
  } catch {
    /* fallback */
  }
  try {
    const res = await ClientAPI.ShowListener()
    if (res.status === 200 && res.data?.status === 200) {
      options.value = res.data.data || []
    }
  } catch {
    /* ignore */
  }
}

const onOSChange = () => {
  if (formData.os === 'windows') {
    formData.filename = formData.payloadFormat === 'dll' ? 'svchost.dll' : 'svchost.exe'
    formData.saveFilename = formData.payloadFormat === 'dll' ? 'svchost.dll' : 'svchost.exe'
    formData.downloadPath = 'C:\\Windows\\Temp'
    formData.stageContentType = 'font/woff'
  } else {
    formData.payloadFormat = 'exe'
    formData.filename = 'systemd-update'
    formData.saveFilename = 'systemd-update'
    formData.downloadPath = '/tmp'
    formData.stageContentType = 'application/octet-stream'
  }
}

const onFormatChange = () => {
  const ext = formData.payloadFormat === 'dll' ? '.dll' : '.exe'
  const base = formData.filename.replace(/\.(exe|dll)$/i, '')
  formData.filename = base + ext
  const saveBase = formData.saveFilename.replace(/\.(exe|dll)$/i, '')
  formData.saveFilename = saveBase + ext
}


const loadDeliveryDefaults = async () => {
  try {
    const res = await ClientAPI.GetDeliveryDefaults()
    if (res.status === 200 && res.data.status === 200) {
      deliveryDefaults.value = res.data.data
      if (res.data.data?.forceHttps && !formData.publicScheme) {
        formData.publicScheme = 'https'
      }
    }
  } catch {
    /* ignore */
  }
}

const doPreviewUrls = async () => {
  try {
    const res = await ClientAPI.PreviewDeliveryURLs({
      listener: formData.listener,
      os: formData.os,
      filename: formData.filename,
      bindMode: formData.bindMode,
      port: formData.port,
      publicHost: formData.publicHost,
      publicScheme: formData.publicScheme,
      basePath: formData.basePath,
      uriPath: formData.uriPath,
    })
    if (res.status === 200 && res.data.status === 200) {
      previewUrls.value = res.data.data
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? 'URL 预览失败' : 'URL preview failed'))
  }
}

const openCreate = () => {
  dialogVisible.value = true
  handleDropdown(true)
  loadDeliveryDefaults()
  doPreviewUrls()
}

// 页内对话框（原全局挂载 → 局部 ref）
const cloneSiteVisible = ref(false)
const cloneSiteKey = ref(0)
const hostFileVisible = ref(false)
const hostFileKey = ref(0)

// 两个弹窗自身都没有「打开时初始化」逻辑：打开前换 key 强制重挂，避免上次表单/文件残留
// （先重挂空实例再打开，保留 el-dialog 的打开动画）
const openCloneSite = async () => {
  cloneSiteKey.value += 1
  await nextTick()
  cloneSiteVisible.value = true
}

const openHostFile = async () => {
  hostFileKey.value += 1
  await nextTick()
  hostFileVisible.value = true
}

const handleCreate = async () => {
  if (!formData.listener || !formData.os || !formData.arch) {
    ElMessage.warning(isZh.value ? '请填写监听器 / OS / Arch' : 'Fill in listener / OS / Arch')
    return
  }
  if (formData.bindMode === 'dedicated' && !formData.port) {
    ElMessage.warning(isZh.value ? 'dedicated 模式必须填写端口' : 'Port is required in dedicated mode')
    return
  }
  creating.value = true
  try {
    const res = await ClientAPI.StartWebDelivery({
      bindMode: formData.bindMode,
      listener: formData.listener,
      os: formData.os,
      arch: formData.arch,
      port: formData.bindMode === 'dedicated' ? formData.port : undefined,
      filename: formData.filename,
      pass: formData.pass,
      format: formData.os === 'windows' ? formData.payloadFormat : undefined,
      downloadPath: formData.downloadPath,
      saveFilename: formData.saveFilename,
      publicHost: formData.publicHost,
      publicScheme: formData.publicScheme,
      basePath: formData.basePath,
      uriPath: formData.uriPath,
      contentType: formData.contentType,
      stageContentType: formData.stageContentType,
      accessToken: formData.accessToken,
      uaAllow: formData.uaAllow,
      maxDownloads: formData.maxDownloads,
      oneTime: formData.oneTime,
      buildStager: formData.buildStager,
      note: formData.note,
    })
    if (res.status === 200 && res.data.status === 200) {
      ElMessage.success(isZh.value ? '投递已创建' : 'Delivery created')
      dialogVisible.value = false
      await getWebDeliveryList()
      const data = res.data.data
      if (data?.commands) {
        commandList.value = data.commands
        commandUrls.value = data.urls
        commandDialogVisible.value = true
      }
    } else {
      ElMessage.error(res.data?.data || res.data?.error || (isZh.value ? '创建失败' : 'Create failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || String(e))
  } finally {
    creating.value = false
  }
}

const handleOpen = async (row: any) => {
  const res = await ClientAPI.OpenWebDelivery({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    port: rowPort(row) || undefined,
  })
  if (res.status === 200 && res.data.status === 200) {
    ElMessage.success(isZh.value ? '已开启' : 'Started')
    await getWebDeliveryList()
  } else {
    ElMessage.error(res.data?.data || (isZh.value ? '开启失败' : 'Failed to start'))
  }
}

const handleClose = async (row: any) => {
  const res = await ClientAPI.CloseWebDelivery({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    port: rowPort(row) || undefined,
  })
  if (res.status === 200 && res.data.status === 200) {
    ElMessage.success(isZh.value ? '已关闭' : 'Stopped')
    await getWebDeliveryList()
  } else {
    ElMessage.error(res.data?.data || (isZh.value ? '关闭失败' : 'Failed to stop'))
  }
}

const handleDelete = async (row: any) => {
  try {
    await ElMessageBox.confirm(isZh.value ? '确认删除该投递？' : 'Delete this delivery?', isZh.value ? '提示' : 'Confirm', { type: 'warning' })
  } catch {
    return
  }
  const res = await ClientAPI.DeleteWebDelivery({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    port: rowPort(row) || undefined,
  })
  if (res.status === 200 && res.data.status === 200) {
    ElMessage.success(isZh.value ? '已删除' : 'Deleted')
    await getWebDeliveryList()
  } else {
    ElMessage.error(res.data?.data || (isZh.value ? '删除失败' : 'Delete failed'))
  }
}

// 批量选择（参考客户端管理）
const selectedRows = ref<any[]>([])
const handleSelectionChange = (rows: any[]) => { selectedRows.value = rows }
const clearSelection = () => { selectedRows.value = [] }

const handleBatchDelete = async () => {
  try {
    await ElMessageBox.confirm(isZh.value ? `确认删除选中的 ${selectedRows.value.length} 个投递？` : `Delete ${selectedRows.value.length} selected deliveries?`, isZh.value ? '批量删除' : 'Bulk Delete', { type: 'warning' })
  } catch {
    return
  }
  let failed = 0
  for (const row of selectedRows.value) {
    const res = await ClientAPI.DeleteWebDelivery({
      id: rowId(row) || undefined,
      publicKey: rowKey(row) || undefined,
      port: rowPort(row) || undefined,
    })
    if (!(res.status === 200 && res.data.status === 200)) failed++
  }
  if (failed === 0) {
    ElMessage.success(isZh.value ? '批量删除成功' : 'Bulk delete succeeded')
  } else {
    ElMessage.error(isZh.value ? `${failed} 个投递删除失败` : `${failed} deliveries failed to delete`)
  }
  clearSelection()
  await getWebDeliveryList()
}

const showCommands = async (row: any) => {
  const res = await ClientAPI.GetDeliveryCommands({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    port: rowPort(row) || undefined,
  })
  if (res.status === 200 && res.data.status === 200) {
    // 【统一响应格式】业务数据在 data 内（{commands, urls}）；兼容旧顶层格式
    const payload = res.data.data || {}
    commandList.value = payload.commands || payload || []
    commandUrls.value = payload.urls || res.data.urls || {
      full: field(row, 'ServerAddress', 'server_address'),
      stager: field(row, 'StagerURL', 'stager_url'),
      shellcode: field(row, 'ShellcodeURL', 'shellcode_url'),
    }
    commandDialogVisible.value = true
  } else {
    ElMessage.error(res.data?.data || (isZh.value ? '获取命令失败' : 'Failed to load commands'))
  }
}

const openScriptDialog = (row: any) => {
  const base = field(row, 'ServerAddress', 'server_address')
  if (!base) {
    ElMessage.warning(isZh.value ? '该投递没有公开 URL，无法生成脚本（shared 模式才有）' : 'This delivery has no public URL; scripts need shared mode')
    return
  }
  // 按客户端 OS 展示对应脚本类型：Windows=ps1/vbs/bat/hta；Linux=sh
  const os = String(field(row, 'OS', 'os') || '').toLowerCase()
  scriptKinds.value = os.includes('linux') ? ['sh'] : ['ps1', 'vbs', 'bat', 'hta']
  scriptBaseUrl.value = base.replace(/\/full\/.*$/, '')
  scriptMode.value = 'url'
  scriptDialogVisible.value = true
}

const handleMoreAction = (cmd: string, row: any) => {
  switch (cmd) {
    case 'open':
      handleOpen(row)
      break
    case 'close':
      handleClose(row)
      break
    case 'tokens':
      showTokens(row)
      break
    case 'logs':
      showLogs(row)
      break
    case 'shellcode':
      openShellcodeDialog(row)
      break
    case 'dynamic':
      openDynamicDialog(row)
      break
    case 'script':
      openScriptDialog(row)
      break
    case 'delete':
      handleDelete(row)
      break
  }
}

// ===== 门禁令牌（防盗门）=====
const tokenDialogVisible = ref(false)
const tokenCurrentKey = ref('')
const tokenRows = ref<any[]>([])
const tokenCreateVisible = ref(false)
const tokenQuota = ref(3)
const tokenNote = ref('')
const tokenCreated = ref<any>(null)
const tokenCreating = ref(false)

const showTokens = async (row: any) => {
  const key = rowKey(row)
  if (!key) {
    ElMessage.warning(isZh.value ? '该投递没有 public_key' : 'This delivery has no public_key')
    return
  }
  tokenCurrentKey.value = key
  await loadTokens()
  tokenDialogVisible.value = true
}

const loadTokens = async () => {
  try {
    const res = await ClientAPI.ListStagerTokens({ public_key: tokenCurrentKey.value })
    tokenRows.value = res.data?.data?.list || []
  } catch {
    tokenRows.value = []
  }
}

const openTokenCreate = () => {
  tokenQuota.value = 3
  tokenNote.value = ''
  tokenCreated.value = null
  tokenCreateVisible.value = true
}

const createToken = async () => {
  tokenCreating.value = true
  try {
    const res = await ClientAPI.CreateStagerToken({
      public_key: tokenCurrentKey.value,
      quota: tokenQuota.value,
      note: tokenNote.value
    })
    if (res.data?.status === 200) {
      tokenCreated.value = res.data.data
      ElMessage.success(isZh.value ? '门禁令牌已生成' : 'Access token generated')
      await loadTokens()
    } else {
      ElMessage.error(res.data?.message || (isZh.value ? '生成令牌失败' : 'Failed to generate token'))
    }
  } catch {
    ElMessage.error(isZh.value ? '生成令牌失败' : 'Failed to generate token')
  } finally {
    tokenCreating.value = false
  }
}

const deleteToken = (row: any) => {
  ElMessageBox.confirm(isZh.value ? '确认删除该门禁令牌？删除后此令牌立即作废。' : 'Delete this access token? It becomes invalid immediately.', isZh.value ? '删除令牌' : 'Delete Token', { type: 'warning' })
    .then(async () => {
      try {
        const res = await ClientAPI.DeleteStagerToken({ id: row.id })
        if (res.data?.status === 200) {
          ElMessage.success(isZh.value ? '令牌已删除' : 'Token deleted')
          await loadTokens()
        } else {
          ElMessage.error(res.data?.message || (isZh.value ? '删除失败' : 'Delete failed'))
        }
      } catch {
        ElMessage.error(isZh.value ? '删除失败' : 'Delete failed')
      }
    })
    .catch(() => {})
}

const showLogs = async (row: any) => {
  const res = await ClientAPI.GetDeliveryAccessLogs({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    limit: 200,
  })
  if (res.status === 200 && res.data.status === 200) {
    accessLogs.value = res.data.data || []
    logsDialogVisible.value = true
  } else {
    ElMessage.error(isZh.value ? '获取日志失败' : 'Failed to load logs')
  }
}

const copyText = async (text: string) => {
  if (!text) {
    ElMessage.warning(isZh.value ? '内容为空' : 'Nothing to copy')
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(isZh.value ? '已复制' : 'Copied')
  } catch {
    ElMessage.error(isZh.value ? '复制失败' : 'Copy failed')
  }
}

const copyUrl = (row: any) => {
  const url = field(row, 'ServerAddress', 'server_address')
  if (!url) {
    ElMessage.warning(isZh.value ? '无 URL' : 'No URL')
    return
  }
  copyText(url)
}

const openShellcodeDialog = (row: any) => {
  lis.value = field(row, 'ListenerConfig', 'listener_config')
  // 兼容旧 StageShellCodeGen：使用 ListeningPort 作为 port
  port.value = String(rowPort(row) || '')
  dialogVisible2.value = true
}

const openDynamicDialog = (row: any) => {
  lis.value = field(row, 'ListenerConfig', 'listener_config')
  port.value = String(rowPort(row) || '')
  const os = field(row, 'OS', 'os')
  dynamicStagerForm.format = os === 'linux' || os === 'darwin' ? 'elf' : 'exe'
  stagerInfo.value = null
  dialogVisible3.value = true
  if (port.value) fetchStagerBuildInfo()
}

// 查询 Stager 构建信息（后端 /shellcode/stager-info?port=...）
const fetchStagerBuildInfo = async () => {
  if (!port.value) {
    ElMessage.warning(isZh.value ? '请先选择 WebDelivery' : 'Select a WebDelivery first')
    return
  }
  stagerInfoLoading.value = true
  try {
    const res = await ClientAPI.GetStagerBuildInfo(port.value)
    if (res.data?.status === 200) {
      stagerInfo.value = res.data.data || null
    } else {
      stagerInfo.value = null
      ElMessage.warning(res.data?.data || (isZh.value ? '暂无构建信息' : 'No build info available'))
    }
  } catch (e: any) {
    stagerInfo.value = null
    ElMessage.error(e?.message || (isZh.value ? '获取构建信息失败' : 'Failed to load build info'))
  } finally {
    stagerInfoLoading.value = false
  }
}

const downloadBlobRes = (res: any, fallback: string) => {
  let fileName = fallback
  const cd = res.headers?.['content-disposition'] || ''
  const m = /filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/.exec(cd)
  if (m && m[1]) fileName = decodeURIComponent(m[1].replace(/['"]/g, ''))
  const blob = new Blob([res.data], {
    type: res.headers?.['content-type'] || 'application/octet-stream',
  })
  const a = document.createElement('a')
  const href = URL.createObjectURL(blob)
  a.href = href
  a.download = fileName
  a.click()
  URL.revokeObjectURL(href)
}

const handleStageShellcode = async () => {
  try {
    const res = await ClientAPI.StageShellCodeGen({
      listener: lis.value,
      port: port.value,
      format: selectedFormat.format,
    })
    if (res.status === 200) {
      downloadBlobRes(res, `stage.${selectedFormat.format}`)
      ElMessage.success(isZh.value ? '生成成功' : 'Generated')
      dialogVisible2.value = false
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '生成失败' : 'Generation failed'))
  }
}

const handleDynamicStager = async () => {
  dynamicGenerating.value = true
  try {
    const res = await ClientAPI.DynamicStageShellCodeGen({
      listener: lis.value,
      port: port.value,
      format: dynamicStagerForm.format,
      anti_debug: dynamicStagerForm.anti_debug,
      obfuscation: dynamicStagerForm.obfuscation,
      user_agent: dynamicStagerForm.user_agent,
      lightweight: dynamicStagerForm.lightweight,
    })
    if (res.status === 200) {
      downloadBlobRes(res, `dynamic_stager.${dynamicStagerForm.format}`)
      ElMessage.success(isZh.value ? '生成成功' : 'Generated')
      dialogVisible3.value = false
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '生成失败（动态 Stager 需要服务端 Go 环境）' : 'Generation failed (dynamic Stager needs Go on the server)'))
  } finally {
    dynamicGenerating.value = false
  }
}

onMounted(() => {
  getWebDeliveryList()
  loadDeliveryDefaults()
})
</script>

<style scoped>
.delivery-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.wd-toolbar {
  min-height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.wd-title-group {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.wd-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.wd-count {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.wd-spacer {
  flex: 1;
}

.wd-table-wrap {
  flex: 1;
  overflow: hidden;
  margin: 16px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.mb-16 {
  margin-bottom: 16px;
}
.mb-12 {
  margin-bottom: 12px;
}
.mono {
  font-family: var(--cs-font-mono);
  font-size: 13px;
}
.sub {
  font-size: 12px;
  color: var(--cs-text-dim);
}
.stager-info-box {
  background: var(--cs-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--cs-text);
  margin-bottom: 8px;
  max-width: 460px;
}
.stager-info-box b {
  color: var(--cs-text-bright);
}
.si-loading,
.si-empty {
  font-size: 12px;
  color: var(--cs-text-dim);
}
.text-danger {
  color: var(--cs-error);
}
.hint {
  margin-left: 8px;
  color: var(--cs-text-dim);
  font-size: 12px;
}
.url-box {
  background: var(--cs-console);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 13px;
}
.url-box code {
  font-size: 13px;
  font-family: var(--cs-font-mono);
  color: var(--cs-active);
  word-break: break-all;
}
.cmd {
  display: block;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  line-height: 1.5;
  background: var(--cs-console);
  color: var(--cs-active);
  padding: 6px 10px;
  border-radius: 6px;
  border: 1px solid var(--cs-card-border);
}
</style>
<style scoped>
.wd-expand-detail {
  background: var(--cs-panel);
  padding: 12px 16px;
  border-bottom: 1px solid var(--cs-card-border);
}
.wd-detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 8px 16px;
  font-size: 13px;
  color: var(--cs-text);
}
.wd-detail-item b {
  color: var(--cs-text-bright);
  font-weight: 600;
}

.wd-action-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  width: 100%;
}
</style>

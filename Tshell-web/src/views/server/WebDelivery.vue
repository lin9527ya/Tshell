<template>
  <div class="delivery-page cs-font-ui">
    <!-- CS Toolbar -->
    <div class="wd-toolbar">
      <div class="wd-title-group">
        <span class="wd-title">{{ isZh ? '站点管理与 Web 投递' : 'Hosted Sites & Payloads' }}</span>
        <span class="wd-count">({{ tableData.length }} {{ isZh ? '个活跃站点' : 'sites active' }})</span>
      </div>

      <div class="wd-spacer" />

      <button class="cs-btn cs-btn-primary" @click="openCreate">
        {{ isZh ? '托管文件 / 新增站点' : 'Host File / Add Site' }}
      </button>
      <button class="cs-btn" @click="getWebDeliveryList">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
    </div>

    <div class="wd-table-wrap">
    <el-table :data="tableData" row-key="id" stripe style="width: 100%; margin-top: 12px" @selection-change="handleSelectionChange">
      <el-table-column type="selection" width="40" :reserve-selection="true" />
      <el-table-column type="expand" width="34">
        <template #default="{ row }">
          <div class="wd-expand-detail cs-font-ui">
            <div class="wd-detail-grid">
              <div class="wd-detail-item"><b>{{ isZh ? '绑定模式' : 'Bind Mode' }}:</b> {{ modeOf(row) }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '公网主机' : 'Public Host' }}:</b> {{ field(row, 'PublicHost', 'public_host') || '-' }}</div>
              <div class="wd-detail-item"><b>Scheme:</b> {{ field(row, 'PublicScheme', 'public_scheme') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '路径前缀' : 'Base Path' }}:</b> {{ field(row, 'BasePath', 'base_path') || '-' }}</div>
              <div class="wd-detail-item"><b>URI:</b> {{ field(row, 'URIPath', 'uri_path') || '-' }}</div>
              <div class="wd-detail-item"><b>Content-Type:</b> {{ field(row, 'ContentType', 'content_type') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? 'Stager 内容类型' : 'Stage CT' }}:</b> {{ field(row, 'StageContentType', 'stage_content_type') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '访问令牌' : 'Access Token' }}:</b> {{ field(row, 'AccessToken', 'access_token') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? 'UA 白名单' : 'UA Allow' }}:</b> {{ field(row, 'UAAllow', 'ua_allow') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '最大下载次数' : 'Max Downloads' }}:</b> {{ field(row, 'MaxDownloads', 'max_downloads') || 0 }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '一次性链接' : 'One Time' }}:</b> {{ boolOf(row, 'OneTime', 'one_time') ? (isZh ? '是' : 'Yes') : (isZh ? '否' : 'No') }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '命中次数' : 'Hit Count' }}:</b> {{ field(row, 'HitCount', 'hit_count') || 0 }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? 'Stager 构建次数' : 'Stager Build Count' }}:</b> {{ field(row, 'BuildCount', 'build_count') || 0 }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '最后命中 IP' : 'Last Hit IP' }}:</b> {{ field(row, 'LastHitIP', 'last_hit_ip') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '最后命中时间' : 'Last Hit At' }}:</b> {{ field(row, 'LastHitAt', 'last_hit_at') || '-' }}</div>
              <div class="wd-detail-item"><b>{{ isZh ? '备注' : 'Note' }}:</b> {{ field(row, 'Note', 'note') || '-' }}</div>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="模式" width="100">
        <template #default="{ row }">
          <el-tag :type="modeOf(row) === 'dedicated' ? 'warning' : 'success'" size="small">
            {{ modeOf(row) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="isZh ? '密钥 / 端口' : 'Key / Port'" min-width="140">
        <template #default="{ row }">
          <div class="mono">{{ rowKey(row) || '-' }}</div>
          <div class="sub" v-if="rowPort(row)">{{ isZh ? '端口' : 'port' }}: {{ rowPort(row) }}</div>
        </template>
      </el-table-column>
      <el-table-column label="监听器" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ field(row, 'ListenerConfig', 'listener_config') }}</template>
      </el-table-column>
      <el-table-column label="目标" width="130">
        <template #default="{ row }">
          {{ field(row, 'OS', 'os') }}_{{ field(row, 'Arch', 'arch') }}
        </template>
      </el-table-column>
      <el-table-column label="命中" width="90">
        <template #default="{ row }">
          {{ field(row, 'HitCount', 'hit_count') || 0 }}
          <div class="sub" v-if="field(row, 'LastHitIP', 'last_hit_ip')">
            {{ field(row, 'LastHitIP', 'last_hit_ip') }}
          </div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="statusOf(row) === 1 ? 'success' : 'danger'" size="small">
            {{ statusOf(row) === 1 ? '开启' : '关闭' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right" align="center">
        <template #default="{ row }">
          <div class="wd-action-row">
            <button
              class="cs-btn cs-btn-xs cs-btn-primary"
              :disabled="statusOf(row) === 2"
              :title="isZh ? '查看上线命令' : 'Show Commands'"
              @click="showCommands(row)"
            >
              {{ isZh ? '上线命令' : 'Commands' }}
            </button>
            <button
              class="cs-btn cs-btn-xs"
              :title="isZh ? '复制投递 URL' : 'Copy Delivery URL'"
              @click="copyUrl(row)"
            >
              {{ isZh ? '复制URL' : 'Copy' }}
            </button>
            <el-dropdown trigger="click" @command="(cmd: string) => handleMoreAction(cmd, row)">
              <button class="cs-btn cs-btn-xs" :title="isZh ? '更多操作' : 'More Actions'">
                ⋮
              </button>
              <template #dropdown>
                <el-dropdown-menu class="cs-menu-popper">
                  <el-dropdown-item v-if="statusOf(row) !== 1" command="open">{{ isZh ? '开启服务' : 'Start' }}</el-dropdown-item>
                  <el-dropdown-item v-if="statusOf(row) === 1" command="close">{{ isZh ? '关闭服务' : 'Stop' }}</el-dropdown-item>
                  <el-dropdown-item command="tokens">{{ isZh ? '门禁令牌' : 'Access Tokens' }}</el-dropdown-item>
                  <el-dropdown-item command="logs">{{ isZh ? '访问日志' : 'Logs' }}</el-dropdown-item>
                  <el-dropdown-item v-if="field(row, 'OS', 'os') === 'windows' && statusOf(row) === 1" command="shellcode">Shellcode</el-dropdown-item>
                  <el-dropdown-item v-if="statusOf(row) === 1" command="dynamic">{{ isZh ? '动态 Stager' : 'Dynamic Stager' }}</el-dropdown-item>
                  <el-dropdown-item v-if="statusOf(row) === 1" command="script">{{ isZh ? '脚本载荷' : 'Script Payload' }}</el-dropdown-item>
                  <el-dropdown-item command="delete" divided class="text-danger">{{ isZh ? '删除' : 'Delete' }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="wd-bottom-bar cs-font-ui">
      <button class="cs-btn cs-btn-primary" @click="openCreate">
        {{ isZh ? '托管文件' : 'Host File' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="selectedRow && copyUrl(selectedRow)">
        {{ isZh ? '复制 URL' : 'Copy URL' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow || statusOf(selectedRow) === 1" @click="selectedRow && handleOpen(selectedRow)">
        {{ isZh ? '启动' : 'Launch' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow || statusOf(selectedRow) === 2" @click="selectedRow && handleClose(selectedRow)">
        {{ isZh ? '停止' : 'Kill / Stop' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="selectedRow && handleDelete(selectedRow)">
        {{ isZh ? '删除' : 'Remove' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- 创建 Delivery Hub -->
    <el-dialog v-model="dialogVisible" title="配置 Delivery Hub" width="720px" destroy-on-close>
      <el-form :model="formData" label-width="130px">
        <el-form-item label="绑定模式">
          <el-radio-group v-model="formData.bindMode">
            <el-radio label="shared">shared（主端口 /d/key，推荐）</el-radio>
            <el-radio label="dedicated">dedicated（独立端口）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="监听器" required>
          <el-select
            v-model="formData.listener"
            placeholder="选择监听器"
            style="width:100%"
            @visible-change="handleDropdown"
          >
            <el-option v-for="item in options" :key="item" :label="item" :value="item" />
          </el-select>
        </el-form-item>
        <el-form-item label="操作系统" required>
          <el-radio-group v-model="formData.os" @change="onOSChange">
            <el-radio label="windows">windows</el-radio>
            <el-radio label="linux">linux</el-radio>
            <el-radio label="darwin">darwin</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="架构" required>
          <el-radio-group v-model="formData.arch">
            <el-radio label="amd64">amd64</el-radio>
            <el-radio label="386">386</el-radio>
            <el-radio label="arm64">arm64</el-radio>
            <el-radio label="arm">arm</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="上线口令">
          <el-input v-model="formData.pass" show-password placeholder="反沙箱启动口令 / Exec Key" />
        </el-form-item>
        <el-form-item label="文件名">
          <el-input v-model="formData.filename" placeholder="托管文件名，如 svchost.exe" />
        </el-form-item>
        <el-form-item v-if="formData.os === 'windows'" label="载荷格式">
          <el-radio-group v-model="formData.payloadFormat" @change="onFormatChange">
            <el-radio label="exe">exe（可执行文件）</el-radio>
            <el-radio label="dll">dll（rundll32 / regsvr32 加载）</el-radio>
          </el-radio-group>
          <span class="hint" v-if="formData.payloadFormat === 'dll'">
            rundll32.exe client.dll,Run 或 regsvr32.exe client.dll 加载
          </span>
        </el-form-item>
        <el-form-item label="保存文件名">
          <el-input v-model="formData.saveFilename" placeholder="目标机落地文件名" />
        </el-form-item>
        <el-form-item label="下载路径">
          <el-input v-model="formData.downloadPath" />
        </el-form-item>
        <el-form-item v-if="formData.bindMode === 'dedicated'" label="独立端口" required>
          <el-input v-model="formData.port" placeholder="如 8000" />
        </el-form-item>

        <el-divider content-position="left">伪装与公开地址</el-divider>
        <el-form-item :label="isZh ? '公网主机' : 'Public Host'">
          <el-input v-model="formData.publicHost" placeholder="留空自动推断，如 1.2.3.4:8089" />
        </el-form-item>
        <el-form-item :label="isZh ? '协议' : 'Scheme'">
          <el-radio-group v-model="formData.publicScheme">
            <el-radio label="">默认</el-radio>
            <el-radio label="http">http</el-radio>
            <el-radio label="https">https</el-radio>
          </el-radio-group>
          <span class="hint">接 CDN 时选 https；空则用 config delivery.default_scheme</span>
        </el-form-item>
        <el-form-item :label="isZh ? '路径前缀' : 'Base Path'">
          <el-input v-model="formData.basePath" placeholder="CDN 路径前缀，如 /static 或留空" />
        </el-form-item>
        <el-form-item label="URL 预览">
          <div class="url-box" v-if="previewUrls">
            <div><b>{{ isZh ? '完整' : 'Full' }}:</b> <code>{{ previewUrls.full }}</code></div>
            <div v-if="previewUrls.stager"><b>{{ isZh ? '分阶段' : 'Stager' }}:</b> <code>{{ previewUrls.stager }}</code></div>
            <div v-if="previewUrls.shellcode"><b>{{ isZh ? 'Shellcode' : 'SC' }}:</b> <code>{{ previewUrls.shellcode }}</code></div>
          </div>
          <el-button size="small" @click="doPreviewUrls" style="margin-top:6px">刷新预览</el-button>
        </el-form-item>
        <el-form-item label="URI 路径">
          <el-input v-model="formData.uriPath" placeholder="可选伪装路径；空则用随机 key" />
        </el-form-item>
        <el-form-item :label="isZh ? '内容类型' : 'Content-Type'">
          <el-select v-model="formData.contentType" filterable allow-create style="width:100%">
            <el-option label="application/octet-stream" value="application/octet-stream" />
            <el-option label="application/javascript" value="application/javascript" />
            <el-option label="font/woff" value="font/woff" />
            <el-option label="image/png" value="image/png" />
            <el-option label="text/css" value="text/css" />
          </el-select>
        </el-form-item>
        <el-form-item :label="isZh ? 'Stager 内容类型' : 'Stage CT'">
          <el-select v-model="formData.stageContentType" filterable allow-create style="width:100%">
            <el-option label="font/woff" value="font/woff" />
            <el-option label="font/woff2" value="font/woff2" />
            <el-option label="application/octet-stream" value="application/octet-stream" />
          </el-select>
        </el-form-item>

        <el-divider content-position="left">防护</el-divider>
        <el-form-item :label="isZh ? '访问令牌' : 'Access Token'">
          <el-input
            v-model="formData.accessToken"
            placeholder="可选；请求需 ?t=token 或 Header X-Delivery-Token"
          />
        </el-form-item>
        <el-form-item label="UA 白名单">
          <el-input v-model="formData.uaAllow" placeholder="逗号分隔子串，如 curl,Wget,PowerShell" />
        </el-form-item>
        <el-form-item label="最大下载次数">
          <el-input-number v-model="formData.maxDownloads" :min="0" />
          <span class="hint">0 = 不限制</span>
        </el-form-item>
        <el-form-item label="一次性链接">
          <el-switch v-model="formData.oneTime" />
        </el-form-item>
        <el-form-item label="构建 Stager">
          <el-switch v-model="formData.buildStager" />
          <span class="hint">需要服务端 Go 工具链</span>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="formData.note" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="handleCreate">创建并启动</el-button>
      </template>
    </el-dialog>

    <!-- 上线命令矩阵 -->
    <el-dialog v-model="commandDialogVisible" title="上线命令矩阵" width="860px">
      <div v-if="commandUrls" class="url-box mb-12">
        <div>
          <b>{{ isZh ? '完整' : 'Full' }}:</b> <code>{{ commandUrls.full }}</code>
          <el-button link type="primary" @click="copyText(commandUrls.full)">复制</el-button>
        </div>
        <div v-if="commandUrls.stager">
          <b>{{ isZh ? '分阶段' : 'Stager' }}:</b> <code>{{ commandUrls.stager }}</code>
          <el-button link type="primary" @click="copyText(commandUrls.stager)">复制</el-button>
        </div>
        <div v-if="commandUrls.shellcode">
          <b>{{ isZh ? 'Shellcode/WOFF' : 'SC/WOFF' }}:</b> <code>{{ commandUrls.shellcode }}</code>
          <el-button link type="primary" @click="copyText(commandUrls.shellcode)">复制</el-button>
        </div>
      </div>
      <el-table :data="commandList" size="small" max-height="480">
        <el-table-column prop="name" label="名称" width="180" />
        <el-table-column prop="os" label="OS" width="90" />
        <el-table-column prop="description" label="说明" width="180" show-overflow-tooltip />
        <el-table-column prop="code" label="命令" min-width="320">
          <template #default="{ row }">
            <code class="cmd">{{ row.code }}</code>
          </template>
        </el-table-column>
        <el-table-column label="" width="80">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="copyText(row.code)">复制</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 访问日志 -->
    <el-dialog v-model="logsDialogVisible" title="投递访问日志" width="900px">
      <el-table :data="accessLogs" size="small" max-height="500">
        <el-table-column prop="created_at" label="时间" width="170" />
        <el-table-column prop="resource" label="资源" width="90" />
        <el-table-column prop="client_ip" label="IP" width="130" />
        <el-table-column prop="status_code" label="状态" width="80" />
        <el-table-column prop="bytes_sent" label="字节" width="100" />
        <el-table-column prop="user_agent" label="UA" min-width="240" show-overflow-tooltip />
      </el-table>
    </el-dialog>

    <!-- StageShellcode（兼容旧接口，port 使用 ListeningPort） -->
    <el-dialog v-model="dialogVisible2" title="StageShellcode">
      <el-form :model="selectedFormat" label-width="120px">
        <el-form-item label="输出格式">
          <el-select v-model="selectedFormat.format">
            <el-option label="十六进制 (hex)" value="hex" />
            <el-option label="C 数组 (c)" value="c" />
            <el-option label="二进制 (bin)" value="bin" />
            <el-option label="原始二进制 (raw)" value="raw" />
            <el-option label="可执行 (exe)" value="exe" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleStageShellcode">生成</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>

    <!-- 动态 Stager（兼容旧接口，port 使用 ListeningPort） -->
    <el-dialog v-model="dialogVisible3" title="动态 Stager 生成" width="640px">
      <el-form :model="dynamicStagerForm" label-width="120px">
        <el-form-item label="格式">
          <el-select v-model="dynamicStagerForm.format">
            <el-option label="exe" value="exe" />
            <el-option label="elf" value="elf" />
            <el-option label="raw" value="raw" />
            <el-option label="bin" value="bin" />
            <el-option label="hex" value="hex" />
            <el-option label="c" value="c" />
          </el-select>
        </el-form-item>
        <el-form-item label="反调试">
          <el-switch v-model="dynamicStagerForm.anti_debug" />
        </el-form-item>
        <el-form-item label="混淆">
          <el-switch v-model="dynamicStagerForm.obfuscation" />
        </el-form-item>
        <el-form-item label="User-Agent">
          <el-input v-model="dynamicStagerForm.user_agent" placeholder="留空随机" />
        </el-form-item>
        <el-form-item label="轻量模式">
          <el-switch v-model="dynamicStagerForm.lightweight" />
        </el-form-item>
        <!-- Stager 构建信息（后端 /shellcode/stager-info?port=...） -->
        <el-form-item label="构建信息">
          <div class="stager-info-box">
            <div v-if="stagerInfoLoading" class="si-loading">{{ isZh ? '加载中...' : 'Loading...' }}</div>
            <template v-else-if="stagerInfo">
              <div><b>{{ isZh ? '端口' : 'Port' }}:</b> {{ stagerInfo.port || port }}</div>
              <div><b>{{ isZh ? '目标' : 'Target' }}:</b> {{ stagerInfo.os }}_{{ stagerInfo.arch }}</div>
              <div><b>URL:</b> {{ stagerInfo.server_url || '-' }}</div>
              <div><b>{{ isZh ? '最后构建' : 'Last Build' }}:</b> {{ stagerInfo.last_build || '-' }}</div>
              <div><b>{{ isZh ? '构建次数' : 'Build Count' }}:</b> {{ stagerInfo.build_count || 0 }}</div>
            </template>
            <div v-else class="si-empty">{{ isZh ? '选择 WebDelivery 后点击【查询构建信息】' : 'Select a WebDelivery and query build info' }}</div>
          </div>
          <el-button size="small" :loading="stagerInfoLoading" @click="fetchStagerBuildInfo">{{ isZh ? '查询构建信息' : 'Query Build Info' }}</el-button>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="dynamicGenerating" @click="handleDynamicStager">生成</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>

    <!-- 脚本载荷（按客户端 OS：Windows=ps1/vbs/bat/hta；Linux=sh） -->
    <el-dialog v-model="scriptDialogVisible" title="脚本载荷" width="720px">
      <el-form label-width="130px">
        <el-form-item label="形态">
          <el-radio-group v-model="scriptMode">
            <el-radio label="url">引用 URL（下载 /full 后执行）</el-radio>
            <el-radio label="embed">内嵌 base64（ps1 / bat / sh 支持）</el-radio>
          </el-radio-group>
        </el-form-item>
        <div v-if="scriptRows.length" class="url-box">
          <div v-for="s in scriptRows" :key="s.kind" style="margin-bottom: 8px">
            <b>{{ s.kind.toUpperCase() }}:</b> <code>{{ s.url }}</code>
            <el-button link type="primary" @click="copyText(s.url)">复制</el-button>
          </div>
        </div>
        <el-alert
          type="info"
          :closable="false"
          title="脚本运行时自动下载完整载荷并执行；embed 形态内嵌载荷字节，无需外部请求。"
        />
      </el-form>
    </el-dialog>

    <!-- 门禁令牌（防盗门）：StagerToken 配额管理 -->
    <el-dialog v-model="tokenDialogVisible" title="门禁令牌" width="720px">
      <div style="margin-bottom: 12px">
        <el-alert
          type="warning"
          :closable="false"
          show-icon
          title="为每个 Stager 生成独立令牌（带使用配额，一次一密）。Stager 下载时携带 X-Delivery-Token 头，配额用完即返回 404。"
        />
      </div>
      <el-table :data="tokenRows" size="small" stripe>
        <el-table-column prop="token" label="令牌" min-width="160">
          <template #default="{ row }"><code>{{ row.token }}</code></template>
        </el-table-column>
        <el-table-column label="配额" width="110">
          <template #default="{ row }">{{ row.used }}/{{ row.quota }}</template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="120">
          <template #default="{ row }">{{ row.note || '-' }}</template>
        </el-table-column>
        <el-table-column prop="last_used_ip" label="最后使用 IP" min-width="120">
          <template #default="{ row }">{{ row.last_used_ip || '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button size="small" type="danger" link @click="deleteToken(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="openTokenCreate">生成新令牌</el-button>
        <el-button type="primary" @click="tokenDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 生成令牌 -->
    <el-dialog v-model="tokenCreateVisible" title="生成门禁令牌" width="460px">
      <el-form label-width="90px">
        <el-form-item label="配额">
          <el-input-number v-model="tokenQuota" :min="1" :max="100" />
          <span style="margin-left: 8px; font-size: 12px; color: #909399">允许下载次数（默认 3）</span>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="tokenNote" placeholder="如：Stager-Win-x64-钓鱼" />
        </el-form-item>
        <el-form-item v-if="tokenCreated" label="生成结果">
          <div style="width: 100%">
            <div class="url-box" style="word-break: break-all">
              <code>{{ tokenCreated.token }}</code>
            </div>
            <div class="url-box" style="margin-top: 6px; word-break: break-all">
              <code>X-Delivery-Token: {{ tokenCreated.token }}</code>
              <el-button link type="primary" @click="copyText('X-Delivery-Token: ' + tokenCreated.token)">复制</el-button>
            </div>
            <div class="url-box" style="margin-top: 6px; word-break: break-all">
              <code>{{ tokenCreated.curl }}</code>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tokenCreateVisible = false">关闭</el-button>
        <el-button type="primary" :loading="tokenCreating" @click="createToken">生成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { Plus, Refresh } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import PageHeader from '@/components/Common/PageHeader.vue'

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
      ElMessage.error(res.data?.data || '获取列表失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '获取列表失败')
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
        ElMessage.warning('暂无运行中的监听器，请先启动监听器')
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
        ElMessage.warning('暂无运行中的监听器，请先启动监听器')
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
    ElMessage.error(e?.message || 'URL 预览失败')
  }
}

const openCreate = () => {
  dialogVisible.value = true
  handleDropdown(true)
  loadDeliveryDefaults()
  doPreviewUrls()
}

const handleCreate = async () => {
  if (!formData.listener || !formData.os || !formData.arch) {
    ElMessage.warning('请填写监听器 / OS / Arch')
    return
  }
  if (formData.bindMode === 'dedicated' && !formData.port) {
    ElMessage.warning('dedicated 模式必须填写端口')
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
      ElMessage.success('投递已创建')
      dialogVisible.value = false
      await getWebDeliveryList()
      const data = res.data.data
      if (data?.commands) {
        commandList.value = data.commands
        commandUrls.value = data.urls
        commandDialogVisible.value = true
      }
    } else {
      ElMessage.error(res.data?.data || res.data?.error || '创建失败')
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
    ElMessage.success('已开启')
    await getWebDeliveryList()
  } else {
    ElMessage.error(res.data?.data || '开启失败')
  }
}

const handleClose = async (row: any) => {
  const res = await ClientAPI.CloseWebDelivery({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    port: rowPort(row) || undefined,
  })
  if (res.status === 200 && res.data.status === 200) {
    ElMessage.success('已关闭')
    await getWebDeliveryList()
  } else {
    ElMessage.error(res.data?.data || '关闭失败')
  }
}

const handleDelete = async (row: any) => {
  try {
    await ElMessageBox.confirm('确认删除该投递？', '提示', { type: 'warning' })
  } catch {
    return
  }
  const res = await ClientAPI.DeleteWebDelivery({
    id: rowId(row) || undefined,
    publicKey: rowKey(row) || undefined,
    port: rowPort(row) || undefined,
  })
  if (res.status === 200 && res.data.status === 200) {
    ElMessage.success('已删除')
    await getWebDeliveryList()
  } else {
    ElMessage.error(res.data?.data || '删除失败')
  }
}

// 批量选择（参考客户端管理）
const selectedRows = ref<any[]>([])
const handleSelectionChange = (rows: any[]) => { selectedRows.value = rows }
const clearSelection = () => { selectedRows.value = [] }

const handleBatchDelete = async () => {
  try {
    await ElMessageBox.confirm(`确认删除选中的 ${selectedRows.value.length} 个投递？`, '批量删除', { type: 'warning' })
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
    ElMessage.success('批量删除成功')
  } else {
    ElMessage.error(`${failed} 个投递删除失败`)
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
    ElMessage.error(res.data?.data || '获取命令失败')
  }
}

const openScriptDialog = (row: any) => {
  const base = field(row, 'ServerAddress', 'server_address')
  if (!base) {
    ElMessage.warning('该投递没有公开 URL，无法生成脚本（shared 模式才有）')
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
    ElMessage.warning('该投递没有 public_key')
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
      ElMessage.success('门禁令牌已生成')
      await loadTokens()
    } else {
      ElMessage.error(res.data?.message || '生成令牌失败')
    }
  } catch {
    ElMessage.error('生成令牌失败')
  } finally {
    tokenCreating.value = false
  }
}

const deleteToken = (row: any) => {
  ElMessageBox.confirm('确认删除该门禁令牌？删除后此令牌立即作废。', '删除令牌', { type: 'warning' })
    .then(async () => {
      try {
        const res = await ClientAPI.DeleteStagerToken({ id: row.id })
        if (res.data?.status === 200) {
          ElMessage.success('令牌已删除')
          await loadTokens()
        } else {
          ElMessage.error(res.data?.message || '删除失败')
        }
      } catch {
        ElMessage.error('删除失败')
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
    ElMessage.error('获取日志失败')
  }
}

const copyText = async (text: string) => {
  if (!text) {
    ElMessage.warning('内容为空')
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}

const copyUrl = (row: any) => {
  const url = field(row, 'ServerAddress', 'server_address')
  if (!url) {
    ElMessage.warning('无 URL')
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
      ElMessage.success('生成成功')
      dialogVisible2.value = false
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '生成失败')
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
      ElMessage.success('生成成功')
      dialogVisible3.value = false
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '生成失败（动态 Stager 需要服务端 Go 环境）')
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
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.wd-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.wd-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.wd-count {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.wd-spacer {
  flex: 1;
}

.wd-table-wrap {
  flex: 1;
  overflow: hidden;
}

.wd-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}
.mb-16 {
  margin-bottom: 16px;
}
.mb-12 {
  margin-bottom: 12px;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}
.sub {
  font-size: 11px;
  color: #909399;
}
.stager-info-box {
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  padding: 6px 10px;
  font-size: 12px;
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
  font-size: 11px;
  color: var(--cs-text-dim);
}
.hint {
  margin-left: 8px;
  color: #909399;
  font-size: 12px;
}
.url-box {
  background: #0d1117;
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  padding: 10px 14px;
  font-size: 15px;
}
.url-box code {
  font-size: 14.5px;
  font-family: var(--cs-font-mono);
  color: #58a6ff;
  word-break: break-all;
}
.cmd {
  display: block;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: var(--cs-font-mono);
  font-size: 14px;
  line-height: 1.5;
  background: #0d1117;
  color: #58a6ff;
  padding: 6px 10px;
  border-radius: 3px;
  border: 1px solid var(--cs-border);
}
</style>
<style scoped>
.wd-expand-detail {
  background: var(--cs-panel);
  padding: 8px 12px;
  border-bottom: 1px solid var(--cs-border);
}
.wd-detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 6px 16px;
  font-size: 12px;
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

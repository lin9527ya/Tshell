<template>
  <div class="cs-file-manager cs-font-ui" @click="handleGlobalClick">
    <!-- 主体双栏区域 -->
    <div class="fm-main-container">
      <!-- 左侧文件树导航栏 -->
      <div class="fm-sidebar" :style="{ width: sidebarWidth + 'px' }">
        <div class="fm-sidebar-header">
          <span class="fm-sidebar-title">
            <el-icon class="folder-gold"><Folder /></el-icon>
            {{ isZh ? '文件系统' : 'Filesystem' }}
          </span>
          <div class="fm-tree-btns">
            <button class="ic-btn" :title="isZh ? '展开全部目录' : 'Expand All'" @click.stop="expandAllTree">
              <el-icon><FolderOpened /></el-icon>
            </button>
            <button class="ic-btn" :title="isZh ? '折叠全部目录' : 'Collapse All'" @click.stop="collapseAllTree">
              <el-icon><Fold /></el-icon>
            </button>
          </div>
        </div>

        <div class="fm-tree-search-wrap">
          <el-input
            v-model="treeSearchText"
            :placeholder="isZh ? '搜索目录...' : 'Filter directories...'"
            :prefix-icon="Search"
            clearable
            class="fm-search-input"
          />
        </div>

        <div class="fm-tree-content">
          <el-tree
            ref="fileTreeRef"
            :data="filteredFileTree"
            :props="treeProps"
            node-key="path"
            :expand-on-click-node="false"
            :highlight-current="true"
            :default-expanded-keys="expandedKeys"
            :filter-node-method="filterNode"
            class="fm-file-tree"
            @node-click="handleTreeNodeClick"
            @node-expand="handleNodeExpand"
            @node-collapse="handleNodeCollapse"
          >
            <template #default="{ data }">
              <div class="fm-tree-node" :class="{ active: data.path === currentPath }">
                <el-icon class="fm-node-icon">
                  <Monitor v-if="data.type === 'group'" />
                  <Loading v-else-if="loadingNodes.has(data.path)" class="is-loading" />
                  <Folder v-else class="folder-gold" />
                </el-icon>
                <span class="fm-node-label" :title="data.name">{{ data.name }}</span>
              </div>
            </template>
          </el-tree>
        </div>
      </div>

      <!-- Resizer handle -->
      <div class="fm-sidebar-resizer" @mousedown="startResizeSidebar" />

      <!-- 右侧内容区域 -->
      <div class="fm-content">
        <!-- 顶部工具栏 -->
        <div class="fm-toolbar">
          <!-- 导航按键 -->
          <div class="fm-nav-group">
            <button class="ic-btn" :title="isZh ? '刷新目录' : 'Refresh'" @click="refreshCurrentFolder">
              <el-icon><Refresh /></el-icon>
            </button>
            <button class="ic-btn" :title="isZh ? '磁盘驱动器 / 根目录' : 'Drives / Home'" @click="goClientHome">
              <el-icon><HomeFilled /></el-icon>
            </button>
            <button class="ic-btn" :disabled="!canGoUp" :title="isZh ? '上一级目录' : 'Up'" @click="goUpLevel">
              <el-icon><Top /></el-icon>
            </button>
          </div>

          <!-- 路径导航与面包屑 -->
          <div class="fm-path-container">
            <div v-if="!isPathEditMode" class="fm-breadcrumbs-bar cs-font-mono" @click="isPathEditMode = true">
              <el-icon class="folder-gold fm-path-icon"><FolderOpened /></el-icon>
              <span class="crumb-root" :title="isZh ? '点击返回根目录' : 'Root'" @click.stop="goClientHome">/</span>
              <template v-for="seg in pathSegments" :key="seg.fullPath">
                <span class="crumb-sep">/</span>
                <span class="crumb-item" :title="seg.fullPath" @click.stop="jumpToSegment(seg.fullPath)">{{ seg.name }}</span>
              </template>
              <el-icon class="crumb-edit-icon" :title="isZh ? '点击手动输入路径' : 'Click to type path'"><Edit /></el-icon>
            </div>
            <el-input
              v-else
              ref="pathInputRef"
              v-model="currentPath"
              class="fm-path-el-input cs-font-mono"
              :placeholder="isZh ? '输入文件路径，按回车或点击前往切换目录...' : 'Enter file path and press Enter...'"
              clearable
              @blur="isPathEditMode = false"
              @keydown.enter="navigateToInputPath(); isPathEditMode = false"
              @keydown.esc="isPathEditMode = false"
            >
              <template #prefix>
                <el-icon class="folder-gold"><FolderOpened /></el-icon>
              </template>
              <template #append>
                <button class="cs-btn cs-btn-primary cs-btn-xs fm-go-btn" @click="navigateToInputPath(); isPathEditMode = false">
                  {{ isZh ? '前往' : 'Go' }}
                </button>
              </template>
            </el-input>
          </div>

          <!-- 快速过滤搜索:输入即过滤当前列表;回车在当前目录起递归搜索远端 -->
          <div class="fm-filter-wrap">
            <el-input
              v-model="tableSearchKeyword"
              :placeholder="isZh ? '过滤当前列表,回车递归搜索' : 'Filter list, Enter to search deep'"
              :prefix-icon="Search"
              clearable
              class="fm-table-search"
              :loading="deepSearching"
              @keydown.enter="deepSearch"
              @clear="onDeepSearchClear"
            >
              <template #suffix>
                <el-icon v-if="deepSearching" class="is-loading"><Loading /></el-icon>
              </template>
            </el-input>
          </div>

          <!-- 顶部快捷操作 -->
          <div class="fm-quick-actions">
            <button class="cs-btn cs-btn-primary" @click="createNewFolder">
              <el-icon><FolderAdd /></el-icon>
              <span>{{ isZh ? '新建文件夹' : 'New Folder' }}</span>
            </button>
            <button class="cs-btn cs-btn-primary" @click="createNewFile">
              <el-icon><DocumentAdd /></el-icon>
              <span>{{ isZh ? '新建文件' : 'New File' }}</span>
            </button>
            <button class="cs-btn cs-btn-primary" @click="showUploadDialog">
              <el-icon><Upload /></el-icon>
              <span>{{ isZh ? '上传文件' : 'Upload' }}</span>
            </button>
            <button class="cs-btn" @click="showWgetDialog">
              <el-icon><Download /></el-icon>
              <span>{{ isZh ? 'wget 下载' : 'wget File' }}</span>
            </button>
          </div>
        </div>

        <!-- 操作条（vshell：操作集中在表格上方，取代原底部按钮行） -->
        <div class="fm-action-bar">
          <button class="cs-btn" @click="fetchDrives">
            {{ isZh ? '驱动器 (Drives)' : 'Drives' }}
          </button>
          <button class="cs-btn" :disabled="selectedFiles.length === 0" @click="handleFileView()">
            {{ isZh ? '查看/编辑 (View)' : 'View' }}
          </button>
          <button class="cs-btn" :disabled="selectedFiles.length === 0" @click="handleFileDownload()">
            {{ isZh ? '下载 (Download)' : 'Download' }}
          </button>
          <button class="cs-btn" :disabled="selectedFiles.length === 0" @click="handleFileExecute()">
            {{ isZh ? '执行 (Execute)...' : 'Execute...' }}
          </button>
          <button class="cs-btn cs-btn-danger" :disabled="selectedFiles.length === 0" @click="deleteSelectedFiles">
            {{ isZh ? '删除 (Delete)' : 'Delete' }}
          </button>
          <button class="cs-btn" @click="handleHelp">
            {{ isZh ? '帮助 (Help)' : 'Help' }}
          </button>

          <div class="fm-spacer" />

          <span class="fm-status-info">
            {{ isZh ? `共 ${filteredFiles.length} 项 | 路径: ${currentPath || '/'}` : `${filteredFiles.length} items | ${currentPath || '/'}` }}
          </span>
        </div>

        <!-- 文件列表表格 -->
        <div class="fm-table-wrap" v-loading="treeLoading">
          <el-table
            ref="tableRef"
            :data="paginatedFiles"
            row-key="path"
            size="small"
            height="100%"
            highlight-current-row
            class="cs-session-table"
            :default-sort="{ prop: 'modifiedTime', order: 'descending' }"
            @selection-change="handleSelectionChange"
            @row-dblclick="handleRowDoubleClick"
            @row-contextmenu="handleRowContextMenu"
            @sort-change="handleSortChange"
          >
            <el-table-column type="selection" width="38" align="center" :reserve-selection="true" />

            <el-table-column prop="name" :label="isZh ? '文件名' : 'Name'" min-width="160" sortable show-overflow-tooltip>
              <template #default="{ row }">
                <div class="fm-file-cell">
                  <el-icon class="fm-item-icon" :class="getFileIconClass(row)">
                    <Folder v-if="row.type === 'D'" />
                    <Document v-else />
                  </el-icon>
                  <span class="fm-file-name" :class="getFileNameClass(row)">{{ row.name }}</span>
                </div>
              </template>
            </el-table-column>

            <el-table-column prop="size" :label="isZh ? '大小' : 'Size'" width="115" sortable align="right">
              <template #default="{ row }">
                <span class="fm-size-text cs-font-mono" :class="{ 'is-dir': row.type === 'D' }">
                  {{ row.type === 'D' ? '<DIR>' : formatFileSize(row.size) }}
                </span>
              </template>
            </el-table-column>

            <el-table-column prop="modifiedTime" :label="isZh ? '修改时间' : 'Modified Date'" width="230" sortable align="center">
              <template #default="{ row }">
                <span class="cs-font-mono text-dim fm-time-text">{{ formatDateTime(row) }}</span>
              </template>
            </el-table-column>

            <el-table-column prop="permissions" :label="isZh ? '权限' : 'Mode'" width="145" align="center">
              <template #default="{ row }">
                <span class="cs-font-mono text-dim fm-perm-text">{{ formatPermissions(row.permissions) }}</span>
              </template>
            </el-table-column>

            <template #empty>
              <div class="fm-empty">
                <el-icon class="fm-empty-icon"><FolderOpened /></el-icon>
                <div class="fm-empty-title">{{ isZh ? '当前目录无文件或尚未加载' : 'Directory Empty or Not Loaded' }}</div>
                <div class="fm-empty-desc">{{ isZh ? '点击【刷新】或在上方输入路径加载目标文件系统。' : 'Click Refresh or enter a path above to browse the target filesystem.' }}</div>
              </div>
            </template>
          </el-table>

          <div v-if="filteredFiles.length > PAGE_SIZE" class="fm-pager">
            <el-pagination
              v-model:current-page="currentPage"
              :page-size="PAGE_SIZE"
              :total="filteredFiles.length"
              layout="prev, pager, next"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- 右键菜单 (CS FlatLaf Context Menu) -->
    <Teleport to="body">
      <div
        v-if="contextMenuVisible"
        ref="contextMenuRef"
        class="cs-file-ctx-menu cs-font-ui"
        :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }"
        @click.stop
        @contextmenu.prevent
      >
        <div class="ctx-item" @click="handleFileView(contextMenuItem)">
          <span class="ctx-text">{{ contextMenuItem?.type === 'D' ? (isZh ? '进入目录 (O)' : 'Open (O)') : (isZh ? '查看 / 编辑文件 (V)' : 'View / Edit (V)') }}</span>
        </div>
        <div v-if="contextMenuItem?.type !== 'D'" class="ctx-item" @click="handleFileDownload(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '下载文件 (D)' : 'Download (D)' }}</span>
        </div>
        <div v-if="contextMenuItem?.type !== 'D'" class="ctx-item" @click="handleFileExecute(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '执行文件 (E)...' : 'Execute (E)...' }}</span>
        </div>
        <div class="ctx-item" @click="renameItem(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '重命名 (R)...' : 'Rename (R)...' }}</span>
        </div>
        <div class="ctx-item" @click="copyItem(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '复制到 (C)...' : 'Copy (C)...' }}</span>
        </div>
        <div class="ctx-item" @click="moveItem(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '移动到 (M)...' : 'Move (M)...' }}</span>
        </div>
        <div class="ctx-item" @click="copyPath(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '拷贝完整路径 (P)' : 'Copy Path (P)' }}</span>
        </div>
        <div class="ctx-item" @click="modifyTime(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '修改时间 (T)...' : 'Touch (T)...' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" @click="createNewFile">
          <span class="ctx-text">{{ isZh ? '新建文件 (N)...' : 'New File (N)...' }}</span>
        </div>
        <div class="ctx-item" @click="makeDirItem">
          <span class="ctx-text">{{ isZh ? '新建文件夹 (F)...' : 'Make Directory (F)...' }}</span>
        </div>
        <div class="ctx-item" @click="uploadToHere">
          <span class="ctx-text">{{ isZh ? '上传文件到此目录 (U)...' : 'Upload Here (U)...' }}</span>
        </div>
        <div class="ctx-item" @click="refreshCurrentFolder">
          <span class="ctx-text">{{ isZh ? '刷新当前目录 (R)' : 'Refresh (R)' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" @click="hideItem(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '隐藏此项' : 'Hide' }}</span>
        </div>
        <div class="ctx-item" @click="unhideItem(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '取消隐藏' : 'Unhide' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item ctx-danger" @click="deleteItem(contextMenuItem)">
          <span class="ctx-text">{{ isZh ? '删除 (Del)' : 'Delete (Del)' }}</span>
        </div>
      </div>
    </Teleport>

    <!-- 文件查看与编辑对话框 -->
    <el-dialog
      v-model="editDialogVisible"
      :title="`${isZh ? '文件查看/编辑: ' : 'Edit File: '}${editingFile?.name || ''}`"
      width="820px"
      :close-on-click-modal="false"
      class="cs-dialog"
      append-to-body
    >
      <div class="cs-dialog-body cs-font-ui" v-loading="editLoading">
        <div class="fm-editor-meta">
          <span><b>{{ isZh ? '路径: ' : 'Path: ' }}</b><code class="cs-font-mono">{{ editingFile?.path }}</code></span>
          <span><b>{{ isZh ? '大小: ' : 'Size: ' }}</b>{{ formatFileSize(editingFile?.size) }}</span>
        </div>
        <div class="fm-editor-box">
          <div ref="editorLineNumbers" class="fm-editor-lines cs-font-mono">
            <div v-for="n in editorLinesCount" :key="n" class="editor-line-num">{{ n }}</div>
          </div>
          <textarea
            ref="editorTextarea"
            v-model="fileContent"
            wrap="off"
            class="fm-editor-textarea cs-font-mono"
            spellcheck="false"
            :placeholder="isZh ? '文件内容为空...' : 'File is empty...'"
            @scroll="syncEditorScroll"
            @input="updateEditorLineNumbers"
          ></textarea>
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" :disabled="editLoading" @click="saveFile">{{ isZh ? '保存 (Save)' : 'Save' }}</button>
          <button class="cs-btn" @click="refreshEditContent">{{ isZh ? '重新读取' : 'Reload' }}</button>
          <button class="cs-btn" @click="editDialogVisible = false">{{ isZh ? '关闭' : 'Close' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- 执行文件对话框 -->
    <el-dialog
      v-model="executeDialogVisible"
      :title="isZh ? '执行文件 (Execute)' : 'Execute File'"
      width="480px"
      :close-on-click-modal="false"
      class="cs-dialog"
      append-to-body
    >
      <div class="cs-dialog-body cs-font-ui">
        <div class="cs-dialog-desc">
          {{ isZh ? '向目标 Beacon 下发执行指令，在宿主操作系统上调用运行指定程序。' : 'Task beacon to execute the selected binary/script on the target.' }}
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '程序路径:' : 'Program:' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="executeForm.path" readonly />
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '运行参数:' : 'Arguments:' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="executeForm.args" :placeholder="isZh ? '命令行参数（可选）' : 'CLI arguments (optional)'" />
          </div>
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="submitExecute">{{ isZh ? '执行' : 'Execute' }}</button>
          <button class="cs-btn" @click="executeDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- 文件上传对话框 -->
    <el-dialog
      v-model="uploadDialogVisible"
      :title="isZh ? '上传文件到目标机器' : 'Upload Files to Target'"
      width="560px"
      :close-on-click-modal="false"
      class="cs-dialog"
      append-to-body
    >
      <div class="cs-dialog-body cs-font-ui">
        <div class="fm-upload-path-hint">
          <b>{{ isZh ? '目标目录: ' : 'Target Dir: ' }}</b>
          <code class="cs-font-mono">{{ currentPath || '/' }}</code>
        </div>
        <el-upload
          ref="uploadDialogRef"
          v-model:file-list="uploadFileList"
          :auto-upload="false"
          :limit="10"
          drag
          multiple
          class="fm-upload-dragger"
        >
          <el-icon class="el-icon--upload"><Upload /></el-icon>
          <div class="el-upload__text">{{ isZh ? '将文件拖到此处，或点击选择' : 'Drop files here or click to select' }}</div>
        </el-upload>

        <div v-if="uploadFileList.length > 0" class="fm-upload-list">
          <div v-for="(f, i) in uploadFileList" :key="i" class="fm-upload-item">
            <span class="fm-upload-name">{{ f.name }}</span>
            <span class="fm-upload-size cs-font-mono">{{ formatUploadFileSize(f.size) }}</span>
            <button class="cs-btn cs-btn-xs cs-btn-danger" @click="uploadFileList.splice(i, 1)">{{ isZh ? '移除' : 'Remove' }}</button>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" :disabled="uploadFileList.length === 0" @click="startUpload">
            {{ isZh ? '开始上传' : 'Upload' }}
          </button>
          <button class="cs-btn" @click="closeUploadDialog">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- wget 远程下载对话框 -->
    <el-dialog
      v-model="wgetDialogVisible"
      :title="isZh ? 'wget 远程下载到目标机' : 'wget Download File'"
      width="500px"
      :close-on-click-modal="false"
      class="cs-dialog"
      append-to-body
    >
      <div class="cs-dialog-body cs-font-ui">
        <div class="cs-form-row">
          <label class="cs-form-label">URL:</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="wgetForm.url" placeholder="http://example.com/payload.exe" />
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '保存路径:' : 'Save Path:' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="wgetForm.path" :placeholder="currentPath" />
          </div>
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" :disabled="!wgetForm.url" @click="executeWget">{{ isZh ? '下载' : 'Download' }}</button>
          <button class="cs-btn" @click="wgetDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- 下载进度对话框 -->
    <el-dialog
      v-model="downloadDialogVisible"
      :title="isZh ? '文件下载进度' : 'Download Progress'"
      @close="stopDownloadPoll"
      width="480px"
      :close-on-click-modal="false"
      class="cs-dialog"
      append-to-body
    >
      <div class="cs-dialog-body cs-font-ui">
        <div class="fm-dl-file">
          <b>{{ isZh ? '下载文件: ' : 'File: ' }}</b><code>{{ currentDownloadFile?.name }}</code>
        </div>
        <el-progress
          :percentage="downloadProgress"
          :status="downloadStatus === 'success' ? 'success' : (downloadStatus === 'error' ? 'exception' : '')"
          :stroke-width="12"
        />
        <div v-if="downloadSpeed" class="fm-dl-speed">
          {{ isZh ? `下载速度: ${downloadSpeed}` : `Speed: ${downloadSpeed}` }}
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button v-if="downloadStatus === 'success'" class="cs-btn cs-btn-primary" @click="openDownloadFolder">
            {{ isZh ? '保存到本地' : 'Save Locally' }}
          </button>
          <button class="cs-btn" @click="downloadDialogVisible = false">{{ isZh ? '关闭' : 'Close' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- 帮助对话框 -->
    <el-dialog
      v-model="helpDialogVisible"
      :title="isZh ? '文件管理帮助' : 'File Browser Help'"
      width="460px"
      :close-on-click-modal="true"
      class="cs-dialog"
      append-to-body
    >
      <div class="cs-dialog-body cs-font-ui fm-help-body">
        <p><b>{{ isZh ? '刷新列表 (List):' : 'List:' }}</b> {{ isZh ? '获取并展示当前目录的所有文件与子目录。' : 'Query and list files in current directory.' }}</p>
        <p><b>{{ isZh ? '驱动器 (Drives):' : 'Drives:' }}</b> {{ isZh ? '列出目标系统的所有盘符与挂载点。' : 'Query all logical drives and mount points.' }}</p>
        <p><b>{{ isZh ? '上传/下载 (Upload/Download):' : 'Upload/Download:' }}</b> {{ isZh ? '在操作机与被控主机之间安全传输文件。' : 'Transfer files between server and beacon.' }}</p>
        <p><b>{{ isZh ? '执行 (Execute):' : 'Execute:' }}</b> {{ isZh ? '在被控主机后台运行指定的程序或脚本。' : 'Execute program on target system.' }}</p>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="helpDialogVisible = false">{{ isZh ? '确定' : 'OK' }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Search, Refresh, FolderAdd, DocumentAdd, Download, Upload,
  Folder, Document, Fold, FolderOpened, Monitor, HomeFilled, Top, Loading
} from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import TaskAPI from '@/api/tasks'
import { useGlobalFileTreeStore } from '@/stores/fileTree'
import { useCurrentFileNode } from '@/stores/currentFileNode'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { detectClientOs } from '@/utils/clientUtils'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const route = useRoute()
const clientsStore = useClientsStore()
const globalFileTree = useGlobalFileTreeStore()
const currentFileNode = useCurrentFileNode()

const realUid = inject<any>('realUid', null)
const uid = computed(() => {
  if (realUid && realUid.value) return String(realUid.value)
  return String(route.params.uid || route.query.uid || '')
})

// 状态管理
const fileTreeRef = ref<any>(null)
const pathInputRef = ref<any>(null)
const contextMenuRef = ref<HTMLElement | null>(null)
const treeSearchText = ref('')
const tableSearchKeyword = ref('')
// 递归搜索(远端 filesearch 命令)状态
const deepSearching = ref(false)
const deepSearchActive = ref(false) // 处于"搜索结果视图"标志(过滤框清空即回到普通列表)
const treeLoading = ref(false)
const loadingNodes = ref(new Set<string>())
const fileTree = ref<any[]>([])
const expandedKeys = ref<string[]>([])
const currentPath = ref('C:/')
const currentFiles = ref<any[]>([])
const selectedFiles = ref<any[]>([])
const pathHistory = ref<string[]>([])
const historyIndex = ref(-1)

// 侧边栏拖拽与面包屑状态
const sidebarWidth = ref(250)
const isResizingSidebar = ref(false)
const isPathEditMode = ref(false)

const pathSegments = computed(() => {
  const p = currentPath.value.replace(/\\/g, '/')
  if (!p) return []
  const isWin = /^[a-zA-Z]:/.test(p)
  const parts = p.split('/').filter(Boolean)
  const segments: { name: string; fullPath: string }[] = []
  let acc = isWin ? '' : '/'
  for (let i = 0; i < parts.length; i++) {
    if (i === 0 && isWin) {
      acc = parts[0] + '/'
    } else {
      acc = acc.endsWith('/') ? acc + parts[i] : acc + '/' + parts[i]
    }
    segments.push({ name: parts[i], fullPath: acc })
  }
  return segments
})

function jumpToSegment(segPath: string) {
  currentPath.value = segPath
  navigateToInputPath()
}

function startResizeSidebar(e: MouseEvent) {
  isResizingSidebar.value = true
  const startX = e.clientX
  const startW = sidebarWidth.value
  const onMove = (moveEvent: MouseEvent) => {
    const newW = Math.max(160, Math.min(500, startW + (moveEvent.clientX - startX)))
    sidebarWidth.value = newW
  }
  const onUp = () => {
    isResizingSidebar.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

// 排序
const sortColumn = ref('modifiedTime')
const sortOrder = ref('descending')

// 对话框状态
const editDialogVisible = ref(false)
const editLoading = ref(false)
const editingFile = ref<any>(null)
const fileContent = ref('')
const editorLineNumbers = ref<HTMLElement | null>(null)
const editorTextarea = ref<HTMLTextAreaElement | null>(null)
const editorLinesCount = ref(1)

const executeDialogVisible = ref(false)
const executeForm = reactive({
  path: '',
  name: '',
  args: '',
})

const uploadDialogVisible = ref(false)
const uploadFileList = ref<any[]>([])
const uploadDialogRef = ref<any>(null)

const wgetDialogVisible = ref(false)
const wgetForm = reactive({
  url: '',
  path: '',
})

const downloadDialogVisible = ref(false)
const downloadProgress = ref(0)
// 下载进度轮询句柄(组件作用域,便于关闭/卸载时清理,见 stopDownloadPoll)
let downloadPollTimer: number | null = null
const downloadStatus = ref('')
const currentDownloadFile = ref<any>(null)
const downloadSpeed = ref('')

const helpDialogVisible = ref(false)

// 右键菜单
const contextMenuVisible = ref(false)
const contextMenuX = ref(0)
const contextMenuY = ref(0)
const contextMenuItem = ref<any>(null)

// 树属性
const treeProps = {
  children: 'children',
  label: 'name',
}

const groupNodeKey = computed(() => `group:${uid.value}`)

const clientGroupLabel = computed(() => {
  const info = clientsStore.getClientByUid(uid.value)
  const id = info?.Uid ? info.Uid.slice(0, 8) : 'Host'
  const ip = info?.ExternalIP || info?.InternalIP || '0.0.0.0'
  return `${id} (${ip})`
})

const filteredFileTree = computed(() => {
  const filterNodes = (nodes: any[]): any[] => {
    if (!nodes || !Array.isArray(nodes)) return []
    return nodes
      .filter((node) => (node.type === 'D' || node.type === 'drive' || node.path?.endsWith(':')) && node.name !== '.' && node.name !== '..')
      .map((node) => ({
        ...node,
        name: node.path?.match(/^[a-zA-Z]:$/) ? node.name.toUpperCase() : node.name,
        path: node.path?.match(/^[a-zA-Z]:$/) ? node.path.toUpperCase() : node.path,
        children: Array.isArray(node.children) ? filterNodes(node.children) : [],
      }))
  }

  const children = filterNodes(fileTree.value)
  return [
    {
      name: clientGroupLabel.value,
      path: groupNodeKey.value,
      type: 'group',
      children,
    },
  ]
})

const canGoUp = computed(() => {
  const p = currentPath.value || ''
  return p.includes('/') || p.includes('\\') || p.length > 2
})

const filteredFiles = computed(() => {
  let files = [...currentFiles.value]

  // 按关键字过滤
  if (tableSearchKeyword.value.trim()) {
    const kw = tableSearchKeyword.value.toLowerCase().trim()
    files = files.filter((f) => (f.name || '').toLowerCase().includes(kw))
  }

  // 排序：默认目录在前，文件在后
  files.sort((a, b) => {
    if (a.type !== b.type) {
      return a.type === 'D' ? -1 : 1
    }
    if (sortColumn.value === 'size') {
      const aSize = parseFileSize(a.size)
      const bSize = parseFileSize(b.size)
      return sortOrder.value === 'ascending' ? aSize - bSize : bSize - aSize
    }
    if (sortColumn.value === 'modifiedTime') {
      const aTime = parseRowToTs(a)
      const bTime = parseRowToTs(b)
      return sortOrder.value === 'ascending' ? aTime - bTime : bTime - aTime
    }
    const aName = (a.name || '').toLowerCase()
    const bName = (b.name || '').toLowerCase()
    return sortOrder.value === 'descending' ? bName.localeCompare(aName) : aName.localeCompare(bName)
  })

  return files
})

const PAGE_SIZE = 200
const currentPage = ref(1)
// 大目录(如 System32)全量渲染会卡;前端分页,勾选经 reserve-selection 跨页保留
const paginatedFiles = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return filteredFiles.value.slice(start, start + PAGE_SIZE)
})

watch(filteredFiles, () => {
  currentPage.value = 1
})

function parseRowToTs(row: any): number {
  if (!row) return 0
  const val = row.modifiedTime ?? row.modified_time ?? row.time ?? row.Time ?? row.mtime ?? row.ModifiedTime
  if (!val) return 0
  const d = parseToDate(val)
  return d ? d.getTime() : 0
}

function parseToDate(t: any): Date | null {
  if (!t || t === '-' || t === 'undefined') return null
  if (typeof t === 'number') {
    return new Date(t > 1e11 ? t : t * 1000)
  }
  const s = String(t).trim()
  if (/^\d+$/.test(s)) {
    const n = parseInt(s, 10)
    return new Date(n > 1e11 ? n : n * 1000)
  }
  // Check DD/MM/YYYY or DD-MM-YYYY (e.g. 17/08/2026 14:32:00)
  const ddmmyyyy = s.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{4})(?:\s+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/)
  if (ddmmyyyy) {
    const day = parseInt(ddmmyyyy[1], 10)
    const month = parseInt(ddmmyyyy[2], 10) - 1
    const year = parseInt(ddmmyyyy[3], 10)
    const hour = parseInt(ddmmyyyy[4] || '0', 10)
    const minute = parseInt(ddmmyyyy[5] || '0', 10)
    const second = parseInt(ddmmyyyy[6] || '0', 10)
    return new Date(year, month, day, hour, minute, second)
  }
  // Check YYYY/MM/DD or YYYY-MM-DD (e.g. 2026-08-17 14:32:00)
  const yyyymmdd = s.match(/^(\d{4})[/-](\d{1,2})[/-](\d{1,2})(?:\s+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/)
  if (yyyymmdd) {
    const year = parseInt(yyyymmdd[1], 10)
    const month = parseInt(yyyymmdd[2], 10) - 1
    const day = parseInt(yyyymmdd[3], 10)
    const hour = parseInt(yyyymmdd[4] || '0', 10)
    const minute = parseInt(yyyymmdd[5] || '0', 10)
    const second = parseInt(yyyymmdd[6] || '0', 10)
    return new Date(year, month, day, hour, minute, second)
  }
  const d = new Date(s)
  return isNaN(d.getTime()) ? null : d
}

function formatDateTime(rowOrTime: any): string {
  const t = typeof rowOrTime === 'object' && rowOrTime !== null
    ? (rowOrTime.modifiedTime ?? rowOrTime.modified_time ?? rowOrTime.time ?? rowOrTime.Time ?? rowOrTime.mtime ?? rowOrTime.ModifiedTime)
    : rowOrTime
  if (!t || t === '-' || t === 'undefined') return '-'
  const d = parseToDate(t)
  if (!d || isNaN(d.getTime())) {
    return String(t).replace(/-/g, '/')
  }
  const yy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${yy}/${mm}/${dd} ${hh}:${min}:${ss}`
}

function parseFileSize(sizeStr: any): number {
  if (!sizeStr || sizeStr === '-' || sizeStr === '<DIR>') return 0
  const match = String(sizeStr).match(/^([\d.]+)\s*(B|KB|MB|GB|TB)$/i)
  if (!match) return 0
  const val = parseFloat(match[1])
  const unit = match[2].toUpperCase()
  const map: Record<string, number> = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4 }
  return val * (map[unit] || 1)
}

function getFileIconClass(row: any) {
  if (row.type === 'D') return 'icon-folder'
  const ext = (row.name || '').split('.').pop()?.toLowerCase()
  if (['exe', 'bin', 'dll', 'so', 'elf', 'dylib'].includes(ext)) return 'icon-binary'
  if (['sh', 'ps1', 'bat', 'cmd', 'py', 'vbs', 'cna'].includes(ext)) return 'icon-script'
  if (['zip', 'tar', 'gz', '7z', 'rar'].includes(ext)) return 'icon-zip'
  return 'icon-doc'
}

function getFileNameClass(row: any) {
  if (row.type === 'D') return 'name-folder'
  const ext = (row.name || '').split('.').pop()?.toLowerCase()
  if (['exe', 'bin', 'dll', 'so', 'elf'].includes(ext)) return 'name-binary'
  if (['sh', 'ps1', 'bat', 'cmd', 'py', 'cna'].includes(ext)) return 'name-script'
  return 'name-doc'
}

function filterNode(value: string, data: any) {
  if (!value) return true
  return (data.name || '').toLowerCase().includes(value.toLowerCase())
}

watch(treeSearchText, (val) => {
  fileTreeRef.value?.filter(val)
})

function handleNodeExpand(data: any) {
  if (data?.path && !expandedKeys.value.includes(data.path)) {
    expandedKeys.value.push(data.path)
  }
}

function handleNodeCollapse(data: any) {
  if (data?.path) {
    expandedKeys.value = expandedKeys.value.filter((k) => k !== data.path)
  }
}

function expandAllTree() {
  const tree = fileTreeRef.value
  if (!tree?.store?.nodesMap) return
  const allKeys: string[] = []
  Object.values(tree.store.nodesMap).forEach((node: any) => {
    if (node.childNodes && node.childNodes.length > 0) {
      node.expanded = true
      if (node.data?.path) allKeys.push(node.data.path)
    }
  })
  expandedKeys.value = Array.from(new Set([...expandedKeys.value, ...allKeys]))
}

function collapseAllTree() {
  const tree = fileTreeRef.value
  if (!tree?.store?.nodesMap) return
  Object.values(tree.store.nodesMap).forEach((node: any) => {
    if (node.data?.type === 'group') {
      node.expanded = true
    } else if (node.childNodes && node.childNodes.length > 0) {
      node.expanded = false
    }
  })
  expandedKeys.value = [groupNodeKey.value]
}

function findTreeNodeByPath(path: string, root: any): any {
  if (Array.isArray(root)) {
    for (const node of root) {
      const res = findTreeNodeByPath(path, node)
      if (res) return res
    }
    return null
  }
  if (!root) return null
  if (root.path === path) return root
  if (Array.isArray(root.children)) {
    for (const child of root.children) {
      const res = findTreeNodeByPath(path, child)
      if (res) return res
    }
  }
  return null
}

function ensureAncestorsExpanded(path: string) {
  if (!path) return
  if (!expandedKeys.value.includes(groupNodeKey.value)) {
    expandedKeys.value.push(groupNodeKey.value)
  }
  if (!expandedKeys.value.includes(path)) {
    expandedKeys.value.push(path)
  }
  // 分解父路径
  const normalized = path.replace(/\\/g, '/')
  const parts = normalized.split('/').filter(Boolean)
  let curr = normalized.startsWith('/') ? '' : parts[0]
  for (let i = 0; i < parts.length - 1; i++) {
    if (normalized.startsWith('/')) {
      curr += '/' + parts[i]
    } else if (i > 0) {
      curr += '/' + parts[i]
    }
    if (curr && !expandedKeys.value.includes(curr)) {
      expandedKeys.value.push(curr)
    }
  }
}

async function fetchFileTree(dirPath: string) {
  if (!uid.value) return
  loadingNodes.value.add(dirPath)
  treeLoading.value = true
  try {
    const res = await ClientAPI.get_file_tree({ uid: uid.value, dirPath })
    if (res.data && res.data.data) {
      fileTree.value = res.data.data
      globalFileTree.updateEntry(uid.value, fileTree.value)

      ensureAncestorsExpanded(dirPath)

      const currentNode = findTreeNodeByPath(dirPath, fileTree.value)
      if (currentNode && Array.isArray(currentNode.children)) {
        currentFiles.value = currentNode.children
      } else {
        currentFiles.value = []
      }
    }
  } catch (err: any) {
    ElMessage.error(isZh.value ? '获取文件列表失败: ' + (err?.message || err) : 'Failed to list files: ' + (err?.message || err))
  } finally {
    loadingNodes.value.delete(dirPath)
    treeLoading.value = false
  }
}

async function navigateToPath(path: string) {
  if (!path || !path.trim()) return
  const normalized = path.trim()
  currentPath.value = normalized
  pathHistory.value.push(normalized)
  historyIndex.value = pathHistory.value.length - 1
  await fetchFileTree(normalized)
  currentFileNode.setFileNode(uid.value, normalized)
}

function navigateToInputPath() {
  if (!currentPath.value || !currentPath.value.trim()) {
    ElMessage.warning(isZh.value ? '请输入有效路径' : 'Please enter a valid path')
    return
  }
  navigateToPath(currentPath.value)
}

function goClientHome() {
  const c = clientsStore.getClientByUid(uid.value)
  const isWin = detectClientOs(c) === 'windows'
  navigateToPath(isWin ? 'C:' : '/')
}

function goUpLevel() {
  const p = currentPath.value || ''
  if (!p) return
  let parent = '/'
  if (/^[a-zA-Z]:[\\/]?$/.test(p)) {
    parent = p[0].toUpperCase() + ':'
  } else {
    const norm = p.replace(/\\/g, '/').replace(/\/+$/, '')
    const idx = norm.lastIndexOf('/')
    parent = idx <= 0 ? '/' : norm.substring(0, idx)
  }
  navigateToPath(parent)
}

function refreshCurrentFolder() {
  deepSearchActive.value = false // 刷新即退出搜索结果视图
  fetchFileTree(currentPath.value)
}

// 递归搜索:当前目录起,关键字匹配文件/目录名(客户端执行,限深 8/限 5000 条/30s)
// 结果复用文件列表渲染;清空过滤框或点刷新即回到普通目录浏览
async function deepSearch() {
  const kw = tableSearchKeyword.value.trim()
  if (!kw || !uid.value || deepSearching.value) return
  deepSearching.value = true
  try {
    const res = await ClientAPI.search_files({ uid: uid.value, dirPath: currentPath.value || '/', keyword: kw })
    const data = res.data
    if (data && data.status === 200) {
      if (data.noMatch) {
        ElMessage.info(isZh.value ? `未找到匹配 "${kw}" 的文件` : `No files matching "${kw}"`)
        return
      }
      if (Array.isArray(data.data)) {
        currentFiles.value = data.data
        deepSearchActive.value = true
        ElMessage.success(isZh.value ? `搜索完成: ${data.data.length} 条结果` : `Search done: ${data.data.length} results`)
      }
    }
  } catch (err: any) {
    ElMessage.error(err?.response?.data?.error || (isZh.value ? '递归搜索失败: ' : 'Search failed: ') + (err?.message || err))
  } finally {
    deepSearching.value = false
  }
}

// 清空过滤框:若处于搜索结果视图,回到普通目录列表
function onDeepSearchClear() {
  if (deepSearchActive.value) {
    deepSearchActive.value = false
    refreshCurrentFolder()
  }
}

async function fetchDrives() {
  if (!uid.value) return
  treeLoading.value = true
  try {
    const res = await ClientAPI.get_drives({ uid: uid.value })
    if (res.data && res.data.status === 200 && Array.isArray(res.data.data)) {
      fileTree.value = res.data.data
      globalFileTree.updateEntry(uid.value, res.data.data)
      if (res.data.data.length > 0) {
        const first = res.data.data[0].path || res.data.data[0].name
        currentPath.value = first
        await fetchFileTree(first)
      }
      ElMessage.success(isZh.value ? `成功获取 ${res.data.data.length} 个驱动器` : `Loaded ${res.data.data.length} drives`)
    }
  } catch {
    ElMessage.warning(isZh.value ? '无法获取驱动器列表' : 'Failed to query drives')
  } finally {
    treeLoading.value = false
  }
}

function handleTreeNodeClick(node: any) {
  if (node.type === 'group') return
  if (node.type === 'D' || node.type === 'drive') {
    navigateToPath(node.path)
  }
}

function handleRowDoubleClick(row: any) {
  if (row.type === 'D') {
    navigateToPath(row.path)
  } else {
    editFile(row)
  }
}

function handleSelectionChange(selection: any[]) {
  selectedFiles.value = selection
}

function handleSortChange({ prop, order }: any) {
  sortColumn.value = prop || 'modifiedTime'
  sortOrder.value = order || 'descending'
}

async function handleRowContextMenu(row: any, col: any, e: MouseEvent) {
  e.preventDefault()
  e.stopPropagation()
  contextMenuItem.value = row

  const menuWidth = 190
  const menuHeight = 440
  const pad = 12

  let x = e.clientX
  let y = e.clientY

  if (x + menuWidth > window.innerWidth - pad) {
    x = Math.max(pad, window.innerWidth - menuWidth - pad)
  }

  if (y + menuHeight > window.innerHeight - pad) {
    if (e.clientY - menuHeight >= pad) {
      y = e.clientY - menuHeight
    } else {
      y = Math.max(pad, window.innerHeight - menuHeight - pad)
    }
  }

  contextMenuX.value = x
  contextMenuY.value = y
  contextMenuVisible.value = true

  await nextTick()
  if (contextMenuRef.value) {
    const rect = contextMenuRef.value.getBoundingClientRect()
    if (rect.bottom > window.innerHeight - pad) {
      contextMenuY.value = Math.max(pad, window.innerHeight - rect.height - pad)
    }
    if (rect.right > window.innerWidth - pad) {
      contextMenuX.value = Math.max(pad, window.innerWidth - rect.width - pad)
    }
  }
}

function closeContextMenu() {
  contextMenuVisible.value = false
}

function handleGlobalClick() {
  closeContextMenu()
}

// 监听窗口大小与点击关闭右键菜单
onMounted(() => {
  window.addEventListener('click', closeContextMenu)
  window.addEventListener('blur', closeContextMenu)
  if (uid.value) {
    fetchDrives()
  }
})

onBeforeUnmount(() => {
  stopDownloadPoll()
  window.removeEventListener('click', closeContextMenu)
  window.removeEventListener('blur', closeContextMenu)
  fileTree.value = []
  currentFiles.value = []
  selectedFiles.value = []
})

// 文件操作
async function createNewFolder() {
  closeContextMenu()
  try {
    const { value } = await ElMessageBox.prompt(
      isZh.value ? '请输入新文件夹名称:' : 'Enter folder name:',
      isZh.value ? '新建文件夹' : 'Make Directory',
      { confirmButtonText: isZh.value ? '创建' : 'Create', cancelButtonText: isZh.value ? '取消' : 'Cancel' }
    )
    if (!value?.trim()) return
    const newPath = `${currentPath.value}/${value.trim()}`
    const res = await ClientAPI.make_dir({ uid: uid.value, dirPath: newPath })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? '文件夹创建成功' : 'Directory created')
      await refreshCurrentFolder()
    } else {
      ElMessage.error(res.data?.message || '创建失败')
    }
  } catch {}
}

async function createNewFile() {
  closeContextMenu()
  try {
    const { value } = await ElMessageBox.prompt(
      isZh.value ? '请输入新文件名称:' : 'Enter file name:',
      isZh.value ? '新建文件' : 'New File',
      { confirmButtonText: isZh.value ? '创建' : 'Create', cancelButtonText: isZh.value ? '取消' : 'Cancel' }
    )
    if (!value?.trim()) return
    const filePath = `${currentPath.value}/${value.trim()}`
    const res = await ClientAPI.create_file({ uid: uid.value, filePath })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? '文件创建成功' : 'File created')
      await refreshCurrentFolder()
    } else {
      ElMessage.error(res.data?.message || '创建失败')
    }
  } catch {}
}

function handleFileView(file?: any) {
  closeContextMenu()
  const target = file || (selectedFiles.value.length > 0 ? selectedFiles.value[0] : null)
  if (!target) return
  if (target.type === 'D') {
    navigateToPath(target.path)
  } else {
    editFile(target)
  }
}

async function editFile(file: any) {
  closeContextMenu()
  editingFile.value = file
  fileContent.value = ''
  editLoading.value = true
  editDialogVisible.value = true

  try {
    const res = await ClientAPI.fetch_file_content({ uid: uid.value, path: file.path })
    let raw = ''
    if (res?.data) {
      if (typeof res.data === 'string') {
        raw = res.data
      } else if (typeof res.data.data === 'string') {
        raw = res.data.data
      } else if (res.data.data?.content !== undefined) {
        raw = res.data.data.content
      } else if (res.data.content !== undefined) {
        raw = res.data.content
      } else if (res.data.data?.fileContent !== undefined) {
        raw = res.data.data.fileContent
      } else if (typeof res.data.data === 'object') {
        raw = JSON.stringify(res.data.data, null, 2)
      }
    }

    // 处理 base64 解码
    if (raw && typeof raw === 'string' && /^[A-Za-z0-9+/=]+$/.test(raw.trim()) && raw.trim().length % 4 === 0 && !raw.includes('\n') && raw.length > 24) {
      try {
        const decoded = atob(raw.trim())
        if (decoded) {
          raw = decodeURIComponent(escape(decoded))
        }
      } catch {}
    }

    fileContent.value = String(raw ?? '')
    updateEditorLineNumbers()
  } catch (err: any) {
    ElMessage.error(isZh.value ? '读取文件内容失败: ' + (err?.message || err) : 'Failed to read file')
  } finally {
    editLoading.value = false
  }
}

function refreshEditContent() {
  if (editingFile.value) {
    editFile(editingFile.value)
  }
}

function updateEditorLineNumbers() {
  const text = fileContent.value || ''
  const lines = text.split(/\r\n|\r|\n/)
  editorLinesCount.value = Math.max(1, lines.length)
}

function syncEditorScroll() {
  if (editorLineNumbers.value && editorTextarea.value) {
    editorLineNumbers.value.scrollTop = editorTextarea.value.scrollTop
  }
}

async function saveFile() {
  if (!editingFile.value) return
  editLoading.value = true
  try {
    const res = await ClientAPI.save_file_content({
      uid: uid.value,
      path: editingFile.value.path,
      content: fileContent.value,
    })
    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? '文件保存成功' : 'File saved')
      editDialogVisible.value = false
      await refreshCurrentFolder()
    } else {
      ElMessage.error(res.data?.message || '保存失败')
    }
  } catch (err: any) {
    ElMessage.error(isZh.value ? '保存文件失败: ' + (err?.message || err) : 'Save failed')
  } finally {
    editLoading.value = false
  }
}

function handleFileDownload(file?: any) {
  closeContextMenu()
  const target = file || (selectedFiles.value.length > 0 ? selectedFiles.value[0] : null)
  if (!target) return
  if (target.type === 'D') {
    ElMessage.warning(isZh.value ? '无法直接下载目录' : 'Cannot download directory directly')
    return
  }
  downloadFile(target)
}

async function downloadFile(file: any) {
  currentDownloadFile.value = file
  downloadProgress.value = 0
  downloadStatus.value = 'downloading'
  downloadSpeed.value = ''
  downloadDialogVisible.value = true

  // 【修复】轮询句柄提升到组件作用域:原实现仅 pct>=100 才 clearInterval,
  // 关掉对话框/卸载组件后 1.5s 轮询仍永久运行(目标文件若被清理则永远到不了 100)。
  stopDownloadPoll()
  try {
    const res = await ClientAPI.download_file({ uid: uid.value, filePath: file.path })
    // 下载已是任务（后端建 kind=download 的任务并回传 ID）：
    // 完成/失败由任务终态给出，字节进度只用于进度条展示。
    // 原实现只盯百分比，失败（目标离线、连接中断）要等 6 分钟轮询超时才提示。
    const taskId = Number(res?.data?.task_id) || 0
    let attempts = 0
    const maxAttempts = 400 // 上限 10 分钟,兜底防永久轮询
    downloadPollTimer = window.setInterval(async () => {
      if (++attempts > maxAttempts) {
        stopDownloadPoll()
        downloadStatus.value = 'error'
        ElMessage.warning(
          isZh.value
            ? '等待下载结果超时,请到任务中心查看该任务状态'
            : 'Timed out waiting for the download; check the Task Center',
        )
        return
      }
      try {
        // 1) 字节进度（真实进度条数据）
        const infoRes = await ClientAPI.get_downloads_info({ uid: uid.value })
        const downloads = infoRes.data?.data || []
        const target = downloads.find((d: any) => d.filePath === file.path)
        if (target) {
          const pct = parseInt(target.downloadPart, 10) || 0
          downloadProgress.value = Math.min(pct, 100)
        }

        // 2) 终态判定：任务状态
        if (taskId) {
          try {
            const taskRes = await TaskAPI.get(taskId)
            const status = taskRes.data?.data?.status
            if (status === 'completed') {
              stopDownloadPoll()
              downloadProgress.value = 100
              downloadStatus.value = 'success'
              ElMessage.success(isZh.value ? `文件 ${file.name} 已下载至团队服务器` : `Downloaded ${file.name}`)
              return
            }
            if (status === 'failed' || status === 'timeout' || status === 'cancelled') {
              stopDownloadPoll()
              downloadStatus.value = 'error'
              const reason = taskRes.data?.data?.error || (isZh.value ? '下载未完成' : 'Download did not finish')
              ElMessage.error(reason)
              return
            }
          } catch { /* 单次查询失败不判定，下一轮重试 */ }
        } else if ((parseInt(target?.downloadPart, 10) || 0) >= 100) {
          // 无任务 ID（后端未建任务）时的兜底：仍按百分比判定
          stopDownloadPoll()
          downloadStatus.value = 'success'
          ElMessage.success(isZh.value ? `文件 ${file.name} 已下载至团队服务器` : `Downloaded ${file.name}`)
        }
      } catch {}
    }, 1500)
  } catch (e: any) {
    downloadStatus.value = 'error'
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '下发下载任务失败' : 'Download failed'))
  }
}

// 停止下载进度轮询(关闭对话框/组件卸载/重新下载时调用)
function stopDownloadPoll() {
  if (downloadPollTimer !== null) {
    clearInterval(downloadPollTimer)
    downloadPollTimer = null
  }
}

async function openDownloadFolder() {
  if (!currentDownloadFile.value) return
  try {
    const res = await ClientAPI.download_downloaded_file({
      uid: uid.value,
      filePath: currentDownloadFile.value.path,
    })
    if (res.status === 200 && res.data) {
      // responseType='blob' 时 res.data 已是 Blob；兼容字符串/JSON 兜底包装
      const blob = res.data instanceof Blob
        ? res.data
        : new Blob([res.data as any], { type: 'application/octet-stream' })
      const link = document.createElement('a')
      link.href = URL.createObjectURL(blob)
      link.download = currentDownloadFile.value.name || 'download'
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      // 延迟 revoke：立即调用会在部分浏览器触发下载前释放对象 URL 导致保存失败
      setTimeout(() => URL.revokeObjectURL(link.href), 2000)
      ElMessage.success(isZh.value ? '文件已保存到本地' : 'Saved locally')
    } else {
      ElMessage.error(isZh.value ? '下载到本地失败（文件不存在或已被清理）' : 'Failed to save (file missing or cleaned up)')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '下载到本地失败' : 'Failed to save'))
  }
}

function handleFileExecute(file?: any) {
  closeContextMenu()
  const target = file || (selectedFiles.value.length > 0 ? selectedFiles.value[0] : null)
  if (!target) return
  if (target.type === 'D') {
    ElMessage.warning(isZh.value ? '无法直接执行文件夹' : 'Cannot execute folder')
    return
  }
  executeForm.path = target.path
  executeForm.name = target.name
  executeForm.args = ''
  executeDialogVisible.value = true
}

async function submitExecute() {
  if (!executeForm.path) return
  const cmd = executeForm.args.trim()
    ? `execute "${executeForm.path}" ${executeForm.args.trim()}`
    : `execute "${executeForm.path}"`
  try {
    await ClientAPI.send_commands({ uid: uid.value, command: cmd })
    ElMessage.success(isZh.value ? `已下发执行指令 (${executeForm.name})，回显见会话控制台` : `Execution tasked for ${executeForm.name}`)
    executeDialogVisible.value = false
  } catch (err: any) {
    ElMessage.error(isZh.value ? '执行失败: ' + (err?.message || err) : 'Execution failed')
  }
}

async function deleteItem(item: any) {
  closeContextMenu()
  if (!item) return
  ElMessageBox.confirm(
    isZh.value ? `确定删除 ${item.type === 'D' ? '文件夹' : '文件'} "${item.name}" 吗？` : `Delete "${item.name}"?`,
    isZh.value ? '删除确认' : 'Delete Confirm',
    { confirmButtonText: isZh.value ? '删除' : 'Delete', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' }
  ).then(async () => {
    try {
      await ClientAPI.delete_file({ uid: uid.value, filePath: item.path })
      ElMessage.success(isZh.value ? '删除成功' : 'Deleted')
      await refreshCurrentFolder()
    } catch {
      ElMessage.error(isZh.value ? '删除失败' : 'Failed to delete')
    }
  }).catch(() => {})
}

async function deleteSelectedFiles() {
  if (selectedFiles.value.length === 0) return
  const count = selectedFiles.value.length
  ElMessageBox.confirm(
    isZh.value ? `确定删除选中的 ${count} 个项目吗？此操作不可撤销。` : `Delete ${count} selected item(s)?`,
    isZh.value ? '批量删除' : 'Delete Items',
    { confirmButtonText: isZh.value ? '删除' : 'Delete', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' }
  ).then(async () => {
    let succ = 0
    for (const item of selectedFiles.value) {
      try {
        await ClientAPI.delete_file({ uid: uid.value, filePath: item.path })
        succ++
      } catch {}
    }
    ElMessage.success(isZh.value ? `已删除 ${succ} / ${count} 项` : `Deleted ${succ} of ${count} items`)
    selectedFiles.value = []
    await refreshCurrentFolder()
  }).catch(() => {})
}

async function renameItem(item: any) {
  closeContextMenu()
  if (!item) return
  ElMessageBox.prompt(
    isZh.value ? '输入新名称:' : 'Enter new name:',
    isZh.value ? '重命名' : 'Rename',
    { inputValue: item.name, confirmButtonText: isZh.value ? '确定' : 'OK', cancelButtonText: isZh.value ? '取消' : 'Cancel' }
  ).then(async ({ value }) => {
    if (!value?.trim() || value.trim() === item.name) return
    const parts = item.path.split('/')
    parts[parts.length - 1] = value.trim()
    const newPath = parts.join('/')
    const res = await ClientAPI.rename_file({ uid: uid.value, oldPath: item.path, newPath })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? '重命名成功' : 'Renamed')
      await refreshCurrentFolder()
    } else {
      ElMessage.error(res.data?.message || '重命名失败')
    }
  }).catch(() => {})
}

async function copyItem(item: any) {
  closeContextMenu()
  if (!item) return
  ElMessageBox.prompt(
    isZh.value ? '输入目标路径:' : 'Enter target path:',
    isZh.value ? '复制' : 'Copy',
    { inputValue: item.path, confirmButtonText: isZh.value ? '确定' : 'OK', cancelButtonText: isZh.value ? '取消' : 'Cancel' }
  ).then(async ({ value }) => {
    if (!value?.trim() || value.trim() === item.path) return
    const res = await ClientAPI.copy_file({ uid: uid.value, oldPath: item.path, newPath: value.trim() })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? '复制成功' : 'Copied')
      await refreshCurrentFolder()
    } else {
      ElMessage.error(res.data?.message || '复制失败')
    }
  }).catch(() => {})
}

async function moveItem(item: any) {
  closeContextMenu()
  if (!item) return
  ElMessageBox.prompt(
    isZh.value ? '输入新目标路径:' : 'Enter new target path:',
    isZh.value ? '移动' : 'Move',
    { inputValue: item.path, confirmButtonText: isZh.value ? '确定' : 'OK', cancelButtonText: isZh.value ? '取消' : 'Cancel' }
  ).then(async ({ value }) => {
    if (!value?.trim() || value.trim() === item.path) return
    const res = await ClientAPI.rename_file({ uid: uid.value, oldPath: item.path, newPath: value.trim() })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? '移动成功' : 'Moved')
      await refreshCurrentFolder()
    } else {
      ElMessage.error(res.data?.message || '移动失败')
    }
  }).catch(() => {})
}

function copyPath(item: any) {
  closeContextMenu()
  if (!item?.path) return
  navigator.clipboard.writeText(item.path)
  ElMessage.success(isZh.value ? '路径已复制到剪贴板' : 'Path copied to clipboard')
}

async function modifyTime(item: any) {
  closeContextMenu()
  if (!item) return
  ElMessageBox.prompt(
    isZh.value ? '设置新时间 (YYYY/MM/DD HH:mm:ss):' : 'Set timestamp (YYYY/MM/DD HH:mm:ss):',
    isZh.value ? '修改文件时间' : 'Touch File Time',
    { confirmButtonText: isZh.value ? '确定' : 'OK', cancelButtonText: isZh.value ? '取消' : 'Cancel' }
  ).then(async ({ value }) => {
    if (!value?.trim()) return
    await ClientAPI.modify_file_time({ uid: uid.value, filePath: item.path, newTime: value.trim() })
    ElMessage.success(isZh.value ? '修改时间指令已发送' : 'Time updated')
    setTimeout(refreshCurrentFolder, 1000)
  }).catch(() => {})
}

async function hideItem(item: any) {
  closeContextMenu()
  if (!item) return
  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定隐藏 "${item.name}" 吗？将修改目标机上该文件的可见属性。` : `Hide "${item.name}"? This changes the file visibility attribute on the target host.`,
      isZh.value ? '隐藏确认' : 'Hide Confirm',
      { confirmButtonText: isZh.value ? '隐藏' : 'Hide', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' }
    )
  } catch {
    return
  }
  await ClientAPI.hide_file({ uid: uid.value, filePath: item.path })
  ElMessage.success(isZh.value ? '已发送隐藏指令' : 'Hidden')
  refreshCurrentFolder()
}

async function unhideItem(item: any) {
  closeContextMenu()
  if (!item) return
  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定取消隐藏 "${item.name}" 吗？将恢复目标机上该文件的可见属性。` : `Unhide "${item.name}"? This restores the file visibility attribute on the target host.`,
      isZh.value ? '取消隐藏确认' : 'Unhide Confirm',
      { confirmButtonText: isZh.value ? '取消隐藏' : 'Unhide', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' }
    )
  } catch {
    return
  }
  await ClientAPI.unhide_file({ uid: uid.value, filePath: item.path })
  ElMessage.success(isZh.value ? '已发送取消隐藏指令' : 'Unhidden')
  refreshCurrentFolder()
}

function makeDirItem() {
  closeContextMenu()
  createNewFolder()
}

function uploadToHere() {
  closeContextMenu()
  showUploadDialog()
}

function showUploadDialog() {
  uploadFileList.value = []
  uploadDialogVisible.value = true
}

function closeUploadDialog() {
  uploadFileList.value = []
  uploadDialogVisible.value = false
}

async function startUpload() {
  if (uploadFileList.value.length === 0) return
  let succ = 0
  for (const f of uploadFileList.value) {
    try {
      const res = await ClientAPI.upload_file({
        uid: uid.value,
        file: f.raw,
        uploadPath: `${currentPath.value}/${f.name}`,
      })
      if (res.data?.status === 200 || res.status === 200) succ++
    } catch {}
  }
  ElMessage.success(isZh.value ? `成功上传 ${succ} 个文件` : `Uploaded ${succ} file(s)`)
  closeUploadDialog()
  await refreshCurrentFolder()
}

function showWgetDialog() {
  wgetForm.url = ''
  wgetForm.path = currentPath.value
  wgetDialogVisible.value = true
}

async function executeWget() {
  if (!wgetForm.url) return
  const cmd = `wget -O "${wgetForm.path}/${wgetForm.url.split('/').pop() || 'download'}" "${wgetForm.url}"`
  try {
    await ClientAPI.send_commands({ uid: uid.value, command: cmd })
    ElMessage.success(isZh.value ? '已下发 wget 下载指令' : 'wget task dispatched')
    wgetDialogVisible.value = false
    setTimeout(refreshCurrentFolder, 2000)
  } catch {
    ElMessage.error(isZh.value ? '下发失败' : 'Failed')
  }
}

function formatFileSize(size: any) {
  if (typeof size === 'string' && size !== '0' && size !== '') {
    if (/^\d+(\.\d+)?(B|KB|MB|GB)$/.test(size)) return size.replace(/(\d+(?:\.\d+)?)(B|KB|MB|GB)$/, '$1 $2')
    return size
  }
  const n = parseInt(size, 10)
  if (isNaN(n) || n === 0) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  return `${(n / 1024 ** 3).toFixed(2)} GB`
}

function formatUploadFileSize(size: number) {
  if (!size) return '0 B'
  if (size < 1024) return `${size} B`
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 ** 2).toFixed(1)} MB`
}

function formatPermissions(perm: any) {
  if (!perm || perm === '-' || perm === 'undefined') return '-'
  return String(perm)
}

function handleHelp() {
  helpDialogVisible.value = true
}
</script>

<style scoped>
.cs-file-manager {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.fm-main-container {
  flex: 1;
  display: flex;
  overflow: hidden;
  padding: 12px;
}

/* 左侧文件树卡片 */
.fm-sidebar {
  width: 280px;
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  overflow: hidden;
}

.fm-sidebar-header {
  height: 38px;
  background: transparent;
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  flex-shrink: 0;
}

.fm-sidebar-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.fm-tree-btns {
  display: flex;
  gap: 2px;
}

/* 工具条图标按钮：32×32 / 6px 圆角 / 悬停浅灰底 */
.ic-btn {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  font-size: 16px;
  color: var(--cs-text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  transition: all 120ms ease;
  flex-shrink: 0;
}

.ic-btn:hover:not(:disabled) {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.ic-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.fm-tree-search-wrap {
  padding: 8px 12px;
  border-bottom: 1px solid var(--cs-card-border);
  background: transparent;
  flex-shrink: 0;
}

.fm-search-input {
  width: 100%;
}

.fm-tree-content {
  flex: 1;
  overflow-y: auto;
  padding: 4px 6px;
}

.fm-file-tree {
  background: transparent !important;
  color: var(--cs-text) !important;
}

.fm-tree-node {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fm-node-icon {
  font-size: 14px;
  flex-shrink: 0;
}

.fm-node-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 右侧主内容卡片 */
.fm-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.fm-toolbar {
  height: 48px;
  background: transparent;
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
  overflow-x: auto;
  scrollbar-width: none;
}

.fm-toolbar::-webkit-scrollbar {
  display: none;
}

.fm-nav-group {
  display: flex;
  align-items: center;
  gap: 2px;
}

.fm-sidebar-resizer {
  width: 8px;
  background: transparent;
  border-radius: 4px;
  cursor: col-resize;
  transition: background 150ms ease;
  user-select: none;
  flex-shrink: 0;
}
.fm-sidebar-resizer:hover {
  background: color-mix(in srgb, var(--cs-active) 35%, transparent);
}

.fm-path-container {
  flex: 1;
  min-width: 200px;
}

.fm-breadcrumbs-bar {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 32px;
  padding: 0 10px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  cursor: pointer;
  overflow-x: auto;
  scrollbar-width: none;
}
.fm-breadcrumbs-bar::-webkit-scrollbar {
  display: none;
}
.fm-path-icon {
  margin-right: 4px;
  flex-shrink: 0;
}
.crumb-root,
.crumb-item {
  padding: 1px 4px;
  border-radius: 4px;
  font-size: 13px;
  color: var(--cs-text);
  white-space: nowrap;
  transition: all 100ms ease;
}
.crumb-root:hover,
.crumb-item:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}
.crumb-sep {
  color: var(--cs-text-dim);
  font-size: 13px;
  user-select: none;
}
.crumb-edit-icon {
  margin-left: auto;
  padding-left: 6px;
  color: var(--cs-text-dim);
  font-size: 13px;
  opacity: 0.7;
  flex-shrink: 0;
}
.crumb-edit-icon:hover {
  color: var(--cs-active);
  opacity: 1;
}

.fm-path-el-input {
  width: 100%;
}

:deep(.fm-path-el-input .el-input__inner) {
  font-family: var(--cs-font-mono);
  font-size: 13px;
}

.fm-go-btn {
  height: 24px;
  padding: 0 12px;
  border-radius: 4px;
}

.fm-filter-wrap {
  width: 200px;
  flex-shrink: 0;
}

.fm-quick-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

/* 操作条（表格上方，取代原底部按钮行） */
.fm-action-bar {
  height: 48px;
  background: transparent;
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

.fm-pager {
  position: absolute;
  right: 16px;
  bottom: 12px;
  z-index: 5;
  padding: 4px 8px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.fm-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

/* 空状态：文件夹图标 48px + 说明 */
.fm-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 40px 16px;
  text-align: center;
}

.fm-empty-icon {
  font-size: 48px;
  color: var(--cs-text-dim);
  line-height: 1;
}

.fm-empty-title {
  font-size: 14px;
  color: var(--cs-text);
}

.fm-empty-desc {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.text-dim {
  color: var(--cs-text-dim);
}

.fm-file-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.fm-file-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fm-time-text {
  white-space: nowrap;
}

.fm-perm-text {
  white-space: nowrap;
}

.fm-item-icon {
  font-size: 16px;
  flex-shrink: 0;
}

/* 文件类型图标配色（图标彩色例外，保留原色） */
.icon-folder { color: #ffc107; }
.icon-binary { color: #52c41a; }
.icon-script { color: #13c2c2; }
.icon-zip { color: #fa8c16; }
.icon-doc { color: #8c8c8c; }

.folder-gold { color: #ffc107; }

.name-folder { color: var(--cs-text-bright); font-weight: 500; }
.name-binary { color: #52c41a; font-weight: 500; }
.name-script { color: #13c2c2; }
.name-doc { color: var(--cs-text); }

.fm-size-text.is-dir {
  color: var(--cs-text-dim);
  font-weight: 500;
}

.fm-spacer {
  flex: 1;
}

.fm-status-info {
  font-size: 13px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}

/* 编辑器对话框 */
.fm-editor-meta {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--cs-text);
  background: var(--cs-panel);
  padding: 8px 12px;
  border: 1px solid var(--cs-card-border);
  border-radius: 6px;
  margin-bottom: 12px;
}

.fm-editor-box {
  display: flex;
  height: 460px;
  background: var(--cs-console);
  border: 1px solid var(--cs-border);
  border-radius: 6px;
  overflow: hidden;
  box-sizing: border-box;
}

.fm-editor-lines {
  width: 52px;
  min-width: 52px;
  padding: 8px 8px 8px 0;
  background: var(--cs-hover);
  border-right: 1px solid var(--cs-border);
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono) !important;
  font-size: 13px !important;
  line-height: 20px !important;
  text-align: right;
  overflow: hidden;
  user-select: none;
  box-sizing: border-box;
}

.editor-line-num {
  height: 20px !important;
  line-height: 20px !important;
  font-size: 13px !important;
  box-sizing: border-box;
}

.fm-editor-textarea {
  flex: 1;
  padding: 8px 12px;
  background: var(--cs-console);
  border: none;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono) !important;
  font-size: 13px !important;
  line-height: 20px !important;
  outline: none;
  resize: none;
  white-space: pre !important;
  word-break: normal !important;
  overflow-wrap: normal !important;
  overflow: auto !important;
  box-sizing: border-box;
  tab-size: 4;
}

/* 上传列表 */
.fm-upload-path-hint {
  font-size: 13px;
  color: var(--cs-text);
  padding: 4px 0 12px 0;
}

.fm-upload-dragger {
  width: 100%;
}

.fm-upload-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 140px;
  overflow-y: auto;
  margin-top: 12px;
}

.fm-upload-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 10px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 6px;
  font-size: 13px;
}

.fm-upload-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fm-upload-size {
  color: var(--cs-text-dim);
  margin: 0 8px;
}

/* 下载进度对话框 */
.fm-dl-file {
  font-size: 13px;
  color: var(--cs-text);
  margin-bottom: 12px;
}

.fm-dl-speed {
  font-size: 13px;
  color: var(--cs-text-dim);
  margin-top: 8px;
}

.fm-help-body {
  line-height: 1.7;
  font-size: 13px;
  color: var(--cs-text);
}
</style>

<!-- 右键上下文菜单全局样式（自适应主题明亮/暗黑） -->
<style>
.cs-file-ctx-menu {
  position: fixed !important;
  z-index: 999999 !important;
  background: var(--cs-panel) !important;
  border: 1px solid var(--cs-card-border) !important;
  border-radius: 8px !important;
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.12), 0 3px 6px -4px rgba(0, 0, 0, 0.12) !important;
  padding: 4px 0 !important;
  min-width: 185px !important;
  max-height: calc(100vh - 20px) !important;
  overflow-y: auto !important;
  user-select: none !important;
}

.cs-file-ctx-menu .ctx-item {
  display: flex !important;
  align-items: center !important;
  padding: 8px 14px !important;
  font-size: 13px !important;
  color: var(--cs-text) !important;
  cursor: pointer !important;
  transition: background 0.1s, color 0.1s !important;
}

.cs-file-ctx-menu .ctx-item:hover {
  background: var(--cs-hover) !important;
  color: var(--cs-text-bright) !important;
}

.cs-file-ctx-menu .ctx-item.ctx-danger {
  color: var(--cs-error) !important;
}

.cs-file-ctx-menu .ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, var(--cs-error) 10%, transparent) !important;
  color: var(--cs-error) !important;
}

.cs-file-ctx-menu .ctx-divider {
  height: 1px !important;
  background: var(--cs-card-border) !important;
  margin: 4px 0 !important;
}
</style>

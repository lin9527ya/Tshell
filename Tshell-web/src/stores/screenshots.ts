import { defineStore } from 'pinia'
import { ref } from 'vue'
import ClientAPI from '@/api/clients'
import { useClientsStore } from '@/stores/clients'

export interface ScreenshotItem {
  id: string
  uid: string
  host: string
  user: string
  computer: string
  session: string
  title: string
  dataUrl: string
  time: string
  quality: number
  qualityLabel: string
  color?: string
}

export const useScreenshotsStore = defineStore('screenshots', () => {
  const screenshots = ref<ScreenshotItem[]>([])
  const isCapturing = ref(false)
  const capturingUid = ref('')

  // 【修复】截图持有完整 base64 图片,无上限会导致内存持续增长(渲染侧
  // GlobalScreenshotsView 还会全量解码);FIFO 截断到 100 张。
  // 需要长期留存的截图应下载/导出,而非依赖内存列表。
  const MAX_SCREENSHOTS = 100

  const formatTime = () => {
    const d = new Date()
    const yy = d.getFullYear()
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    const hh = String(d.getHours()).padStart(2, '0')
    const min = String(d.getMinutes()).padStart(2, '0')
    const ss = String(d.getSeconds()).padStart(2, '0')
    return `${yy}/${mm}/${dd} ${hh}:${min}:${ss}`
  }

  const qualityLabelsZh = ['低画质', '中画质', '高画质']
  const qualityLabelsEn = ['Low', 'Medium', 'High']

  const addScreenshot = (item: ScreenshotItem) => {
    screenshots.value.unshift(item)
    if (screenshots.value.length > MAX_SCREENSHOTS) {
      screenshots.value.length = MAX_SCREENSHOTS // FIFO 截断,防止 base64 累积撑爆内存
    }
  }

  const removeScreenshot = (id: string) => {
    screenshots.value = screenshots.value.filter((s) => s.id !== id)
  }

  const clearScreenshots = (uid?: string) => {
    if (uid) {
      screenshots.value = screenshots.value.filter((s) => s.uid !== uid)
    } else {
      screenshots.value = []
    }
  }

  const capture = async (uid: string, quality: number = 1, isZh: boolean = true) => {
    if (!uid) {
      throw new Error(isZh ? '未指定客户端 UID' : 'No client UID specified')
    }
    const clientsStore = useClientsStore()
    const client = clientsStore.getClientByUid(uid)

    isCapturing.value = true
    capturingUid.value = uid
    try {
      const res = await ClientAPI.take_screenshot({ uid, quality })
      let dataUrl = res?.data?.data || res?.data
      if ((res?.status === 200 || res?.data?.status === 200) && dataUrl) {
        if (typeof dataUrl === 'string' && !dataUrl.startsWith('data:image')) {
          dataUrl = `data:image/png;base64,${dataUrl}`
        }
        const qualityName = isZh
          ? (qualityLabelsZh[quality] || '中画质')
          : (qualityLabelsEn[quality] || 'Medium')

        const clientUser = client?.Username || 'SYSTEM *'
        const clientComp = client?.Computer || client?.ExternalIP || 'Host'
        const clientHost = `${clientUser}@${clientComp}`

        const newItem: ScreenshotItem = {
          id: `sc_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
          uid,
          host: clientHost,
          user: clientUser,
          computer: clientComp,
          session: client?.Session || '1',
          title: client ? `${clientComp} - [${client.InternalIP || client.ExternalIP || '127.0.0.1'}]` : 'Tshell Screen',
          dataUrl,
          time: formatTime(),
          quality,
          qualityLabel: qualityName,
        }

        addScreenshot(newItem)
        return newItem
      } else {
        throw new Error(res?.data?.error || res?.data?.message || (isZh ? '抓取截图失败' : 'Screenshot capture failed'))
      }
    } finally {
      isCapturing.value = false
      capturingUid.value = ''
    }
  }

  return {
    screenshots,
    isCapturing,
    capturingUid,
    addScreenshot,
    removeScreenshot,
    clearScreenshots,
    capture,
  }
})

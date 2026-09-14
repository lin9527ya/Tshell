import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ScriptLogLine {
  time: string
  prefix: string
  text: string
  type: 'info' | 'output' | 'error' | 'warn' | 'cmd'
}

export const useScriptConsoleStore = defineStore('scriptConsole', () => {
  const lines = ref<ScriptLogLine[]>([])

  const getTime = () => new Date().toLocaleTimeString('en-GB')

  function appendLine(text: string, type: ScriptLogLine['type'] = 'output', prefix = '') {
    lines.value.push({
      time: getTime(),
      prefix: prefix ? `${prefix} ` : '',
      text,
      type,
    })
    if (lines.value.length > 1000) {
      lines.value.splice(0, lines.value.length - 1000)
    }
  }

  function clear() {
    lines.value = []
  }

  function logScriptLoaded(name: string, path?: string) {
    appendLine(`[+] Aggressor script loaded: ${name}${path ? ` (${path})` : ''}`, 'info')
  }

  function logScriptUnloaded(name: string) {
    appendLine(`[-] Aggressor script unloaded: ${name}`, 'warn')
  }

  function logScriptReloaded(name: string) {
    appendLine(`[*] Aggressor script reloaded: ${name}`, 'info')
  }

  function logScriptError(name: string, err: string) {
    appendLine(`[!] Script error in ${name}: ${err}`, 'error')
  }

  return {
    lines,
    appendLine,
    clear,
    logScriptLoaded,
    logScriptUnloaded,
    logScriptReloaded,
    logScriptError,
  }
})

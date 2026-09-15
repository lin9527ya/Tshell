/**
 * 上线提示音(Web Audio 合成,零资源文件)。
 * 服务端为无头进程播音频无人听——语音/提示音统一由前端播放(P1-7);
 * 浏览器自动播放策略: 需用户交互后才能出声,登录成功后调 primeAudio() 解锁。
 */

let ctx: AudioContext | null = null

/** 解锁音频上下文(用户首次交互/登录成功后调用一次) */
export function primeAudio() {
  try {
    if (!ctx) ctx = new AudioContext()
    if (ctx.state === 'suspended') ctx.resume().catch(() => {})
  } catch { /* AudioContext 不可用则静默 */ }
}

/** 两音上行提示音(客户端上线) */
export function playOnlineChime() {
  try {
    if (!ctx) ctx = new AudioContext()
    if (ctx.state === 'suspended') { ctx.resume().catch(() => {}); return }
    const t0 = ctx.currentTime
    const mk = (freq: number, start: number, dur: number) => {
      const osc = ctx!.createOscillator()
      const gain = ctx!.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, t0 + start)
      gain.gain.exponentialRampToValueAtTime(0.12, t0 + start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, t0 + start + dur)
      osc.connect(gain).connect(ctx!.destination)
      osc.start(t0 + start)
      osc.stop(t0 + start + dur + 0.05)
    }
    mk(880, 0, 0.15)   // A5
    mk(1174.7, 0.12, 0.2) // D6
  } catch { /* 静默失败 */ }
}

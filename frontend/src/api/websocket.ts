import { ref, type Ref } from 'vue'

export interface WsMessage {
  type: string
  data: any
}

type MessageHandler = (msg: WsMessage) => void

class WebSocketClient {
  private ws: WebSocket | null = null
  private url: string = ''
  private handlers: Map<string, Set<MessageHandler>> = new Map()
  private reconnectTimer: number | null = null
  private reconnectDelay: number = 3000
  private maxReconnectDelay: number = 30000
  private currentDelay: number = 3000
  private manualClose: boolean = false

  public connected: Ref<boolean> = ref(false)

  connect() {
    this.manualClose = false

    // 构建WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = import.meta.env.VITE_WS_URL || '/ws'
    
    // 如果配置了完整URL则使用，否则使用当前host
    if (wsUrl.startsWith('ws://') || wsUrl.startsWith('wss://')) {
      this.url = wsUrl
    } else {
      // 相对路径，使用当前host
      const host = window.location.host
      const path = wsUrl.startsWith('/') ? wsUrl : `/${wsUrl}`
      this.url = `${protocol}//${host}${path}`
    }

    this.doConnect()
  }

  private doConnect() {
    if (this.ws?.readyState === WebSocket.OPEN) return

    try {
      this.ws = new WebSocket(this.url)

      this.ws.onopen = () => {
        this.connected.value = true
        this.currentDelay = this.reconnectDelay
        console.log('[WS] 已连接')
      }

      this.ws.onmessage = (event) => {
        try {
          const msg: WsMessage = JSON.parse(event.data)
          this.dispatch(msg)
        } catch (e) {
          console.warn('[WS] 解析消息失败:', e)
        }
      }

      this.ws.onclose = () => {
        this.connected.value = false
        if (!this.manualClose) {
          this.scheduleReconnect()
        }
      }

      this.ws.onerror = () => {
        this.connected.value = false
      }
    } catch (e) {
      this.scheduleReconnect()
    }
  }

  private scheduleReconnect() {
    if (this.reconnectTimer) return
    console.log(`[WS] ${this.currentDelay / 1000}秒后重连...`)
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null
      this.doConnect()
      // 指数退避
      this.currentDelay = Math.min(this.currentDelay * 1.5, this.maxReconnectDelay)
    }, this.currentDelay)
  }

  private dispatch(msg: WsMessage) {
    const handlers = this.handlers.get(msg.type)
    if (handlers) {
      handlers.forEach(fn => {
        try { fn(msg) } catch (e) { console.error('[WS] handler error:', e) }
      })
    }
    // 也触发通配符监听
    const allHandlers = this.handlers.get('*')
    if (allHandlers) {
      allHandlers.forEach(fn => {
        try { fn(msg) } catch (e) { console.error('[WS] handler error:', e) }
      })
    }
  }

  on(type: string, handler: MessageHandler) {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set())
    }
    this.handlers.get(type)!.add(handler)
  }

  off(type: string, handler: MessageHandler) {
    this.handlers.get(type)?.delete(handler)
  }

  disconnect() {
    this.manualClose = true
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.ws?.close()
    this.ws = null
    this.connected.value = false
  }
}

// 全局单例
export const wsClient = new WebSocketClient()

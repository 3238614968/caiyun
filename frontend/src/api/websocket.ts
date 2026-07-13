import { ref, type Ref } from 'vue'

export interface WsMessage {
  type: string
  data: any
  message_id?: string
  sequence?: number
  created_at?: number
  expires_at?: number
}

type MessageHandler = (msg: WsMessage) => void

export class WebSocketClient {
  private ws: WebSocket | null = null
  private url = ''
  private handlers: Map<string, Set<MessageHandler>> = new Map()
  private reconnectTimer: number | null = null
  private reconnectDelay = 3000
  private maxReconnectDelay = 30000
  private currentDelay = 3000
  private heartbeatInterval = 30000
  private heartbeatTimeout = 10000
  private heartbeatTimer: number | null = null
  private heartbeatTimeoutTimer: number | null = null
  private awaitingPong = false
  private manualClose = false
  private suppressNextReconnect = false
  private globalListenersBound = false


  private seenMessageIds = new Set<string>()
  private readonly maxSeenMessageIds = 2048
  public connected: Ref<boolean> = ref(false)

  connect() {
    this.manualClose = false
    this.bindGlobalListeners()

    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = import.meta.env.VITE_WS_URL || '/ws'

    if (wsUrl.startsWith('ws://') || wsUrl.startsWith('wss://')) {
      this.url = wsUrl
    } else {
      const host = window.location.host
      const path = wsUrl.startsWith('/') ? wsUrl : `/${wsUrl}`
      this.url = `${protocol}//${host}${path}`
    }

    this.doConnect()
  }

  private bindGlobalListeners() {
    if (this.globalListenersBound) {
      return
    }
    window.addEventListener('online', this.handleOnline)
    window.addEventListener('offline', this.handleOffline)
    document.addEventListener('visibilitychange', this.handleVisibilityChange)
    this.globalListenersBound = true
  }

  private readonly handleOnline = () => {
    if (this.manualClose) {
      return
    }
    console.log('[WS] 网络已恢复，强制刷新 WebSocket 连接')
    this.forceReconnect()
  }

  private readonly handleOffline = () => {
    this.connected.value = false
    this.closeStaleSocket()
  }

  private readonly handleVisibilityChange = () => {
    if (this.manualClose || document.visibilityState !== 'visible') {
      return
    }
    if (!this.ws || this.ws.readyState === WebSocket.CLOSED) {
      console.log('[WS] 页面恢复可见，立即尝试重连')
      this.reconnectNow()
      return
    }
    if (!this.connected.value || this.awaitingPong) {
      console.log('[WS] 页面恢复可见，检测到连接可能半开，强制重连')
      this.forceReconnect()
    }
  }

  private doConnect() {
    if (!this.url) {
      return
    }
    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return
    }

    try {
      const socket = new WebSocket(this.url)
      this.ws = socket

      socket.onopen = () => {
        this.connected.value = true
        this.currentDelay = this.reconnectDelay
        this.clearReconnectTimer()
        this.startHeartbeat()
        console.log('[WS] 已连接')
      }

      socket.onmessage = (event) => {
        try {
          const msg: WsMessage = JSON.parse(event.data)
          if (msg.type === 'pong') {
            this.markHeartbeatAlive()
          }
          if (msg.message_id && this.seenMessageIds.has(msg.message_id)) {
            this.acknowledge(socket, msg)
            return
          }
          const handled = this.dispatch(msg)
          if (handled && msg.message_id) {
            this.rememberMessage(msg.message_id)
            this.acknowledge(socket, msg)
          }
        } catch (e) {
          console.warn('[WS] 解析消息失败:', e)
        }
      }

      socket.onclose = (event) => {
        if (this.ws === socket) {
          this.ws = null
        }
        this.stopHeartbeat()
        this.connected.value = false
        if (this.suppressNextReconnect) {
          this.suppressNextReconnect = false
          return
        }
        if (this.isAuthClose(event)) {
          this.manualClose = true
          window.dispatchEvent(new Event('auth:clear'))
          void import('@/router').then(({ default: router }) => {
            router.replace({ name: 'Login' })
          })
          return
        }
        if (!this.manualClose) {
          this.scheduleReconnect()
        }
      }

      socket.onerror = () => {
        this.connected.value = false
        if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
          socket.close()
        }
      }
    } catch {
      this.connected.value = false
      this.scheduleReconnect()
    }
  }

  private isAuthClose(event: CloseEvent) {
    return event.code === 1008 || event.code === 4001 || event.code === 4401
  }

  private startHeartbeat() {
    this.stopHeartbeat()
    this.heartbeatTimer = window.setInterval(() => {
      this.sendHeartbeat()
    }, this.heartbeatInterval)
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
    this.clearHeartbeatTimeout()
    this.awaitingPong = false
  }

  private clearHeartbeatTimeout() {
    if (this.heartbeatTimeoutTimer) {
      clearTimeout(this.heartbeatTimeoutTimer)
      this.heartbeatTimeoutTimer = null
    }
  }

  private markHeartbeatAlive() {
    this.awaitingPong = false
    this.clearHeartbeatTimeout()
  }

  private sendHeartbeat() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }
    if (this.awaitingPong) {
      console.warn('[WS] 心跳超时未收到 pong，关闭并重连')
      this.closeStaleSocket()
      this.scheduleReconnect()
      return
    }
    try {
      this.awaitingPong = true
      this.ws.send(JSON.stringify({ type: 'ping', data: { ts: Date.now() } }))
      this.clearHeartbeatTimeout()
      this.heartbeatTimeoutTimer = window.setTimeout(() => {
        if (this.awaitingPong) {
          console.warn('[WS] 心跳 pong 等待超时，关闭并重连')
          this.closeStaleSocket()
          this.scheduleReconnect()
        }
      }, this.heartbeatTimeout)
    } catch (error) {
      console.warn('[WS] 心跳发送失败:', error)
      this.closeStaleSocket()
      this.scheduleReconnect()
    }
  }

  private getJitteredDelay(baseDelay: number) {
    const bounded = Math.min(baseDelay, this.maxReconnectDelay)
    const jitter = Math.min(1000, Math.floor(bounded * 0.2))
    return Math.min(bounded + Math.floor(Math.random() * (jitter + 1)), this.maxReconnectDelay)
  }

  private scheduleReconnect() {
    if (this.manualClose || this.reconnectTimer) {
      return
    }
    const delay = this.getJitteredDelay(this.currentDelay)
    console.log(`[WS] ${Math.round(delay / 100) / 10}秒后重连...`)
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null
      this.doConnect()
      this.currentDelay = Math.min(Math.round(this.currentDelay * 1.6), this.maxReconnectDelay)
    }, delay)
  }

  private reconnectNow() {
    this.clearReconnectTimer()
    this.currentDelay = this.reconnectDelay
    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return
    }
    this.doConnect()
  }

  private forceReconnect() {
    this.clearReconnectTimer()
    this.currentDelay = this.reconnectDelay
    this.closeStaleSocket()
    this.doConnect()
  }

  private closeStaleSocket() {
    const socket = this.ws
    this.ws = null
    this.stopHeartbeat()
    if (!socket) {
      return
    }
    try {
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
        this.suppressNextReconnect = true
        socket.close()
      }
    } catch (error) {
      console.warn('[WS] 关闭旧连接失败:', error)
    }
  }

  private clearReconnectTimer() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }

  private dispatch(msg: WsMessage) {
    let handled = true
    const invoke = (handlers?: Set<MessageHandler>) => {
      handlers?.forEach(fn => {
        try {
          fn(msg)
        } catch (e) {
          handled = false
          console.error('[WS] handler error:', e)
        }
      })
    }
    invoke(this.handlers.get(msg.type))
    invoke(this.handlers.get('*'))
    return handled
  }

  private rememberMessage(messageId: string) {
    this.seenMessageIds.add(messageId)
    if (this.seenMessageIds.size <= this.maxSeenMessageIds) {
      return
    }
    const oldest = this.seenMessageIds.values().next().value
    if (oldest) {
      this.seenMessageIds.delete(oldest)
    }
  }

  private acknowledge(socket: WebSocket, msg: WsMessage) {
    if (!msg.message_id || socket.readyState !== WebSocket.OPEN) {
      return
    }
    try {
      socket.send(JSON.stringify({
        type: 'ack',
        message_id: msg.message_id,
        sequence: msg.sequence
      }))
    } catch (error) {
      console.warn('[WS] 消息确认发送失败:', error)
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
    this.clearReconnectTimer()
    this.stopHeartbeat()
    this.ws?.close()
    this.ws = null
    this.connected.value = false
  }
}

export const wsClient = new WebSocketClient()

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WebSocketClient } from './websocket'

vi.mock('@/router', () => ({
  default: {
    replace: vi.fn()
  }
}))

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  static instances: MockWebSocket[] = []

  readyState = MockWebSocket.CONNECTING
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: (() => void) | null = null
  sent: string[] = []
  closeCalls = 0

  constructor(public url: string) {
    MockWebSocket.instances.push(this)
  }

  send(data: string) {
    this.sent.push(data)
  }

  close(code = 1000) {
    this.closeCalls++
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.({ code } as CloseEvent)
  }

  open() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.()
  }

  message(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }
}

describe('WebSocketClient', () => {
  let originalWebSocket: typeof WebSocket

  beforeEach(() => {
    vi.useFakeTimers()
    MockWebSocket.instances = []
    originalWebSocket = globalThis.WebSocket
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('sends heartbeat ping while connected', () => {
    const client = new WebSocketClient()

    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()

    vi.advanceTimersByTime(30000)

    expect(socket.sent).toHaveLength(1)
    expect(JSON.parse(socket.sent[0])).toMatchObject({ type: 'ping' })

    client.disconnect()
  })



  it('acknowledges an event after dispatch and suppresses retry duplicates', () => {
    const client = new WebSocketClient()
    const handler = vi.fn()
    client.on('task_complete', handler)

    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()
    const event = { type: 'task_complete', data: { ok: true }, message_id: 'm-1', sequence: 7 }
    socket.message(event)
    socket.message(event)

    expect(handler).toHaveBeenCalledTimes(1)
    expect(socket.sent.map(item => JSON.parse(item))).toEqual([
      { type: 'ack', message_id: 'm-1', sequence: 7 },
      { type: 'ack', message_id: 'm-1', sequence: 7 }
    ])
    client.disconnect()
  })
  it('force reconnects stale socket after offline then online', () => {
    const client = new WebSocketClient()

    client.connect()
    const firstSocket = MockWebSocket.instances[0]
    firstSocket.open()

    window.dispatchEvent(new Event('offline'))
    expect(firstSocket.closeCalls).toBe(1)
    expect(client.connected.value).toBe(false)

    window.dispatchEvent(new Event('online'))
    expect(MockWebSocket.instances).toHaveLength(2)

    client.disconnect()
  })

  it('closes half-open socket when heartbeat pong times out and reconnects', () => {
    const client = new WebSocketClient()

    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()

    vi.advanceTimersByTime(30000)
    expect(socket.sent).toHaveLength(1)

    vi.advanceTimersByTime(10000)
    expect(socket.closeCalls).toBe(1)

    vi.advanceTimersByTime(5000)
    expect(MockWebSocket.instances).toHaveLength(2)

    client.disconnect()
  })

  it('keeps connection alive when heartbeat pong is received', () => {
    const client = new WebSocketClient()

    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()

    vi.advanceTimersByTime(30000)
    socket.message({ type: 'pong', data: { ts: Date.now() } })
    vi.advanceTimersByTime(10000)

    expect(socket.closeCalls).toBe(0)
    expect(MockWebSocket.instances).toHaveLength(1)

    client.disconnect()
  })


  it('dispatches auth clear and skips reconnect on auth close code', async () => {
    const client = new WebSocketClient()
    const authClear = vi.fn()
    window.addEventListener('auth:clear', authClear)

    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()
    socket.close(1008)

    vi.advanceTimersByTime(30000)
    await Promise.resolve()

    expect(authClear).toHaveBeenCalledTimes(1)
    expect(MockWebSocket.instances).toHaveLength(1)

    window.removeEventListener('auth:clear', authClear)
  })
})

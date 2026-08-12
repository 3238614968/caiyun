import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { isOperationUpdatedMessage, WebSocketClient, type WsMessage } from './websocket'

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

class MockEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances: MockEventSource[] = []

  readyState = MockEventSource.CONNECTING
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  closeCalls = 0

  constructor(public url: string, public options?: EventSourceInit) {
    MockEventSource.instances.push(this)
  }

  close() {
    this.closeCalls++
    this.readyState = MockEventSource.CLOSED
  }

  open() {
    this.readyState = MockEventSource.OPEN
    this.onopen?.()
  }

  message(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }

  fail() {
    this.readyState = MockEventSource.CLOSED
    this.onerror?.()
  }
}


describe('operation.updated contract guard', () => {
  it('accepts complete generated operation payloads and rejects incomplete messages', () => {
    const valid: WsMessage = {
      type: 'operation.updated',
      message_id: 'operation-event-1',
      sequence: 9,
      data: {
        operation_id: 'op-1',
        type: 'account_tasks',
        status: 'succeeded',
        attempt_count: 1,
        queued_at: '2026-07-25T00:00:00Z',
        updated_at: '2026-07-25T00:00:01Z'
      }
    }
    expect(isOperationUpdatedMessage(valid)).toBe(true)
    expect(isOperationUpdatedMessage({ ...valid, data: { operation_id: 'op-1', status: 'succeeded' } })).toBe(false)
  })
})

describe('WebSocketClient', () => {
  let originalWebSocket: typeof WebSocket
  let originalEventSource: typeof EventSource

  beforeEach(() => {
    vi.useFakeTimers()
    MockWebSocket.instances = []
    MockEventSource.instances = []
    vi.stubEnv('VITE_PUSH_TRANSPORT', 'ws')
    originalWebSocket = globalThis.WebSocket
    originalEventSource = globalThis.EventSource
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket
    globalThis.EventSource = MockEventSource as unknown as typeof EventSource
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    globalThis.EventSource = originalEventSource
    vi.unstubAllEnvs()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('uses SSE directly when the deployment explicitly configures it', () => {
    vi.stubEnv('VITE_PUSH_TRANSPORT', 'sse')
    vi.stubEnv('VITE_SSE_URL', '/events')
    const client = new WebSocketClient()
    const handler = vi.fn()
    client.on('task_complete', handler)

    client.connect()

    expect(MockWebSocket.instances).toHaveLength(0)
    const source = MockEventSource.instances[0]
    expect(source.url).toBe('/events')
    source.open()
    source.message({ type: 'task_complete', data: { ok: true }, message_id: 'sse-1', sequence: 8 })

    expect(client.connected.value).toBe(true)
    expect(client.transport.value).toBe('sse')
    expect(handler).toHaveBeenCalledTimes(1)
    client.disconnect()
    expect(source.closeCalls).toBe(1)
  })

  it('dispatches durable operation updates received over SSE', () => {
    vi.stubEnv('VITE_PUSH_TRANSPORT', 'sse')
    const client = new WebSocketClient()
    const handler = vi.fn()
    client.on('operation.updated', handler)

    client.connect()
    const source = MockEventSource.instances[0]
    source.open()
    source.message({
      type: 'operation.updated',
      data: {
        operation_id: 'op-1',
        type: 'account_tasks',
        status: 'succeeded',
        attempt_count: 1,
        queued_at: '2026-07-25T00:00:00Z',
        updated_at: '2026-07-25T00:00:01Z'
      },
      message_id: 'operation-event-1',
      sequence: 9
    })

    expect(handler).toHaveBeenCalledWith(expect.objectContaining({
      type: 'operation.updated',
      data: expect.objectContaining({ operation_id: 'op-1', status: 'succeeded' })
    }))
    client.disconnect()
  })

  it('does not dispatch or acknowledge malformed WebSocket durable events', () => {
    const client = new WebSocketClient()
    const handler = vi.fn()
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    client.on('operation.updated', handler)
    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()

    socket.message({ type: '', data: {}, message_id: 'empty-type', sequence: 31 })
    socket.message({ type: 'task_complete', message_id: 'missing-data', sequence: 32 })
    socket.message({
      type: 'operation.updated',
      data: { operation_id: 'op-1', status: 'succeeded' },
      message_id: 'invalid-operation',
      sequence: 33
    })

    expect(handler).not.toHaveBeenCalled()
    expect(consoleWarn).toHaveBeenCalledTimes(3)
    expect(socket.sent).toHaveLength(0)
    client.disconnect()
  })

  it('does not advance the SSE replay cursor for malformed events', () => {
    vi.stubEnv('VITE_PUSH_TRANSPORT', 'sse')
    vi.spyOn(Math, 'random').mockReturnValue(0)
    const client = new WebSocketClient()
    const handler = vi.fn()
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    client.on('task_complete', handler)
    client.connect()
    const first = MockEventSource.instances[0]
    first.open()

    first.message({ type: 'task_complete', message_id: 'missing-data', sequence: 37 })
    first.message({ type: 'operation.updated', data: { operation_id: 'op-1' }, message_id: 'invalid-operation', sequence: 38 })
    first.fail()
    vi.advanceTimersByTime(3000)

    expect(handler).not.toHaveBeenCalled()
    expect(consoleWarn).toHaveBeenCalledTimes(2)
    expect(MockEventSource.instances[1].url).toBe('/events')
    client.disconnect()
  })

  it('reconnects SSE with exponential backoff and preserves the last event sequence', () => {
    vi.stubEnv('VITE_PUSH_TRANSPORT', 'sse')
    vi.spyOn(Math, 'random').mockReturnValue(0)
    const client = new WebSocketClient()
    client.connect()
    const first = MockEventSource.instances[0]
    first.open()
    first.message({ type: 'task_complete', data: { ok: true }, message_id: 'sse-21', sequence: 21 })
    first.fail()

    vi.advanceTimersByTime(3000)

    expect(MockEventSource.instances).toHaveLength(2)
    expect(MockEventSource.instances[1].url).toBe('/events?last_event_id=21')
    client.disconnect()
  })

  it('periodically probes WebSocket again after auto mode falls back to SSE', () => {
    vi.stubEnv('VITE_PUSH_TRANSPORT', 'auto')
    const client = new WebSocketClient()
    client.connect()
    const initialSocket = MockWebSocket.instances[0]
    initialSocket.close()
    expect(MockEventSource.instances).toHaveLength(1)
    const source = MockEventSource.instances[0]
    source.open()

    vi.advanceTimersByTime(60000)
    expect(MockWebSocket.instances).toHaveLength(2)
    const recoveredSocket = MockWebSocket.instances[1]
    recoveredSocket.open()

    expect(source.closeCalls).toBe(1)
    expect(client.transport.value).toBe('ws')
    client.disconnect()
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

  it('acknowledges a durable event even when a UI handler throws', () => {
    const client = new WebSocketClient()
    client.on('notification', () => { throw new Error('render failed') })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    client.connect()
    const socket = MockWebSocket.instances[0]
    socket.open()
    socket.message({ type: 'notification', data: { title: 'notice' }, message_id: 'notice-1', sequence: 10 })

    expect(consoleError).toHaveBeenCalled()
    expect(socket.sent.map(item => JSON.parse(item))).toContainEqual({
      type: 'ack', message_id: 'notice-1', sequence: 10
    })
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

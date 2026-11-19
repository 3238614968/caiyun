import axios, { AxiosError, type AxiosAdapter, type AxiosResponse } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('element-plus', () => ({ ElMessage: { error: vi.fn() } }))
vi.mock('@/router', () => ({ default: { replace: vi.fn() } }))

describe('browser session refresh coordination', () => {
  const defaultAdapter = axios.defaults.adapter
  beforeEach(() => {
    // Each imported axios module below represents an independent browser tab;
    // only the browser lock manager and cookie-backed server session are shared.
    let tail = Promise.resolve()
    vi.stubGlobal('navigator', {
      locks: {
        request: (_name: string, action: () => Promise<unknown>) => {
          const result = tail.then(action)
          tail = result.then(() => undefined, () => undefined)
          return result
        }
      }
    })
  })

  afterEach(() => {
    axios.defaults.adapter = defaultAdapter
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  async function tab(adapter: AxiosAdapter) {
    vi.resetModules()
    const module = await import('./axios')
    return { get: (url: string) => module.http.get(url, { adapter }) }
  }

  it('rotates once for simultaneous 401s in separate tabs and retries both requests', async () => {
    let active = false
    let rotations = 0
    let protectedRequests = 0
    const adapter: AxiosAdapter = async config => {
      const response: AxiosResponse = { config, status: 200, statusText: 'OK', headers: {}, data: { ok: true } }
      if (config.url === '/api/v1/auth/refresh') {
        rotations++
        await new Promise(resolve => setTimeout(resolve, 10))
        active = true
      } else {
        if (config.url === '/protected') protectedRequests++
        if (!active) {
          response.status = 401
          throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, response)
        }
      }
      return response
    }
    axios.defaults.adapter = adapter
    const first = await tab(adapter)
    const second = await tab(adapter)

    await expect(Promise.all([first.get('/protected'), second.get('/protected')]))
      .resolves.toEqual([{ ok: true }, { ok: true }])
    expect(rotations).toBe(1)
    expect(protectedRequests).toBe(4)
  })

  it('does not consume a refresh token when another tab already renewed the session', async () => {
    let rotations = 0
    let requests = 0
    const adapter: AxiosAdapter = async config => {
      const response: AxiosResponse = { config, status: 200, statusText: 'OK', headers: {}, data: {} }
      if (config.url === '/api/v1/auth/refresh') rotations++
      if (config.url === '/protected' && requests++ === 0) {
        response.status = 401
        throw new AxiosError('Stale response', 'ERR_BAD_REQUEST', config, undefined, response)
      }
      return response
    }
    axios.defaults.adapter = adapter
    const client = await tab(adapter)
    await client.get('/protected')
    expect(rotations).toBe(0)
    expect(requests).toBe(2)
  })

  it('does not rotate credentials when the session probe fails with a server error', async () => {
    let rotations = 0
    const adapter: AxiosAdapter = async config => {
      if (config.url === '/api/v1/auth/refresh') rotations++
      const response: AxiosResponse = { config, status: config.url === '/protected' ? 401 : 503, statusText: 'Unavailable', headers: {}, data: {} }
      throw new AxiosError('Unavailable', 'ERR_BAD_RESPONSE', config, undefined, response)
    }
    axios.defaults.adapter = adapter
    const client = await tab(adapter)
    await expect(client.get('/protected')).rejects.toBeDefined()
    expect(rotations).toBe(0)
  })
})

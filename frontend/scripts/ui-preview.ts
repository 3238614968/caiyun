import type { Plugin, ViteDevServer } from 'vite'
import type { IncomingMessage, ServerResponse } from 'node:http'
import type { Page, Route } from '@playwright/test'
import { adminUser, mockBackend } from '../e2e/helpers/mockApi'

/** Explicit, loopback-only development fixture. Never included in production app builds. */
export function uiPreviewPlugin(): Plugin {
  let handler: ((route: Route) => Promise<unknown>) | undefined
  let signedIn = true
  let previewUser = { ...adminUser, username: '预览管理员' }
  const configure = async (server: Pick<ViteDevServer, 'middlewares'>) => {
    await mockBackend({
      addInitScript: async () => undefined,
      route: async (_pattern: string, callback: typeof handler) => { handler = callback }
    } as unknown as Page)
    server.middlewares.use(async (req: IncomingMessage, res: ServerResponse, next) => {
      const url = req.url || ''
      if (!url.startsWith('/api/v1/') && !url.startsWith('/events')) { next(); return }
      if (!['127.0.0.1', '::1', '::ffff:127.0.0.1'].includes(req.socket.remoteAddress || '')) {
        res.statusCode = 403
        res.end('UI preview is only available on loopback')
        return
      }
      res.setHeader('X-Caiyun-Preview', 'example-data')
      if (url.startsWith('/events')) {
        res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache', Connection: 'keep-alive' })
        res.write('retry: 3000\n\n')
        const timer = setInterval(() => res.write(': preview heartbeat\n\n'), 3000)
        req.on('close', () => clearInterval(timer))
        return
      }
      try {
        let body = ''
        for await (const chunk of req) body += chunk.toString()
        const authPath = new URL(url, 'http://127.0.0.1').pathname
        if (authPath === '/api/v1/auth/login' || authPath === '/api/v1/auth/register') {
          const input = body ? JSON.parse(body) : {}
          signedIn = true
          previewUser = { ...adminUser, username: input.username || '预览管理员', email: input.email || adminUser.email, role: authPath.endsWith('/register') ? 'user' : 'admin' }
          res.writeHead(200, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify({ code: 0, message: 'success', data: { user: previewUser, expires_at: Math.floor(Date.now() / 1000) + 3600 } }))
          return
        }
        if (authPath === '/api/v1/auth/me') {
          res.writeHead(signedIn ? 200 : 401, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify(signedIn ? { code: 0, message: 'success', data: previewUser } : { code: 401, message: '请登录预览账号' }))
          return
        }
        if (url.startsWith('/api/v1/auth/logout')) signedIn = false
        if (url.startsWith('/api/v1/auth/refresh')) {
          res.writeHead(signedIn ? 200 : 401, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify(signedIn ? { code: 0, message: 'success', data: {} } : { code: 401, message: '请登录预览账号' }))
          return
        }
        const request = {
          url: () => `http://${req.headers.host}${url}`,
          method: () => req.method || 'GET',
          headers: () => ({ ...req.headers, cookie: signedIn ? 'e2e_auth=1' : '' }),
          postDataJSON: () => body ? JSON.parse(body) : {}
        }
        await handler?.({
          request: () => request,
          fulfill: async (response: { status?: number; contentType?: string; headers?: Record<string, string>; body?: string; json?: unknown }) => {
            res.writeHead(response.status || 200, { 'Content-Type': response.contentType || 'application/json', ...response.headers })
            let payload = response.body || JSON.stringify(response.json || {})
            // Readable fixture labels make the preview useful outside tests.
            payload = payload.replaceAll('e2e-admin', '预览管理员').replaceAll('e2e-user', '示例用户').replaceAll('E2E 公告', '工作空间使用小贴士').replaceAll('这是一条端到端测试公告。', '新增账号后，可以在这里查看任务动态与兑换结果。').replaceAll('E2E月卡', '移动云盘 · 会员月卡').replaceAll('E2E售罄券', '日常好礼 · 饮品兑换券')
            res.end(payload)
          }
        } as unknown as Route)
      } catch {
        if (!res.headersSent) res.writeHead(500, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 500, message: '预览数据处理失败' }))
      }
    })
  }
  return {
    name: 'caiyun-local-ui-preview',
    apply: 'serve',
    configureServer: configure,
    configurePreviewServer: configure
  }
}

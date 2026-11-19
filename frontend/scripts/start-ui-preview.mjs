import { build, preview } from 'vite'

process.env.CAIYUN_UI_PREVIEW = '1'
// Serve a dedicated build so first-time navigation never waits for dev dependency optimization.
const outDir = '.local/ui-preview-dist'
await build({ mode: 'ui-preview', logLevel: 'warn', build: { outDir, emptyOutDir: true } })
const server = await preview({
  mode: 'ui-preview',
  build: { outDir },
  preview: { host: '127.0.0.1', port: Number(process.env.UI_PREVIEW_PORT || 5173), strictPort: true }
})
server.printUrls()
server.bindCLIShortcuts({ print: true })
console.log('交互预览使用示例数据；不会连接真实云盘账号。')
process.on('SIGINT', async () => {
  await server.close()
  process.exit(0)
})

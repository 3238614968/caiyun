import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const configPath = resolve(import.meta.dirname, '..', 'vite.config.ts')
const config = await readFile(configPath, 'utf8')
const requiredProxies = ['/api', '/events', '/ws']
const missing = requiredProxies.filter(path => !new RegExp(`['"]${path}['"]\\s*:`).test(config))

if (missing.length > 0) {
  console.error(`Vite development proxy is missing: ${missing.join(', ')}`)
  process.exit(1)
}

if (!/['"]\/events['"]\s*:\s*\{[\s\S]*?target:\s*['"]http:\/\/localhost:8080['"]/.test(config)) {
  console.error('Vite /events proxy must target the backend HTTP server')
  process.exit(1)
}

console.log('Vite development proxies cover /api, /events and /ws')

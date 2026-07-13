import { readFileSync } from 'node:fs'
import { mkdir, readdir, stat, writeFile } from 'node:fs/promises'
import { join, relative } from 'node:path'
import { gzipSync } from 'node:zlib'
import { fileURLToPath } from 'node:url'

export const distDir = fileURLToPath(new URL('../dist/', import.meta.url))
export const reportDir = fileURLToPath(new URL('../reports/', import.meta.url))
export const reportPath = join(reportDir, 'bundle-report.json')

export async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...await walk(full))
    } else if (entry.isFile()) {
      files.push(full)
    }
  }
  return files
}

export function bucketOf(file) {
  const name = file.split(/[\\/]/).pop() ?? file
  if (name.endsWith('.css')) return 'css'
  if (name.endsWith('.js')) return 'js'
  if (name.endsWith('.html')) return 'html'
  if (/\.(png|jpe?g|gif|webp|avif|svg|ico)$/i.test(name)) return 'image'
  if (/\.(woff2?|ttf|eot)$/i.test(name)) return 'font'
  return 'other'
}

export async function collectBundleRows(rootDir = distDir) {
  const files = await walk(rootDir)
  const rows = []
  for (const file of files) {
    const info = await stat(file)
    const bytes = readFileSync(file)
    rows.push({
      file: relative(rootDir, file).replace(/\\/g, '/'),
      type: bucketOf(file),
      bytes: info.size,
      gzipBytes: gzipSync(bytes).length
    })
  }
  rows.sort((a, b) => b.bytes - a.bytes)
  return rows
}

export function summarizeRows(rows) {
  const totals = rows.reduce((acc, row) => {
    acc.bytes += row.bytes
    acc.gzipBytes += row.gzipBytes
    acc.byType[row.type] ??= { bytes: 0, gzipBytes: 0, files: 0 }
    acc.byType[row.type].bytes += row.bytes
    acc.byType[row.type].gzipBytes += row.gzipBytes
    acc.byType[row.type].files += 1
    return acc
  }, { bytes: 0, gzipBytes: 0, byType: {} })

  return {
    generatedAt: new Date().toISOString(),
    totals,
    largest: rows.slice(0, 20)
  }
}

export async function generateBundleReport(options = {}) {
  const { rootDir = distDir, persist = true } = options
  const rows = await collectBundleRows(rootDir)
  const report = summarizeRows(rows)
  if (persist) {
    await mkdir(reportDir, { recursive: true })
    await writeFile(reportPath, JSON.stringify(report, null, 2))
  }
  return report
}

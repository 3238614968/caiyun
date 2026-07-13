import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { generateBundleReport } from './bundle-report-lib.mjs'

const budgetPath = fileURLToPath(new URL('./bundle-budget.json', import.meta.url))
const budget = JSON.parse(await readFile(budgetPath, 'utf-8'))
const report = await generateBundleReport()
const failures = []

function formatBytes(bytes) {
  return `${(bytes / 1024).toFixed(1)} KB`
}

function assertWithin(label, actual, limit) {
  if (typeof limit !== 'number') return
  if (actual > limit) {
    failures.push(`${label}: ${formatBytes(actual)} > ${formatBytes(limit)}`)
  }
}

assertWithin('total bytes', report.totals.bytes, budget.totals?.bytes)
assertWithin('total gzip bytes', report.totals.gzipBytes, budget.totals?.gzipBytes)

for (const [type, limits] of Object.entries(budget.byType ?? {})) {
  const totals = report.totals.byType[type] ?? { bytes: 0, gzipBytes: 0 }
  assertWithin(`${type} bytes`, totals.bytes, limits.bytes)
  assertWithin(`${type} gzip bytes`, totals.gzipBytes, limits.gzipBytes)
}

for (const [type, limits] of Object.entries(budget.largest ?? {})) {
  const largest = report.largest.find(row => row.type === type)
  if (!largest) continue
  assertWithin(`largest ${type} asset`, largest.bytes, limits.bytes)
  assertWithin(`largest ${type} gzip asset`, largest.gzipBytes, limits.gzipBytes)
}

if (failures.length > 0) {
  console.error('Bundle budget check failed:')
  for (const failure of failures) {
    console.error(`- ${failure}`)
  }
  process.exit(1)
}

console.log('Bundle budget check passed.')
console.table([
  { metric: 'total bytes', value: formatBytes(report.totals.bytes), budget: formatBytes(budget.totals.bytes) },
  { metric: 'total gzip bytes', value: formatBytes(report.totals.gzipBytes), budget: formatBytes(budget.totals.gzipBytes) },
  { metric: 'js bytes', value: formatBytes(report.totals.byType.js?.bytes ?? 0), budget: formatBytes(budget.byType.js.bytes) },
  { metric: 'css bytes', value: formatBytes(report.totals.byType.css?.bytes ?? 0), budget: formatBytes(budget.byType.css.bytes) },
  { metric: 'image bytes', value: formatBytes(report.totals.byType.image?.bytes ?? 0), budget: formatBytes(budget.byType.image.bytes) }
])

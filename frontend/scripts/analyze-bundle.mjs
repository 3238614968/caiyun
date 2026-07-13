import { generateBundleReport } from './bundle-report-lib.mjs'

const report = await generateBundleReport()

console.log('Bundle report written to reports/bundle-report.json')
console.table(report.largest.slice(0, 10).map(row => ({
  file: row.file,
  type: row.type,
  kb: (row.bytes / 1024).toFixed(1),
  gzipKb: (row.gzipBytes / 1024).toFixed(1)
})))

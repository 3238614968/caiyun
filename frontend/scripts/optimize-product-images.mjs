import { access, mkdir, readdir, readFile, stat, writeFile } from 'node:fs/promises'
import { join, parse, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const sourceDir = join(root, 'assets-source/products-original')
const outDir = join(root, 'public/images/products-modern')
const mappingPath = join(root, 'public/images/products/image_mapping.json')

async function loadSharp() {
  try {
    return (await import('sharp')).default
  } catch {
    console.error('sharp is required for image conversion. Install it with: npm i -D sharp')
    process.exitCode = 1
    return null
  }
}

async function exists(path) {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

async function main() {
  const sharp = await loadSharp()
  if (!sharp) return

  if (!await exists(sourceDir)) {
    console.log('No product image directory found, skipping:', sourceDir)
    return
  }

  await mkdir(outDir, { recursive: true })
  const rewriteMapping = {}
  const entries = await readdir(sourceDir, { withFileTypes: true })
  const outputs = []
  for (const entry of entries) {
    if (!entry.isFile() || !/\.(png|jpe?g)$/i.test(entry.name)) continue
    const src = join(sourceDir, entry.name)
    const { name } = parse(entry.name)
    const webp = join(outDir, `${name}.webp`)
    const avif = join(outDir, `${name}.avif`)
    await sharp(src).resize({ width: 480, withoutEnlargement: true }).webp({ quality: 78 }).toFile(webp)
    await sharp(src).resize({ width: 480, withoutEnlargement: true }).avif({ quality: 50 }).toFile(avif)
    const srcSize = (await stat(src)).size
    const webpSize = (await stat(webp)).size
    const avifSize = (await stat(avif)).size
    outputs.push({
      source: relative(root, src).replace(/\\/g, '/'),
      webp: relative(root, webp).replace(/\\/g, '/'),
      avif: relative(root, avif).replace(/\\/g, '/'),
      sourceBytes: srcSize,
      webpBytes: webpSize,
      avifBytes: avifSize
    })
    rewriteMapping[name] = `/images/products-modern/${name}.webp`
  }

  const reportDir = join(root, 'reports')
  await mkdir(reportDir, { recursive: true })
  await writeFile(join(reportDir, 'product-image-report.json'), JSON.stringify({ generatedAt: new Date().toISOString(), images: outputs }, null, 2))
  if (await exists(mappingPath)) {
    const currentMapping = JSON.parse(await readFile(mappingPath, 'utf8'))
    const nextMapping = Object.fromEntries(
      Object.entries(currentMapping).map(([prizeId, source]) => {
        const { name } = parse(String(source))
        return [prizeId, rewriteMapping[name] || source]
      })
    )
    await writeFile(mappingPath, `${JSON.stringify(nextMapping, null, 2)}\n`)
    console.log('Updated image_mapping.json to point to products-modern/*.webp')
  }
  console.table(outputs.slice(0, 20).map(row => ({ file: row.source, pngKb: (row.sourceBytes / 1024).toFixed(1), webpKb: (row.webpBytes / 1024).toFixed(1), avifKb: (row.avifBytes / 1024).toFixed(1) })))
}

main().catch(error => {
  console.error(error)
  process.exit(1)
})

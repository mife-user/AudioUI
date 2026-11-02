import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('..', import.meta.url))
const src = join(root, 'src')
const allowed = join(src, 'styles', 'tokens.css')
const colorPattern = /#[0-9a-fA-F]{3,8}\b|rgba?\(/g

function walk(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    return statSync(path).isDirectory() ? walk(path) : [path]
  })
}

const violations = []
for (const file of walk(src)) {
  if (!/\.(css|ts|tsx)$/.test(file) || file === allowed) continue
  const text = readFileSync(file, 'utf8')
  text.split(/\r?\n/).forEach((line, index) => {
    if (colorPattern.test(line)) {
      violations.push(`${relative(root, file)}:${index + 1}: ${line.trim()}`)
    }
    colorPattern.lastIndex = 0
  })
}

if (violations.length > 0) {
  console.error('发现令牌文件之外的颜色字面量：')
  for (const item of violations) console.error(`  ${item}`)
  process.exit(1)
}

console.log('token check: ok')

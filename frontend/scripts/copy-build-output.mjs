import { cpSync, existsSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = join(scriptDir, '..')
const sourceDir = join(frontendDir, 'dist')
const destDir = join(frontendDir, '..', 'backend', 'internal', 'web', 'dist')

if (!existsSync(sourceDir)) {
  console.log(`[postbuild] source dist not found: ${sourceDir}`)
  process.exit(0)
}

mkdirSync(destDir, { recursive: true })

let lastError
for (let attempt = 1; attempt <= 8; attempt += 1) {
  try {
    cpSync(sourceDir, destDir, { recursive: true, force: true })
    console.log(`[postbuild] copied build output from ${sourceDir} to ${destDir}`)
    process.exit(0)
  } catch (error) {
    lastError = error
    if (attempt < 8) {
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 250 * attempt)
    }
  }
}

throw lastError

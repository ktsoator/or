import { mkdirSync } from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const desktopDirectory = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const outputDirectory = path.join(desktopDirectory, 'dist', 'sidecar')
const binaryName = process.platform === 'win32' ? 'coding-sidecar.exe' : 'coding-sidecar'
const output = path.join(outputDirectory, binaryName)

mkdirSync(outputDirectory, { recursive: true })
const result = spawnSync(
  'go',
  ['build', '-trimpath', '-o', output, '../cmd/coding-desktop'],
  { cwd: desktopDirectory, stdio: 'inherit' },
)

if (result.error) throw result.error
if (result.status !== 0) process.exit(result.status ?? 1)

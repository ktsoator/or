import { rm } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const desktopDirectory = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..',
)
const allowedTargets = new Map([
  ['dist', path.join(desktopDirectory, 'dist')],
  ['release', path.join(desktopDirectory, 'release')],
  ['client-dist', path.resolve(desktopDirectory, '../client/dist')],
])
const requestedTargets = process.argv.slice(2)

if (requestedTargets.length === 0) {
  throw new Error(`Specify one or more targets: ${[...allowedTargets.keys()].join(', ')}`)
}

for (const name of requestedTargets) {
  const target = allowedTargets.get(name)
  if (!target) throw new Error(`Unsupported clean target: ${name}`)
  await rm(target, { recursive: true, force: true })
  console.log(`Removed ${path.relative(desktopDirectory, target)}`)
}

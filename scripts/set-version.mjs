import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const manifestPaths = [
  'coding/client/package.json',
  'coding/desktop/package.json',
]

const args = process.argv.slice(2)
const requested = args.find((arg) => arg !== '--check')
const check = args.includes('--check')

if (!requested || args.some((arg) => arg !== '--check' && arg !== requested)) {
  fail('usage: node scripts/set-version.mjs <vX.Y.Z|X.Y.Z> [--check]')
}

const version = normalizeVersion(requested)
const mismatches = []

for (const relativePath of manifestPaths) {
  const absolutePath = path.join(repositoryRoot, relativePath)
  const source = await readFile(absolutePath, 'utf8')
  const manifest = JSON.parse(source)

  if (check) {
    if (manifest.version !== version) {
      mismatches.push(`${relativePath}: ${manifest.version ?? '<missing>'} (want ${version})`)
    }
    continue
  }

  if (manifest.version === version) {
    console.log(`${relativePath} already at ${version}`)
    continue
  }
  manifest.version = version
  await writeFile(absolutePath, `${JSON.stringify(manifest, null, 2)}\n`)
  console.log(`updated ${relativePath} to ${version}`)
}

if (mismatches.length > 0) {
  fail(`release tag and Or versions differ:\n${mismatches.join('\n')}`)
}
if (check) {
  console.log(`Or manifests match ${version}`)
}

function normalizeVersion(value) {
  const normalized = value.startsWith('v') ? value.slice(1) : value
  const semver =
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/
  if (!semver.test(normalized)) {
    fail(`invalid release version ${JSON.stringify(value)}; expected vX.Y.Z or X.Y.Z`)
  }
  return normalized
}

function fail(message) {
  console.error(message)
  process.exit(1)
}

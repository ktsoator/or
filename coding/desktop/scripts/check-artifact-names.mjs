import { readFile } from 'node:fs/promises'

const manifestURL = new URL('../package.json', import.meta.url)
const manifest = JSON.parse(await readFile(manifestURL, 'utf8'))
const expectedTemplate = '${productName}-${version}-${arch}.${ext}'
const actualTemplate = manifest.build?.artifactName

if (actualTemplate !== expectedTemplate) {
  console.error(
    `Coding artifactName is ${JSON.stringify(actualTemplate)}; want ${JSON.stringify(expectedTemplate)}`,
  )
  process.exit(1)
}

const names = ['arm64', 'x64'].flatMap((arch) =>
  ['dmg', 'zip'].map(
    (ext) => `${manifest.build.productName}-${manifest.version}-${arch}.${ext}`,
  ),
)
console.log(`Coding release artifacts: ${names.join(', ')}`)

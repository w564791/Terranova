#!/usr/bin/env node
/**
 * 从 hashicorp/syntax 拉取官方 TextMate grammar(与 marketplace hashicorp.hcl / terraform 高亮同源)。
 *
 * 用法:
 *   node scripts/update-hcl-grammar.mjs
 *   node scripts/update-hcl-grammar.mjs --ref v0.7.1
 *   REF=main node scripts/update-hcl-grammar.mjs
 *
 * 输出:
 *   src/pages/admin/ManifestEditorV2/extensions/assets/grammars/*.tmGrammar.json
 *   + VERSION.json 记录来源,便于审计
 */
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const outDir = path.resolve(
  __dirname,
  '../src/pages/admin/ManifestEditorV2/extensions/assets/grammars',
)

const args = process.argv.slice(2)
const refFlag = args.indexOf('--ref')
const ref =
  (refFlag >= 0 && args[refFlag + 1]) ||
  process.env.REF ||
  'main'

const files = ['hcl.tmGrammar.json', 'terraform.tmGrammar.json']

async function fetchOne(name) {
  const url = `https://raw.githubusercontent.com/hashicorp/syntax/${ref}/syntaxes/${name}`
  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(`GET ${url} → ${res.status} ${res.statusText}`)
  }
  const text = await res.text()
  // 校验 JSON
  JSON.parse(text)
  const dest = path.join(outDir, name)
  fs.writeFileSync(dest, text)
  return { name, bytes: Buffer.byteLength(text), url }
}

fs.mkdirSync(outDir, { recursive: true })

const results = []
for (const f of files) {
  // eslint-disable-next-line no-await-in-loop
  results.push(await fetchOne(f))
  console.log(`✓ ${f}`)
}

const meta = {
  source: 'https://github.com/hashicorp/syntax',
  ref,
  fetchedAt: new Date().toISOString(),
  files: results,
  marketplaceEquivalents: ['hashicorp.hcl', 'hashicorp.terraform (highlight only)'],
}
fs.writeFileSync(path.join(outDir, 'VERSION.json'), JSON.stringify(meta, null, 2) + '\n')
console.log(`\nUpdated grammars @ ${ref} → ${outDir}`)
console.log('Commit the grammar files + VERSION.json to ship the update.')

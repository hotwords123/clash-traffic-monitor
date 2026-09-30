import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
const source = await readFile(new URL('../src/filters.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const { toggleSelection, filterQuery, brushRange, chartBucket } = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'))

test('click toggles, plain click switches and Ctrl adds across pages', () => {
  assert.deepEqual(toggleSelection([], 'a', false), ['a'])
  assert.deepEqual(toggleSelection(['a'], 'b', false), ['b'])
  assert.deepEqual(toggleSelection(['a'], 'b', true), ['a', 'b'])
  assert.deepEqual(toggleSelection(['a', 'b'], 'a', false), ['b'])
  assert.deepEqual(toggleSelection(['a'], 'a', true), [])
})
test('selected names override search; clearing selection restores the full search', () => {
  const range = { from: 0, to: 3600 }
  const selected = filterQuery(range, ['off-page.exe', 'a&b.exe'], ['one.test'], 'no-match', 'no-match')
  assert.deepEqual(selected.getAll('app_selected'), ['off-page.exe', 'a&b.exe'])
  assert.equal(selected.has('app'), false)
  assert.equal(selected.has('domain'), false)
  const search = filterQuery(range, [], [], '.exe', '.test')
  assert.equal(search.get('app'), '.exe')
  assert.equal(search.get('domain'), '.test')
})
test('brush clamps and snaps to minutes without changing the display range', () => {
  const display = { from: 60, to: 3600 }
  assert.deepEqual(brushRange([500000, 100000], display), { from: 60, to: 540 })
  assert.deepEqual(brushRange([0, 9000000], display), display)
  assert.equal(brushRange([NaN, 1000], display), null)
  assert.equal(brushRange([0, 1000], display), null)
  assert.deepEqual(display, { from: 60, to: 3600 })
})
test('preset bucket sizes keep charts within 72 buckets', () => {
  for (const [hours, expected] of [[1, 1], [24, 20], [168, 180], [720, 720], [366*24, 10080]]) {
    const range = { from: 0, to: hours*3600 }
    assert.equal(chartBucket(range), expected)
    assert.ok(Math.ceil(hours*60/chartBucket(range)) <= 72)
  }
})

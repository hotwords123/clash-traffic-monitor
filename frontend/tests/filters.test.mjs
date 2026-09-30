import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
const source = await readFile(new URL('../src/filters.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const { toggleSelection, filterQuery, rankingQuery, brushRange, chartBucket, trafficAxis } = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'))

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

test('rankings apply the opposite selection without collapsing their own list', () => {
  const range = { from: 0, to: 3600 }
  const apps = rankingQuery('app', range, ['a.exe'], ['one.test', 'two.test'], '.exe', 'unmatched')
  assert.deepEqual(apps.getAll('app_selected'), [])
  assert.equal(apps.get('app'), '.exe')
  assert.deepEqual(apps.getAll('domain_selected'), ['one.test', 'two.test'])
  assert.equal(apps.has('domain'), false)
  const domains = rankingQuery('domain', range, ['a.exe', 'b.exe'], ['one.test'], 'unmatched', '.test')
  assert.deepEqual(domains.getAll('domain_selected'), [])
  assert.equal(domains.get('domain'), '.test')
  assert.deepEqual(domains.getAll('app_selected'), ['a.exe', 'b.exe'])
  assert.equal(domains.has('app'), false)
})
test('rankings fall back to the opposite search when its selection is cleared', () => {
  const range = { from: 0, to: 3600 }
  const domains = rankingQuery('domain', range, [], [], 'browser', '.test')
  assert.equal(domains.get('app'), 'browser')
  assert.equal(domains.get('domain'), '.test')
  const apps = rankingQuery('app', range, [], [], '.exe', 'example')
  assert.equal(apps.get('app'), '.exe')
  assert.equal(apps.get('domain'), 'example')
})

test('traffic axis uses integer labels and covers peaks across unit boundaries', () => {
  for (const peak of [0, NaN, 1, 1023, 1024, 1.1 * 1024 ** 2, 118.35 * 1024 ** 2, 1024 ** 3, 9 * 1024 ** 4]) {
    const axis = trafficAxis(peak)
    assert.ok(axis.max >= (Number.isFinite(peak) ? peak : 0))
    assert.ok(axis.interval > 0)
    assert.ok(axis.max / axis.interval <= 5)
    for (let value = 0; value <= axis.max; value += axis.interval) {
      assert.match(axis.formatter(value), /^\d+ (B|KB|MB|GB|TB)$/)
    }
  }
  const axis = trafficAxis(118.35 * 1024 ** 2)
  assert.equal(axis.formatter(axis.interval), '50 MB')
})

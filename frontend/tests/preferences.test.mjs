import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
const source = await readFile(new URL('../src/preferences.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const { loadPreferences, savePreferences, presetRange } = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'))
function memory(initial = null) {
  let stored = initial
  return { getItem: () => stored, setItem: (_, value) => { stored = value } }
}
const now = 1800000000000

test('rolling presets restore against current time, retaining only proxies', () => {
  for (const preset of [1, 24, 168, 720]) {
    const storage = memory()
    savePreferences({ preset, bounds: presetRange(preset, now), selectedTime: null, proxies: ['Group', 'DIRECT', ''], apps: ['app'], domains: ['site'] }, storage)
    const restored = loadPreferences(storage, now + 5 * 86400000)
    assert.deepEqual(restored.bounds, presetRange(preset, now + 5 * 86400000))
    assert.equal(restored.preset, preset)
    assert.deepEqual(restored.proxies, ['Group', 'DIRECT', ''])
    assert.equal(restored.selectedTime, null)
    const saved = JSON.parse(storage.getItem())
    assert.equal('apps' in saved, false)
    assert.equal('domains' in saved, false)
    assert.equal('range' in saved, false)
  }
})
test('custom dates and brush remain absolute on reopening', () => {
  const storage = memory()
  const value = { preset: 0, bounds: { from: 600, to: 3600 }, selectedTime: { from: 1200, to: 1800 }, proxies: ['Group'] }
  savePreferences(value, storage)
  assert.deepEqual(loadPreferences(storage, now + 86400000), value)
})
test('brush on rolling presets shifts with the current window', () => {
  const storage = memory()
  const bounds = presetRange(1, now)
  savePreferences({ preset: 1, bounds, selectedTime: { from: bounds.from + 600, to: bounds.from + 1200 }, proxies: [] }, storage)
  const restored = loadPreferences(storage, now + 86400000)
  assert.deepEqual(restored.selectedTime, { from: restored.bounds.from + 600, to: restored.bounds.from + 1200 })
})
test('invalid preferences fall back safely and sanitize selections', () => {
  for (const invalid of [null, '{bad', 'null', '{"version":2,"preset":1}']) {
    assert.deepEqual(loadPreferences(memory(invalid), now), { preset: 24, bounds: presetRange(24, now), selectedTime: null, proxies: [] })
  }
  const storage = memory(JSON.stringify({ version: 1, preset: 0, range: { from: 180, to: 60 }, proxies: ['A', 'A', '', 42, 'x'.repeat(1025)] }))
  const restored = loadPreferences(storage, now)
  assert.equal(restored.preset, 24)
  assert.deepEqual(restored.proxies, ['A', ''])
  const invalidBrush = memory(JSON.stringify({ version: 1, preset: 1, selectionOffsets: { from: -60, to: 120 } }))
  assert.equal(loadPreferences(invalidBrush, now).selectedTime, null)
})
test('clearing proxies and brushes persists, unavailable storage stays usable', () => {
  const storage = memory()
  const value = { preset: 1, bounds: presetRange(1, now), selectedTime: null, proxies: [] }
  savePreferences(value, storage)
  assert.deepEqual(loadPreferences(storage, now), value)
  const blocked = { getItem() { throw new Error('blocked') }, setItem() { throw new Error('quota') } }
  assert.equal(loadPreferences(blocked, now).preset, 24)
  assert.doesNotThrow(() => savePreferences(value, blocked))
})

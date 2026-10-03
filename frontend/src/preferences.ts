import type { Range } from './filters'

const key = 'clash-traffic-monitor.filters.v1'
const presetHours = [1, 24, 168, 720]
type Storage = Pick<globalThis.Storage, 'getItem' | 'setItem'>
export type Preferences = { preset: number; bounds: Range; selectedTime: Range | null; proxies: string[] }

export function presetRange(hours: number, now = Date.now()): Range {
  const to = Math.floor(now / 60000) * 60 + 60
  return { from: to - hours * 3600, to }
}
function validRange(value: unknown): value is Range {
  if (!value || typeof value !== 'object') return false
  const { from, to } = value as Range
  return Number.isSafeInteger(from) && Number.isSafeInteger(to) && from >= 0 && to <= 253402300799 && from % 60 === 0 && to % 60 === 0 && from < to && to - from <= 366 * 86400
}
function within(value: unknown, bounds: Range): value is Range {
  return validRange(value) && value.from >= bounds.from && value.to <= bounds.to
}

export function loadPreferences(storage?: Storage, now = Date.now()): Preferences {
  const result: Preferences = { preset: 24, bounds: presetRange(24, now), selectedTime: null, proxies: [] }
  try {
    const saved = JSON.parse((storage ?? globalThis.localStorage).getItem(key) ?? 'null')
    if (!saved || saved.version !== 1) return result
    if (Array.isArray(saved.proxies)) {
      result.proxies = [...new Set<string>(saved.proxies.filter((name: unknown) => typeof name === 'string' && name.length <= 1024))].slice(0, 200)
    }
    if (saved.preset === 0 && validRange(saved.range)) {
      result.preset = 0; result.bounds = saved.range
      if (within(saved.selection, result.bounds)) result.selectedTime = saved.selection
    } else if (presetHours.includes(saved.preset)) {
      result.preset = saved.preset; result.bounds = presetRange(saved.preset, now)
      // A brush on a rolling preset follows the new display range on reopening.
      const offsets = saved.selectionOffsets
      if (offsets && typeof offsets.from === 'number' && typeof offsets.to === 'number') {
        const selection = { from: result.bounds.from + offsets.from, to: result.bounds.from + offsets.to }
        if (within(selection, result.bounds)) result.selectedTime = selection
      }
    }
  } catch { /* Unavailable storage or invalid data should not block the dashboard. */ }
  return result
}

export function savePreferences(value: Preferences, storage?: Storage) {
  const saved = {
    version: 1,
    preset: value.preset,
    proxies: value.proxies,
    ...(value.preset === 0 ? { range: value.bounds, selection: value.selectedTime } : {
      selectionOffsets: value.selectedTime ? { from: value.selectedTime.from - value.bounds.from, to: value.selectedTime.to - value.bounds.from } : null,
    }),
  }
  try {
    const target = storage ?? globalThis.localStorage
    const encoded = JSON.stringify(saved)
    if (target.getItem(key) !== encoded) target.setItem(key, encoded)
  } catch { /* Browser privacy settings and quota failures leave the session usable. */ }
}

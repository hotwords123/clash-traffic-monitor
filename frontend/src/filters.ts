export type Range = { from: number; to: number }

export function toggleSelection(selected: string[], name: string, multiple: boolean): string[] {
  if (selected.includes(name)) return selected.filter(value => value !== name)
  return multiple ? [...selected, name] : [name]
}

export function brushRange(range: number[], display: Range): Range | null {
  if (range.length !== 2 || !range.every(Number.isFinite)) return null
  const from = Math.max(display.from, Math.floor(Math.min(...range) / 60000) * 60)
  const to = Math.min(display.to, Math.ceil(Math.max(...range) / 60000) * 60)
  return from < to ? { from, to } : null
}

export function filterQuery(range: Range, apps: string[], domains: string[], appSearch: string, domainSearch: string): URLSearchParams {
  const params = new URLSearchParams({ from: String(range.from), to: String(range.to) })
  if (apps.length) apps.forEach(name => params.append('app_selected', name))
  else if (appSearch) params.set('app', appSearch)
  if (domains.length) domains.forEach(name => params.append('domain_selected', name))
  else if (domainSearch) params.set('domain', domainSearch)
  return params
}

export const bucketMinutes = [1, 5, 10, 20, 30, 60, 180, 360, 720, 1440, 4320, 10080]
export function chartBucket(range: Range): number {
  const minutes = (range.to - range.from) / 60
  return bucketMinutes.find(size => Math.ceil(minutes / size) <= 72) ?? 10080
}

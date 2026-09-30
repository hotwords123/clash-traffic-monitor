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

// Each ranking ignores its own selection, while respecting the opposite filter.
export function rankingQuery(dimension: 'app' | 'domain', range: Range, apps: string[], domains: string[], appSearch: string, domainSearch: string): URLSearchParams {
  return filterQuery(range, dimension === 'domain' ? apps : [], dimension === 'app' ? domains : [], appSearch, domainSearch)
}

export const bucketMinutes = [1, 5, 10, 20, 30, 60, 180, 360, 720, 1440, 4320, 10080]
export function chartBucket(range: Range): number {
  const minutes = (range.to - range.from) / 60
  return bucketMinutes.find(size => Math.ceil(minutes / size) <= 72) ?? 10080
}

// Fix the unit for the whole axis and use integer 1/2/5 steps in that unit.
export function trafficAxis(peak: number) {
  const maximum = Number.isFinite(peak) && peak > 0 ? peak : 0
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let index = maximum > 0 ? Math.min(Math.floor(Math.log(maximum) / Math.log(1024)), 4) : 0
  index = Math.max(index, 0)
  if (index > 0 && maximum / 1024 ** index < 4) index--
  const unit = 1024 ** index
  const target = Math.max(1, maximum / unit / 5)
  const magnitude = 10 ** Math.floor(Math.log10(target))
  const step = ([1, 2, 5, 10].find(value => value * magnitude >= target) ?? 10) * magnitude
  const interval = step * unit
  return {
    min: 0,
    max: Math.max(interval, Math.ceil(maximum / interval) * interval),
    interval,
    formatter: (value: number) => `${Math.round(value / unit)} ${units[index]}`,
  }
}

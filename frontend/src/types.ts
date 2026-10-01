export type RankedItem = { name: string; upload: number; download: number; connections: number; proxies: string; domain_count: number }
export type RankedPage = { items: RankedItem[]; total: number; page: number; page_size: number }

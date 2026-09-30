<script setup lang="ts">
import { LineChart } from 'echarts/charts'
import { BrushComponent, GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import RankingList from './RankingList.vue'
import { brushRange, chartBucket, filterQuery, rankingQuery, toggleSelection, trafficAxis, type Range } from './filters'
import type { RankedPage } from './types'

echarts.use([LineChart, BrushComponent, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

type Status = { connected: boolean; paused: boolean; last_snapshot: string; last_error?: string; active_connections: number }
type Summary = { upload: number; download: number; connections: number }
type TimePoint = { time: number; upload: number; download: number; observed_minutes: number }
type Details = { chains?: string[]; start?: string; rule_payload?: string; metadata?: { network?: string; type?: string; processPath?: string; sourceIP?: string; sourcePort?: string; destinationIP?: string; destinationPort?: string } }
type Event = { connection_id: string; time: number; app: string; domain: string; upload: number; download: number; rule: string; proxy: string; details: Details }
type EventPage = { items: Event[]; total: number; page: number; page_size: number }
const view = ref<'overview' | 'connections'>('overview')
const presets = [{ hours: 1, label: '1h' }, { hours: 24, label: '24h' }, { hours: 168, label: '7d' }, { hours: 720, label: '30d' }, { hours: 0, label: '自定义' }]
const preset = ref(24)
function presetRange(hours: number): Range { const to = Math.floor(Date.now() / 60000) * 60 + 60; return { from: to - hours * 3600, to } }
const bounds = ref<Range>(presetRange(24))
const selectedTime = ref<Range | null>(null)
const bucket = computed(() => chartBucket(bounds.value))
const bucketLabel = computed(() => bucket.value < 60 ? `${bucket.value} 分钟` : bucket.value < 1440 ? `${bucket.value / 60} 小时` : `${bucket.value / 1440} 天`)
const effectiveTime = computed(() => selectedTime.value || bounds.value)
function localInput(seconds: number) { const date = new Date(seconds * 1000); const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000); return local.toISOString().slice(0, 16) }
const from = ref(localInput(bounds.value.from))
const to = ref(localInput(bounds.value.to))
const appSearch = ref('')
const domainSearch = ref('')
const appliedAppSearch = ref('')
const appliedDomainSearch = ref('')
const selectedApps = ref<string[]>([])
const selectedDomains = ref<string[]>([])
const emptyRanking = (): RankedPage => ({ items: [], total: 0, page: 1, page_size: 10 })
const apps = ref<RankedPage>(emptyRanking())
const domains = ref<RankedPage>(emptyRanking())
const events = ref<EventPage>({ items: [], total: 0, page: 1, page_size: 50 })
const page = ref(1)
const error = ref('')
const status = ref<Status>({ connected: false, paused: false, last_snapshot: '', active_connections: 0 })
const summary = ref<Summary>({ upload: 0, download: 0, connections: 0 })
const series = ref<TimePoint[]>([])
const loading = ref(false)
const chartElement = ref<HTMLElement>()
let chart: echarts.ECharts | undefined
let timer: number | undefined
let appDebounce: number | undefined
let domainDebounce: number | undefined
let controller: AbortController | undefined
let requestID = 0
let drawing = false
const total = computed(() => summary.value.upload + summary.value.download)
const updatedAt = computed(() => status.value.last_snapshot && !status.value.last_snapshot.startsWith('0001-') ? new Date(status.value.last_snapshot).toLocaleTimeString('zh-CN') : '等待数据')
function bytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), 4)
  return `${(value / 1024 ** index).toFixed(index > 1 ? 2 : 0)} ${units[index]}`
}
function dateTime(seconds: number) { return new Date(seconds * 1000).toLocaleString('zh-CN') }
function chain(item: Event) { return item.details.chains?.slice().reverse().join(' → ') || item.proxy || '未记录' }
function resetPages() { apps.value.page = 1; domains.value.page = 1; page.value = 1 }
function choosePreset(hours: number) {
  preset.value = hours
  if (hours === 0) return
  bounds.value = presetRange(hours)
  from.value = localInput(bounds.value.from); to.value = localInput(bounds.value.to)
  selectedTime.value = null
  resetPages()
  void refresh()
}
function applyDates() {
  const start = new Date(from.value).getTime() / 1000
  const end = new Date(to.value).getTime() / 1000
  if (!Number.isFinite(start) || !Number.isFinite(end) || start < 0 || start >= end || end - start > 366 * 86400) { error.value = '请选择有效的开始和结束时间，范围不超过 366 天'; return }
  bounds.value = { from: Math.floor(start / 60) * 60, to: Math.floor(end / 60) * 60 }
  selectedTime.value = null; resetPages(); void refresh()
}
function clearTime() { selectedTime.value = null; resetPages(); void refresh() }
function selectItem(dimension: 'app' | 'domain', name: string, multiple: boolean) {
  const selected = dimension === 'app' ? selectedApps : selectedDomains
  selected.value = toggleSelection(selected.value, name, multiple)
  if (dimension === 'app') domains.value.page = 1; else apps.value.page = 1
  page.value = 1
  void refresh()
}
function clearSelection(dimension: 'app' | 'domain') {
  if (dimension === 'app') { selectedApps.value = []; domains.value.page = 1 } else { selectedDomains.value = []; apps.value.page = 1 }
  page.value = 1; void refresh()
}
function changeRankingPage(dimension: 'app' | 'domain', value: number) {
  if (dimension === 'app') apps.value.page = value; else domains.value.page = value
  void refresh()
}
async function getJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error || `查询失败（${response.status}）`) }
  return response.json()
}
function query(range: Range) { return filterQuery(range, selectedApps.value, selectedDomains.value, appliedAppSearch.value, appliedDomainSearch.value).toString() }
async function refresh() {
  const id = ++requestID
  controller?.abort(); controller = new AbortController()
  const signal = controller.signal
  loading.value = true; error.value = ''
  try {
    const filter = query(effectiveTime.value)
    if (view.value === 'connections') {
      const [nextStatus, nextEvents] = await Promise.all([
        getJSON<Status>('/api/status', signal),
        getJSON<EventPage>(`/api/connections?${filter}&page=${page.value}&page_size=50`, signal),
      ])
      if (id !== requestID) return
      status.value = nextStatus; events.value = nextEvents
    } else {
      // Keep each selection surface broad, but apply the opposite dimension filter.
      const appQuery = rankingQuery('app', effectiveTime.value, selectedApps.value, selectedDomains.value, appliedAppSearch.value, appliedDomainSearch.value).toString()
      const domainQuery = rankingQuery('domain', effectiveTime.value, selectedApps.value, selectedDomains.value, appliedAppSearch.value, appliedDomainSearch.value).toString()
      const [nextStatus, nextSummary, nextApps, nextDomains, nextSeries] = await Promise.all([
        getJSON<Status>('/api/status', signal),
        getJSON<Summary>('/api/summary?' + filter, signal),
        getJSON<RankedPage>(`/api/top/apps?${appQuery}&page=${apps.value.page}&page_size=10`, signal),
        getJSON<RankedPage>(`/api/top/domains?${domainQuery}&page=${domains.value.page}&page_size=10`, signal),
        // The time selection never crops the chart's display range or data.
        getJSON<TimePoint[]>('/api/series?' + query(bounds.value) + '&bucket_minutes=' + bucket.value, signal),
      ])
      if (id !== requestID) return
      status.value = nextStatus; summary.value = nextSummary; apps.value = nextApps; domains.value = nextDomains; series.value = nextSeries
      await renderChart()
    }
  } catch (cause) {
    if (id === requestID && !signal.aborted) error.value = cause instanceof Error ? cause.message : '查询失败，请重试'
  } finally { if (id === requestID) loading.value = false }
}
function chartData(key: 'upload' | 'download'): [number, number | null][] {
  const data: [number, number | null][] = []
  let previous: number | undefined
  const step = bucket.value * 60
  for (const point of series.value) {
    if (previous !== undefined && point.time - previous > step) data.push([(previous + step) * 1000, null])
    data.push([point.time * 1000, point[key]])
    previous = point.time
  }
  return data
}
function onBrushEnd(value: unknown) {
  const payload = value as { areas?: { coordRange?: number[] }[] }
  drawing = false
  const range = payload.areas?.[0]?.coordRange
  const selected = range ? brushRange(range, bounds.value) : null
  if (selected?.from === selectedTime.value?.from && selected?.to === selectedTime.value?.to) return
  selectedTime.value = selected
  resetPages(); void refresh()
}
async function renderChart() {
  await nextTick()
  if (!chartElement.value || view.value !== 'overview' || drawing) return
  if (!chart) {
    chart = echarts.init(chartElement.value)
    chart.on('brushEnd', onBrushEnd)
    chart.getZr().on('mousedown', () => { drawing = true })
    chart.getZr().on('mouseup', () => { drawing = false })
    chart.getZr().on('globalout', () => { drawing = false })
  }
  chart.resize()
  const axis = trafficAxis(series.value.reduce((peak, point) => Math.max(peak, point.upload, point.download), 0))
  chart.setOption({
    animation: false,
    tooltip: { trigger: 'axis', valueFormatter: (value: number | [number, number | null]) => { const amount = Array.isArray(value) ? value[1] : value; return amount === null ? '无记录' : bytes(amount) } },
    legend: { top: 0, data: ['下载', '上传'], textStyle: { color: '#9ba8c7' } },
    grid: { left: 64, right: 24, top: 45, bottom: 34 },
    brush: { xAxisIndex: 0, brushType: 'lineX', brushMode: 'single', toolbox: [], removeOnClick: true, transformable: true, brushStyle: { color: 'rgba(110,231,199,.12)', borderColor: '#6ee7c7', borderWidth: 1 }, inBrush: { colorAlpha: 1 }, outOfBrush: { colorAlpha: .45 } },
    xAxis: { type: 'time', min: bounds.value.from * 1000, max: bounds.value.to * 1000, axisLabel: { color: '#7e8bab', hideOverlap: true }, axisLine: { lineStyle: { color: '#29324b' } } },
    yAxis: { type: 'value', min: axis.min, max: axis.max, interval: axis.interval, axisLabel: { color: '#7e8bab', formatter: axis.formatter }, splitLine: { lineStyle: { color: '#1e2740' } } },
    series: [
      { name: '下载', type: 'line', connectNulls: false, showSymbol: true, symbolSize: 3, areaStyle: { opacity: .12 }, data: chartData('download'), color: '#6ee7c7' },
      { name: '上传', type: 'line', connectNulls: false, showSymbol: true, symbolSize: 3, areaStyle: { opacity: .08 }, data: chartData('upload'), color: '#7aa2ff' },
    ],
  })
  chart.dispatchAction({ type: 'brush', areas: selectedTime.value ? [{ brushType: 'lineX', xAxisIndex: 0, coordRange: [selectedTime.value.from * 1000, selectedTime.value.to * 1000] }] : [] })
  chart.dispatchAction({ type: 'takeGlobalCursor', key: 'brush', brushOption: { brushType: 'lineX', brushMode: 'single' } })
}
function resize() { chart?.resize() }
watch(appSearch, value => {
  window.clearTimeout(appDebounce)
  appDebounce = window.setTimeout(() => { appliedAppSearch.value = value.trim(); apps.value.page = 1; if (!selectedApps.value.length) domains.value.page = 1; page.value = 1; void refresh() }, 300)
})
watch(domainSearch, value => {
  window.clearTimeout(domainDebounce)
  domainDebounce = window.setTimeout(() => { appliedDomainSearch.value = value.trim(); domains.value.page = 1; if (!selectedDomains.value.length) apps.value.page = 1; page.value = 1; void refresh() }, 300)
})
watch(view, () => { page.value = 1; void refresh() })
onMounted(() => {
  void refresh()
  timer = window.setInterval(() => {
    if (view.value !== 'overview' || loading.value || drawing) return
    if (preset.value > 0 && !selectedTime.value) {
      bounds.value = presetRange(preset.value)
      from.value = localInput(bounds.value.from); to.value = localInput(bounds.value.to)
    }
    void refresh()
  }, 5000)
  window.addEventListener('resize', resize)
})
onBeforeUnmount(() => {
  window.clearInterval(timer); window.clearTimeout(appDebounce); window.clearTimeout(domainDebounce)
  window.removeEventListener('resize', resize); controller?.abort(); chart?.dispose()
})
</script>

<template>
  <main>
    <header>
      <div><p class="eyebrow">LOCAL TRAFFIC ANALYTICS</p><h1>Clash 流量观察</h1><p class="subtitle">看清哪些应用、哪些域名，在什么时候消耗了流量。</p></div>
      <span class="status" :class="{ online: status.connected && !status.paused, paused: status.paused }"><i></i>{{ status.paused ? '已暂停' : status.connected ? '采集中' : '未连接' }} · {{ updatedAt }}</span>
    </header>
    <section v-if="status.last_error && !status.connected" class="warning">无法连接 Clash Verge Rev：{{ status.last_error }}</section>
    <nav class="tabs" aria-label="页面切换"><button :class="{ selected: view === 'overview' }" @click="view = 'overview'">流量统计</button><button :class="{ selected: view === 'connections' }" @click="view = 'connections'">连接查询</button></nav>
    <section v-if="error" class="warning" role="alert">{{ error }}</section>
    <div class="filter-summary">
      <span>统计时间：{{ dateTime(effectiveTime.from) }} — {{ dateTime(effectiveTime.to) }}（不含）</span>
      <span>应用：{{ selectedApps.length ? `已选 ${selectedApps.length} 项` : appliedAppSearch ? `包含“${appliedAppSearch}”` : '全部' }}</span>
      <span>域名：{{ selectedDomains.length ? `已选 ${selectedDomains.length} 项` : appliedDomainSearch ? `包含“${appliedDomainSearch}”` : '全部' }}</span>
      <button v-if="selectedTime" @click="clearTime">清除框选</button><button v-if="selectedApps.length" @click="clearSelection('app')">清除应用选择</button><button v-if="selectedDomains.length" @click="clearSelection('domain')">清除域名选择</button>
      <span class="loading-label" role="status">{{ loading ? '正在更新…' : '' }}</span>
    </div>
    <div v-show="view === 'overview'">
      <section class="panel chart-panel">
        <div class="panel-title"><div><span>时间趋势</span><small>每桶 {{ bucketLabel }} · 上传与下载 · 拖拽框选时段，点击空白处清除，保留完整时间轴</small></div></div>
        <div class="time-range-toolbar">
          <div class="range-presets" role="group" aria-label="展示时间范围"><button v-for="option in presets" :key="option.hours" type="button" :class="{ active: preset === option.hours }" :aria-pressed="preset === option.hours" @click="choosePreset(option.hours)">{{ option.label }}</button></div>
          <form v-if="preset === 0" class="date-range" @submit.prevent="applyDates"><label>开始时间<input v-model="from" type="datetime-local" required></label><label>结束时间（不含）<input v-model="to" type="datetime-local" required></label><button type="submit">应用时间范围</button></form>
        </div>
        <p class="hint">展示范围：{{ dateTime(bounds.from) }} — {{ dateTime(bounds.to) }}。无记录区间留空。</p>
        <div ref="chartElement" class="chart" aria-label="流量趋势，可拖拽框选时间段"></div>
      </section>
      <section class="metrics">
        <article><span>总流量</span><strong>{{ bytes(total) }}</strong><small>当前筛选条件</small></article>
        <article><span>下载</span><strong>{{ bytes(summary.download) }}</strong><small>{{ total ? Math.round(summary.download / total * 100) : 0 }}% 占比</small></article>
        <article><span>上传</span><strong>{{ bytes(summary.upload) }}</strong><small>{{ total ? Math.round(summary.upload / total * 100) : 0 }}% 占比</small></article>
        <article><span title="所选时段内首次采集到的连接数量">连接数</span><strong>{{ summary.connections }}</strong><small>当前活跃：{{ status.active_connections }}</small></article>
      </section>
      <section class="rankings">
        <RankingList title="应用排行" dimension="app" :result="apps" :selected="selectedApps" :search="appSearch" :loading="loading" @search="appSearch = $event" @select="(name, multiple) => selectItem('app', name, multiple)" @page="changeRankingPage('app', $event)" @clear="clearSelection('app')" />
        <RankingList title="域名排行" dimension="domain" :result="domains" :selected="selectedDomains" :search="domainSearch" :loading="loading" @search="domainSearch = $event" @select="(name, multiple) => selectItem('domain', name, multiple)" @page="changeRankingPage('domain', $event)" @clear="clearSelection('domain')" />
      </section>
    </div>
    <section v-if="view === 'connections'" class="panel">
      <div class="panel-title"><div><span>连接明细</span><small>最近 7 天内保留的记录 · 每个连接按分钟合并 · 时间降序</small></div><div class="detail-actions"><button @click="view = 'overview'">调整筛选</button><button @click="refresh">刷新</button></div></div>
      <p class="hint">上传、下载为该分钟采集到的增量。旧记录仅包含已保存字段；连接起始时间不代表完整的活跃区间。</p>
      <div class="table-scroll">
        <table>
          <thead><tr><th>采集分钟</th><th>进程</th><th>域名</th><th>代理组 / 出口</th><th>下载</th><th>上传</th><th>规则</th><th>详情</th></tr></thead>
          <tbody>
            <tr v-for="item in events.items" :key="`${item.time}-${item.connection_id}`">
              <td class="nowrap">{{ dateTime(item.time) }}</td>
              <td><button class="name-link" :aria-pressed="selectedApps.includes(item.app)" @click="selectItem('app', item.app, $event.ctrlKey || $event.metaKey)">{{ item.app }}</button></td>
              <td><button class="name-link" :aria-pressed="selectedDomains.includes(item.domain)" @click="selectItem('domain', item.domain, $event.ctrlKey || $event.metaKey)">{{ item.domain }}</button></td>
              <td>{{ item.proxy || '未记录' }}</td><td class="nowrap">{{ bytes(item.download) }}</td><td class="nowrap">{{ bytes(item.upload) }}</td><td>{{ item.rule || '未记录' }}</td>
              <td><details><summary>展开</summary><dl>
                <dt>连接 ID</dt><dd>{{ item.connection_id }}</dd>
                <dt>代理链（组 → 节点）</dt><dd>{{ chain(item) }}</dd>
                <dt>连接起始时间</dt><dd>{{ item.details.start ? new Date(item.details.start).toLocaleString('zh-CN') : '未记录' }}</dd>
                <dt>进程路径</dt><dd>{{ item.details.metadata?.processPath || '未记录' }}</dd>
                <dt>网络 / 类型</dt><dd>{{ item.details.metadata?.network || '未记录' }} / {{ item.details.metadata?.type || '未记录' }}</dd>
                <dt>源地址</dt><dd>{{ item.details.metadata?.sourceIP || '未记录' }} : {{ item.details.metadata?.sourcePort || '—' }}</dd>
                <dt>目标地址</dt><dd>{{ item.details.metadata?.destinationIP || '未记录' }} : {{ item.details.metadata?.destinationPort || '—' }}</dd>
                <dt>规则内容</dt><dd>{{ item.details.rule_payload || '未记录' }}</dd>
              </dl></details></td>
            </tr>
            <tr v-if="!events.items.length"><td colspan="8" class="empty">{{ loading ? '正在读取…' : '所选条件下没有保留的连接记录' }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="pagination"><span>共 {{ events.total }} 条 · 第 {{ page }} / {{ Math.max(1, Math.ceil(events.total / 50)) }} 页</span><button :disabled="page <= 1 || loading" @click="page--; refresh()">上一页</button><button :disabled="page * 50 >= events.total || loading" @click="page++; refresh()">下一页</button></div>
    </section>
  </main>
</template>

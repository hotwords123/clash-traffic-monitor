<script setup lang="ts">
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

type Status = { connected: boolean; paused: boolean; last_snapshot: string; last_error?: string; active_connections: number }
type Summary = { upload: number; download: number; connections: number }
type RankedItem = { name: string; upload: number; download: number; connections: number; proxies: string }
type TimePoint = { time: number; upload: number; download: number }

type Details = { chains?: string[]; start?: string; rule_payload?: string; metadata?: { network?: string; type?: string; processPath?: string; sourceIP?: string; sourcePort?: string; destinationIP?: string; destinationPort?: string } }
type Event = { connection_id: string; time: number; app: string; domain: string; upload: number; download: number; rule: string; proxy: string; details: Details }
type EventPage = { items: Event[]; total: number; page: number; page_size: number }
const view = ref<'overview' | 'connections'>('overview')
const hours = ref(24)
const from = ref('')
const to = ref('')
const app = ref('')
const domain = ref('')
const appExact = ref(false)
const domainExact = ref(false)
const filters = ref({ hours: 24, from: '', to: '', app: '', domain: '', appExact: false, domainExact: false })
const bounds = ref({ from: 0, to: 0 })
const events = ref<EventPage>({ items: [], total: 0, page: 1, page_size: 50 })
const page = ref(1)
const error = ref('')
let controller: AbortController | undefined
let requestID = 0
const status = ref<Status>({ connected: false, paused: false, last_snapshot: '', active_connections: 0 })
const summary = ref<Summary>({ upload: 0, download: 0, connections: 0 })
const apps = ref<RankedItem[]>([])
const domains = ref<RankedItem[]>([])
const series = ref<TimePoint[]>([])
const loading = ref(true)
const chartElement = ref<HTMLElement>()
let chart: echarts.ECharts | undefined
let timer: number | undefined

const total = computed(() => summary.value.upload + summary.value.download)
const updatedAt = computed(() => status.value.last_snapshot ? new Date(status.value.last_snapshot).toLocaleTimeString('zh-CN') : '等待数据')

function bytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index > 1 ? 2 : 0)} ${units[index]}`
}

async function getJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error || `查询失败（${response.status}）`)
  }
  return response.json()
}

function applyFilters() {
  if (hours.value === 0 && (!from.value || !to.value)) { error.value = '请选择开始和结束时间'; return }
  if (hours.value === 0 && (new Date(from.value).getTime() >= new Date(to.value).getTime() || new Date(to.value).getTime() - new Date(from.value).getTime() > 366 * 86400000)) {
    error.value = '时间范围必须大于零且不超过 366 天'; return
  }
  filters.value = { hours: hours.value, from: from.value, to: to.value, app: app.value.trim(), domain: domain.value.trim(), appExact: appExact.value, domainExact: domainExact.value }
  page.value = 1
  void refresh()
}
function resetFilters() {
  hours.value = 24; from.value = ''; to.value = ''; app.value = ''; domain.value = ''; appExact.value = false; domainExact.value = false
  applyFilters()
}
function drill(dimension: 'app' | 'domain', name: string) {
  if (dimension === 'app') { app.value = name; appExact.value = true } else { domain.value = name; domainExact.value = true }
  applyFilters()
}
function dateTime(seconds: number) { return new Date(seconds * 1000).toLocaleString('zh-CN') }
function chain(item: Event) { return item.details.chains?.slice().reverse().join(' → ') || item.proxy || '未记录' }

async function refresh() {
  const id = ++requestID
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  loading.value = true
  error.value = ''
  try {
    const f = filters.value
    const end = f.hours === 0 ? Math.floor(new Date(f.to).getTime() / 60000) * 60 : Math.floor(Date.now() / 60000) * 60 + 60
    const start = f.hours === 0 ? Math.floor(new Date(f.from).getTime() / 60000) * 60 : Math.floor(Date.now() / 60000) * 60 - f.hours * 3600
    const params = new URLSearchParams({ from: String(start), to: String(end), app: f.app, domain: f.domain, app_exact: f.appExact ? '1' : '0', domain_exact: f.domainExact ? '1' : '0' })
    const query = '?' + params.toString()
    if (view.value === 'connections') {
      const [nextStatus, nextEvents] = await Promise.all([
        getJSON<Status>('/api/status', signal),
        getJSON<EventPage>(`/api/connections${query}&page=${page.value}&page_size=50`, signal),
      ])
      if (id !== requestID) return
      status.value = nextStatus
      events.value = nextEvents
    } else {
      const [nextStatus, nextSummary, nextApps, nextDomains, nextSeries] = await Promise.all([
        getJSON<Status>('/api/status', signal),
        getJSON<Summary>('/api/summary' + query, signal),
        getJSON<RankedItem[]>('/api/top/apps' + query + '&limit=100', signal),
        getJSON<RankedItem[]>('/api/top/domains' + query + '&limit=100', signal),
        getJSON<TimePoint[]>('/api/series' + query, signal),
      ])
      if (id !== requestID) return
      status.value = nextStatus; summary.value = nextSummary; apps.value = nextApps; domains.value = nextDomains; series.value = nextSeries
      bounds.value = { from: start, to: end }
      await renderChart()
    }
  } catch (cause) {
    if (id === requestID && !signal.aborted) error.value = cause instanceof Error ? cause.message : '查询失败，请重试'
  } finally {
    if (id === requestID) loading.value = false
  }
}

function chartData(key: 'upload' | 'download'): [number, number | null][] {
  const data: [number, number | null][] = []
  let previous: number | undefined
  for (const point of series.value) {
    if (previous !== undefined && point.time - previous > 60) data.push([(previous + 60) * 1000, null])
    data.push([point.time * 1000, point[key]])
    previous = point.time
  }
  return data
}

async function renderChart() {
  await nextTick()
  if (!chartElement.value) return
  chart ||= echarts.init(chartElement.value)
  chart.resize()
  chart.setOption({
    animation: false,
    tooltip: { trigger: 'axis', valueFormatter: (value: number | [number, number | null]) => { const amount = Array.isArray(value) ? value[1] : value; return amount === null ? '无记录' : bytes(amount) } },
    legend: { top: 0, data: ['下载', '上传'], textStyle: { color: '#9ba8c7' } },
    grid: { left: 54, right: 20, top: 45, bottom: 34 },
    xAxis: {
      type: 'time',
      min: bounds.value.from * 1000,
      max: bounds.value.to * 1000,
      axisLabel: { color: '#7e8bab', hideOverlap: true },
      axisLine: { lineStyle: { color: '#29324b' } },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#7e8bab', formatter: (value: number) => bytes(value) },
      splitLine: { lineStyle: { color: '#1e2740' } },
    },
    series: [
      { name: '下载', type: 'line', smooth: false, connectNulls: false, showSymbol: true, symbolSize: 3, areaStyle: { opacity: .12 }, data: chartData('download'), color: '#6ee7c7' },
      { name: '上传', type: 'line', smooth: false, connectNulls: false, showSymbol: true, symbolSize: 3, areaStyle: { opacity: .08 }, data: chartData('upload'), color: '#7aa2ff' },
    ],
  })
}

function resize() { chart?.resize() }
watch(view, () => { page.value = 1; void refresh() })
onMounted(() => {
  refresh()
  timer = window.setInterval(() => { if (view.value === 'overview' && !loading.value) void refresh() }, 5000)
  window.addEventListener('resize', resize)
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  window.removeEventListener('resize', resize)
  controller?.abort()
  chart?.dispose()
})
</script>

<template>
  <main>
    <header>
      <div>
        <p class="eyebrow">LOCAL TRAFFIC ANALYTICS</p>
        <h1>Clash 流量观察</h1>
        <p class="subtitle">看清哪些应用、哪些域名，在什么时候消耗了流量。</p>
      </div>
      <div class="controls">
        <span class="status" :class="{ online: status.connected && !status.paused, paused: status.paused }">
          <i></i>{{ status.paused ? '已暂停' : status.connected ? '采集中' : '未连接' }} · {{ updatedAt }}
        </span>
      </div>
    </header>

    <section v-if="status.last_error && !status.connected" class="warning">
      无法连接 Clash Verge Rev：{{ status.last_error }}
    </section>

    <nav class="tabs" aria-label="页面切换">
      <button :class="{ selected: view === 'overview' }" @click="view = 'overview'">流量统计</button>
      <button :class="{ selected: view === 'connections' }" @click="view = 'connections'">连接查询</button>
    </nav>
    <form class="panel filters" @submit.prevent="applyFilters">
      <label>时间范围<select v-model="hours"><option :value="1">最近 1 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option><option :value="0">自定义时间</option></select></label>
      <template v-if="hours === 0">
        <label>开始时间<input v-model="from" type="datetime-local" required></label>
        <label>结束时间（不含）<input v-model="to" type="datetime-local" required></label>
      </template>
      <label>进程<input v-model="app" placeholder="例如 browser.exe" @input="appExact = false"><span><input v-model="appExact" type="checkbox">精确匹配</span></label>
      <label>域名<input v-model="domain" placeholder="例如 example.com" @input="domainExact = false"><span><input v-model="domainExact" type="checkbox">精确匹配</span></label>
      <div class="filter-actions"><button type="submit">查询</button><button type="button" @click="resetFilters">重置</button></div>
      <p class="hint">筛选同时作用于趋势、排行和连接明细；默认按名称包含匹配，时间精度为分钟。点击排行名称可精确筛选。</p>
    </form>
    <section v-if="error" class="warning" role="alert">{{ error }}</section>
    <p v-if="loading" class="hint" role="status">正在读取…</p>

    <div v-show="view === 'overview'">
    <section class="metrics">
      <article><span>总流量</span><strong>{{ bytes(total) }}</strong><small>所选时段</small></article>
      <article><span>下载</span><strong>{{ bytes(summary.download) }}</strong><small>{{ total ? Math.round(summary.download / total * 100) : 0 }}% 占比</small></article>
      <article><span>上传</span><strong>{{ bytes(summary.upload) }}</strong><small>{{ total ? Math.round(summary.upload / total * 100) : 0 }}% 占比</small></article>
      <article><span>活跃连接</span><strong>{{ status.active_connections }}</strong><small>当前连接数</small></article>
    </section>

    <section class="panel chart-panel">
      <div class="panel-title"><div><span>时间趋势</span><small>每分钟上传与下载 · 无记录区间保留空缺（可能是中断、暂停或无匹配流量）</small></div></div>
      <div ref="chartElement" class="chart"></div>
    </section>

    <section class="rankings">
      <article class="panel">
        <div class="panel-title"><div><span>应用排行</span><small>按总流量排序 · 最多 100 项</small></div></div>
        <div v-if="!apps.length" class="empty">{{ loading ? '正在读取…' : '尚无流量数据' }}</div>
        <div v-for="(item, index) in apps" :key="item.name" class="rank-row">
          <b>{{ String(index + 1).padStart(2, '0') }}</b>
          <div><button class="name-link" @click="drill('app', item.name)">{{ item.name }}</button><small>{{ item.connections }} 个新观察连接</small></div>
          <span>{{ bytes(item.upload + item.download) }}</span>
        </div>
      </article>
      <article class="panel">
        <div class="panel-title"><div><span>域名排行</span><small>按总流量排序 · 代理信息来自所选时段最近 7 天保留的明细</small></div></div>
        <div v-if="!domains.length" class="empty">{{ loading ? '正在读取…' : '尚无流量数据' }}</div>
        <div v-for="(item, index) in domains" :key="item.name" class="rank-row">
          <b>{{ String(index + 1).padStart(2, '0') }}</b>
          <div><button class="name-link" @click="drill('domain', item.name)">{{ item.name }}</button><small>代理组 / 出口：{{ item.proxies || '未记录' }}</small><small>↓ {{ bytes(item.download) }} · ↑ {{ bytes(item.upload) }}</small></div>
          <span>{{ bytes(item.upload + item.download) }}</span>
        </div>
      </article>
    </section>
    </div>
    <section v-if="view === 'connections'" class="panel">
      <div class="panel-title"><div><span>连接明细</span><small>最近 7 天内保留的记录 · 每个连接按分钟合并 · 时间降序</small></div><button @click="refresh">刷新</button></div>
      <p class="hint">上传、下载为该分钟采集到的增量。旧记录仅包含已保存字段；连接起始时间不代表完整的活跃区间。</p>
      <div class="table-scroll">
        <table>
          <thead><tr><th>采集分钟</th><th>进程</th><th>域名</th><th>代理组 / 出口</th><th>下载</th><th>上传</th><th>规则</th><th>详情</th></tr></thead>
          <tbody>
            <tr v-for="item in events.items" :key="`${item.time}-${item.connection_id}`">
              <td class="nowrap">{{ dateTime(item.time) }}</td>
              <td><button class="name-link" @click="drill('app', item.app)">{{ item.app }}</button></td>
              <td><button class="name-link" @click="drill('domain', item.domain)">{{ item.domain }}</button></td>
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

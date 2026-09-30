<script setup lang="ts">
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

type Status = { connected: boolean; last_snapshot: string; last_error?: string; active_connections: number }
type Summary = { upload: number; download: number; connections: number }
type RankedItem = { name: string; upload: number; download: number; connections: number }
type TimePoint = { time: number; upload: number; download: number }

const hours = ref(24)
const status = ref<Status>({ connected: false, last_snapshot: '', active_connections: 0 })
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

async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(path)
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`)
  return response.json()
}

async function refresh() {
  try {
    const query = `?hours=${hours.value}`
    const [nextStatus, nextSummary, nextApps, nextDomains, nextSeries] = await Promise.all([
      getJSON<Status>('/api/status'),
      getJSON<Summary>('/api/summary' + query),
      getJSON<RankedItem[]>('/api/top/apps' + query),
      getJSON<RankedItem[]>('/api/top/domains' + query),
      getJSON<TimePoint[]>('/api/series' + query),
    ])
    status.value = nextStatus
    summary.value = nextSummary
    apps.value = nextApps
    domains.value = nextDomains
    series.value = nextSeries
    await renderChart()
  } finally {
    loading.value = false
  }
}

async function renderChart() {
  await nextTick()
  if (!chartElement.value) return
  chart ||= echarts.init(chartElement.value)
  chart.setOption({
    tooltip: { trigger: 'axis', valueFormatter: bytes },
    legend: { data: ['下载', '上传'], textStyle: { color: '#9ba8c7' } },
    grid: { left: 54, right: 20, top: 45, bottom: 34 },
    xAxis: {
      type: 'category',
      data: series.value.map(point => new Date(point.time * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })),
      axisLabel: { color: '#7e8bab', hideOverlap: true },
      axisLine: { lineStyle: { color: '#29324b' } },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#7e8bab', formatter: (value: number) => bytes(value) },
      splitLine: { lineStyle: { color: '#1e2740' } },
    },
    series: [
      { name: '下载', type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: .12 }, data: series.value.map(point => point.download), color: '#6ee7c7' },
      { name: '上传', type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: .08 }, data: series.value.map(point => point.upload), color: '#7aa2ff' },
    ],
  })
}

function resize() { chart?.resize() }
watch(hours, refresh)
onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, 5000)
  window.addEventListener('resize', resize)
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  window.removeEventListener('resize', resize)
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
        <select v-model="hours" aria-label="统计时间范围">
          <option :value="1">最近 1 小时</option>
          <option :value="24">最近 24 小时</option>
          <option :value="168">最近 7 天</option>
          <option :value="720">最近 30 天</option>
        </select>
        <span class="status" :class="{ online: status.connected }">
          <i></i>{{ status.connected ? '采集中' : '未连接' }} · {{ updatedAt }}
        </span>
      </div>
    </header>

    <section v-if="status.last_error && !status.connected" class="warning">
      无法连接 Clash Verge Rev：{{ status.last_error }}
    </section>

    <section class="metrics">
      <article><span>总流量</span><strong>{{ bytes(total) }}</strong><small>所选时段</small></article>
      <article><span>下载</span><strong>{{ bytes(summary.download) }}</strong><small>{{ total ? Math.round(summary.download / total * 100) : 0 }}% 占比</small></article>
      <article><span>上传</span><strong>{{ bytes(summary.upload) }}</strong><small>{{ total ? Math.round(summary.upload / total * 100) : 0 }}% 占比</small></article>
      <article><span>活跃连接</span><strong>{{ status.active_connections }}</strong><small>当前连接数</small></article>
    </section>

    <section class="panel chart-panel">
      <div class="panel-title"><div><span>时间趋势</span><small>每分钟上传与下载</small></div></div>
      <div ref="chartElement" class="chart"></div>
    </section>

    <section class="rankings">
      <article class="panel">
        <div class="panel-title"><div><span>应用排行</span><small>按总流量排序</small></div></div>
        <div v-if="!apps.length" class="empty">{{ loading ? '正在读取…' : '尚无流量数据' }}</div>
        <div v-for="(item, index) in apps" :key="item.name" class="rank-row">
          <b>{{ String(index + 1).padStart(2, '0') }}</b>
          <div><strong>{{ item.name }}</strong><small>{{ item.connections }} 个连接</small></div>
          <span>{{ bytes(item.upload + item.download) }}</span>
        </div>
      </article>
      <article class="panel">
        <div class="panel-title"><div><span>域名排行</span><small>按总流量排序</small></div></div>
        <div v-if="!domains.length" class="empty">{{ loading ? '正在读取…' : '尚无流量数据' }}</div>
        <div v-for="(item, index) in domains" :key="item.name" class="rank-row">
          <b>{{ String(index + 1).padStart(2, '0') }}</b>
          <div><strong>{{ item.name }}</strong><small>↓ {{ bytes(item.download) }} · ↑ {{ bytes(item.upload) }}</small></div>
          <span>{{ bytes(item.upload + item.download) }}</span>
        </div>
      </article>
    </section>
  </main>
</template>

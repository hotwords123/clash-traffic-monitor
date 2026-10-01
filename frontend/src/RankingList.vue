<script setup lang="ts">
import type { RankedItem, RankedPage } from './types'
defineProps<{ title: string; dimension: 'app' | 'domain'; result: RankedPage; selected: string[]; search: string; loading: boolean }>()
const emit = defineEmits<{ search: [value: string]; select: [name: string, multiple: boolean]; page: [value: number]; clear: [] }>()
function bytes(value: number) {
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(value) / Math.log(1024)), 4)
  return `${(value / 1024 ** i).toFixed(i > 1 ? 2 : 0)} ${units[i]}`
}
function choose(item: RankedItem, event: MouseEvent) { emit('select', item.name, event.ctrlKey || event.metaKey) }
</script>
<template>
  <article class="panel ranking-panel">
    <div class="panel-title"><div><span>{{ title }}</span><small>按所选时间内的总流量排序</small></div><button v-if="selected.length" @click="emit('clear')">清除选择</button></div>
    <label class="list-search"><span class="sr-only">搜索{{ dimension === 'app' ? '应用' : '域名' }}</span><input :value="search" :placeholder="dimension === 'app' ? '搜索应用名称…' : '搜索域名…'" @input="emit('search', ($event.target as HTMLInputElement).value)"></label>
    <p class="hint">点击选择或取消，Ctrl + 点击多选。{{ selected.length ? `已选 ${selected.length} 项，搜索仅影响列表。` : '未选择时，统计使用搜索匹配的全部项。' }}</p>
    <div v-if="selected.length" class="selection-chips" :aria-label="`${title}已选项`"><button v-for="name in selected" :key="name" :aria-label="`取消选择 ${name}`" @click="emit('select', name, true)">{{ name }} ×</button></div>
    <div class="table-scroll">
      <table class="ranking-table" :aria-label="title">
        <thead><tr><th>#</th><th>{{ dimension === 'app' ? '应用' : '域名' }}</th><th>总流量</th><th title="所选时段内首次采集到的连接数量">连接数</th></tr></thead>
        <tbody>
          <tr v-for="(item, index) in result.items" :key="item.name" :class="{ 'selected-row': selected.includes(item.name) }" @click="choose(item, $event)">
            <td>{{ (result.page - 1) * result.page_size + index + 1 }}</td>
            <td><button class="name-link" :aria-pressed="selected.includes(item.name)" @click.stop="choose(item, $event)">{{ item.name }}</button><small v-if="dimension === 'app'" title="当前时间和筛选条件下访问的不同域名数量">{{ item.domain_count }} 个域名</small><small v-else>代理组 / 出口：{{ item.proxies || '未记录' }}</small></td>
            <td class="nowrap">{{ bytes(item.upload + item.download) }}<small>↓ {{ bytes(item.download) }} · ↑ {{ bytes(item.upload) }}</small></td><td>{{ item.connections }}</td>
          </tr>
          <tr v-if="!result.items.length"><td colspan="4" class="empty">{{ loading ? '正在读取…' : '没有匹配的记录' }}</td></tr>
        </tbody>
      </table>
    </div>
    <div class="pagination"><span>共 {{ result.total }} 项 · {{ result.page }} / {{ Math.max(1, Math.ceil(result.total / result.page_size)) }} 页</span><button :disabled="result.page <= 1 || loading" :aria-label="`${title}上一页`" @click="emit('page', result.page - 1)">上一页</button><button :disabled="result.page * result.page_size >= result.total || loading" :aria-label="`${title}下一页`" @click="emit('page', result.page + 1)">下一页</button></div>
    <p v-if="dimension === 'domain'" class="hint">代理信息来自所选时段最近 7 天保留的明细。</p>
  </article>
</template>

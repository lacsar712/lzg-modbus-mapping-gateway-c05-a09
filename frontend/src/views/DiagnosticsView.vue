<template>
  <div style="display:flex;flex-direction:column;gap:16px">
    <!-- 健康状态 -->
    <div class="card-panel">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:14px">
        <div>
          <h2 style="margin:0 0 4px;font-size:18px">连接诊断</h2>
          <div class="sub" style="margin:0">健康检查 · 设备连通性探测 · 故障注入（仅开发）</div>
        </div>
        <el-button :loading="healthLoading" @click="loadHealth">刷新</el-button>
      </div>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="Status">
          <el-tag :type="health.status === 'ok' ? 'success' : 'warning'" size="small">
            {{ health.status || '-' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="Mapping Loaded">{{ health.mappingLoaded ? '是' : '否' }}</el-descriptions-item>
        <el-descriptions-item label="Device Count">{{ health.deviceCount ?? '-' }}</el-descriptions-item>
        <el-descriptions-item label="Last Modbus Error">
          <span class="mono">{{ health.lastModbusError || '无' }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="Last Error At">{{ health.lastErrorAt || '-' }}</el-descriptions-item>
      </el-descriptions>

      <h3 style="font-size:14px;margin:16px 0 8px">最近探测结果（按设备）</h3>
      <el-table :data="health.probeSummary || []" empty-text="尚未探测" size="small">
        <el-table-column prop="deviceId" label="设备" min-width="130" />
        <el-table-column prop="endpoint" label="Endpoint" min-width="150">
          <template #default="{ row }"><span class="mono">{{ row.endpoint }}</span></template>
        </el-table-column>
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <el-tag :type="row.ok ? 'success' : 'danger'" size="small">{{ row.ok ? '成功' : '失败' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="100">
          <template #default="{ row }"><span class="mono">{{ formatMs(row.latencyMs) }}</span></template>
        </el-table-column>
        <el-table-column prop="checkedAt" label="时间" min-width="180" />
        <el-table-column label="错误" min-width="200">
          <template #default="{ row }"><span class="mono">{{ row.error || '-' }}</span></template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 连通性探测 -->
    <div class="card-panel">
      <h2 style="margin:0 0 4px;font-size:16px">连通性探测</h2>
      <p class="sub">对指定设备建立 TCP 连接并读取 1 个保持寄存器，展示往返耗时与错误（非通用 APM，仅一次性手动探测）。</p>
      <div style="display:flex;gap:10px;align-items:center;margin-bottom:14px;flex-wrap:wrap">
        <el-select v-model="probeDevice" placeholder="选择设备" style="width:260px">
          <el-option v-for="d in devices" :key="d.id" :label="`${d.id} · ${d.name}`" :value="d.id" />
        </el-select>
        <el-button type="primary" :loading="probing" :disabled="!probeDevice" @click="runProbe">探测</el-button>
      </div>

      <h3 style="font-size:14px;margin:0 0 8px">探测历史（最近 20 条）</h3>
      <el-table :data="recentProbes" v-loading="probing" empty-text="暂无探测记录" size="small">
        <el-table-column prop="checkedAt" label="时间" min-width="180" />
        <el-table-column prop="deviceId" label="设备" min-width="130" />
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <el-tag :type="row.ok ? 'success' : 'danger'" size="small">{{ row.ok ? '成功' : '失败' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="100">
          <template #default="{ row }"><span class="mono">{{ formatMs(row.latencyMs) }}</span></template>
        </el-table-column>
        <el-table-column label="错误" min-width="240">
          <template #default="{ row }"><span class="mono">{{ row.error || '-' }}</span></template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 故障注入（仅开发） -->
    <div class="card-panel" v-if="fault.supported">
      <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:12px;flex-wrap:wrap">
        <div>
          <h2 style="margin:0 0 4px;font-size:16px">
            故障注入（仅开发）
            <el-tag type="warning" size="small" style="margin-left:6px">DEV ONLY</el-tag>
          </h2>
          <p class="sub" style="margin:0">
            开启后，网关对目标设备的所有 Modbus 调用（含探测）直接失败，用于模拟断连。
            需要后端以 <span class="mono">DEV_FAULT_INJECTION=1</span> 启动，默认关闭。
          </p>
        </div>
        <el-switch
          v-model="faultEnabled"
          :disabled="!auth.canWrite || faultSaving"
          :loading="faultSaving"
          active-text="注入断连"
          @change="onFaultToggle"
        />
      </div>
      <div style="display:flex;gap:10px;align-items:center;margin-top:12px;flex-wrap:wrap">
        <span class="sub" style="margin:0">目标设备</span>
        <el-select v-model="faultTarget" style="width:260px" :disabled="!auth.canWrite || faultSaving" @change="onFaultToggle()">
          <el-option label="全部设备" value="" />
          <el-option v-for="d in devices" :key="d.id" :label="`${d.id} · ${d.name}`" :value="d.id" />
        </el-select>
        <el-tag v-if="!auth.canWrite" type="info" size="small">仅 engineer 可操作</el-tag>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api/client'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

function formatMs(ms) {
  const n = Number(ms) || 0
  return `${Number.isInteger(n) ? String(n) : n.toFixed(2)} ms`
}
const health = ref({})
const healthLoading = ref(false)
const devices = ref([])
const probeDevice = ref('')
const probing = ref(false)
const recentProbes = ref([])
const fault = ref({ supported: false, enabled: false, target: '' })
const faultEnabled = ref(false)
const faultTarget = ref('')
const faultSaving = ref(false)

async function loadHealth() {
  healthLoading.value = true
  try {
    const { data } = await api.get('/health')
    health.value = data
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '健康检查失败')
  } finally {
    healthLoading.value = false
  }
}

async function loadDevices() {
  const { data } = await api.get('/devices')
  devices.value = data.devices || []
  if (!probeDevice.value && devices.value.length) probeDevice.value = devices.value[0].id
}

async function loadProbes() {
  const { data } = await api.get('/diagnostics/probes')
  // newest first for display
  recentProbes.value = (data.probes || []).slice().reverse()
}

async function runProbe() {
  probing.value = true
  try {
    const { data } = await api.post(`/devices/${probeDevice.value}/probe`)
    ElMessage.success(`探测成功：${formatMs(data.latencyMs)}`)
  } catch (e) {
    const body = e.response?.data
    if (body && typeof body.ok === 'boolean') {
      ElMessage.error(`探测失败：${body.error || '未知错误'}（${formatMs(body.latencyMs)}）`)
    } else {
      ElMessage.error(e.response?.data?.error || '探测请求失败')
    }
  } finally {
    probing.value = false
    await Promise.all([loadProbes(), loadHealth()])
  }
}

async function loadFault() {
  const { data } = await api.get('/diagnostics/fault')
  fault.value = data
  faultEnabled.value = !!data.enabled
  faultTarget.value = mapEndpointToId(data.target)
}

// fault.target is an endpoint; map back to a device id for the selector ("" = all).
function mapEndpointToId(endpoint) {
  if (!endpoint) return ''
  const d = devices.value.find((x) => x.endpoint === endpoint)
  return d ? d.id : ''
}

async function onFaultToggle(forceEnabled) {
  const enabled = typeof forceEnabled === 'boolean' ? forceEnabled : faultEnabled.value
  faultSaving.value = true
  try {
    const { data } = await api.post('/diagnostics/fault', {
      enabled,
      deviceId: faultTarget.value || '',
    })
    fault.value = data
    faultEnabled.value = !!data.enabled
    ElMessage.warning(enabled ? '故障注入已开启：目标设备将断连' : '故障注入已关闭：设备恢复')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '故障注入开关失败')
    faultEnabled.value = false
  } finally {
    faultSaving.value = false
  }
}

onMounted(async () => {
  await loadDevices()
  await Promise.all([loadHealth(), loadProbes(), loadFault()])
})
</script>

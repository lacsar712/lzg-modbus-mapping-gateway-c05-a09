# 工业 Modbus 点位监控台（Modbus Mapping Gateway）

Mock PLC + Go 映射网关 + Vue3 监控前端，Docker Compose 一键启动。

## How to Run

```bash
cd projects/05-modbus-mapping-gateway
docker compose up --build
```

启动后访问：

| 服务 | 地址 |
|------|------|
| Frontend | http://localhost:3175 |
| Backend API | http://localhost:8175 |
| Mock Modbus PLC | localhost:15025 → 容器内 `5020` |

禁止端口：3264 / 8264 / 33264（本项目未使用）。

## 账号

| 用户 | 密码 | 权限 |
|------|------|------|
| engineer | mod123456 | 可读 / 写点 / reload |
| observer | obs123456 | 只读 |

## 架构

- **mock-plc**：纯 Python Modbus TCP Server（FC 0x03/0x06/0x10），预置 holding registers
- **backend**：Go + Gin，Hexagonal 分层；YAML DSL；自研类型编解码；寄存器区间合并 snapshot
- **frontend**：Vue 3 + Vite + Element Plus + nginx `/api` 反代

## API

- `POST /api/auth/login`
- `GET  /api/health`
- `POST /api/reload`（body 可选 `{ "yaml": "..." }`；失败保留旧配置）
- `GET  /api/mapping`
- `GET  /api/devices`
- `GET  /api/devices/{id}/points`
- `GET  /api/devices/{id}/points/{name}`
- `PUT  /api/devices/{id}/points/{name}` body `{ "value": <number> }`
- `GET  /api/devices/{id}/snapshot`
- `POST /api/devices/{id}/probe`（连通性探测：TCP + 读 1 个保持寄存器，返回 `ok/latencyMs/error`；失败时 HTTP 502，body 仍为结构化结果）
- `GET  /api/diagnostics/probes`（最近 20 条探测记录）
- `GET  /api/diagnostics/fault`（故障注入开关状态）
- `POST /api/diagnostics/fault`（仅 engineer；body `{"enabled":bool,"deviceId"?:string}`，`deviceId` 留空 = 全部设备）

## 连接诊断与故障注入（仅开发）

诊断页（连接诊断）可对指定设备做一次性连通性探测，展示往返耗时（毫秒）与错误原因；`/api/health`
会聚合**每台设备最近一次探测结果**（`probeSummary`），任一设备最近探测失败时整体 `status=degraded`。
这是面向本网关设备连通性的轻量诊断，不是通用 APM（无指标采集/存储/告警平台）。

故障注入用于在开发环境模拟设备断连：开启后网关对目标设备的所有 Modbus 调用（snapshot/写值/探测）
直接返回 `fault injection: simulated ... failure`。

- **默认关闭**，且只有后端以环境变量 `DEV_FAULT_INJECTION=1`（`1/true/yes/on`）启动时才可用；
  未设置时 `GET /api/diagnostics/fault` 返回 `{"supported":false}`，尝试开启返回 400，前端不显示开关卡片。
- 运行时开关不持久化、默认关闭；重启进程即复位。
- **仅用于本地/开发**，生产环境不要设置该环境变量（docker-compose.yml 未配置此项）。
- 本地开发开启方式：

```bash
DEV_FAULT_INJECTION=1 \
MODBUS_HOST_OVERRIDE=127.0.0.1 MODBUS_PORT_OVERRIDE=5020 \
go run ./cmd/server
```

## YAML DSL

支持 `float32_abcd` / `float32_cdab` / `int16` / `uint16` / `bool_bit`；`scale`/`offset`；写回逆运算并校验 `min`/`max`。

默认设备 `plc-line-a`，点位含 `motor_rpm`、`temperature`、`pressure`、`status_word`、`run_flag`、`setpoint`。

## Verification

```bash
# 1) 登录
TOKEN=$(curl -s -X POST http://localhost:8175/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"engineer","password":"mod123456"}' | jq -r .token)

# 2) snapshot
curl -s http://localhost:8175/api/devices/plc-line-a/snapshot \
  -H "Authorization: Bearer $TOKEN" | jq .

# 3) 写 motor_rpm
curl -s -X PUT http://localhost:8175/api/devices/plc-line-a/points/motor_rpm \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":1800}' | jq .

# 4) 再次 snapshot 确认写回
curl -s http://localhost:8175/api/devices/plc-line-a/snapshot \
  -H "Authorization: Bearer $TOKEN" | jq '.points[] | select(.name=="motor_rpm")'

# 5) 非法 reload 应失败并保留旧配置
curl -s -X POST http://localhost:8175/api/reload \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"yaml":"devices: []"}' | jq .
```

故障注入自测（后端需以 `DEV_FAULT_INJECTION=1` 启动）：

```bash
# 6) 正常探测：ok=true
curl -s -X POST http://localhost:8175/api/devices/plc-line-a/probe \
  -H "Authorization: Bearer $TOKEN" | jq .

# 7) 开启断连注入（deviceId 留空表示全部设备）
curl -s -X POST http://localhost:8175/api/diagnostics/fault \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"enabled":true,"deviceId":"plc-line-a"}' | jq .

# 8) 再次探测：HTTP 502, ok=false, error 含 "fault injection"；health status=degraded
curl -s -X POST http://localhost:8175/api/devices/plc-line-a/probe \
  -H "Authorization: Bearer $TOKEN" | jq .
curl -s http://localhost:8175/api/health | jq .

# 9) 关闭注入后探测恢复 ok=true，health status=ok
curl -s -X POST http://localhost:8175/api/diagnostics/fault \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"enabled":false,"deviceId":"plc-line-a"}' | jq .
curl -s -X POST http://localhost:8175/api/devices/plc-line-a/probe \
  -H "Authorization: Bearer $TOKEN" | jq .
```

浏览器路径：登录 → 设备列表 → 点位监控（看 snapshot）→ 写 `motor_rpm` → 映射配置页提交非法 YAML 应提示保留旧配置。

## 本地开发（可选）

```bash
# mock-plc
python mock-plc/server.py

# backend
cd backend && go run ./cmd/server

# frontend
cd frontend && npm install && npm run dev
```

## 单测

```bash
cd backend && go test ./...
```

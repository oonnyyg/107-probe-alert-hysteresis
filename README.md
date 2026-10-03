# probe-alert-hysteresis

运维回放用的健康探测告警工具。它**不访问真实服务**，只把历史探测结果
(JSONL) 按配置的迟滞阈值转换为稳定的告警时间线，并支持计划维护窗口。
仅使用 Go 标准库。

## 用法

```sh
go run . -config config.json -events events.jsonl [-out report.json]
```

不指定 `-out` 时报告输出到标准输出。任何输入错误都会以非零码退出，
且不会生成部分结果。

## 配置 (config.json)

```json
{
  "targets": [
    {"name": "api", "failure_threshold": 3, "recovery_threshold": 2}
  ],
  "maintenance": [
    {"target": "api", "start": "2026-01-01T00:10:00Z", "end": "2026-01-01T00:20:00Z"}
  ]
}
```

- `failure_threshold` / `recovery_threshold` 必须为正整数，否则配置被拒绝。
- 维护窗口按**左闭右开** `[start, end)` 判断，可重叠、可相邻；
  相邻窗口的交界时刻属于后一个窗口。

## 探测事件 (events.jsonl)

每行一个 JSON 对象：

```json
{"id": "e1", "target": "api", "time": "2026-01-01T00:00:00Z", "result": "failure"}
```

- `result` 仅支持 `success` / `failure`。
- 未知对象、非法结果、非法时间都会报错并**定位到输入行号**。
- `id` 相同且内容完全一致的重复事件只处理一次；
  `id` 相同但内容冲突时**拒绝整批输入**。

## 告警与恢复语义

- 每个对象初始为健康 (`healthy`)。
- 连续失败达到 `failure_threshold` 才进入告警 (`alerting`)；
  告警期间连续成功达到 `recovery_threshold` 才恢复健康。
- 相反结果会打断当前连续计数；状态切换后重新计数。
- 告警持续期间不会重复发送同一轮告警通知。

## 静默 (维护) 语义

- 维护期间**状态和连续计数照常更新**，但状态切换的通知被标记为
  `suppressed: true`，不进入实际通知列表。
- 维护结束**不补发**被抑制的通知；维护后若仍处于告警状态，
  需要新的状态切换才会产生通知。
- 缺少探测数据不会推定服务故障——没有事件就没有状态变化。

## 输出

```json
{
  "transitions": [
    {"target": "api", "from": "healthy", "to": "alerting",
     "time": "...", "event_id": "e3", "line": 3, "suppressed": false}
  ],
  "notifications": [
    {"target": "api", "kind": "alerting", "time": "...", "event_id": "e3", "line": 3}
  ],
  "final_states": [
    {"target": "api", "state": "alerting", "consecutive": 1, "consecutive_kind": "failure"}
  ]
}
```

- `transitions`：全部状态切换明细，含被抑制的切换；
  按 (时间, 输入顺序) 稳定排序，并通过 `event_id` / `line` 追溯触发事件。
- `notifications`：实际发出的通知（不含被抑制项）。
- `final_states`：回放结束时每个对象的状态与当前连续计数，
  按配置声明顺序排列。

## 测试

```sh
go test ./...
```

覆盖阈值边界、连续计数打断、维护边界（左闭右开、相邻窗口）、
通知抑制与不补发、重复事件去重与冲突拒绝、行号定位、稳定排序。

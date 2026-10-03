# probe-alert-hysteresis

运维回放工具：把历史探测结果（JSONL）转换为稳定的告警时间线。
仅使用 Go 标准库，不访问任何真实服务。

## 用法

```sh
go run . -config config.json -events events.jsonl
```

### 配置（config.json）

```json
{
  "targets": [
    {
      "name": "api",
      "failure_threshold": 3,
      "success_threshold": 2,
      "maintenance": [
        {"start": "2026-01-01T00:00:00Z", "end": "2026-01-01T01:00:00Z"}
      ]
    }
  ]
}
```

- `failure_threshold` / `success_threshold` 必须为正整数，否则报错退出。
- `maintenance` 为维护时间段，可重叠或相邻，按左闭右开 `[start, end)` 判断。

### 事件（events.jsonl，每行一个 JSON）

```json
{"id":"e1","target":"api","time":"2026-01-01T00:00:01Z","result":"failure"}
```

- `result` 只能是 `success` 或 `failure`。
- 未知对象、非法结果、非法时间等错误都会定位到具体输入行号。
- 相同 `id` 且内容完全相同的重复事件只处理一次；
  相同 `id` 但内容不同则拒绝整批输入（报错并引用两处行号），不生成部分结果。

## 迟滞与恢复语义

- 每个对象初始为健康（`healthy`）。
- 连续失败达到 `failure_threshold` 才切换到 `alert`；
  告警期间连续成功达到 `success_threshold` 才恢复 `healthy`。
- 相反结果会打断当前连续计数（清零）；状态切换后重新计数。
- 持续故障期间不会重复发送同一轮告警：每次状态切换只产生一条通知。

## 维护与静默语义

- 维护期间状态和连续计数照常更新，但切换通知被标记为
  `suppressed: true`，不进入 `notifications`。
- 维护结束不补发被静默的通知；维护窗口本身不产生任何通知。
- 缺少探测数据不会推定服务故障：没有事件就没有状态变化。

## 输出

```json
{
  "transitions":   [ {"target","time","from","to","event_id","line","suppressed"} ],
  "notifications": [ {"target","time","state","event_id","line"} ],
  "final_states":  [ {"target","state","healthy"} ]
}
```

- `transitions` 为全部状态切换明细（含被静默的），
  `notifications` 为实际发出的通知（不含被静默的）。
- 结果按事件时间排序，同一时间保持输入文件顺序（稳定排序）。
- 每条切换都通过 `event_id` 和 `line` 追溯到触发它的探测事件。

## 测试

```sh
go test ./...
```

覆盖阈值边界、恢复迟滞与计数重置、维护窗口边界（左闭右开）、
静默不补发、重复事件（相同合并 / 冲突拒绝）、行号错误定位与稳定排序。

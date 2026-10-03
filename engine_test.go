package main

import (
	"strings"
	"testing"
)

func parseConfig(t *testing.T, jsonCfg string) *Config {
	t.Helper()
	cfg, err := ParseConfig(strings.NewReader(jsonCfg))
	if err != nil {
		t.Fatalf("ParseConfig 失败: %v", err)
	}
	return cfg
}

func parseEvents(t *testing.T, cfg *Config, jsonl string) []Event {
	t.Helper()
	events, err := ParseEvents(strings.NewReader(jsonl), cfg)
	if err != nil {
		t.Fatalf("ParseEvents 失败: %v", err)
	}
	return events
}

const basicConfig = `{
  "targets": [
    {"name": "api", "failure_threshold": 3, "recovery_threshold": 2}
  ]
}`

func TestThresholdBoundary(t *testing.T) {
	cfg := parseConfig(t, basicConfig)
	// 前两次失败不触发, 第三次失败恰好达到阈值触发告警
	events := parseEvents(t, cfg, `
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"failure"}
{"id":"e2","target":"api","time":"2026-01-01T00:01:00Z","result":"failure"}
{"id":"e3","target":"api","time":"2026-01-01T00:02:00Z","result":"failure"}
`)
	rep := Run(cfg, events)
	if len(rep.Transitions) != 1 {
		t.Fatalf("期望 1 次切换, 实际 %d", len(rep.Transitions))
	}
	tr := rep.Transitions[0]
	if tr.To != StateAlerting || tr.EventID != "e3" {
		t.Fatalf("期望由 e3 触发告警, 实际 %+v", tr)
	}
	if len(rep.Notifications) != 1 {
		t.Fatalf("期望 1 条通知, 实际 %d", len(rep.Notifications))
	}
}

func TestStreakInterruptedAndRecovery(t *testing.T) {
	cfg := parseConfig(t, basicConfig)
	// 失败-失败-成功 打断计数, 之后重新累计三次失败才告警;
	// 恢复需连续两次成功, 中间失败会打断。
	events := parseEvents(t, cfg, `
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"failure"}
{"id":"e2","target":"api","time":"2026-01-01T00:01:00Z","result":"failure"}
{"id":"e3","target":"api","time":"2026-01-01T00:02:00Z","result":"success"}
{"id":"e4","target":"api","time":"2026-01-01T00:03:00Z","result":"failure"}
{"id":"e5","target":"api","time":"2026-01-01T00:04:00Z","result":"failure"}
{"id":"e6","target":"api","time":"2026-01-01T00:05:00Z","result":"failure"}
{"id":"e7","target":"api","time":"2026-01-01T00:06:00Z","result":"success"}
{"id":"e8","target":"api","time":"2026-01-01T00:07:00Z","result":"failure"}
{"id":"e9","target":"api","time":"2026-01-01T00:08:00Z","result":"success"}
{"id":"e10","target":"api","time":"2026-01-01T00:09:00Z","result":"success"}
`)
	rep := Run(cfg, events)
	if len(rep.Transitions) != 2 {
		t.Fatalf("期望 2 次切换, 实际 %d: %+v", len(rep.Transitions), rep.Transitions)
	}
	if rep.Transitions[0].To != StateAlerting || rep.Transitions[0].EventID != "e6" {
		t.Fatalf("首次告警应由 e6 触发, 实际 %+v", rep.Transitions[0])
	}
	if rep.Transitions[1].To != StateHealthy || rep.Transitions[1].EventID != "e10" {
		t.Fatalf("恢复应由 e10 触发, 实际 %+v", rep.Transitions[1])
	}
	if rep.FinalStates[0].State != StateHealthy {
		t.Fatalf("最终应为健康, 实际 %+v", rep.FinalStates[0])
	}
}

func TestNoDuplicateAlertWhileAlerting(t *testing.T) {
	cfg := parseConfig(t, basicConfig)
	events := parseEvents(t, cfg, `
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"failure"}
{"id":"e2","target":"api","time":"2026-01-01T00:01:00Z","result":"failure"}
{"id":"e3","target":"api","time":"2026-01-01T00:02:00Z","result":"failure"}
{"id":"e4","target":"api","time":"2026-01-01T00:03:00Z","result":"failure"}
{"id":"e5","target":"api","time":"2026-01-01T00:04:00Z","result":"failure"}
`)
	rep := Run(cfg, events)
	if len(rep.Transitions) != 1 || len(rep.Notifications) != 1 {
		t.Fatalf("持续故障不得重复告警, 实际切换 %d 通知 %d",
			len(rep.Transitions), len(rep.Notifications))
	}
	if rep.FinalStates[0].State != StateAlerting {
		t.Fatalf("最终应为告警中, 实际 %+v", rep.FinalStates[0])
	}
}

const maintConfig = `{
  "targets": [
    {"name": "api", "failure_threshold": 2, "recovery_threshold": 1}
  ],
  "maintenance": [
    {"target": "api", "start": "2026-01-01T00:10:00Z", "end": "2026-01-01T00:20:00Z"}
  ]
}`

func TestMaintenanceBoundaryAndSuppression(t *testing.T) {
	cfg := parseConfig(t, maintConfig)
	// e1/e2 在窗口起点 (左闭) 进入维护, 切换被抑制;
	// e3 在窗口内恢复, 同样抑制; e4 在 end (右开) 已出维护, 正常通知。
	events := parseEvents(t, cfg, `
{"id":"e1","target":"api","time":"2026-01-01T00:10:00Z","result":"failure"}
{"id":"e2","target":"api","time":"2026-01-01T00:11:00Z","result":"failure"}
{"id":"e3","target":"api","time":"2026-01-01T00:15:00Z","result":"success"}
{"id":"e4","target":"api","time":"2026-01-01T00:20:00Z","result":"failure"}
{"id":"e5","target":"api","time":"2026-01-01T00:21:00Z","result":"failure"}
`)
	rep := Run(cfg, events)
	if len(rep.Transitions) != 3 {
		t.Fatalf("期望 3 次切换, 实际 %d: %+v", len(rep.Transitions), rep.Transitions)
	}
	if !rep.Transitions[0].Suppressed || rep.Transitions[0].To != StateAlerting {
		t.Fatalf("维护内告警应被抑制, 实际 %+v", rep.Transitions[0])
	}
	if !rep.Transitions[1].Suppressed || rep.Transitions[1].To != StateHealthy {
		t.Fatalf("维护内恢复应被抑制, 实际 %+v", rep.Transitions[1])
	}
	if rep.Transitions[2].Suppressed || rep.Transitions[2].EventID != "e5" {
		t.Fatalf("维护结束后切换不应抑制, 实际 %+v", rep.Transitions[2])
	}
	// 维护结束不补发: 只有最后一次切换产生通知
	if len(rep.Notifications) != 1 || rep.Notifications[0].EventID != "e5" {
		t.Fatalf("期望仅 1 条通知且来自 e5, 实际 %+v", rep.Notifications)
	}
}

func TestAdjacentMaintenanceWindows(t *testing.T) {
	cfg := parseConfig(t, `{
	  "targets": [{"name": "api", "failure_threshold": 2, "recovery_threshold": 1}],
	  "maintenance": [
	    {"target": "api", "start": "2026-01-01T00:00:00Z", "end": "2026-01-01T00:10:00Z"},
	    {"target": "api", "start": "2026-01-01T00:10:00Z", "end": "2026-01-01T00:20:00Z"}
	  ]
	}`)
	events := parseEvents(t, cfg, `
{"id":"e1","target":"api","time":"2026-01-01T00:10:00Z","result":"failure"}
{"id":"e2","target":"api","time":"2026-01-01T00:11:00Z","result":"failure"}
`)
	rep := Run(cfg, events)
	// 相邻窗口交界处 00:10 属于第二个窗口 (左闭), 仍应抑制
	if len(rep.Transitions) != 1 || !rep.Transitions[0].Suppressed {
		t.Fatalf("相邻窗口交界处应视为维护中, 实际 %+v", rep.Transitions)
	}
}

func TestDuplicateEvents(t *testing.T) {
	cfg := parseConfig(t, basicConfig)
	// 完全相同的重复事件只处理一次: e1 重复不会推进计数
	events := parseEvents(t, cfg, `
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"failure"}
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"failure"}
{"id":"e2","target":"api","time":"2026-01-01T00:01:00Z","result":"failure"}
`)
	rep := Run(cfg, events)
	if len(rep.Transitions) != 0 {
		t.Fatalf("重复事件不应重复计数, 实际切换 %+v", rep.Transitions)
	}
	if rep.FinalStates[0].Consecutive != 2 {
		t.Fatalf("连续失败应为 2, 实际 %+v", rep.FinalStates[0])
	}
}

func TestConflictingDuplicateRejectsBatch(t *testing.T) {
	cfg := parseConfig(t, basicConfig)
	_, err := ParseEvents(strings.NewReader(`
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"failure"}
{"id":"e1","target":"api","time":"2026-01-01T00:00:00Z","result":"success"}
`), cfg)
	if err == nil || !strings.Contains(err.Error(), "第 3 行") || !strings.Contains(err.Error(), "e1") {
		t.Fatalf("内容冲突的重复 id 应拒绝整批并定位行号, 实际 %v", err)
	}
}

func TestUnknownTargetAndInvalidResultLineNumbers(t *testing.T) {
	cfg := parseConfig(t, basicConfig)
	_, err := ParseEvents(strings.NewReader(
		"{\"id\":\"e1\",\"target\":\"ghost\",\"time\":\"2026-01-01T00:00:00Z\",\"result\":\"failure\"}\n"), cfg)
	if err == nil || !strings.Contains(err.Error(), "第 1 行") || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("未知对象应定位行号, 实际 %v", err)
	}
	_, err = ParseEvents(strings.NewReader(
		"{\"id\":\"e1\",\"target\":\"api\",\"time\":\"2026-01-01T00:00:00Z\",\"result\":\"timeout\"}\n"), cfg)
	if err == nil || !strings.Contains(err.Error(), "第 1 行") || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("非法结果应定位行号, 实际 %v", err)
	}
}

func TestInvalidThresholds(t *testing.T) {
	for _, cfg := range []string{
		`{"targets":[{"name":"a","failure_threshold":0,"recovery_threshold":1}]}`,
		`{"targets":[{"name":"a","failure_threshold":1,"recovery_threshold":-2}]}`,
	} {
		if _, err := ParseConfig(strings.NewReader(cfg)); err == nil {
			t.Fatalf("非正阈值应被拒绝: %s", cfg)
		}
	}
}

func TestStableOrderingByTimeThenInputSeq(t *testing.T) {
	cfg := parseConfig(t, `{
	  "targets": [
	    {"name": "a", "failure_threshold": 1, "recovery_threshold": 1},
	    {"name": "b", "failure_threshold": 1, "recovery_threshold": 1}
	  ]
	}`)
	// 输入乱序, 同一时间的事件保持文件顺序
	events := parseEvents(t, cfg, `
{"id":"e2","target":"b","time":"2026-01-01T00:05:00Z","result":"failure"}
{"id":"e1","target":"a","time":"2026-01-01T00:05:00Z","result":"success"}
{"id":"e0","target":"a","time":"2026-01-01T00:01:00Z","result":"failure"}
`)
	rep := Run(cfg, events)
	if len(rep.Transitions) != 3 {
		t.Fatalf("期望 3 次切换, 实际 %+v", rep.Transitions)
	}
	got := []string{rep.Transitions[0].EventID, rep.Transitions[1].EventID, rep.Transitions[2].EventID}
	want := []string{"e0", "e2", "e1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序应为 %v, 实际 %v", want, got)
		}
	}
}

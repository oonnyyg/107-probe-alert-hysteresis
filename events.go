package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Result 是单次探测结果。
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
)

// Event 是一条探测事件。
type Event struct {
	ID     string
	Target string
	Time   time.Time
	Result Result
	Line   int // 在 JSONL 输入中的行号, 便于追溯
	Seq    int // 去重后的全局输入顺序
}

type eventJSON struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Time   string `json:"time"`
	Result string `json:"result"`
}

// ParseEvents 读取 JSONL 探测事件。未知对象或非法结果会定位到输入行号。
// 相同 ID 且内容完全一致的事件只保留第一条; 内容冲突则返回错误, 拒绝整批输入。
func ParseEvents(r io.Reader, cfg *Config) ([]Event, error) {
	sc := bufio.NewScanner(skipBOM(r))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var events []Event
	seen := map[string]eventJSON{}
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var raw eventJSON
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			return nil, fmt.Errorf("第 %d 行: JSON 解析失败: %w", line, err)
		}
		if raw.ID == "" {
			return nil, fmt.Errorf("第 %d 行: 缺少事件 id", line)
		}
		if raw.Target == "" {
			return nil, fmt.Errorf("第 %d 行: 缺少 target", line)
		}
		if _, ok := cfg.Targets[raw.Target]; !ok {
			return nil, fmt.Errorf("第 %d 行: 未知对象 %q", line, raw.Target)
		}
		if raw.Result != string(ResultSuccess) && raw.Result != string(ResultFailure) {
			return nil, fmt.Errorf("第 %d 行: 非法探测结果 %q, 仅支持 success/failure", line, raw.Result)
		}
		ts, err := time.Parse(time.RFC3339, raw.Time)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行: 非法时间 %q: %w", line, raw.Time, err)
		}
		if prev, dup := seen[raw.ID]; dup {
			if prev != raw {
				return nil, fmt.Errorf("第 %d 行: 事件 id %q 与之前内容冲突, 拒绝整批输入", line, raw.ID)
			}
			continue // 完全相同的重复事件只处理一次
		}
		seen[raw.ID] = raw
		events = append(events, Event{
			ID:     raw.ID,
			Target: raw.Target,
			Time:   ts,
			Result: Result(raw.Result),
			Line:   line,
			Seq:    len(events),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取事件流失败: %w", err)
	}
	return events, nil
}

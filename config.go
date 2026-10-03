package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Thresholds 描述单个受监控对象的告警与恢复迟滞参数。
type Thresholds struct {
	Failure  int // 进入告警所需连续失败次数
	Recovery int // 恢复健康所需连续成功次数
}

// Window 表示一个计划维护时间段，左闭右开 [Start, End)。
type Window struct {
	Start time.Time
	End   time.Time
}

// Contains 按左闭右开判断 t 是否处于维护窗口内。
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

// Target 是受监控对象及其阈值、维护窗口的集合。
type Target struct {
	Name       string
	Thresholds Thresholds
	Windows    []Window
}

// InMaintenance 判断 t 是否落在该对象任一维护窗口内。
func (t *Target) InMaintenance(ts time.Time) bool {
	for _, w := range t.Windows {
		if w.Contains(ts) {
			return true
		}
	}
	return false
}

// Config 是解析后的完整配置。
type Config struct {
	Targets map[string]*Target
	Order   []string // 保持配置中的声明顺序
}

type configJSON struct {
	Targets []struct {
		Name              string `json:"name"`
		FailureThreshold  int    `json:"failure_threshold"`
		RecoveryThreshold int    `json:"recovery_threshold"`
	} `json:"targets"`
	Maintenance []struct {
		Target string `json:"target"`
		Start  string `json:"start"`
		End    string `json:"end"`
	} `json:"maintenance"`
}

// ParseConfig 从 JSON 读取配置并校验阈值必须为正整数。
func ParseConfig(r io.Reader) (*Config, error) {
	var raw configJSON
	dec := json.NewDecoder(skipBOM(r))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg := &Config{Targets: map[string]*Target{}}
	for _, t := range raw.Targets {
		if t.Name == "" {
			return nil, fmt.Errorf("配置中存在缺少 name 的对象")
		}
		if _, dup := cfg.Targets[t.Name]; dup {
			return nil, fmt.Errorf("配置中对象 %q 重复定义", t.Name)
		}
		if t.FailureThreshold <= 0 {
			return nil, fmt.Errorf("对象 %q 的 failure_threshold 必须为正整数, 实际为 %d", t.Name, t.FailureThreshold)
		}
		if t.RecoveryThreshold <= 0 {
			return nil, fmt.Errorf("对象 %q 的 recovery_threshold 必须为正整数, 实际为 %d", t.Name, t.RecoveryThreshold)
		}
		cfg.Targets[t.Name] = &Target{
			Name: t.Name,
			Thresholds: Thresholds{
				Failure:  t.FailureThreshold,
				Recovery: t.RecoveryThreshold,
			},
		}
		cfg.Order = append(cfg.Order, t.Name)
	}
	for _, m := range raw.Maintenance {
		tgt, ok := cfg.Targets[m.Target]
		if !ok {
			return nil, fmt.Errorf("维护窗口引用了未知对象 %q", m.Target)
		}
		start, err := time.Parse(time.RFC3339, m.Start)
		if err != nil {
			return nil, fmt.Errorf("对象 %q 维护窗口 start 非法: %w", m.Target, err)
		}
		end, err := time.Parse(time.RFC3339, m.End)
		if err != nil {
			return nil, fmt.Errorf("对象 %q 维护窗口 end 非法: %w", m.Target, err)
		}
		if !end.After(start) {
			return nil, fmt.Errorf("对象 %q 维护窗口 end 必须晚于 start", m.Target)
		}
		tgt.Windows = append(tgt.Windows, Window{Start: start, End: end})
	}
	return cfg, nil
}

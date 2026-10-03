package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

type transitionJSON struct {
	Target     string `json:"target"`
	From       State  `json:"from"`
	To         State  `json:"to"`
	Time       string `json:"time"`
	EventID    string `json:"event_id"`
	Line       int    `json:"line"`
	Suppressed bool   `json:"suppressed"`
}

type notificationJSON struct {
	Target  string `json:"target"`
	Kind    State  `json:"kind"`
	Time    string `json:"time"`
	EventID string `json:"event_id"`
	Line    int    `json:"line"`
}

type finalStateJSON struct {
	Target          string `json:"target"`
	State           State  `json:"state"`
	Consecutive     int    `json:"consecutive,omitempty"`
	ConsecutiveKind string `json:"consecutive_kind,omitempty"`
}

type reportJSON struct {
	Transitions   []transitionJSON   `json:"transitions"`
	Notifications []notificationJSON `json:"notifications"`
	FinalStates   []finalStateJSON   `json:"final_states"`
}

func toJSON(rep *Report) reportJSON {
	out := reportJSON{
		Transitions:   []transitionJSON{},
		Notifications: []notificationJSON{},
		FinalStates:   []finalStateJSON{},
	}
	for _, tr := range rep.Transitions {
		out.Transitions = append(out.Transitions, transitionJSON{
			Target: tr.Target, From: tr.From, To: tr.To,
			Time:    tr.Time.UTC().Format(time.RFC3339),
			EventID: tr.EventID, Line: tr.Line, Suppressed: tr.Suppressed,
		})
	}
	for _, n := range rep.Notifications {
		out.Notifications = append(out.Notifications, notificationJSON{
			Target: n.Target, Kind: n.Kind,
			Time:    n.Time.UTC().Format(time.RFC3339),
			EventID: n.EventID, Line: n.Line,
		})
	}
	for _, fs := range rep.FinalStates {
		out.FinalStates = append(out.FinalStates, finalStateJSON{
			Target: fs.Target, State: fs.State,
			Consecutive: fs.Consecutive, ConsecutiveKind: fs.ConsecutiveKind,
		})
	}
	return out
}

func run(configPath, eventsPath string, out *os.File) error {
	cfgFile, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("打开配置失败: %w", err)
	}
	defer cfgFile.Close()
	cfg, err := ParseConfig(cfgFile)
	if err != nil {
		return err
	}

	evFile, err := os.Open(eventsPath)
	if err != nil {
		return fmt.Errorf("打开事件文件失败: %w", err)
	}
	defer evFile.Close()
	events, err := ParseEvents(evFile, cfg)
	if err != nil {
		return err
	}

	rep := Run(cfg, events)
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(toJSON(rep))
}

func main() {
	configPath := flag.String("config", "", "JSON 配置文件路径")
	eventsPath := flag.String("events", "", "JSONL 探测事件文件路径")
	outputPath := flag.String("out", "", "输出文件路径, 默认为标准输出")
	flag.Parse()
	if *configPath == "" || *eventsPath == "" {
		fmt.Fprintln(os.Stderr, "用法: probe-alert -config config.json -events events.jsonl [-out report.json]")
		os.Exit(2)
	}
	out := os.Stdout
	if *outputPath != "" {
		f, err := os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建输出文件失败: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}
	if err := run(*configPath, *eventsPath, out); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

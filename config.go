package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Window is a maintenance window, left-closed right-open: [Start, End).
type Window struct {
	Start time.Time
	End   time.Time
}

// Contains reports whether t falls inside the window [Start, End).
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

// Target describes one monitored object and its hysteresis thresholds.
type Target struct {
	Name             string
	FailureThreshold int
	SuccessThreshold int
	Maintenance      []Window
}

type windowJSON struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type targetJSON struct {
	Name             string       `json:"name"`
	FailureThreshold int          `json:"failure_threshold"`
	SuccessThreshold int          `json:"success_threshold"`
	Maintenance      []windowJSON `json:"maintenance"`
}

type configJSON struct {
	Targets []targetJSON `json:"targets"`
}

// LoadConfig parses the JSON configuration from r.
func LoadConfig(r io.Reader) ([]Target, error) {
	var cfg configJSON
	dec := json.NewDecoder(r)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("config must define at least one target")
	}
	seen := map[string]bool{}
	var targets []Target
	for i, tj := range cfg.Targets {
		where := fmt.Sprintf("config target #%d", i+1)
		if tj.Name == "" {
			return nil, fmt.Errorf("%s: name must not be empty", where)
		}
		if seen[tj.Name] {
			return nil, fmt.Errorf("%s: duplicate target name %q", where, tj.Name)
		}
		seen[tj.Name] = true
		if tj.FailureThreshold <= 0 {
			return nil, fmt.Errorf("%s (%q): failure_threshold must be a positive integer, got %d", where, tj.Name, tj.FailureThreshold)
		}
		if tj.SuccessThreshold <= 0 {
			return nil, fmt.Errorf("%s (%q): success_threshold must be a positive integer, got %d", where, tj.Name, tj.SuccessThreshold)
		}
		t := Target{
			Name:             tj.Name,
			FailureThreshold: tj.FailureThreshold,
			SuccessThreshold: tj.SuccessThreshold,
		}
		for j, wj := range tj.Maintenance {
			start, err := time.Parse(time.RFC3339, wj.Start)
			if err != nil {
				return nil, fmt.Errorf("%s (%q) maintenance #%d: invalid start: %v", where, tj.Name, j+1, err)
			}
			end, err := time.Parse(time.RFC3339, wj.End)
			if err != nil {
				return nil, fmt.Errorf("%s (%q) maintenance #%d: invalid end: %v", where, tj.Name, j+1, err)
			}
			if !end.After(start) {
				return nil, fmt.Errorf("%s (%q) maintenance #%d: end must be after start", where, tj.Name, j+1)
			}
			t.Maintenance = append(t.Maintenance, Window{Start: start, End: end})
		}
		targets = append(targets, t)
	}
	return targets, nil
}

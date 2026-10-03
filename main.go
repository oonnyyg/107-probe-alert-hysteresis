package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

func run(configPath, eventsPath string, out io.Writer) error {
	cfgFile, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer cfgFile.Close()
	targets, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	eventsFile, err := os.Open(eventsPath)
	if err != nil {
		return fmt.Errorf("open events: %w", err)
	}
	defer eventsFile.Close()
	known := make(map[string]bool, len(targets))
	for _, t := range targets {
		known[t.Name] = true
	}
	events, err := LoadEvents(eventsFile, known)
	if err != nil {
		return err
	}

	report := Replay(targets, events)
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func main() {
	configPath := flag.String("config", "config.json", "path to JSON config file")
	eventsPath := flag.String("events", "events.jsonl", "path to JSONL probe events file")
	flag.Parse()
	if err := run(*configPath, *eventsPath, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

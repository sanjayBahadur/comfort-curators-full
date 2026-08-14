package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type OracleBehavior struct {
	Phase    int      `json:"phase"`
	Driver   string   `json:"driver"`
	Observes []string `json:"observes"`
}

type Oracle struct {
	Version   int                       `json:"version"`
	Behaviors map[string]OracleBehavior `json:"behaviors"`
}

func loadOracle(root string) (*Oracle, error) {
	p := filepath.Join(root, "contracts", "acceptance", "oracle.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read oracle: %w", err)
	}
	var o Oracle
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("parse oracle: %w", err)
	}
	if o.Version != 1 {
		return nil, fmt.Errorf("unsupported oracle version: %d", o.Version)
	}
	if len(o.Behaviors) != 55 {
		return nil, fmt.Errorf("oracle has %d behaviors, expected 55", len(o.Behaviors))
	}
	return &o, nil
}

func (o *Oracle) BehaviorsForPhase(phase int) []string {
	var names []string
	for name, b := range o.Behaviors {
		if b.Phase <= phase {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (o *Oracle) AllNames() []string {
	names := make([]string, 0, len(o.Behaviors))
	for name := range o.Behaviors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"
)

// App types supported by the monitor.
const (
	TypeProcessor = "processor"
	TypeMemory    = "memory"
	TypeDiskFree  = "disk_free"
	TypeHealth    = "health"
)

type App struct {
	ID           string `json:"-"`
	Group        string `json:"-"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Source       string `json:"source"`
	Schedule     string `json:"schedule"`
	Threshold    string `json:"threshold"`
	Notification string `json:"notification"`
	// Key is the bearer token sent to the exporter. Falls back to the group key.
	Key string `json:"key,omitempty"`
}

type Group struct {
	Group string `json:"group"`
	Key   string `json:"key,omitempty"`
	Apps  []App  `json:"apps"`
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// Load reads and validates the monitor config file.
func Load(path string) ([]Group, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var groups []Group
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	seen := map[string]bool{}
	for gi := range groups {
		g := &groups[gi]
		if g.Group == "" {
			return nil, fmt.Errorf("group #%d: missing group name", gi+1)
		}
		for ai := range g.Apps {
			a := &g.Apps[ai]
			a.Group = g.Group
			a.ID = slug(g.Group) + "--" + slug(a.Name)
			if seen[a.ID] {
				return nil, fmt.Errorf("%s / %s: duplicate app name in group", g.Group, a.Name)
			}
			seen[a.ID] = true
			if a.Key == "" {
				a.Key = g.Key
			}
			if err := a.validate(); err != nil {
				return nil, fmt.Errorf("%s / %s: %w", g.Group, a.Name, err)
			}
		}
	}
	return groups, nil
}

func (a *App) validate() error {
	if a.Name == "" {
		return fmt.Errorf("missing name")
	}
	if a.Source == "" {
		return fmt.Errorf("missing source")
	}
	if _, err := cron.ParseStandard(a.Schedule); err != nil {
		return fmt.Errorf("invalid schedule %q: %w", a.Schedule, err)
	}
	switch a.Type {
	case TypeProcessor, TypeMemory, TypeDiskFree:
		if _, err := strconv.ParseFloat(a.Threshold, 64); err != nil {
			return fmt.Errorf("threshold %q must be a number for type %s", a.Threshold, a.Type)
		}
	case TypeHealth:
		if a.Threshold != "" && a.Threshold != "true" && a.Threshold != "false" {
			return fmt.Errorf("threshold for health must be \"true\" or \"false\"")
		}
	default:
		return fmt.Errorf("unknown type %q", a.Type)
	}
	return nil
}

// ThresholdValue returns the numeric threshold (0 for health checks).
func (a *App) ThresholdValue() float64 {
	v, _ := strconv.ParseFloat(a.Threshold, 64)
	return v
}

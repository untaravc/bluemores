// Package monitor runs the scheduled checks defined in the config file.
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"bluemores/internal/config"
	"bluemores/internal/notify"
	"bluemores/internal/store"
)

type Monitor struct {
	groups    []config.Group
	store     *store.Store
	cron      *cron.Cron
	client    *http.Client
	retention time.Duration
	// RunOnStart runs every check once immediately when Start is called.
	runOnStart bool

	mu        sync.Mutex
	lastState map[string]string
}

func New(groups []config.Group, st *store.Store, timeout, retention time.Duration, runOnStart bool) *Monitor {
	return &Monitor{
		groups:     groups,
		store:      st,
		client:     &http.Client{Timeout: timeout},
		retention:  retention,
		runOnStart: runOnStart,
		lastState:  map[string]string{},
	}
}

// Start schedules every app and begins running checks.
func (m *Monitor) Start() error {
	if latest, err := m.store.Latest(); err == nil {
		for id, r := range latest {
			m.lastState[id] = r.Status
		}
	}

	logger := cron.VerbosePrintfLogger(log.New(io.Discard, "", 0))
	m.cron = cron.New(cron.WithChain(cron.Recover(logger)))
	for _, g := range m.groups {
		for _, app := range g.Apps {
			app := app
			job := cron.NewChain(cron.SkipIfStillRunning(logger)).Then(cron.FuncJob(func() { m.Check(app) }))
			if _, err := m.cron.AddJob(app.Schedule, job); err != nil {
				return fmt.Errorf("schedule %s: %w", app.ID, err)
			}
			log.Printf("monitor: scheduled %s / %s (%s) at %q", app.Group, app.Name, app.Type, app.Schedule)
		}
	}
	if m.retention > 0 {
		m.cron.AddFunc("@hourly", m.prune)
	}
	m.cron.Start()
	if m.runOnStart {
		for _, g := range m.groups {
			for _, app := range g.Apps {
				go m.Check(app)
			}
		}
	}
	return nil
}

func (m *Monitor) Stop() {
	if m.cron != nil {
		<-m.cron.Stop().Done()
	}
}

func (m *Monitor) Groups() []config.Group { return m.groups }

// FindApp returns the app with the given ID.
func (m *Monitor) FindApp(id string) (config.App, bool) {
	for _, g := range m.groups {
		for _, a := range g.Apps {
			if a.ID == id {
				return a, true
			}
		}
	}
	return config.App{}, false
}

func (m *Monitor) prune() {
	n, err := m.store.Prune(time.Now().Add(-m.retention))
	if err != nil {
		log.Printf("monitor: prune failed: %v", err)
	} else if n > 0 {
		log.Printf("monitor: pruned %d old results", n)
	}
}

// exporterResponse matches the exporter's /mon/* JSON shape.
type exporterResponse struct {
	Status bool `json:"status"`
	Data   struct {
		Unit  string  `json:"unit"`
		Value float64 `json:"value"`
		Total float64 `json:"total"`
	} `json:"data"`
	Message string `json:"message"`
}

// Check runs a single check for an app, stores the result and notifies on state change.
func (m *Monitor) Check(app config.App) store.Result {
	r := m.run(app)
	if err := m.store.Insert(r); err != nil {
		log.Printf("monitor: store %s: %v", app.ID, err)
	}

	m.mu.Lock()
	prev, known := m.lastState[app.ID]
	m.lastState[app.ID] = r.Status
	m.mu.Unlock()

	if !known {
		prev = store.StatusOK
	}
	if prev != r.Status {
		go m.notify(app, prev, r)
	}
	return r
}

func (m *Monitor) run(app config.App) store.Result {
	r := store.Result{AppID: app.ID, CreatedAt: time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), m.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.Source, nil)
	if err != nil {
		return fail(r, err.Error())
	}
	if app.Key != "" {
		req.Header.Set("Authorization", "Bearer "+app.Key)
	}

	start := time.Now()
	resp, err := m.client.Do(req)
	r.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return fail(r, err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if app.Type == config.TypeHealth {
		return evalHealth(app, r, resp.StatusCode, body)
	}

	if resp.StatusCode != http.StatusOK {
		return fail(r, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	var er exporterResponse
	if err := json.Unmarshal(body, &er); err != nil {
		return fail(r, "invalid JSON: "+err.Error())
	}
	if !er.Status {
		return fail(r, "exporter reported failure: "+er.Message)
	}
	r.Value, r.Total, r.Unit = er.Data.Value, er.Data.Total, er.Data.Unit
	unit := ""
	if r.Unit != "" {
		unit = " " + r.Unit
	}

	threshold := app.ThresholdValue()
	r.Status = store.StatusOK
	switch app.Type {
	case config.TypeDiskFree:
		// The exporter reports free space; alert when used space (total - free) exceeds the threshold.
		if used := r.Total - r.Value; used > threshold {
			r.Status = store.StatusAlert
			r.Message = fmt.Sprintf("used %.2f%s above threshold %v (%.2f%s free)", used, unit, threshold, r.Value, unit)
		}
	default:
		if r.Value > threshold {
			r.Status = store.StatusAlert
			r.Message = fmt.Sprintf("usage %.2f%s above threshold %v", r.Value, unit, threshold)
		}
	}
	return r
}

func evalHealth(app config.App, r store.Result, code int, body []byte) store.Result {
	healthy := code >= 200 && code < 300
	msg := fmt.Sprintf("HTTP %d", code)
	// If the endpoint returns {"status": false}, treat it as unhealthy too.
	var payload struct {
		Status *bool `json:"status"`
	}
	if healthy && json.Unmarshal(body, &payload) == nil && payload.Status != nil && !*payload.Status {
		healthy = false
		msg += ", status=false"
	}

	expected := app.Threshold != "false"
	if healthy {
		r.Value = 1
	}
	r.Total = 1
	r.Message = msg
	if healthy == expected {
		r.Status = store.StatusOK
	} else {
		r.Status = store.StatusAlert
	}
	return r
}

func fail(r store.Result, msg string) store.Result {
	r.Status = store.StatusError
	r.Message = msg
	return r
}

func (m *Monitor) notify(app config.App, prev string, r store.Result) {
	var msg string
	switch r.Status {
	case store.StatusOK:
		msg = fmt.Sprintf("✅ **RECOVERED** — %s / %s is back to normal", app.Group, app.Name)
		if app.Type != config.TypeHealth {
			msg += fmt.Sprintf(" (%v / %v %s)", r.Value, r.Total, r.Unit)
		}
	case store.StatusAlert:
		msg = fmt.Sprintf("🚨 **ALERT** — %s / %s: %s", app.Group, app.Name, r.Message)
	case store.StatusError:
		msg = fmt.Sprintf("⚠️ **ERROR** — %s / %s: check failed: %s", app.Group, app.Name, r.Message)
	}
	msg += fmt.Sprintf("\n`%s` · was %s · %s", app.Source, prev, r.CreatedAt.Format(time.RFC1123))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := notify.Send(ctx, app.Notification, msg); err != nil {
		log.Printf("monitor: notify %s: %v", app.ID, err)
	}
}

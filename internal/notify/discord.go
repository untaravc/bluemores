// Package notify sends alert messages to webhooks.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Second}

// Send posts a message to a Discord-compatible webhook ({"content": "..."}).
func Send(ctx context.Context, webhook, message string) error {
	if webhook == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"content": message})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded %s", resp.Status)
	}
	return nil
}

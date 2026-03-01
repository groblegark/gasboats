package eventbus

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// BusSSEEvent represents a parsed Server-Sent Event from the /v1/bus/events endpoint.
type BusSSEEvent struct {
	ID      string          `json:"-"`       // SSE id field
	SSEType string          `json:"-"`       // SSE event field (stream name)
	Stream  string          `json:"stream"`  // Short stream name
	Type    string          `json:"type"`    // Event type
	Subject string          `json:"subject"` // Full NATS subject
	Seq     uint64          `json:"seq"`     // JetStream sequence
	TS      string          `json:"ts"`      // ISO 8601 timestamp
	Payload json.RawMessage `json:"payload"` // Raw event JSON
}

// BusSSEClientOptions configures the bus SSE client connection.
type BusSSEClientOptions struct {
	BaseURL string // e.g., "http://localhost:8080"
	Token   string // Bearer auth token
	Stream  string // comma-separated stream names or "all"
	Filter  string // event type filter
}

// ConnectBusSSE connects to the server's SSE /v1/bus/events endpoint and returns
// a channel of parsed bus events. The channel is closed when the context is
// canceled or the connection drops. Errors are sent to the returned error channel.
func ConnectBusSSE(ctx context.Context, opts BusSSEClientOptions) (<-chan BusSSEEvent, <-chan error) {
	events := make(chan BusSSEEvent, 64)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		url := fmt.Sprintf("%s/v1/bus/events", strings.TrimSuffix(opts.BaseURL, "/"))
		sep := "?"
		if opts.Stream != "" {
			url += fmt.Sprintf("%sstream=%s", sep, opts.Stream)
			sep = "&"
		}
		if opts.Filter != "" {
			url += fmt.Sprintf("%sfilter=%s", sep, opts.Filter)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			errs <- fmt.Errorf("creating bus SSE request: %w", err)
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		if opts.Token != "" {
			req.Header.Set("Authorization", "Bearer "+opts.Token)
		}

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: os.Getenv("BEADS_INSECURE_SKIP_VERIFY") == "1", // #nosec G402
				},
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			errs <- fmt.Errorf("bus SSE connection failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errs <- fmt.Errorf("bus SSE endpoint returned status %d", resp.StatusCode)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		// Allow large SSE events (1MB).
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var currentID, currentEvent, currentData string

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				// Empty line = event boundary, dispatch if we have data.
				if currentData != "" {
					var evt BusSSEEvent
					if err := json.Unmarshal([]byte(currentData), &evt); err == nil {
						evt.ID = currentID
						evt.SSEType = currentEvent
						select {
						case events <- evt:
						case <-ctx.Done():
							return
						}
					}
				}
				currentID = ""
				currentEvent = ""
				currentData = ""
				continue
			}

			// Parse SSE fields.
			switch {
			case strings.HasPrefix(line, "id:"):
				currentID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			case strings.HasPrefix(line, "event:"):
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line, "data:")
				if currentData != "" {
					currentData += "\n" + data
				} else {
					currentData = data
				}
			}
			// Ignore comment lines (starting with :) and unknown fields.
		}

		if err := scanner.Err(); err != nil {
			if ctx.Err() == nil {
				errs <- fmt.Errorf("bus SSE stream error: %w", err)
			}
		}
	}()

	return events, errs
}

package poolmanager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gasboat/controller/internal/beadsapi"
	"gasboat/controller/internal/config"

	"log/slog"
)

// mockDaemon creates a test HTTP server that serves bead list/get/update
// requests for pool assign testing.
func mockDaemon(t *testing.T, agents []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/beads":
			// Return the mock agents list.
			beads := make([]map[string]any, len(agents))
			for i, a := range agents {
				beads[i] = map[string]any{
					"id":     a["id"],
					"title":  a["title"],
					"type":   "agent",
					"status": "open",
					"fields": a["fields"],
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"beads": beads,
				"total": len(beads),
			})

		case r.Method == http.MethodGet:
			// GetBead: return the first matching agent.
			for _, a := range agents {
				if "/v1/beads/"+a["id"].(string) == r.URL.Path {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":     a["id"],
						"title":  a["title"],
						"type":   "agent",
						"status": "open",
						"fields": a["fields"],
					})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})

		case r.Method == http.MethodPatch:
			// UpdateBeadFields: just acknowledge.
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAssignPrewarmed_Success(t *testing.T) {
	agents := []map[string]any{
		{
			"id":    "kd-prewarm-1",
			"title": "prewarmed-1",
			"fields": map[string]string{
				"agent":       "prewarmed-1",
				"agent_state": "prewarmed",
				"mode":        "crew",
				"role":        "thread",
				"project":     "gasboat",
			},
		},
	}
	srv := mockDaemon(t, agents)
	defer srv.Close()

	daemon, err := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	pm := New(daemon, &config.Config{
		PrewarmedPoolMinSize: 2,
		PrewarmedPoolMaxSize: 5,
		PrewarmedPoolRole:    "thread",
		PrewarmedPoolMode:    "crew",
	}, slog.Default())

	result, err := pm.AssignPrewarmed(context.Background(), AssignRequest{
		Channel:     "C-test",
		ThreadTS:    "1234.5678",
		Description: "test thread",
		Project:     "gasboat",
	})
	if err != nil {
		t.Fatalf("AssignPrewarmed returned error: %v", err)
	}
	if result.BeadID != "kd-prewarm-1" {
		t.Errorf("expected bead ID kd-prewarm-1, got %s", result.BeadID)
	}
	if result.AgentName != "prewarmed-1" {
		t.Errorf("expected agent name prewarmed-1, got %s", result.AgentName)
	}
}

func TestAssignPrewarmed_EmptyPool(t *testing.T) {
	srv := mockDaemon(t, nil)
	defer srv.Close()

	daemon, err := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	pm := New(daemon, &config.Config{
		PrewarmedPoolMinSize: 2,
		PrewarmedPoolMaxSize: 5,
		PrewarmedPoolRole:    "thread",
		PrewarmedPoolMode:    "crew",
	}, slog.Default())

	_, err = pm.AssignPrewarmed(context.Background(), AssignRequest{
		Channel:  "C-test",
		ThreadTS: "1234.5678",
	})
	if err != ErrPoolEmpty {
		t.Fatalf("expected ErrPoolEmpty, got: %v", err)
	}
}

func TestAssignPrewarmed_PicksOldest(t *testing.T) {
	now := time.Now()
	agents := []map[string]any{
		{
			"id":    "kd-new",
			"title": "prewarmed-new",
			"fields": map[string]string{
				"agent":       "prewarmed-new",
				"agent_state": "prewarmed",
				"mode":        "crew",
				"role":        "thread",
				"project":     "gasboat",
				"created_at":  now.Add(-5 * time.Minute).Format(time.RFC3339),
			},
		},
		{
			"id":    "kd-old",
			"title": "prewarmed-old",
			"fields": map[string]string{
				"agent":       "prewarmed-old",
				"agent_state": "prewarmed",
				"mode":        "crew",
				"role":        "thread",
				"project":     "gasboat",
				"created_at":  now.Add(-20 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	srv := mockDaemon(t, agents)
	defer srv.Close()

	daemon, err := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	pm := New(daemon, &config.Config{
		PrewarmedPoolMinSize: 2,
		PrewarmedPoolMaxSize: 5,
		PrewarmedPoolRole:    "thread",
		PrewarmedPoolMode:    "crew",
	}, slog.Default())

	result, err := pm.AssignPrewarmed(context.Background(), AssignRequest{
		Channel:  "C-test",
		ThreadTS: "1234.5678",
	})
	if err != nil {
		t.Fatalf("AssignPrewarmed returned error: %v", err)
	}
	if result.BeadID != "kd-old" {
		t.Errorf("expected oldest agent kd-old, got %s", result.BeadID)
	}
}

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

// testConfig returns a Config with the project cache pre-populated for testing.
func testConfig() *config.Config {
	cfg := &config.Config{
		PrewarmedPoolInterval: 30 * time.Second,
		ProjectCache: map[string]config.ProjectCacheEntry{
			"gasboat": {
				PrewarmedPool: &beadsapi.PrewarmedPoolConfig{
					Enabled: true,
					MinSize: 2,
					MaxSize: 5,
					Role:    "thread",
					Mode:    "crew",
				},
			},
		},
	}
	return cfg
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

	pm := New(daemon, testConfig(), slog.Default())

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

	pm := New(daemon, testConfig(), slog.Default())

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

	pm := New(daemon, testConfig(), slog.Default())

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

func TestReconcile_CreatesAgentsForProject(t *testing.T) {
	var created int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/beads":
			// Return empty list (no prewarmed agents yet).
			_ = json.NewEncoder(w).Encode(map[string]any{
				"beads": []any{},
				"total": 0,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/beads":
			created++
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id": "kd-new-" + string(rune('0'+created)),
			})
		case r.Method == http.MethodPost:
			// AddLabel: just acknowledge.
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	daemon, err := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	pm := New(daemon, cfg, slog.Default())

	if err := pm.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	// Should create 2 agents (min_size=2, pool was empty).
	if created != 2 {
		t.Errorf("expected 2 agents created, got %d", created)
	}
}

func TestReconcile_NoPoolConfig(t *testing.T) {
	srv := mockDaemon(t, nil)
	defer srv.Close()

	daemon, err := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	// Empty project cache = no pools configured.
	cfg := &config.Config{
		ProjectCache: map[string]config.ProjectCacheEntry{},
	}
	pm := New(daemon, cfg, slog.Default())

	if err := pm.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	// Should be a no-op — no API calls expected.
}

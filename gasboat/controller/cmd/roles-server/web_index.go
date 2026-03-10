package main

import (
	"log/slog"
	"net/http"
	"sync"

	"gasboat/controller/internal/beadsapi"
)

type indexData struct {
	ConfigCount int
	AdviceCount int
	RoleCount   int
	AgentCount  int
}

func handleIndex(client *beadsapi.Client, logger *slog.Logger, ts *templateSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		ctx := r.Context()
		var (
			data      indexData
			configErr error
			adviceErr error
			agentsErr error
			wg        sync.WaitGroup
		)

		wg.Add(3)
		go func() {
			defer wg.Done()
			result, err := client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
				Types: []string{"config"},
			})
			if err != nil {
				configErr = err
				return
			}
			data.ConfigCount = result.Total
		}()
		go func() {
			defer wg.Done()
			result, err := client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
				Types:    []string{"advice"},
				Statuses: []string{"open", "in_progress"},
			})
			if err != nil {
				adviceErr = err
				return
			}
			data.AdviceCount = result.Total
		}()
		go func() {
			defer wg.Done()
			agents, err := client.ListAgentBeads(ctx)
			if err != nil {
				agentsErr = err
				return
			}
			data.AgentCount = len(agents)
			// Count unique roles.
			roles := make(map[string]bool)
			for _, a := range agents {
				if a.Role != "" {
					roles[a.Role] = true
				}
			}
			data.RoleCount = len(roles)
		}()
		wg.Wait()

		if configErr != nil {
			logger.Error("failed to fetch config beads for index", "error", configErr)
		}
		if adviceErr != nil {
			logger.Error("failed to fetch advice beads for index", "error", adviceErr)
		}
		if agentsErr != nil {
			logger.Error("failed to fetch agents for index", "error", agentsErr)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := ts.t.ExecuteTemplate(w, "index", data); err != nil {
			logger.Error("template execution failed", "error", err)
		}
	}
}

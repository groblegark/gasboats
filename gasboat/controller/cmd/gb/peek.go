package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	peekPlain bool
)

var peekCmd = &cobra.Command{
	Use:   "peek [target]",
	Short: "View agent terminal sessions via coopmux",
	Long: `View agent terminal sessions through the coopmux multiplexer.

Without arguments, lists all registered sessions.
With a target, shows the current terminal screen for that session.

Target can be a session UUID, partial session ID, pod name, or agent name.

Requires COOP_MUX_URL to be set (and optionally COOP_MUX_TOKEN for auth).`,
	GroupID: "orchestration",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runPeek,
}

func init() {
	peekCmd.Flags().BoolVar(&peekPlain, "plain", false, "plain text output (no ANSI escape codes)")
}

func runPeek(cmd *cobra.Command, args []string) error {
	muxURL, token, err := resolveMuxEnv()
	if err != nil {
		return err
	}

	client := muxHTTPClient()

	if len(args) == 0 {
		return peekListSessions(client, muxURL, token)
	}
	return peekShowScreen(client, muxURL, token, args[0])
}

// resolveMuxEnv reads and validates the required coopmux env vars.
func resolveMuxEnv() (muxURL, token string, err error) {
	muxURL = os.Getenv("COOP_MUX_URL")
	if muxURL == "" {
		return "", "", fmt.Errorf("COOP_MUX_URL is not set — cannot reach coopmux")
	}
	muxURL = strings.TrimRight(muxURL, "/")
	token = os.Getenv("COOP_MUX_TOKEN")
	return muxURL, token, nil
}

// muxHTTPClient returns an HTTP client with a reasonable timeout.
func muxHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// muxGet performs an authenticated GET request to the mux.
func muxGet(client *http.Client, url, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return client.Do(req)
}

// peekListSessions lists all registered coopmux sessions.
func peekListSessions(client *http.Client, muxURL, token string) error {
	resp, err := muxGet(client, muxURL+"/api/v1/sessions", token)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coopmux returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var sessions []peekSessionInfo
	if err := json.Unmarshal(body, &sessions); err != nil {
		// If we can't parse as structured data, dump raw response.
		fmt.Println(string(body))
		return nil
	}

	if jsonOutput {
		printJSON(sessions)
		return nil
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions registered.")
		return nil
	}

	fmt.Printf("%-38s %-30s %-12s %s\n", "SESSION ID", "POD", "STATE", "HEALTH")
	fmt.Println(strings.Repeat("-", 88))
	for _, s := range sessions {
		health := "ok"
		if s.HealthFailures > 0 {
			health = fmt.Sprintf("%d failures", s.HealthFailures)
		}
		fmt.Printf("%-38s %-30s %-12s %s\n", s.ID, s.Pod(), s.State(), health)
	}
	fmt.Printf("\n%d session(s)\n", len(sessions))

	return nil
}

// peekShowScreen shows the current terminal screen for a session.
func peekShowScreen(client *http.Client, muxURL, token, target string) error {
	sessionID, err := resolveSessionTarget(client, muxURL, token, target)
	if err != nil {
		return err
	}

	resp, err := muxGet(client, muxURL+"/api/v1/sessions/"+sessionID+"/screen", token)
	if err != nil {
		return fmt.Errorf("fetching screen: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coopmux returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var screen peekScreenSnapshot
	if err := json.Unmarshal(body, &screen); err != nil {
		fmt.Println(string(body))
		return nil
	}

	if jsonOutput {
		printJSON(screen)
		return nil
	}

	// Choose ANSI or plain output.
	lines := screen.Lines
	if !peekPlain && isTerminal() && len(screen.ANSI) > 0 {
		lines = screen.ANSI
	}

	for _, line := range lines {
		fmt.Println(line)
	}

	// Show dimensions on stderr so they don't pollute piped output.
	if screen.Cols > 0 && screen.Rows > 0 {
		fmt.Fprintf(os.Stderr, "[%dx%d]\n", screen.Cols, screen.Rows)
	}

	return nil
}

// resolveSessionTarget resolves a target (session ID, partial ID, pod name, or agent name)
// to a full session UUID.
func resolveSessionTarget(client *http.Client, muxURL, token, target string) (string, error) {
	// If it looks like a full UUID, use it directly.
	if len(target) == 36 && strings.Count(target, "-") == 4 {
		return target, nil
	}

	// List sessions and try to match.
	resp, err := muxGet(client, muxURL+"/api/v1/sessions", token)
	if err != nil {
		return "", fmt.Errorf("listing sessions for resolution: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var sessions []peekSessionInfo
	if err := json.Unmarshal(body, &sessions); err != nil {
		return "", fmt.Errorf("parsing sessions: %w", err)
	}

	targetLower := strings.ToLower(target)
	var matches []peekSessionInfo

	for _, s := range sessions {
		if strings.HasPrefix(strings.ToLower(s.ID), targetLower) {
			matches = append(matches, s)
		} else if strings.Contains(strings.ToLower(s.Pod()), targetLower) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matching '%s'", target)
	case 1:
		return matches[0].ID, nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "'%s' matches %d sessions:\n", target, len(matches))
		for _, m := range matches {
			fmt.Fprintf(&b, "  %s  (%s)\n", m.ID, m.Pod())
		}
		return "", fmt.Errorf("%s", b.String())
	}
}

// isTerminal returns true if stdout is a terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ── types ───────────────────────────────────────────────────────────

type peekSessionInfo struct {
	ID             string          `json:"id"`
	URL            string          `json:"url"`
	Metadata       json.RawMessage `json:"metadata"`
	RegisteredAtMS uint64          `json:"registered_at_ms"`
	HealthFailures uint32          `json:"health_failures"`
	CachedState    *string         `json:"cached_state,omitempty"`
}

// Pod extracts the pod name from session metadata.
func (s *peekSessionInfo) Pod() string {
	var meta struct {
		K8s struct {
			Pod string `json:"pod"`
		} `json:"k8s"`
	}
	if err := json.Unmarshal(s.Metadata, &meta); err == nil && meta.K8s.Pod != "" {
		return meta.K8s.Pod
	}
	return "-"
}

// State returns the cached state or "unknown".
func (s *peekSessionInfo) State() string {
	if s.CachedState != nil {
		return *s.CachedState
	}
	return "unknown"
}

type peekScreenSnapshot struct {
	Lines     []string `json:"lines"`
	ANSI      []string `json:"ansi"`
	Cols      int      `json:"cols"`
	Rows      int      `json:"rows"`
	AltScreen bool     `json:"alt_screen"`
	Seq       uint64   `json:"seq"`
}

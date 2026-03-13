package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gasboat/controller/internal/coopapi"

	"github.com/spf13/cobra"
)

var initAgentCmd = &cobra.Command{
	Use:   "init",
	Short: "Manage agent lifecycle alongside coop",
	Long: `Orchestrates the agent lifecycle by communicating with the coop REST API.

Replaces the curl-based functions in entrypoint.sh with a single Go binary:
  - Bypass startup prompts (trust dialog, setup wizard, resume picker)
  - Inject initial work prompt (via nudge-prompt resolution)
  - Monitor agent state (idle/working/exited/rate_limited)
  - Refresh OAuth credentials
  - Handle graceful shutdown signals

Runs as a long-lived process alongside coop, exiting when the agent exits.`,
	GroupID: "agent",
	RunE:   runInitAgent,
}

var (
	initCoopURL  string
	initStandby  bool
	initMockMode bool
)

func init() { //nolint:gochecknoinits
	initAgentCmd.Flags().StringVar(&initCoopURL, "coop-url", "http://localhost:8080", "coop API base URL")
	initAgentCmd.Flags().BoolVar(&initStandby, "standby", false, "enable standby mode (prewarmed pool agents)")
	initAgentCmd.Flags().BoolVar(&initMockMode, "mock", false, "mock mode (skip OAuth refresh)")
}

func runInitAgent(cmd *cobra.Command, args []string) error {
	coop := coopapi.New(initCoopURL)
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Wait for coop to become available.
	if err := waitForCoop(ctx, coop); err != nil {
		return fmt.Errorf("coop not available: %w", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	// Signal handling: catch SIGTERM/SIGINT for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleSignals(ctx, cancel, coop, sigCh)
	}()

	// Main lifecycle chain: bypass startup → [standby] → inject prompt.
	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := bypassStartup(ctx, coop); err != nil {
			logInit("bypass startup failed: %v", err)
			return
		}

		if initStandby && os.Getenv("BOAT_AGENT_BEAD_ID") != "" {
			if err := standbyWait(ctx, coop); err != nil {
				logInit("standby wait failed: %v", err)
				return
			}
		}

		if err := injectPrompt(ctx, coop); err != nil {
			logInit("inject prompt failed: %v", err)
		}
	}()

	// Monitor agent exit — this drives the process lifecycle.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := monitorExit(ctx, coop); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
		cancel() // agent exited, shut everything down
	}()

	// Monitor agent idle state (update bead).
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitorIdle(ctx, coop)
	}()

	// OAuth credential refresh (unless mock mode).
	if !initMockMode {
		wg.Add(1)
		go func() {
			defer wg.Done()
			refreshCredentials(ctx, coop)
		}()
	}

	wg.Wait()
	return nil
}

// waitForCoop polls the coop API until it responds.
func waitForCoop(ctx context.Context, coop *coopapi.Client) error {
	logInit("Waiting for coop at %s", initCoopURL)
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := coop.GetAgentState(ctx); err == nil {
			logInit("Coop is ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out after 120s waiting for coop")
}

// bypassStartup handles startup prompts (trust dialog, setup wizard, resume picker).
func bypassStartup(ctx context.Context, coop *coopapi.Client) error {
	logInit("Bypassing startup prompts")
	falsePositiveCount := 0

	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		time.Sleep(2 * time.Second)

		state, err := coop.GetAgentState(ctx)
		if err != nil {
			continue
		}

		promptType := ""
		promptSubtype := ""
		if state.Prompt != nil {
			promptType = state.Prompt.Type
			promptSubtype = state.Prompt.Subtype
		}
		_ = promptSubtype

		// Handle interactive prompts while agent is starting.
		if state.State == "starting" {
			screen, err := coop.GetScreenText(ctx)
			if err == nil {
				// "Resume Session" picker — press Enter to resume.
				if containsCI(screen, "resume") && containsCI(screen, "session") {
					logInit("Detected resume session picker, selecting resume")
					_ = coop.SendKeys(ctx, []string{"Return"})
					time.Sleep(3 * time.Second)
					continue
				}
			}
		}

		// Handle "Detected a custom API key" or trust prompts.
		if state.State == "starting" || promptType == "permission" {
			screen, err := coop.GetScreenText(ctx)
			if err == nil {
				if strings.Contains(screen, "Detected a custom API key") {
					logInit("Detected API key prompt, selecting 'Yes'")
					_ = coop.SendKeys(ctx, []string{"Up", "Return"})
					time.Sleep(3 * time.Second)
					continue
				}
				if strings.Contains(screen, "trust this folder") {
					logInit("Auto-accepting trust folder prompt")
					_ = coop.Respond(ctx, 0)
					time.Sleep(3 * time.Second)
					continue
				}
			}
		}

		// Handle setup prompts.
		if promptType == "setup" {
			screen, err := coop.GetScreen(ctx)
			if err == nil {
				if strings.Contains(screen, "No, exit") {
					logInit("Auto-accepting setup prompt (subtype: %s)", promptSubtype)
					_ = coop.Respond(ctx, 2)
					falsePositiveCount = 0
					time.Sleep(5 * time.Second)
					continue
				}
				falsePositiveCount++
				if falsePositiveCount >= 5 {
					logInit("Skipping false-positive setup prompt (no dialog after %d checks)", falsePositiveCount)
					return nil
				}
				continue
			}
		}

		// If agent is past setup prompts, we're done.
		if state.State == "idle" || state.State == "working" {
			logInit("Startup bypass complete (agent state: %s)", state.State)
			return nil
		}
	}

	logInit("WARNING: auto-bypass timed out after 120s")
	return nil
}

// injectPrompt resolves the nudge prompt and injects it into the agent.
func injectPrompt(ctx context.Context, coop *coopapi.Client) error {
	// Wait for agent to reach a nudge-ready state (idle or working).
	//
	// After startup prompt bypass, the agent may briefly transition to
	// "working" as SessionStart hooks fire (e.g., gb hook prime). Coop
	// accepts nudges in both idle and working states — the agent picks up
	// the message when the current generation finishes. We must NOT skip
	// on "working" or the initial nudge is lost.
	var readyState string
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		state, err := coop.GetAgentState(ctx)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if state.State == "idle" || state.State == "working" {
			readyState = state.State
			break
		}

		time.Sleep(2 * time.Second)
	}

	if readyState == "" {
		logInit("WARNING: timed out waiting for agent to be ready for nudge")
		return nil
	}

	// Resolve nudge prompt. Use the daemon (beadsapi) client which is
	// connected by PersistentPreRunE in main.go.
	promptType := detectNudgeType()
	vars := buildNudgeVars()
	nudgeMsg := resolveNudgeFromConfig(promptType, vars)

	if nudgeMsg == "" {
		logInit("WARNING: nudge-prompt resolution failed — using fallback")
		nudgeMsg = "Run gb ready to find work."
	}

	role := os.Getenv("BOAT_ROLE")
	logInit("Injecting initial work prompt (role: %s, agent state: %s)", role, readyState)

	// Deliver with retry. Coop accepts nudges during both idle and working
	// states, but transient errors or brief state transitions may cause a
	// single attempt to fail.
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := coop.Nudge(ctx, nudgeMsg)
		if err != nil {
			lastErr = err
			logInit("WARNING: nudge attempt %d/%d failed: %v", attempt, maxAttempts, err)
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.Delivered {
			logInit("Initial prompt delivered successfully (attempt %d)", attempt)
			return nil
		}

		lastErr = fmt.Errorf("not delivered: %s", resp.Reason)
		logInit("WARNING: nudge attempt %d/%d not delivered: %s", attempt, maxAttempts, resp.Reason)
		time.Sleep(2 * time.Second)
	}

	logInit("WARNING: all %d nudge attempts failed: %v", maxAttempts, lastErr)
	return nil
}

// standbyWait polls the agent bead until the pool manager assigns work.
func standbyWait(ctx context.Context, coop *coopapi.Client) error {
	// Signal monitor_agent_idle to skip state updates.
	if err := os.WriteFile("/tmp/standby_active", []byte("1"), 0644); err != nil {
		logInit("WARNING: failed to write standby sentinel: %v", err)
	}

	beadID := os.Getenv("BOAT_AGENT_BEAD_ID")
	logInit("Standby mode: Claude is idle, waiting for assignment (bead: %s)", beadID)

	pollInterval := envDuration("BOAT_STANDBY_POLL", 5*time.Second)
	maxWait := envDuration("BOAT_STANDBY_MAX_TTL", 24*time.Hour)
	deadline := time.Now().Add(maxWait)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			logInit("Standby safety-net TTL (%s) exceeded, shutting down", maxWait)
			os.Remove("/tmp/standby_active")
			_ = os.WriteFile("/tmp/standby_expired", []byte("1"), 0644)
			_ = coop.Shutdown(ctx)
			return fmt.Errorf("standby TTL exceeded")
		}

		// Check agent_state via beads daemon.
		currentState := "prewarmed"
		if daemon != nil {
			bead, err := daemon.GetBead(ctx, beadID)
			if err == nil {
				if s := bead.Fields["agent_state"]; s != "" {
					currentState = s
				}
			}
		}

		if currentState != "prewarmed" {
			logInit("Assignment received (state: %s), exiting standby", currentState)

			// Hydrate env vars from the assigned bead's fields.
			if daemon != nil {
				hydrateAssignmentEnv(ctx, beadID)
			}

			os.Remove("/tmp/standby_active")
			return nil
		}

		time.Sleep(pollInterval)
	}
}

// hydrateAssignmentEnv fetches the agent bead and exports thread/task env vars.
func hydrateAssignmentEnv(ctx context.Context, beadID string) {
	bead, err := daemon.GetBead(ctx, beadID)
	if err != nil {
		return
	}

	if ch := bead.Fields["slack_thread_channel"]; ch != "" {
		os.Setenv("SLACK_THREAD_CHANNEL", ch)
		logInit("Hydrated SLACK_THREAD_CHANNEL=%s", ch)
	}
	if ts := bead.Fields["slack_thread_ts"]; ts != "" {
		os.Setenv("SLACK_THREAD_TS", ts)
		logInit("Hydrated SLACK_THREAD_TS=%s", ts)
	}
	if proj := bead.Fields["project"]; proj != "" && os.Getenv("PROJECT") == "" {
		os.Setenv("PROJECT", proj)
		logInit("Hydrated PROJECT=%s", proj)
	}
	if tid := bead.Fields["task_id"]; tid != "" {
		os.Setenv("BOAT_TASK_ID", tid)
		logInit("Hydrated BOAT_TASK_ID=%s", tid)
	}
}

// monitorExit polls agent state and triggers shutdown when agent exits or is rate-limited.
func monitorExit(ctx context.Context, coop *coopapi.Client) error {
	time.Sleep(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		time.Sleep(5 * time.Second)

		state, err := coop.GetAgentState(ctx)
		if err != nil {
			// coop is gone, agent exited
			return nil
		}

		if state.State == "exited" {
			logInit("Agent exited, requesting coop shutdown")
			_ = coop.Shutdown(ctx)
			return nil
		}

		// Detect rate-limited state and park the pod.
		if state.ErrorCategory == "rate_limited" {
			logInit("Agent rate-limited, dismissing prompt")
			_ = coop.SendKeys(ctx, []string{"Return"})
			logInit("Rate limit info: %s", state.LastMessage)

			// Report rate_limited status to agent bead.
			agentID := os.Getenv("KD_AGENT_ID")
			if agentID != "" && daemon != nil {
				_ = daemon.UpdateAgentState(ctx, agentID, "rate_limited")
			}

			// Write sentinel so the restart loop knows to sleep until reset.
			_ = os.WriteFile("/tmp/rate_limit_reset", []byte(state.LastMessage), 0644)
			time.Sleep(2 * time.Second)

			logInit("Requesting coop shutdown (rate-limited)")
			_ = coop.Shutdown(ctx)
			return nil
		}
	}
}

// monitorIdle polls agent state and updates the agent bead.
func monitorIdle(ctx context.Context, coop *coopapi.Client) {
	agentID := os.Getenv("KD_AGENT_ID")
	if agentID == "" || daemon == nil {
		return
	}

	time.Sleep(15 * time.Second)
	prevState := ""

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		time.Sleep(10 * time.Second)

		state, err := coop.GetAgentState(ctx)
		if err != nil {
			return // coop gone
		}

		if state.State != prevState && state.State != "" {
			// Skip state updates while in standby.
			if _, err := os.Stat("/tmp/standby_active"); err != nil {
				switch state.State {
				case "idle", "working":
					_ = daemon.UpdateAgentState(ctx, agentID, state.State)
				}
			}
			prevState = state.State
		}

		if state.State == "exited" {
			return
		}
	}
}

// oauthCredentials holds the Claude AI OAuth credentials file structure.
type oauthCredentials struct {
	ClaudeAiOauth *oauthTokenData `json:"claudeAiOauth,omitempty"`
}

type oauthTokenData struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// refreshCredentials handles OAuth token refresh in a loop.
func refreshCredentials(ctx context.Context, coop *coopapi.Client) {
	// Skip if API key mode (no OAuth credentials to refresh).
	credsFile := os.Getenv("XDG_STATE_HOME")
	if credsFile == "" {
		credsFile = os.Getenv("HOME") + "/.local/state"
	}
	credsFile += "/claude-code/.credentials.json"

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		if _, err := os.Stat(credsFile); os.IsNotExist(err) {
			logInit("API key mode — skipping OAuth refresh loop")
			return
		}
	}

	time.Sleep(30 * time.Second) // let Claude start first
	consecutiveFailures := 0
	maxFailures := 5

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		time.Sleep(5 * time.Minute)

		data, err := os.ReadFile(credsFile)
		if err != nil {
			continue
		}

		var creds oauthCredentials
		if err := json.Unmarshal(data, &creds); err != nil || creds.ClaudeAiOauth == nil {
			continue
		}

		token := creds.ClaudeAiOauth
		if token.RefreshToken == "" || token.ExpiresAt == 0 {
			continue
		}

		// Coop-provisioned credentials use a sentinel expiresAt (>= 10^12 ms).
		if token.ExpiresAt >= 9999999999000 {
			consecutiveFailures = 0
			continue
		}

		// Check if within 1 hour of expiry (3600000ms).
		nowMs := time.Now().UnixMilli()
		remainingMs := token.ExpiresAt - nowMs
		if remainingMs > 3600000 {
			consecutiveFailures = 0
			continue
		}

		logInit("OAuth token expires in %dm, refreshing...", remainingMs/60000)

		newAccess, newRefresh, expiresIn, err := doOAuthRefresh(ctx, token.RefreshToken)
		if err != nil {
			consecutiveFailures++
			logInit("WARNING: OAuth refresh failed (attempt %d/%d): %v", consecutiveFailures, maxFailures, err)
			if consecutiveFailures >= maxFailures {
				if shouldTerminateOnAuthFailure(ctx, coop) {
					logInit("FATAL: OAuth refresh failed %d consecutive times, terminating pod", maxFailures)
					syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
					return
				}
				consecutiveFailures = 0
			}
			continue
		}

		consecutiveFailures = 0
		newExpiresAt := time.Now().UnixMilli() + int64(expiresIn)*1000

		// Update credentials file.
		creds.ClaudeAiOauth.AccessToken = newAccess
		creds.ClaudeAiOauth.RefreshToken = newRefresh
		creds.ClaudeAiOauth.ExpiresAt = newExpiresAt

		updatedData, err := json.MarshalIndent(creds, "", "  ")
		if err != nil {
			logInit("WARNING: failed to marshal updated credentials: %v", err)
			continue
		}

		tmpFile := credsFile + ".tmp"
		if err := os.WriteFile(tmpFile, updatedData, 0600); err != nil {
			logInit("WARNING: failed to write credentials: %v", err)
			continue
		}
		if err := os.Rename(tmpFile, credsFile); err != nil {
			logInit("WARNING: failed to rename credentials: %v", err)
			continue
		}

		logInit("OAuth credentials refreshed (expires in %dh)", expiresIn/3600)
	}
}

// oauthRefreshRequest is the request body for token refresh.
type oauthRefreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
}

// oauthRefreshResponse is the response from the token endpoint.
type oauthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// doOAuthRefresh performs the actual token refresh HTTP request.
func doOAuthRefresh(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, expiresIn int, err error) {
	reqBody := oauthRefreshRequest{
		GrantType:    "refresh_token",
		RefreshToken: refreshToken,
		ClientID:     oauthClientID,
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", 0, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(string(bodyData)))
	if err != nil {
		return "", "", 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var tokenResp oauthRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", 0, fmt.Errorf("decode response: %w", err)
	}

	if tokenResp.AccessToken == "" || tokenResp.RefreshToken == "" {
		return "", "", 0, fmt.Errorf("response missing tokens")
	}

	return tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn, nil
}

// shouldTerminateOnAuthFailure checks if the agent is still active (don't kill a working agent).
func shouldTerminateOnAuthFailure(ctx context.Context, coop *coopapi.Client) bool {
	state, err := coop.GetAgentState(ctx)
	if err != nil {
		return true
	}
	if state.State == "working" || state.State == "idle" {
		logInit("WARNING: OAuth refresh failing but agent is %s, not terminating", state.State)
		return false
	}
	return true
}

// handleSignals catches SIGTERM/SIGINT and performs graceful shutdown via coop API.
func handleSignals(ctx context.Context, cancel context.CancelFunc, coop *coopapi.Client, sigCh <-chan os.Signal) {
	select {
	case sig := <-sigCh:
		logInit("Graceful shutdown: interrupting Claude before forwarding %s", sig)
		// Send Escape to interrupt Claude mid-generation.
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = coop.SendKeys(shutCtx, []string{"Escape"})
		time.Sleep(2 * time.Second)
		// Request graceful coop shutdown via API.
		_ = coop.Shutdown(shutCtx)
		time.Sleep(3 * time.Second)
		cancel()
	case <-ctx.Done():
	}
}

// containsCI performs a case-insensitive substring search.
func containsCI(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// envDuration reads a duration from an environment variable (as seconds).
func envDuration(key string, defaultVal time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	var secs int
	if _, err := fmt.Sscanf(s, "%d", &secs); err != nil {
		return defaultVal
	}
	return time.Duration(secs) * time.Second
}

func logInit(format string, args ...interface{}) {
	log.Printf("[gb init] "+format, args...)
}

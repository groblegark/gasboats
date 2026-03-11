package bridge

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gasboat/controller/internal/beadsapi"

	"github.com/slack-go/slack/slackevents"
)

func TestThreadAgentsPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Create state, add thread agent, close.
	state1, err := NewStateManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := state1.SetThreadAgent("C-test", "1111.2222", "gasboat/crew/hq"); err != nil {
		t.Fatal(err)
	}

	// Reload from disk.
	state2, err := NewStateManager(path)
	if err != nil {
		t.Fatal(err)
	}

	agent, ok := state2.GetThreadAgent("C-test", "1111.2222")
	if !ok || agent != "gasboat/crew/hq" {
		t.Errorf("after reload: got (%q, %v), want (%q, true)", agent, ok, "gasboat/crew/hq")
	}

	// Remove and verify.
	if err := state2.RemoveThreadAgent("C-test", "1111.2222"); err != nil {
		t.Fatal(err)
	}
	if _, ok := state2.GetThreadAgent("C-test", "1111.2222"); ok {
		t.Error("expected thread agent to be removed")
	}
}

func TestRemoveThreadAgentByAgent(t *testing.T) {
	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	_ = state.SetThreadAgent("C-a", "1.1", "gasboat/crew/hq")
	_ = state.SetThreadAgent("C-b", "2.2", "gasboat/crew/hq")
	_ = state.SetThreadAgent("C-c", "3.3", "gasboat/crew/k8s")

	if err := state.RemoveThreadAgentByAgent("gasboat/crew/hq"); err != nil {
		t.Fatal(err)
	}

	// hq entries should be gone.
	if _, ok := state.GetThreadAgent("C-a", "1.1"); ok {
		t.Error("expected C-a:1.1 to be removed")
	}
	if _, ok := state.GetThreadAgent("C-b", "2.2"); ok {
		t.Error("expected C-b:2.2 to be removed")
	}
	// k8s entry should remain.
	if agent, ok := state.GetThreadAgent("C-c", "3.3"); !ok || agent != "gasboat/crew/k8s" {
		t.Errorf("expected C-c:3.3 to remain, got (%q, %v)", agent, ok)
	}
}

func TestRemoveThreadAgentByAgent_CleansListenFlags(t *testing.T) {
	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Set up threads with listen mode.
	_ = state.SetThreadAgent("C-a", "1.1", "agent-a")
	_ = state.SetListenThread("C-a", "1.1")
	_ = state.SetThreadAgent("C-b", "2.2", "agent-b")
	_ = state.SetListenThread("C-b", "2.2")

	// Remove agent-a — should also clean up its listen flag.
	if err := state.RemoveThreadAgentByAgent("agent-a"); err != nil {
		t.Fatal(err)
	}

	// agent-a's listen flag should be gone.
	if state.IsListenThread("C-a", "1.1") {
		t.Error("expected listen flag for C-a:1.1 to be removed")
	}
	// agent-b's listen flag should remain.
	if !state.IsListenThread("C-b", "2.2") {
		t.Error("expected listen flag for C-b:2.2 to remain")
	}
}

func TestHandleThreadSpawn_CreatesBeadAndState(t *testing.T) {
	daemon := newMockDaemon()

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	b := &Bot{
		daemon:     daemon,
		state:      state,
		logger:     slog.Default(),
		botUserID:  "U-BOT",
		agentCards: map[string]MessageRef{},
	}

	channel := "C-thread-test"
	threadTS := "1111.2222"

	// Verify no agent bound to this thread initially.
	if agent := b.getAgentByThread(channel, threadTS); agent != "" {
		t.Fatalf("expected no agent for thread, got %q", agent)
	}

	agentName := "thread-" + sanitizeTS(threadTS)
	if agentName != "thread-1111-2222" {
		t.Fatalf("expected agent name thread-1111-2222, got %q", agentName)
	}

	ctx := context.Background()
	beadID, err := daemon.CreateBead(ctx, beadsapi.CreateBeadRequest{
		Title:       agentName,
		Type:        "agent",
		Description: "Thread-spawned agent for test",
		Labels:      []string{"slack-thread"},
	})
	if err != nil {
		t.Fatalf("CreateBead failed: %v", err)
	}

	// Record thread→agent mapping.
	if err := state.SetThreadAgent(channel, threadTS, agentName); err != nil {
		t.Fatalf("SetThreadAgent failed: %v", err)
	}

	// Verify bead was created.
	bead, err := daemon.GetBead(ctx, beadID)
	if err != nil {
		t.Fatalf("GetBead failed: %v", err)
	}
	if bead.Type != "agent" {
		t.Errorf("bead type = %q, want agent", bead.Type)
	}
	if bead.Title != agentName {
		t.Errorf("bead title = %q, want %q", bead.Title, agentName)
	}
	if !hasLabel(bead.Labels, "slack-thread") {
		t.Errorf("bead labels = %v, want slack-thread", bead.Labels)
	}

	// Verify thread→agent state.
	agent, ok := state.GetThreadAgent(channel, threadTS)
	if !ok || agent != agentName {
		t.Errorf("thread agent = (%q, %v), want (%q, true)", agent, ok, agentName)
	}

	// Verify getAgentByThread now resolves.
	if got := b.getAgentByThread(channel, threadTS); got != agentName {
		t.Errorf("getAgentByThread = %q, want %q", got, agentName)
	}
}

func TestResolveAgentThread_ThreadBound(t *testing.T) {
	daemon := newMockDaemon()
	daemon.beads["thread-agent"] = &beadsapi.BeadDetail{
		ID:    "bd-thread-agent",
		Title: "thread-agent",
		Type:  "agent",
		Fields: map[string]string{
			"agent":                "thread-agent",
			"slack_thread_channel": "C-thread",
			"slack_thread_ts":      "1234.5678",
			"spawn_source":         "slack-thread",
		},
	}

	b := &Bot{
		daemon: daemon,
		logger: slog.Default(),
	}

	channel, ts := b.resolveAgentThread(context.Background(), "thread-agent")
	if channel != "C-thread" {
		t.Errorf("channel = %q, want C-thread", channel)
	}
	if ts != "1234.5678" {
		t.Errorf("ts = %q, want 1234.5678", ts)
	}
}

func TestResolveAgentThread_RegularAgent(t *testing.T) {
	daemon := newMockDaemon()
	daemon.beads["regular-agent"] = &beadsapi.BeadDetail{
		ID:    "bd-regular-agent",
		Title: "regular-agent",
		Type:  "agent",
		Fields: map[string]string{
			"agent":   "regular-agent",
			"project": "gasboat",
		},
	}

	b := &Bot{
		daemon: daemon,
		logger: slog.Default(),
	}

	channel, ts := b.resolveAgentThread(context.Background(), "regular-agent")
	if channel != "" {
		t.Errorf("expected empty channel for regular agent, got %q", channel)
	}
	if ts != "" {
		t.Errorf("expected empty ts for regular agent, got %q", ts)
	}
}

func TestResolveAgentThread_NotFound(t *testing.T) {
	daemon := newMockDaemon()

	b := &Bot{
		daemon: daemon,
		logger: slog.Default(),
	}

	channel, ts := b.resolveAgentThread(context.Background(), "nonexistent")
	if channel != "" || ts != "" {
		t.Errorf("expected empty for nonexistent agent, got channel=%q ts=%q", channel, ts)
	}
}

func TestRespawnThreadAgent_CreatesSameNameBead(t *testing.T) {
	daemon := newMockDaemon()
	// Add a project bead so the channel maps to a project.
	daemon.beads["proj-test"] = &beadsapi.BeadDetail{
		ID: "proj-test", Title: "testproject", Type: "project",
		Fields: map[string]string{"slack_channel": "C-thread-test"},
	}

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	channel := "C-thread-test"
	threadTS := "1111.2222"
	agentName := "thread-1111-2222"

	// Pre-populate thread→agent mapping.
	_ = state.SetThreadAgent(channel, threadTS, agentName)

	b := &Bot{
		daemon:          daemon,
		state:           state,
		logger:          slog.Default(),
		botUserID:       "U-BOT",
		agentCards:      map[string]MessageRef{},
		threadSpawnMsgs: make(map[string]MessageRef),
	}

	b.respawnThreadAgent(context.Background(), channel, threadTS, agentName, "wake up please")

	// Thread→agent mapping should still exist.
	agent, ok := state.GetThreadAgent(channel, threadTS)
	if !ok {
		t.Fatal("expected thread agent mapping to be preserved after respawn")
	}
	if agent != agentName {
		t.Errorf("expected agent name %q preserved, got %q", agentName, agent)
	}

	// Should have created an agent bead with the SAME name.
	var found *beadsapi.BeadDetail
	for _, bead := range daemon.beads {
		if bead.Type == "agent" && bead.Title == agentName {
			found = bead
			break
		}
	}
	if found == nil {
		t.Fatal("expected agent bead to be created with same name")
	}

	// Verify fields.
	if found.Fields["agent"] != agentName {
		t.Errorf("agent field = %q, want %q", found.Fields["agent"], agentName)
	}
	if found.Fields["slack_thread_channel"] != channel {
		t.Errorf("slack_thread_channel = %q, want %q", found.Fields["slack_thread_channel"], channel)
	}
	if found.Fields["slack_thread_ts"] != threadTS {
		t.Errorf("slack_thread_ts = %q, want %q", found.Fields["slack_thread_ts"], threadTS)
	}
	if found.Fields["spawn_source"] != "slack-thread-resume" {
		t.Errorf("spawn_source = %q, want slack-thread-resume", found.Fields["spawn_source"])
	}
	if !hasLabel(found.Labels, "slack-thread") {
		t.Errorf("expected slack-thread label, got %v", found.Labels)
	}
}

func TestRespawnThreadAgent_RejectsNoProject(t *testing.T) {
	daemon := newMockDaemon()
	// No project bead → no channel mapping → should reject.

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = state.SetThreadAgent("C-unmapped", "1111.2222", "thread-1111-2222")

	b := &Bot{
		daemon:          daemon,
		state:           state,
		logger:          slog.Default(),
		botUserID:       "U-BOT",
		agentCards:      map[string]MessageRef{},
		threadSpawnMsgs: make(map[string]MessageRef),
	}

	b.respawnThreadAgent(context.Background(), "C-unmapped", "1111.2222", "thread-1111-2222", "wake up")

	// Should NOT have created any agent bead.
	for _, bead := range daemon.beads {
		if bead.Type == "agent" {
			t.Errorf("expected no agent bead, got %q", bead.Title)
		}
	}
}

func TestRespawnThreadAgent_InfersProjectFromChannel(t *testing.T) {
	daemon := newMockDaemon()
	daemon.seedProjectWithChannel("gasboat", "C-gasboat")

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	b := &Bot{
		daemon:          daemon,
		state:           state,
		logger:          slog.Default(),
		botUserID:       "U-BOT",
		agentCards:      map[string]MessageRef{},
		threadSpawnMsgs: make(map[string]MessageRef),
	}

	b.respawnThreadAgent(context.Background(), "C-gasboat", "2222.3333", "my-agent", "do the thing")

	// Find the created agent bead.
	var found *beadsapi.BeadDetail
	for _, bead := range daemon.beads {
		if bead.Type == "agent" && bead.Title == "my-agent" {
			found = bead
			break
		}
	}
	if found == nil {
		t.Fatal("expected agent bead to be created")
	}

	if found.Fields["project"] != "gasboat" {
		t.Errorf("project = %q, want gasboat", found.Fields["project"])
	}
	if !hasLabel(found.Labels, "project:gasboat") {
		t.Errorf("expected project:gasboat label, got %v", found.Labels)
	}
}

func TestNotifyAgentState_DonePreservesThreadMapping(t *testing.T) {
	daemon := newMockDaemon()
	agentName := "thread-1111-2222"

	// Agent bead exists but WITHOUT thread fields — so resolveAgentThread
	// returns empty and we skip the postThreadStateReply path (which needs
	// a real Slack API). The mapping preservation is tested via the fact
	// that NotifyAgentState no longer calls RemoveThreadAgentByAgent.
	daemon.beads[agentName] = &beadsapi.BeadDetail{
		ID:    "bd-agent-1",
		Title: agentName,
		Type:  "agent",
		Fields: map[string]string{
			"agent":       agentName,
			"agent_state": "done",
		},
	}

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = state.SetThreadAgent("C-test", "1111.2222", agentName)

	b := &Bot{
		daemon:          daemon,
		state:           state,
		logger:          slog.Default(),
		botUserID:       "U-BOT",
		agentCards:      map[string]MessageRef{},
		agentState:      map[string]string{agentName: "working"},
		agentSeen:       map[string]time.Time{},
		agentPodName:    map[string]string{},
		agentImageTag:   map[string]string{},
		agentRole:       map[string]string{},
		threadSpawnMsgs: make(map[string]MessageRef),
	}

	// Simulate agent transitioning to "done".
	b.NotifyAgentState(context.Background(), BeadEvent{
		Assignee: agentName,
		Fields: map[string]string{
			"agent_state": "done",
			"agent":       agentName,
		},
	})

	// The thread→agent mapping should be PRESERVED (not cleared).
	agent, ok := state.GetThreadAgent("C-test", "1111.2222")
	if !ok {
		t.Fatal("expected thread→agent mapping to be preserved after agent done")
	}
	if agent != agentName {
		t.Errorf("mapping agent = %q, want %q", agent, agentName)
	}
}

func TestNotifyAgentState_FailedPreservesThreadMapping(t *testing.T) {
	daemon := newMockDaemon()
	agentName := "thread-2222-3333"

	daemon.beads[agentName] = &beadsapi.BeadDetail{
		ID:    "bd-agent-2",
		Title: agentName,
		Type:  "agent",
		Fields: map[string]string{
			"agent":       agentName,
			"agent_state": "failed",
		},
	}

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = state.SetThreadAgent("C-test", "2222.3333", agentName)

	b := &Bot{
		daemon:          daemon,
		state:           state,
		logger:          slog.Default(),
		botUserID:       "U-BOT",
		agentCards:      map[string]MessageRef{},
		agentState:      map[string]string{agentName: "working"},
		agentSeen:       map[string]time.Time{},
		agentPodName:    map[string]string{},
		agentImageTag:   map[string]string{},
		agentRole:       map[string]string{},
		threadSpawnMsgs: make(map[string]MessageRef),
	}

	b.NotifyAgentState(context.Background(), BeadEvent{
		Assignee: agentName,
		Fields: map[string]string{
			"agent_state": "failed",
			"agent":       agentName,
		},
	})

	// Mapping should still be there after failed state too.
	if _, ok := state.GetThreadAgent("C-test", "2222.3333"); !ok {
		t.Fatal("expected thread→agent mapping to be preserved after agent failed")
	}
}

func TestHandleAppMention_InactiveThreadAgent_Respawns(t *testing.T) {
	daemon := newMockDaemon()
	// Agent NOT in daemon → FindAgentBead will fail.
	// Add a project bead so the channel maps to a project.
	daemon.beads["proj-test"] = &beadsapi.BeadDetail{
		ID: "proj-test", Title: "testproject", Type: "project",
		Fields: map[string]string{"slack_channel": "C-test"},
	}

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = state.SetThreadAgent("C-test", "1111.2222", "thread-1111-2222")

	b := &Bot{
		daemon:          daemon,
		state:           state,
		logger:          slog.Default(),
		botUserID:       "U-BOT",
		agentCards:      map[string]MessageRef{},
		threadSpawnMsgs: make(map[string]MessageRef),
	}

	// Simulate an @mention in the thread with a dead agent.
	ev := &slackevents.AppMentionEvent{
		User:            "U-HUMAN",
		Text:            "<@U-BOT> check on this",
		Channel:         "C-test",
		TimeStamp:       "1111.3333",
		ThreadTimeStamp: "1111.2222",
	}

	b.handleAppMention(context.Background(), ev)

	// Should have respawned the SAME agent (not created a new thread-XXXX).
	var foundRespawn bool
	for _, bead := range daemon.beads {
		if bead.Type == "agent" && bead.Title == "thread-1111-2222" {
			foundRespawn = true
			if bead.Fields["spawn_source"] != "slack-thread-resume" {
				t.Errorf("expected spawn_source=slack-thread-resume, got %q", bead.Fields["spawn_source"])
			}
		}
		// Should NOT have spawned a different agent name.
		if bead.Type == "agent" && bead.Title != "thread-1111-2222" {
			t.Errorf("should not spawn a new agent, found %q", bead.Title)
		}
	}
	if !foundRespawn {
		t.Error("expected respawn of same agent thread-1111-2222")
	}

	// Mapping should be preserved.
	if _, ok := state.GetThreadAgent("C-test", "1111.2222"); !ok {
		t.Error("expected thread→agent mapping to be preserved after respawn")
	}
}

func TestHandleThreadSpawn_ConcurrentMentionsSpawnOnce(t *testing.T) {
	daemon := newMockDaemon()
	// Add a project bead so the channel maps to a project.
	daemon.beads["proj-race"] = &beadsapi.BeadDetail{
		ID: "proj-race", Title: "raceproject", Type: "project",
		Fields: map[string]string{"slack_channel": "C-race-test"},
	}
	slackSrv := newFakeSlackServer(t)
	defer slackSrv.Close()

	bot := newTestBot(daemon, slackSrv)

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	bot.state = state
	bot.botUserID = "U-BOT"
	bot.lastThreadNudge = make(map[string]time.Time)

	channel := "C-race-test"
	threadTS := "9999.1111"

	// Fire 5 concurrent mentions to the same thread.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bot.handleThreadSpawn(context.Background(), &slackevents.AppMentionEvent{
				User:            "U-USER",
				Text:            "<@U-BOT> help me",
				TimeStamp:       "2222.3333",
				ThreadTimeStamp: threadTS,
				Channel:         channel,
			}, "help me")
		}()
	}
	wg.Wait()

	// Count how many agent beads were created for this thread.
	daemon.mu.Lock()
	var agentCount int
	for _, b := range daemon.beads {
		if b.Type == "agent" {
			agentCount++
		}
	}
	daemon.mu.Unlock()

	if agentCount != 1 {
		t.Errorf("expected exactly 1 agent bead from concurrent spawns, got %d", agentCount)
	}
}

func TestReconcileThreadAgents(t *testing.T) {
	daemon := newMockDaemon()

	dir := t.TempDir()
	state, err := NewStateManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	b := &Bot{
		daemon:     daemon,
		state:      state,
		logger:     slog.Default(),
		agentCards: make(map[string]MessageRef),
	}

	t.Run("restores thread mappings from agent beads", func(t *testing.T) {
		// Simulate an agent bead with thread fields (spawned by a previous bridge).
		daemon.mu.Lock()
		daemon.beads["bd-thread-agent"] = &beadsapi.BeadDetail{
			ID:    "bd-thread-agent",
			Title: "my-thread-agent",
			Type:  "agent",
			Fields: map[string]string{
				"agent":                "my-thread-agent",
				"role":                 "thread",
				"project":              "gasboat",
				"slack_thread_channel": "C-test",
				"slack_thread_ts":      "1111.2222",
			},
		}
		daemon.mu.Unlock()

		b.ReconcileThreadAgents(context.Background())

		agent, ok := state.GetThreadAgent("C-test", "1111.2222")
		if !ok {
			t.Fatal("expected thread→agent mapping to be restored")
		}
		if agent != "my-thread-agent" {
			t.Errorf("got %q, want %q", agent, "my-thread-agent")
		}
	})

	t.Run("does not overwrite existing mappings", func(t *testing.T) {
		// Pre-set a mapping for the same thread.
		_ = state.SetThreadAgent("C-test", "1111.2222", "existing-agent")

		b.ReconcileThreadAgents(context.Background())

		agent, _ := state.GetThreadAgent("C-test", "1111.2222")
		if agent != "existing-agent" {
			t.Errorf("existing mapping was overwritten: got %q, want %q", agent, "existing-agent")
		}
	})

	t.Run("skips agents without thread fields", func(t *testing.T) {
		// Clear all previous state and beads.
		state.ClearAllThreadAgents()
		daemon.mu.Lock()
		daemon.beads = map[string]*beadsapi.BeadDetail{
			"bd-card-agent": {
				ID:    "bd-card-agent",
				Title: "card-only-agent",
				Type:  "agent",
				Fields: map[string]string{
					"agent":   "card-only-agent",
					"role":    "crew",
					"project": "gasboat",
				},
			},
		}
		daemon.mu.Unlock()

		b.ReconcileThreadAgents(context.Background())

		if len(state.AllThreadAgents()) != 0 {
			t.Errorf("expected no thread agents, got %d", len(state.AllThreadAgents()))
		}
	})
}

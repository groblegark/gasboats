package config

import (
	"testing"
	"time"
)

// syncEnvVars lists all sync-related env vars that must be cleared between tests.
var syncEnvVars = []string{
	"BEADS_SYNC_INTERVAL", "BEADS_SYNC_S3_BUCKET", "BEADS_SYNC_S3_ENDPOINT",
	"BEADS_SYNC_S3_REGION", "BEADS_SYNC_S3_KEY", "BEADS_SYNC_GIT_REPO",
	"BEADS_SYNC_GIT_FILE", "BEADS_SYNC_GIT_BRANCH",
}

func clearAllEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"BEADS_DATABASE_URL", "BEADS_GRPC_ADDR", "BEADS_HTTP_ADDR", "BEADS_NATS_URL"} {
		t.Setenv(key, "")
	}
	for _, key := range syncEnvVars {
		t.Setenv(key, "")
	}
}

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name         string
		env          map[string]string
		wantErr      bool
		wantGRPCAddr string
		wantHTTPAddr string
		wantNATSURL  string
	}{
		{
			name:    "MissingDatabaseURL",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:         "DefaultAddresses",
			env:          map[string]string{"BEADS_DATABASE_URL": "postgres://localhost/beads"},
			wantGRPCAddr: ":9090",
			wantHTTPAddr: ":8080",
		},
		{
			name: "CustomAddresses",
			env: map[string]string{
				"BEADS_DATABASE_URL": "postgres://db:5432/beads",
				"BEADS_GRPC_ADDR":   ":5050",
				"BEADS_HTTP_ADDR":   ":3000",
				"BEADS_NATS_URL":    "nats://localhost:4222",
			},
			wantGRPCAddr: ":5050",
			wantHTTPAddr: ":3000",
			wantNATSURL:  "nats://localhost:4222",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAllEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.DatabaseURL != tc.env["BEADS_DATABASE_URL"] {
				t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, tc.env["BEADS_DATABASE_URL"])
			}
			if cfg.GRPCAddr != tc.wantGRPCAddr {
				t.Errorf("GRPCAddr = %q, want %q", cfg.GRPCAddr, tc.wantGRPCAddr)
			}
			if cfg.HTTPAddr != tc.wantHTTPAddr {
				t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, tc.wantHTTPAddr)
			}
			if cfg.NATSURL != tc.wantNATSURL {
				t.Errorf("NATSURL = %q, want %q", cfg.NATSURL, tc.wantNATSURL)
			}
		})
	}
}

func TestLoadSyncDefaults(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("BEADS_DATABASE_URL", "postgres://localhost/beads")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SyncInterval != 3*time.Minute {
		t.Errorf("SyncInterval = %v, want 3m", cfg.SyncInterval)
	}
	if cfg.SyncS3Region != "us-east-1" {
		t.Errorf("SyncS3Region = %q, want %q", cfg.SyncS3Region, "us-east-1")
	}
	if cfg.SyncS3Key != "beads/backup.jsonl" {
		t.Errorf("SyncS3Key = %q, want %q", cfg.SyncS3Key, "beads/backup.jsonl")
	}
	if cfg.SyncGitFile != "beads.jsonl" {
		t.Errorf("SyncGitFile = %q, want %q", cfg.SyncGitFile, "beads.jsonl")
	}
	if cfg.SyncGitBranch != "main" {
		t.Errorf("SyncGitBranch = %q, want %q", cfg.SyncGitBranch, "main")
	}
}

func TestLoadSyncCustom(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("BEADS_DATABASE_URL", "postgres://localhost/beads")
	t.Setenv("BEADS_SYNC_INTERVAL", "10m")
	t.Setenv("BEADS_SYNC_S3_BUCKET", "my-bucket")
	t.Setenv("BEADS_SYNC_S3_ENDPOINT", "http://minio:9000")
	t.Setenv("BEADS_SYNC_S3_REGION", "eu-west-1")
	t.Setenv("BEADS_SYNC_S3_KEY", "custom/key.jsonl")
	t.Setenv("BEADS_SYNC_GIT_REPO", "/tmp/repo")
	t.Setenv("BEADS_SYNC_GIT_FILE", "custom.jsonl")
	t.Setenv("BEADS_SYNC_GIT_BRANCH", "backup")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SyncInterval != 10*time.Minute {
		t.Errorf("SyncInterval = %v, want 10m", cfg.SyncInterval)
	}
	if cfg.SyncS3Bucket != "my-bucket" {
		t.Errorf("SyncS3Bucket = %q", cfg.SyncS3Bucket)
	}
	if cfg.SyncS3Endpoint != "http://minio:9000" {
		t.Errorf("SyncS3Endpoint = %q", cfg.SyncS3Endpoint)
	}
	if cfg.SyncS3Region != "eu-west-1" {
		t.Errorf("SyncS3Region = %q", cfg.SyncS3Region)
	}
	if cfg.SyncS3Key != "custom/key.jsonl" {
		t.Errorf("SyncS3Key = %q", cfg.SyncS3Key)
	}
	if cfg.SyncGitRepo != "/tmp/repo" {
		t.Errorf("SyncGitRepo = %q", cfg.SyncGitRepo)
	}
	if cfg.SyncGitFile != "custom.jsonl" {
		t.Errorf("SyncGitFile = %q", cfg.SyncGitFile)
	}
	if cfg.SyncGitBranch != "backup" {
		t.Errorf("SyncGitBranch = %q", cfg.SyncGitBranch)
	}
}

func TestLoadSyncInvalidInterval(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("BEADS_DATABASE_URL", "postgres://localhost/beads")
	t.Setenv("BEADS_SYNC_INTERVAL", "not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid BEADS_SYNC_INTERVAL")
	}
}

func TestLoadSyncDisabled(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("BEADS_DATABASE_URL", "postgres://localhost/beads")
	t.Setenv("BEADS_SYNC_INTERVAL", "0s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SyncInterval != 0 {
		t.Errorf("SyncInterval = %v, want 0 (disabled)", cfg.SyncInterval)
	}
}

func TestEnvOrDefault(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      string
		envVal   string
		fallback string
		want     string
	}{
		{"EmptyUsesDefault", "TEST_ENVDEFAULT_EMPTY", "", "default-val", "default-val"},
		{"SetUsesEnv", "TEST_ENVDEFAULT_SET", "custom", "default-val", "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.envVal)
			got := envOrDefault(tc.key, tc.fallback)
			if got != tc.want {
				t.Errorf("envOrDefault(%q, %q) = %q, want %q", tc.key, tc.fallback, got, tc.want)
			}
		})
	}
}

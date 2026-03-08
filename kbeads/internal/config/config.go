package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL string // BEADS_DATABASE_URL (required)
	GRPCAddr    string // BEADS_GRPC_ADDR (default ":9090")
	HTTPAddr    string // BEADS_HTTP_ADDR (default ":8080")
	NATSURL     string // BEADS_NATS_URL (optional, empty = no events)
	AuthToken   string // BEADS_AUTH_TOKEN (optional, empty = auth disabled)

	// Sync settings
	SyncInterval   time.Duration // BEADS_SYNC_INTERVAL (default 3m; 0 = disabled)
	SyncS3Bucket   string        // BEADS_SYNC_S3_BUCKET (enables S3 when set)
	SyncS3Endpoint string        // BEADS_SYNC_S3_ENDPOINT (custom endpoint for MinIO)
	SyncS3Region   string        // BEADS_SYNC_S3_REGION (default "us-east-1")
	SyncS3Key      string        // BEADS_SYNC_S3_KEY (default "beads/backup.jsonl")
	SyncGitRepo    string        // BEADS_SYNC_GIT_REPO (enables git when set; path to clone)
	SyncGitFile    string        // BEADS_SYNC_GIT_FILE (default "beads.jsonl")
	SyncGitBranch  string        // BEADS_SYNC_GIT_BRANCH (default "main")
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:    os.Getenv("BEADS_DATABASE_URL"),
		GRPCAddr:       envOrDefault("BEADS_GRPC_ADDR", ":9090"),
		HTTPAddr:       envOrDefault("BEADS_HTTP_ADDR", ":8080"),
		NATSURL:        os.Getenv("BEADS_NATS_URL"),
		AuthToken:      os.Getenv("BEADS_AUTH_TOKEN"),
		SyncS3Bucket:   os.Getenv("BEADS_SYNC_S3_BUCKET"),
		SyncS3Endpoint: os.Getenv("BEADS_SYNC_S3_ENDPOINT"),
		SyncS3Region:   envOrDefault("BEADS_SYNC_S3_REGION", "us-east-1"),
		SyncS3Key:      envOrDefault("BEADS_SYNC_S3_KEY", "beads/backup.jsonl"),
		SyncGitRepo:    os.Getenv("BEADS_SYNC_GIT_REPO"),
		SyncGitFile:    envOrDefault("BEADS_SYNC_GIT_FILE", "beads.jsonl"),
		SyncGitBranch:  envOrDefault("BEADS_SYNC_GIT_BRANCH", "main"),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("BEADS_DATABASE_URL is required")
	}

	intervalStr := envOrDefault("BEADS_SYNC_INTERVAL", "3m")
	if intervalStr != "" {
		d, err := time.ParseDuration(intervalStr)
		if err != nil {
			return nil, fmt.Errorf("BEADS_SYNC_INTERVAL: %w", err)
		}
		c.SyncInterval = d
	}

	return c, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

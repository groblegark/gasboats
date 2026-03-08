package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/groblegark/kbeads/internal/config"
	"github.com/groblegark/kbeads/internal/eventbus"
	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/hooks"
	"github.com/groblegark/kbeads/internal/presence"
	"github.com/groblegark/kbeads/internal/server"
	"github.com/groblegark/kbeads/internal/store/postgres"
	beadsync "github.com/groblegark/kbeads/internal/sync"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:     "serve",
	Short:   "Start the Beads gRPC server",
	GroupID: "system",
	// Override PersistentPreRunE so we don't create a gRPC client connection.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

		// Load configuration.
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// Connect to Postgres.
		store, err := postgres.New(cfg.DatabaseURL)
		if err != nil {
			return err
		}

		// Create event publisher.
		var publisher events.Publisher
		if cfg.NATSURL != "" {
			pub, err := events.NewNATSPublisher(cfg.NATSURL)
			if err != nil {
				store.Close()
				return err
			}
			publisher = pub
			logger.Info("events enabled", "nats_url", cfg.NATSURL)
		} else {
			publisher = &events.NoopPublisher{}
			logger.Info("events disabled (BEADS_NATS_URL not set)")
		}

		// Create event bus with JetStream if NATS is available.
		bus := eventbus.New()
		if pub, ok := publisher.(*events.NATSPublisher); ok {
			js, err := pub.Conn().JetStream()
			if err != nil {
				logger.Error("failed to create JetStream context", "err", err)
			} else {
				if err := eventbus.EnsureStreams(js); err != nil {
					logger.Error("failed to ensure JetStream streams", "err", err)
				} else {
					bus.SetJetStream(js)
					logger.Info("eventbus JetStream enabled")
				}
			}
		}

		// Load persisted external handlers from config table.
		allConfigs, err := store.ListAllConfigs(context.Background())
		if err != nil {
			logger.Warn("failed to list configs for handler loading", "err", err)
		} else {
			configMap := make(map[string]string, len(allConfigs))
			for _, c := range allConfigs {
				configMap[c.Key] = string(c.Value)
			}
			if n := bus.LoadPersistedHandlers(configMap); n > 0 {
				logger.Info("loaded persisted bus handlers", "count", n)
			}
		}

		// Create server components.
		beadsServer := server.NewBeadsServer(store, publisher)
		beadsServer.SetBus(bus)
		grpcServer := server.NewGRPCServer(beadsServer, cfg.AuthToken)

		// Start gRPC listener.
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			publisher.Close()
			store.Close()
			return err
		}

		go func() {
			logger.Info("gRPC server listening", "addr", cfg.GRPCAddr)
			if err := grpcServer.Serve(lis); err != nil {
				logger.Error("gRPC server error", "err", err)
			}
		}()

		// Start HTTP server.
		httpHandler := beadsServer.NewHTTPHandler(cfg.AuthToken)
		httpServer := &http.Server{
			Addr:    cfg.HTTPAddr,
			Handler: httpHandler,
		}

		go func() {
			logger.Info("HTTP server listening", "addr", cfg.HTTPAddr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP server error", "err", err)
			}
		}()

		// Start sync scheduler if any destinations are configured.
		var scheduler *beadsync.Scheduler
		if cfg.SyncInterval > 0 {
			var dests []beadsync.Destination

			if cfg.SyncS3Bucket != "" {
				s3Dest, err := beadsync.NewS3Destination(
					context.Background(),
					cfg.SyncS3Bucket,
					cfg.SyncS3Key,
					cfg.SyncS3Region,
					cfg.SyncS3Endpoint,
				)
				if err != nil {
					logger.Error("failed to create S3 sync destination", "err", err)
				} else {
					dests = append(dests, s3Dest)
					logger.Info("sync S3 destination enabled", "bucket", cfg.SyncS3Bucket, "key", cfg.SyncS3Key)
				}
			}

			if cfg.SyncGitRepo != "" {
				gitDest := beadsync.NewGitDestination(cfg.SyncGitRepo, cfg.SyncGitFile, cfg.SyncGitBranch)
				dests = append(dests, gitDest)
				logger.Info("sync git destination enabled", "repo", cfg.SyncGitRepo, "file", cfg.SyncGitFile)
			}

			if len(dests) > 0 {
				scheduler = beadsync.NewScheduler(store, dests, cfg.SyncInterval, logger)
				scheduler.Start()
				logger.Info("sync scheduler started", "interval", cfg.SyncInterval)
			}
		}

		// Start advice hooks subscriber if NATS is available.
		var hooksCancel context.CancelFunc
		if cfg.NATSURL != "" {
			hooksSub, err := events.NewNATSSubscriber(cfg.NATSURL)
			if err != nil {
				logger.Error("failed to create hooks subscriber", "err", err)
			} else {
				hooksHandler := hooks.NewHandler(store, logger)
				var hooksCtx context.Context
				hooksCtx, hooksCancel = context.WithCancel(context.Background())
				go func() {
					if err := hooksHandler.StartSubscriber(hooksCtx, hooksSub); err != nil {
						logger.Error("hooks subscriber error", "err", err)
					}
					hooksSub.Close()
				}()
				logger.Info("hooks subscriber started")
			}
		}

		// Start presence tracker reaper for agent roster.
		beadsServer.Presence.StartReaper(&presence.ReaperConfig{
			DeadThreshold: 15 * time.Minute,
			EvictAfter:    30 * time.Minute,
			SweepInterval: 60 * time.Second,
		})

		// Log startup info.
		logger.Info("beads server started",
			"grpc_addr", cfg.GRPCAddr,
			"http_addr", cfg.HTTPAddr,
			"auth_enabled", cfg.AuthToken != "",
		)

		// Wait for SIGINT or SIGTERM.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)

		// Graceful shutdown.
		beadsServer.Presence.Stop()
		logger.Info("presence tracker stopped")

		if hooksCancel != nil {
			hooksCancel()
			logger.Info("hooks subscriber stopped")
		}

		if scheduler != nil {
			scheduler.Stop()
			logger.Info("sync scheduler stopped")
		}

		grpcServer.GracefulStop()
		logger.Info("gRPC server stopped")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown error", "err", err)
		}
		logger.Info("HTTP server stopped")

		if err := publisher.Close(); err != nil {
			logger.Error("error closing publisher", "err", err)
		}
		if err := store.Close(); err != nil {
			logger.Error("error closing store", "err", err)
		}

		logger.Info("shutdown complete")
		return nil
	},
}

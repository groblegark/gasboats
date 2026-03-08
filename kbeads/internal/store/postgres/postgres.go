// Package postgres implements the store.Store interface backed by PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/groblegark/kbeads/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PostgresStore implements store.Store backed by a PostgreSQL database.
type PostgresStore struct {
	db *sql.DB
}

// Compile-time check that PostgresStore implements store.Store.
var _ store.Store = (*PostgresStore)(nil)

// New opens a connection to the PostgreSQL database at the given URL,
// configures the connection pool, and runs any pending migrations.
func New(databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

func runMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

// Close closes the underlying database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) CreateBead(ctx context.Context, bead *model.Bead) error {
	return queryCreateBead(ctx, s.db, bead)
}

func (s *PostgresStore) GetBead(ctx context.Context, id string) (*model.Bead, error) {
	return queryGetBead(ctx, s.db, id)
}

func (s *PostgresStore) ListBeads(ctx context.Context, filter model.BeadFilter) ([]*model.Bead, int, error) {
	return queryListBeads(ctx, s.db, filter)
}

func (s *PostgresStore) UpdateBead(ctx context.Context, bead *model.Bead) error {
	return queryUpdateBead(ctx, s.db, bead)
}

func (s *PostgresStore) CloseBead(ctx context.Context, id string, closedBy string) (*model.Bead, error) {
	return queryCloseBead(ctx, s.db, id, closedBy)
}

func (s *PostgresStore) DeleteBead(ctx context.Context, id string) error {
	return queryDeleteBead(ctx, s.db, id)
}

func (s *PostgresStore) GetStats(ctx context.Context) (*model.GraphStats, error) {
	return queryGetStats(ctx, s.db)
}

func (s *PostgresStore) GetDependenciesForBeads(ctx context.Context, beadIDs []string) (map[string][]*model.Dependency, error) {
	return queryGetDependenciesForBeads(ctx, s.db, beadIDs)
}

func (s *PostgresStore) GetReverseDependenciesForBeads(ctx context.Context, beadIDs []string) (map[string][]*model.Dependency, error) {
	return queryGetReverseDependenciesForBeads(ctx, s.db, beadIDs)
}

func (s *PostgresStore) GetDependencyCounts(ctx context.Context, beadIDs []string) (map[string]*model.DependencyCounts, error) {
	return queryGetDependencyCounts(ctx, s.db, beadIDs)
}

func (s *PostgresStore) GetLabelsForBeads(ctx context.Context, beadIDs []string) (map[string][]string, error) {
	return queryGetLabelsForBeads(ctx, s.db, beadIDs)
}

func (s *PostgresStore) GetBlockedByForBeads(ctx context.Context, beadIDs []string) (map[string][]string, error) {
	return queryGetBlockedByForBeads(ctx, s.db, beadIDs)
}

func (s *PostgresStore) GetBeadsByIDs(ctx context.Context, ids []string) ([]*model.Bead, error) {
	return queryGetBeadsByIDs(ctx, s.db, ids)
}

func (s *PostgresStore) AddDependency(ctx context.Context, dep *model.Dependency) error {
	return queryAddDependency(ctx, s.db, dep)
}

func (s *PostgresStore) RemoveDependency(ctx context.Context, beadID, dependsOnID string, depType model.DependencyType) error {
	return queryRemoveDependency(ctx, s.db, beadID, dependsOnID, depType)
}

func (s *PostgresStore) GetDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	return queryGetDependencies(ctx, s.db, beadID)
}

func (s *PostgresStore) GetReverseDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	return queryGetReverseDependencies(ctx, s.db, beadID)
}

func (s *PostgresStore) AddLabel(ctx context.Context, beadID string, label string) error {
	return queryAddLabel(ctx, s.db, beadID, label)
}

func (s *PostgresStore) RemoveLabel(ctx context.Context, beadID string, label string) error {
	return queryRemoveLabel(ctx, s.db, beadID, label)
}

func (s *PostgresStore) GetLabels(ctx context.Context, beadID string) ([]string, error) {
	return queryGetLabels(ctx, s.db, beadID)
}

func (s *PostgresStore) AddComment(ctx context.Context, comment *model.Comment) error {
	return queryAddComment(ctx, s.db, comment)
}

func (s *PostgresStore) GetComments(ctx context.Context, beadID string) ([]*model.Comment, error) {
	return queryGetComments(ctx, s.db, beadID)
}

func (s *PostgresStore) RecordEvent(ctx context.Context, event *model.Event) error {
	return queryRecordEvent(ctx, s.db, event)
}

func (s *PostgresStore) GetEvents(ctx context.Context, beadID string) ([]*model.Event, error) {
	return queryGetEvents(ctx, s.db, beadID)
}

func (s *PostgresStore) SetConfig(ctx context.Context, config *model.Config) error {
	return querySetConfig(ctx, s.db, config)
}

func (s *PostgresStore) GetConfig(ctx context.Context, key string) (*model.Config, error) {
	return queryGetConfig(ctx, s.db, key)
}

func (s *PostgresStore) ListConfigs(ctx context.Context, namespace string) ([]*model.Config, error) {
	return queryListConfigs(ctx, s.db, namespace)
}

func (s *PostgresStore) ListAllConfigs(ctx context.Context) ([]*model.Config, error) {
	return queryListAllConfigs(ctx, s.db)
}

func (s *PostgresStore) DeleteConfig(ctx context.Context, key string) error {
	return queryDeleteConfig(ctx, s.db, key)
}

// RunInTransaction begins a database transaction, creates a txStore that
// delegates to it, calls fn, and commits on success or rolls back on error.
func (s *PostgresStore) RunInTransaction(ctx context.Context, fn func(tx store.Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txS := &txStore{tx: tx}
	if err := fn(txS); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// txStore implements store.Store using a *sql.Tx.
type txStore struct {
	tx *sql.Tx
}

// Compile-time check that txStore implements store.Store.
var _ store.Store = (*txStore)(nil)

func (s *txStore) CreateBead(ctx context.Context, bead *model.Bead) error {
	return queryCreateBead(ctx, s.tx, bead)
}

func (s *txStore) GetBead(ctx context.Context, id string) (*model.Bead, error) {
	return queryGetBead(ctx, s.tx, id)
}

func (s *txStore) ListBeads(ctx context.Context, filter model.BeadFilter) ([]*model.Bead, int, error) {
	return queryListBeads(ctx, s.tx, filter)
}

func (s *txStore) UpdateBead(ctx context.Context, bead *model.Bead) error {
	return queryUpdateBead(ctx, s.tx, bead)
}

func (s *txStore) CloseBead(ctx context.Context, id string, closedBy string) (*model.Bead, error) {
	return queryCloseBead(ctx, s.tx, id, closedBy)
}

func (s *txStore) DeleteBead(ctx context.Context, id string) error {
	return queryDeleteBead(ctx, s.tx, id)
}

func (s *txStore) GetStats(ctx context.Context) (*model.GraphStats, error) {
	return queryGetStats(ctx, s.tx)
}

func (s *txStore) GetDependenciesForBeads(ctx context.Context, beadIDs []string) (map[string][]*model.Dependency, error) {
	return queryGetDependenciesForBeads(ctx, s.tx, beadIDs)
}

func (s *txStore) GetReverseDependenciesForBeads(ctx context.Context, beadIDs []string) (map[string][]*model.Dependency, error) {
	return queryGetReverseDependenciesForBeads(ctx, s.tx, beadIDs)
}

func (s *txStore) GetDependencyCounts(ctx context.Context, beadIDs []string) (map[string]*model.DependencyCounts, error) {
	return queryGetDependencyCounts(ctx, s.tx, beadIDs)
}

func (s *txStore) GetLabelsForBeads(ctx context.Context, beadIDs []string) (map[string][]string, error) {
	return queryGetLabelsForBeads(ctx, s.tx, beadIDs)
}

func (s *txStore) GetBlockedByForBeads(ctx context.Context, beadIDs []string) (map[string][]string, error) {
	return queryGetBlockedByForBeads(ctx, s.tx, beadIDs)
}

func (s *txStore) GetBeadsByIDs(ctx context.Context, ids []string) ([]*model.Bead, error) {
	return queryGetBeadsByIDs(ctx, s.tx, ids)
}

func (s *txStore) AddDependency(ctx context.Context, dep *model.Dependency) error {
	return queryAddDependency(ctx, s.tx, dep)
}

func (s *txStore) RemoveDependency(ctx context.Context, beadID, dependsOnID string, depType model.DependencyType) error {
	return queryRemoveDependency(ctx, s.tx, beadID, dependsOnID, depType)
}

func (s *txStore) GetDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	return queryGetDependencies(ctx, s.tx, beadID)
}

func (s *txStore) GetReverseDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	return queryGetReverseDependencies(ctx, s.tx, beadID)
}

func (s *txStore) AddLabel(ctx context.Context, beadID string, label string) error {
	return queryAddLabel(ctx, s.tx, beadID, label)
}

func (s *txStore) RemoveLabel(ctx context.Context, beadID string, label string) error {
	return queryRemoveLabel(ctx, s.tx, beadID, label)
}

func (s *txStore) GetLabels(ctx context.Context, beadID string) ([]string, error) {
	return queryGetLabels(ctx, s.tx, beadID)
}

func (s *txStore) AddComment(ctx context.Context, comment *model.Comment) error {
	return queryAddComment(ctx, s.tx, comment)
}

func (s *txStore) GetComments(ctx context.Context, beadID string) ([]*model.Comment, error) {
	return queryGetComments(ctx, s.tx, beadID)
}

func (s *txStore) RecordEvent(ctx context.Context, event *model.Event) error {
	return queryRecordEvent(ctx, s.tx, event)
}

func (s *txStore) GetEvents(ctx context.Context, beadID string) ([]*model.Event, error) {
	return queryGetEvents(ctx, s.tx, beadID)
}

func (s *txStore) SetConfig(ctx context.Context, config *model.Config) error {
	return querySetConfig(ctx, s.tx, config)
}

func (s *txStore) GetConfig(ctx context.Context, key string) (*model.Config, error) {
	return queryGetConfig(ctx, s.tx, key)
}

func (s *txStore) ListConfigs(ctx context.Context, namespace string) ([]*model.Config, error) {
	return queryListConfigs(ctx, s.tx, namespace)
}

func (s *txStore) ListAllConfigs(ctx context.Context) ([]*model.Config, error) {
	return queryListAllConfigs(ctx, s.tx)
}

func (s *txStore) DeleteConfig(ctx context.Context, key string) error {
	return queryDeleteConfig(ctx, s.tx, key)
}

// RunInTransaction on a txStore reuses the existing transaction (no nesting).
func (s *txStore) RunInTransaction(ctx context.Context, fn func(tx store.Store) error) error {
	return fn(s)
}

// Close is a no-op for a transaction store; the parent store owns the connection.
func (s *txStore) Close() error {
	return nil
}

package history

import (
	"context"
	"database/sql"
	"time"
)

// AgentSearch is the durable audit trail for an agent recommendation request.
type AgentSearch struct {
	RequestID        string
	Query            string
	StartLocation    string
	Destination      string
	Preference       string
	MaxDetourMinutes int
	ResultCount      int
	Summary          string
	CreatedAt        time.Time
}

// Repository stores agent search history. Implementations must be safe for
// request handlers to call; callers treat persistence as best effort.
type Repository interface {
	SaveAgentSearch(ctx context.Context, search AgentSearch) error
}

// NoopRepository keeps persistence optional for local development and demos.
type NoopRepository struct{}

func (NoopRepository) SaveAgentSearch(context.Context, AgentSearch) error {
	return nil
}

// PostgresRepository stores agent search history in PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) SaveAgentSearch(ctx context.Context, search AgentSearch) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_searches (
			request_id,
			query,
			start_location,
			destination,
			preference,
			max_detour_minutes,
			result_count,
			summary
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		search.RequestID,
		search.Query,
		search.StartLocation,
		search.Destination,
		search.Preference,
		search.MaxDetourMinutes,
		search.ResultCount,
		search.Summary,
	)
	return err
}

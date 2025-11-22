package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/leafsii/leafsii-backend/internal/onchain"
	"go.uber.org/zap"
)

type Repository struct {
	db     *sql.DB
	logger *zap.SugaredLogger
}

func NewRepository(db *sql.DB, logger *zap.SugaredLogger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

// Event storage
func (r *Repository) StoreEvent(ctx context.Context, event onchain.Event) error {
	fieldsJSON, err := json.Marshal(event.Fields)
	if err != nil {
		return fmt.Errorf("failed to marshal event fields: %w", err)
	}

	query := `
		INSERT INTO events (checkpoint, sequence_number, ts, type, tx_digest, sender, fields)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (checkpoint, sequence_number) DO NOTHING
	`

	_, err = r.db.ExecContext(ctx, query,
		event.Checkpoint,
		event.SequenceNumber,
		event.Timestamp,
		event.Type,
		event.TxDigest,
		event.Sender,
		fieldsJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	return nil
}

func (r *Repository) StoreBatchEvents(ctx context.Context, events []onchain.Event) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (checkpoint, sequence_number, ts, type, tx_digest, sender, fields)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (checkpoint, sequence_number) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		fieldsJSON, err := json.Marshal(event.Fields)
		if err != nil {
			return fmt.Errorf("failed to marshal event fields: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			event.Checkpoint,
			event.SequenceNumber,
			event.Timestamp,
			event.Type,
			event.TxDigest,
			event.Sender,
			fieldsJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Debugw("Stored batch of events", "count", len(events))
	return nil
}

// Protocol state snapshots
func (r *Repository) StoreProtocolSnapshot(ctx context.Context, state onchain.ProtocolState) error {
	query := `
		INSERT INTO protocol_snapshots (at, cr, cr_target, reserves_r, supply_f, supply_x, peg_deviation, mode, oracle_age_sec)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (at) DO UPDATE SET
			cr = EXCLUDED.cr,
			cr_target = EXCLUDED.cr_target,
			reserves_r = EXCLUDED.reserves_r,
			supply_f = EXCLUDED.supply_f,
			supply_x = EXCLUDED.supply_x,
			peg_deviation = EXCLUDED.peg_deviation,
			mode = EXCLUDED.mode,
			oracle_age_sec = EXCLUDED.oracle_age_sec
	`

	_, err := r.db.ExecContext(ctx, query,
		state.AsOf,
		state.CR,
		state.CRTarget,
		state.ReservesR,
		state.SupplyF,
		state.SupplyX,
		state.PegDeviation,
		state.Mode,
		state.OracleAgeSec,
	)

	if err != nil {
		return fmt.Errorf("failed to store protocol snapshot: %w", err)
	}

	return nil
}

// SP index storage
func (r *Repository) StoreSPIndex(ctx context.Context, index onchain.SPIndex) error {
	query := `
		INSERT INTO sp_index (at, index_value, tvl_f, total_rewards_r)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (at) DO UPDATE SET
			index_value = EXCLUDED.index_value,
			tvl_f = EXCLUDED.tvl_f,
			total_rewards_r = EXCLUDED.total_rewards_r
	`

	_, err := r.db.ExecContext(ctx, query,
		index.AsOf,
		index.IndexValue,
		index.TVLF,
		index.TotalRewardsR,
	)

	if err != nil {
		return fmt.Errorf("failed to store SP index: %w", err)
	}

	return nil
}

// User positions
func (r *Repository) UpdateUserPosition(ctx context.Context, position onchain.UserPositions) error {
	query := `
		INSERT INTO user_positions (address, balance_f, balance_x, balance_r, stake_f, index_at_join, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (address) DO UPDATE SET
			balance_f = EXCLUDED.balance_f,
			balance_x = EXCLUDED.balance_x,
			balance_r = EXCLUDED.balance_r,
			stake_f = EXCLUDED.stake_f,
			index_at_join = EXCLUDED.index_at_join,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.ExecContext(ctx, query,
		position.Address,
		position.BalanceF,
		position.BalanceX,
		position.BalanceR,
		position.StakeF,
		position.IndexAtJoin,
		position.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update user position: %w", err)
	}

	return nil
}

// Indexer state management
func (r *Repository) GetIndexerState(ctx context.Context, name string) (uint64, uint64, error) {
	var checkpoint, sequenceNumber uint64
	query := `SELECT last_checkpoint, last_sequence_number FROM indexer_state WHERE name = $1`

	err := r.db.QueryRowContext(ctx, query, name).Scan(&checkpoint, &sequenceNumber)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil // Start from beginning if no state found
		}
		return 0, 0, fmt.Errorf("failed to get indexer state: %w", err)
	}

	return checkpoint, sequenceNumber, nil
}

func (r *Repository) UpdateIndexerState(ctx context.Context, name string, checkpoint, sequenceNumber uint64) error {
	query := `
		INSERT INTO indexer_state (name, last_checkpoint, last_sequence_number, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (name) DO UPDATE SET
			last_checkpoint = EXCLUDED.last_checkpoint,
			last_sequence_number = EXCLUDED.last_sequence_number,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.ExecContext(ctx, query, name, checkpoint, sequenceNumber)
	if err != nil {
		return fmt.Errorf("failed to update indexer state: %w", err)
	}

	return nil
}

// Query methods for API
func (r *Repository) GetUserEvents(ctx context.Context, address string, limit int, cursor string) ([]onchain.Event, string, error) {
	// Parse cursor (checkpoint:sequence_number format)
	var fromCheckpoint, fromSequence uint64
	if cursor != "" {
		_, err := fmt.Sscanf(cursor, "%d:%d", &fromCheckpoint, &fromSequence)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor format: %w", err)
		}
	}

	query := `
		SELECT id, checkpoint, sequence_number, ts, type, tx_digest, sender, fields
		FROM events
		WHERE (fields->>'address' = $1 OR sender = $1)
		AND (checkpoint > $2 OR (checkpoint = $2 AND sequence_number > $3))
		ORDER BY checkpoint DESC, sequence_number DESC
		LIMIT $4
	`

	rows, err := r.db.QueryContext(ctx, query, address, fromCheckpoint, fromSequence, limit+1) // +1 to check if there are more
	if err != nil {
		return nil, "", fmt.Errorf("failed to query user events: %w", err)
	}
	defer rows.Close()

	var events []onchain.Event
	var hasMore bool

	for rows.Next() {
		if len(events) >= limit {
			hasMore = true
			break
		}

		var event onchain.Event
		var fieldsJSON []byte

		err := rows.Scan(
			&event.ID,
			&event.Checkpoint,
			&event.SequenceNumber,
			&event.Timestamp,
			&event.Type,
			&event.TxDigest,
			&event.Sender,
			&fieldsJSON,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan event: %w", err)
		}

		if err := json.Unmarshal(fieldsJSON, &event.Fields); err != nil {
			return nil, "", fmt.Errorf("failed to unmarshal event fields: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("row iteration error: %w", err)
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		lastEvent := events[len(events)-1]
		nextCursor = fmt.Sprintf("%d:%d", lastEvent.Checkpoint, lastEvent.SequenceNumber)
	}

	return events, nextCursor, nil
}

// Health check
func (r *Repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

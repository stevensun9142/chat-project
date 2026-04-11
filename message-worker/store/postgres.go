package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Postgres handles room membership lookups.
type Postgres struct {
	db *sql.DB
}

func NewPostgres(dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Close() {
	p.db.Close()
}

// IsRoomMember checks whether a user is a member of a room.
func (p *Postgres) IsRoomMember(ctx context.Context, roomID, userID string) (bool, error) {
	var exists int
	err := p.db.QueryRowContext(ctx,
		"SELECT 1 FROM room_members WHERE room_id = $1 AND user_id = $2",
		roomID, userID,
	).Scan(&exists)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("membership check: %w", err)
	}
	return true, nil
}

// GetRoomMemberIDs returns all user_id values for a given room.
func (p *Postgres) GetRoomMemberIDs(ctx context.Context, roomID string) ([]string, error) {
	rows, err := p.db.QueryContext(ctx,
		"SELECT user_id FROM room_members WHERE room_id = $1",
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("get room members: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan member id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

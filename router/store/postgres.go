package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Postgres provides room membership lookups for the delivery consumer.
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

// RoomMemberIDs returns all user IDs in a room.
func (p *Postgres) RoomMemberIDs(ctx context.Context, roomID string) ([]string, error) {
	rows, err := p.db.QueryContext(ctx,
		"SELECT user_id::text FROM room_members WHERE room_id = $1",
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("query room members: %w", err)
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

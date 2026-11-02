// ABOUTME: Reads new messages from the bbs SQLite database, read-only
// ABOUTME: Tracks position by rowid so multi-line posts and odd timestamps can't confuse it

package main

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

type board struct {
	db *sql.DB
}

func openBoard(path string) (*board, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(3000)", url.PathEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open bbs database %s: %w", path, err)
	}
	return &board{db: db}, nil
}

func (b *board) Close() error { return b.db.Close() }

func (b *board) latestRowID() (int64, error) {
	var id sql.NullInt64
	err := b.db.QueryRow(`SELECT MAX(rowid) FROM messages`).Scan(&id)
	return id.Int64, err
}

func (b *board) since(rowid int64) ([]Message, error) {
	rows, err := b.db.Query(`
		SELECT m.rowid, m.id, m.thread_id, COALESCE(t.subject, ''), m.created_by, m.content,
		       CAST(m.created_at AS TEXT)
		FROM messages m LEFT JOIN threads t ON t.id = m.thread_id
		WHERE m.rowid > ?
		ORDER BY m.rowid`, rowid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.RowID, &m.ID, &m.Thread, &m.Subject, &m.Author, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

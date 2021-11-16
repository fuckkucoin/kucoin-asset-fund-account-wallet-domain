package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"account-assets.kucoin.net/funding/internal/persist"
)

type BookLife struct {
	UserID    string
	Book      string
	Status    string
	Reason    string
	UpdatedAt time.Time
}

type BookLives struct{ db *persist.SQLDB }

func NewBookLives(db *persist.SQLDB) *BookLives { return &BookLives{db: db} }

func (r *BookLives) Put(ctx context.Context, row BookLife) error {
	if r == nil || !sqlOn(r.db) {
		return ErrUnavailable
	}
	row.UserID = strings.TrimSpace(row.UserID)
	row.Book = strings.TrimSpace(row.Book)
	row.Status = strings.TrimSpace(row.Status)
	if row.UserID == "" || row.Book == "" || row.Status == "" {
		return ErrConflict
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	q := `INSERT INTO aa_book_life (user_id, book, status, reason, updated_at)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE status=VALUES(status), reason=VALUES(reason), updated_at=VALUES(updated_at)`
	if r.db.Dialect == persist.Postgres {
		q = `INSERT INTO aa_book_life (user_id, book, status, reason, updated_at)
VALUES (?,?,?,?,?)
ON CONFLICT (user_id, book) DO UPDATE SET status=EXCLUDED.status, reason=EXCLUDED.reason, updated_at=EXCLUDED.updated_at`
	}
	_, err := r.db.Conn(ctx).ExecContext(ctx, q, row.UserID, row.Book, row.Status, row.Reason, row.UpdatedAt)
	return err
}

func (r *BookLives) Get(ctx context.Context, userID, book string) (BookLife, error) {
	if r == nil || !sqlOn(r.db) {
		return BookLife{}, ErrUnavailable
	}
	var row BookLife
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT user_id, book, status, reason, updated_at FROM aa_book_life WHERE user_id=? AND book=?`,
		strings.TrimSpace(userID), strings.TrimSpace(book),
	).Scan(&row.UserID, &row.Book, &row.Status, &row.Reason, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BookLife{}, ErrNotFound
	}
	return row, err
}

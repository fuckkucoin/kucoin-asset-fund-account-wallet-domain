package account

import (
	"context"
	"errors"
	"time"

	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	if s != nil {
		s.sql = db
	}
	return s
}

func (s *Service) sqlOn() bool {
	return s != nil && s.sql != nil && s.sql.DB != nil
}

func (s *Service) persistLife(userID string, book Book, st Status, reason string) error {
	if !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewBookLives(s.sql).Put(context.Background(), sqlrepo.BookLife{
		UserID: userID, Book: string(book), Status: string(st), Reason: reason, UpdatedAt: time.Now().UTC(),
	})
}

func (s *Service) loadLife(userID string, book Book) (Status, string, bool) {
	if !s.sqlOn() {
		return "", "", false
	}
	row, err := sqlrepo.NewBookLives(s.sql).Get(context.Background(), userID, string(book))
	if err != nil {
		if errors.Is(err, sqlrepo.ErrNotFound) {
			return "", "", false
		}
		return "", "", false
	}
	return Status(row.Status), row.Reason, true
}

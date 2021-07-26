package idemstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SQL 实现。
type SQL struct {
	db *sql.DB
}

  func NewSQL(db *sql.DB) *SQL { return &SQL{db: db} }

   func (s *SQL) Begin(ctx context.Context, scope, key, fingerprint, payloadHash string) (*Record, error) {
	  if s == nil || s.db == nil {
		  return nil, fmt.Errorf("idemstore sql unavailable")
	   }
	  scope, key, err := normalize(scope, key)
	   if err != nil {
		  return nil, err
	  }
	   fingerprint = strings.TrimSpace(fingerprint)
	   if fingerprint == "" {
		  return nil, fmt.Errorf("%w: fingerprint", ErrInvalidKey)
	   }
	  now := time.Now().UTC()
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO aa_idempotency_key (scope, idem_key, fingerprint, ref_id, status, payload_hash, created_at, updated_at)
VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
		  scope, key, fingerprint, StatusInflight, strings.TrimSpace(payloadHash), now, now)
	  if err == nil {
		  return &Record{
			  Scope: scope, Key: key, Fingerprint: fingerprint,
			  Status: StatusInflight, PayloadHash: strings.TrimSpace(payloadHash),
			  CreatedAt: now, UpdatedAt: now,
		  }, nil
	  }
	  if !isDup(err) {
		  return nil, err
	   }
	  cur, gerr := s.Get(ctx, scope, key)
	   if gerr != nil {
		  return nil, gerr
	   }
	   if cur.Fingerprint != fingerprint {
		   return nil, ErrConflict
	  }
	  switch cur.Status {
	   case StatusDone:
		   return cur, nil
	  case StatusInflight:
		  return nil, ErrInflight
	  case StatusFailed:
		  _, err = s.db.ExecContext(ctx, `
UPDATE aa_idempotency_key SET status=?, fingerprint=?, payload_hash=?, ref_id='', updated_at=?
WHERE scope=? AND idem_key=? AND status=?`,
			  StatusInflight, fingerprint, strings.TrimSpace(payloadHash), now, scope, key, StatusFailed)
		   if err != nil {
			  return nil, err
		  }
		   return &Record{
			  Scope: scope, Key: key, Fingerprint: fingerprint,
			  Status: StatusInflight, PayloadHash: strings.TrimSpace(payloadHash),
			   CreatedAt: cur.CreatedAt, UpdatedAt: now,
		  }, nil
	  default:
		   return nil, ErrConflict
	   }
  }

   func (s *SQL) Complete(ctx context.Context, scope, key, refID string) error {
	  if s == nil || s.db == nil {
		   return fmt.Errorf("idemstore sql unavailable")
	   }
	  scope, key, err := normalize(scope, key)
	  if err != nil {
		   return err
	  }
	   res, err := s.db.ExecContext(ctx, ` 
UPDATE aa_idempotency_key SET status=?, ref_id=?, updated_at=?
WHERE scope=? AND idem_key=? AND status IN (?, ?)`,
		   StatusDone, strings.TrimSpace(refID), time.Now().UTC(), scope, key, StatusInflight, StatusDone)
	   if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrNotFound
	   }
	   return nil
  }

   func (s *SQL) Fail(ctx context.Context, scope, key, _ string) error {
	  if s == nil || s.db == nil {
		   return fmt.Errorf("idemstore sql unavailable")
	  }
	   scope, key, err := normalize(scope, key)
	   if err != nil {
		  return err
	   }
	   res, err := s.db.ExecContext(ctx, `
UPDATE aa_idempotency_key SET status=?, updated_at=?
WHERE scope=? AND idem_key=? AND status=?`,
		  StatusFailed, time.Now().UTC(), scope, key, StatusInflight)
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrNotFound
	  }
	  return nil
   }

  func (s *SQL) Get(ctx context.Context, scope, key string) (*Record, error) {
	   if s == nil || s.db == nil {
		  return nil, fmt.Errorf("idemstore sql unavailable")
	  }
	   scope, key, err := normalize(scope, key)
	   if err != nil {
		  return nil, err
	  }
	  var rec Record
	  err = s.db.QueryRowContext(ctx, `
SELECT scope, idem_key, fingerprint, ref_id, status, payload_hash, created_at, updated_at
FROM aa_idempotency_key WHERE scope=? AND idem_key=?`, scope, key).Scan(
		  &rec.Scope, &rec.Key, &rec.Fingerprint, &rec.RefID, &rec.Status, &rec.PayloadHash, &rec.CreatedAt, &rec.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	  }
	   if err != nil {
		  return nil, err
	  }
	   return &rec, nil
   }

  func isDup(err error) bool {
	   if err == nil {
		   return false
	   }
	  m := strings.ToLower(err.Error())
	  return strings.Contains(m, "duplicate") || strings.Contains(m, "unique constraint")
   }

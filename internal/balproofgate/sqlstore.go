package balproofgate

import (
	"context"
	"database/sql"
	"time"
)

// SQLStore 可选 SQL 实现。
type SQLStore struct {
	db *sql.DB
	mem *MemStore
}

// NewSQLStore 构造。db 可为 nil。

  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db, mem: NewMemStore()}
  }

  func (s *SQLStore) live() Store {
	   if s == nil {
		   return NewMemStore()
	  }
	   if s.db == nil {
		  return s.mem
	   }
	  return s.mem
  }

  func (s *SQLStore) CurrentGeneration(ctx context.Context, userID, book, asset string) (int64, error) {
	  if s != nil && s.db != nil {
		  var gen int64
		   err := s.db.QueryRowContext(ctx,
			   `SELECT generation FROM balproof_generations WHERE user_id=? AND book=? AND asset=?`,
			   userID, book, asset,
		   ).Scan(&gen)
		  if err == sql.ErrNoRows {
			  return 0, nil
		   }
		   if err != nil {
			  return 0, err
		   }
		   return gen, nil
	  }
	   return s.live().CurrentGeneration(ctx, userID, book, asset)
   }

   func (s *SQLStore) BumpGeneration(ctx context.Context, userID, book, asset string, expected int64, at time.Time) (int64, error) {
	   if s != nil && s.db != nil {
		  res, err := s.db.ExecContext(ctx, SQLBumpGeneration, userID, book, asset, expected)
		   if err != nil {
			   return 0, err
		  }
		  n, err := res.RowsAffected()
		   if err != nil {
			  return 0, err
		   }
		   if n == 0 {
			   if expected == 0 {
				   _, err := s.db.ExecContext(ctx, SQLInsertGenerationFirst, newID(), userID, book, asset)
				   if err != nil {
					  return 0, ErrStaleGen
				   }
				  return 1, nil
			  }
			   return 0, ErrStaleGen
		   }
		  return expected + 1, nil
	   }
	   return s.live().BumpGeneration(ctx, userID, book, asset, expected, at)
   }

  func (s *SQLStore) GetGeneration(ctx context.Context, userID, book, asset string) (*ProofGeneration, error) {
	  return s.live().GetGeneration(ctx,
		   userID, book, asset)
  }

   func (s *SQLStore) SaveLease(ctx context.Context, l *ProofLease) error {
	  return s.live().SaveLease(ctx, l)
   }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*ProofLease, error) {
	   return s.live().GetLease(ctx, leaseID)
  }

   func (s *SQLStore) GetLeaseByNonce(ctx context.Context, userID, book, asset, nonce string) (*ProofLease, error) {
	   return s.live().GetLeaseByNonce(ctx, userID, book, asset, nonce)
  }

   func (s *SQLStore) ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*ProofLease, error) {
	  return s.live().ListLeasesByStatus(ctx, status, limit)
   }

  func (s *SQLStore) ListLeasesBySubject(ctx context.Context, userID, book, asset string) ([]*ProofLease, error) {
	  return s.live().ListLeasesBySubject(ctx, userID, book, asset)
  }

  func (s *SQLStore) ListExpiredBound(ctx context.Context, now time.Time, limit int) ([]*ProofLease, error) {
	   return s.live().ListExpiredBound(ctx, now, limit)
  }

   func (s *SQLStore) SaveDrift(ctx context.Context, d *DriftProof) error {
	  return s.live().SaveDrift(ctx, d)
   }

   func (s *SQLStore) GetDrift(ctx context.Context, id string) (*DriftProof, error) {
	  return s.live().GetDrift(ctx, id)
  }

   func (s *SQLStore) FindOpenDrift(ctx context.Context, leaseID string) (*DriftProof, error) {
	  return s.live().FindOpenDrift(ctx, leaseID)
  }

   func (s *SQLStore) ListBlockingDrifts(ctx context.Context, userID, book, asset string) ([]*DriftProof, error) {
	   return s.live().ListBlockingDrifts(ctx, userID, book, asset)
  }

  func (s *SQLStore) ListOpenDrifts(ctx context.Context, limit int) ([]*DriftProof, error) {
	  return s.live().ListOpenDrifts(ctx,
		  limit)
  }

   func (s *SQLStore) SaveRevocation(ctx context.Context, r *Revocation) error {
	   return s.live().SaveRevocation(ctx, r)
  }

  func (s *SQLStore) GetRevocationByLease(ctx context.Context, leaseID string) (*Revocation, error) {
	   return s.live().GetRevocationByLease(ctx, leaseID)
  }

   func (s *SQLStore) ListUnpropagated(ctx context.Context, limit int) ([]*Revocation, error) {
	   return s.live().ListUnpropagated(ctx, limit)
   }

  func newID() string {
	   return time.Now().UTC().Format("20060102150405.000000000")
  }

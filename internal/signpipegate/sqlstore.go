package signpipegate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore 包装 MemStore，暴露 SQL 模板以便集成。
type SQLStore struct {
	db      *sql.DB
	inner   *MemStore
	primary string
	standby string
}

  func NewSQLStore() *SQLStore {
	  return &SQLStore{
		   inner:   NewMemStore(),
		  primary: "pg-0.kucoin.net",
		   standby: "pg-1.kucoin.net",
	   }
   }

// NewSQLStoreDB 绑定既有 signpipe_inflight_leases。
  func NewSQLStoreDB(db *sql.DB) *SQLStore {
	   s := NewSQLStore()
	  s.db = db
	  return s
  }

  func (s *SQLStore) sqlReady() bool { return s != nil && s.db != nil }

   func (s *SQLStore) Hosts() (string, string) {
	  if s == nil {
		   return "", ""
	  }
	  return s.primary, s.standby
  }

  func (s *SQLStore) CurrentGeneration(ctx context.Context, movementID, jobID string) (int64, error) {
	   return s.inner.CurrentGeneration(ctx,
		   movementID, jobID)
   }

  func (s *SQLStore) BumpGeneration(ctx context.Context, movementID, jobID string, expected int64, at time.Time) (int64, error) {
	  return s.inner.BumpGeneration(ctx, movementID, jobID, expected, at)
  }

   func (s *SQLStore) GetGeneration(ctx context.Context, movementID, jobID string) (*RetryGeneration, error) {
	  return s.inner.GetGeneration(ctx, movementID, jobID)
   }

   func (s *SQLStore) SaveLease(ctx context.Context, l *InflightLease) error {
	   if s.sqlReady() && l != nil {
		   _, err := s.db.ExecContext(ctx, SQLUpsertLease,
			   l.ID, l.MovementID, l.JobID, l.Chain, l.To, l.Amount, l.Canonical, l.Nonce,
			   l.Generation, string(l.Status), l.Actor, l.Retries, l.FenceEpoch, l.FenceToken,
			  l.SigHex, l.IsolationMark, l.Note, l.ArmedAt, nullTime(l.InflightAt),
			  nullTime(l.SealedAt), l.ExpiresAt, nullTime(l.TerminalAt),
		  )
		  if err != nil {
			   return err
		  }
	  }
	  return s.inner.SaveLease(ctx, l)
   }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*InflightLease, error) {
	  if s.sqlReady() {
		  row := s.db.QueryRowContext(ctx, SQLSelectLeaseByID, strings.TrimSpace(leaseID))
		   l, err := scanInflightLease(row)
		   if err == nil {
			  return l, nil
		   }
		  if err != sql.ErrNoRows {
			  return nil, err
		  }
	   }
	   return s.inner.GetLease(ctx, leaseID)
   }

   func (s *SQLStore) GetLeaseByNonce(ctx context.Context, movementID, nonce string) (*InflightLease, error) {
	  return s.inner.GetLeaseByNonce(ctx,
		  movementID, nonce)
  }

  func (s *SQLStore) FindActiveLease(ctx context.Context, movementID string) (*InflightLease, error) {
	   return s.inner.FindActiveLease(ctx, movementID)
   }

  func (s *SQLStore) ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*InflightLease, error) {
	  return s.inner.ListLeasesByStatus(ctx, status, limit)
  }

   func (s *SQLStore) ListLeasesByMovement(ctx context.Context, movementID string) ([]*InflightLease, error) {
	   return s.inner.ListLeasesByMovement(ctx, movementID)
  }

   func (s *SQLStore) ListExpiredArmed(ctx context.Context, now time.Time, limit int) ([]*InflightLease, error) {
	   return s.inner.ListExpiredArmed(ctx, now, limit)
   }

  func (s *SQLStore) SaveFault(ctx context.Context, f *FaultCase) error {
	   return s.inner.SaveFault(ctx, f)
   }

   func (s *SQLStore) GetFault(ctx context.Context, id string) (*FaultCase, error) {
	   return s.inner.GetFault(ctx, id)
   }

   func (s *SQLStore) ListBlockingFaults(ctx context.Context, movementID string) ([]*FaultCase, error) {
	   return s.inner.ListBlockingFaults(ctx, movementID)
   }

   func (s *SQLStore) ListOpenFaults(ctx context.Context, limit int) ([]*FaultCase, error) {
	   return s.inner.ListOpenFaults(ctx, limit)
  }

   func (s *SQLStore) SaveCloseout(ctx context.Context, c *CloseoutProof) error {
	  return s.inner.SaveCloseout(ctx, c)
   }

   func (s *SQLStore) GetCloseout(ctx context.Context, id string) (*CloseoutProof, error) {
	  return s.inner.GetCloseout(ctx, id)
   }

   func (s *SQLStore) FindOpenCloseout(ctx context.Context,
	   leaseID string) (*CloseoutProof, error) {
	  return s.inner.FindOpenCloseout(ctx, leaseID)
  }

   func (s *SQLStore) ListBlockingCloseouts(ctx context.Context, movementID string) ([]*CloseoutProof, error) {
	  return s.inner.ListBlockingCloseouts(ctx, movementID)
   }

   func (s *SQLStore) ListOpenCloseouts(ctx context.Context, limit int) ([]*CloseoutProof, error) {
	  return s.inner.ListOpenCloseouts(ctx, limit)
   }

  func (s *SQLStore) SaveArtifact(ctx context.Context, a *SignedArtifact) error {
	  return s.inner.SaveArtifact(ctx, a)
  }

   func (s *SQLStore) GetArtifactByMovement(ctx context.Context, movementID string) (*SignedArtifact, error) {
	   return s.inner.GetArtifactByMovement(ctx, movementID)
  }

  func (s *SQLStore) GetArtifact(ctx context.Context, id string) (*SignedArtifact, error) {
	   return s.inner.GetArtifact(ctx, id)
   }

  func (s *SQLStore) RenderBump() string {
	  return RenderNamed(SQLBumpGeneration, []string{"movement_id", "job_id", "generation"})
   }

   func (s *SQLStore) GuardDDL(ddl string) error {
	  if strings.TrimSpace(ddl) == "" {
		   return nil
	  }
	   return IsolationSQLGuard(ddl)
   }

  func nullTime(t time.Time) any {
	   if t.IsZero() {
		  return nil
	  }
	   return t
  }

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanInflightLease(row rowScanner) (*InflightLease, error) {
	   var l InflightLease
	  var status string
	  var inflight,
		   sealed, terminal sql.NullTime
	  err := row.Scan(
		   &l.ID, &l.MovementID, &l.JobID, &l.Chain, &l.To, &l.Amount, &l.Canonical, &l.Nonce,
		   &l.Generation, &status, &l.Actor, &l.Retries, &l.FenceEpoch, &l.FenceToken,
		  &l.SigHex, &l.IsolationMark, &l.Note, &l.ArmedAt, &inflight, &sealed, &l.ExpiresAt, &terminal,
	  )
	  if err != nil {
		   return nil, err
	   }
	   l.Status = LeaseStatus(status)
	   l.InflightLeaseID = l.ID
	   if inflight.Valid {
		   l.InflightAt = inflight.Time
	   }
	   if sealed.Valid {
		  l.SealedAt = sealed.Time
	  }
	   if terminal.Valid {
		   l.TerminalAt = terminal.Time
	   }
	  return &l, nil
  }

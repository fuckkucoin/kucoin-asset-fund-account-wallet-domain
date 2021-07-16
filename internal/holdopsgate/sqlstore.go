package holdopsgate

import (
	"context"
	"database/sql"
	"time"
)

type SQLStore struct {
	db *sql.DB
}

  func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

   func (s *SQLStore) GetGeneration(ctx context.Context) (HoldOpsGeneration, error) {
	   if s == nil || s.db == nil {
		  return HoldOpsGeneration{}, ErrUnavailable
	  }
	  var g HoldOpsGeneration
	   var fence int
	  err := s.db.QueryRowContext(ctx, sqlSelectGeneration).Scan(&g.ID, &g.OpsGen, &fence, &g.Note, &g.UpdatedAt)
	   if err != nil {
		   return HoldOpsGeneration{}, err
	   }
	   g.FenceOn = fence == 1
	   return g, nil
   }

  func (s *SQLStore) CASBumpOps(ctx context.Context, expect int64, fence bool, note string, at time.Time) (HoldOpsGeneration, error) {
	  if s == nil || s.db == nil {
		  return HoldOpsGeneration{},
			   ErrUnavailable
	  }
	  on := 0
	  if fence {
		   on = 1
	   }
	   res, err := s.db.ExecContext(ctx, sqlCASBumpOps, on, note, at, expect)
	   if err != nil {
		  return HoldOpsGeneration{}, err
	   }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  got, _ := s.GetGeneration(ctx)
		  return got, ErrStaleGen
	   }
	  return s.GetGeneration(ctx)
  }

   func (s *SQLStore) PutLease(ctx context.Context, lease HoldOpsLease) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, sqlInsertLease,
		  lease.ID,
		  lease.OpsID, string(lease.Kind), lease.UserID, lease.Asset, lease.Amount,
		  lease.TokenID, lease.GapID, lease.Ref, lease.Reason, lease.Actor, lease.Nonce,
		  lease.OpsGen, string(lease.Status), lease.IsolationMark, lease.Note,
		   lease.IssuedAt, nullTime(lease.BoundAt), nullTime(lease.ConsumedAt),
		  lease.ExpiresAt, nullTime(lease.TerminalAt), lease.BindTicket,
	   )
	  return err
   }

   func (s *SQLStore) GetLease(ctx context.Context, id string) (HoldOpsLease, error) {
	  if s == nil || s.db == nil {
		  return HoldOpsLease{},
			   ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, sqlSelectLease, id)
	  return scanLease(row)
  }

   func (s *SQLStore) FindLeaseByNonce(ctx context.Context, actor, nonce string) (HoldOpsLease, bool, error) {
	  if s == nil || s.db == nil {
		  return HoldOpsLease{}, false, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, sqlSelectLeaseNonce, actor, nonce)
	   got, err := scanLease(row)
	   if err == sql.ErrNoRows {
		  return HoldOpsLease{}, false, nil
	  }
	   if err != nil {
		  return HoldOpsLease{}, false, err
	   }
	   return got, true, nil
   }

   func (s *SQLStore) ListOpenLeases(ctx context.Context) ([]HoldOpsLease, error) {
	   return s.queryLeases(ctx, sqlListOpenLeases)
   }

  func (s *SQLStore) ListTokenOpen(ctx context.Context, tokenID string) ([]HoldOpsLease, error) {
	   return s.queryLeases(ctx,
		  sqlListTokenOpen, tokenID)
   }

  func (s *SQLStore) ListActorOpen(ctx context.Context, actor string) ([]HoldOpsLease, error) {
	  return s.queryLeases(ctx, sqlListActorOpen, actor)
   }

  func (s *SQLStore) UpdateLease(ctx context.Context, lease HoldOpsLease) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, sqlUpdateLease,
		  string(lease.Status), lease.Note, nullTime(lease.BoundAt), nullTime(lease.ConsumedAt),
		  nullTime(lease.TerminalAt), lease.BindTicket, lease.ID,
	  )
	   return err
   }

   func (s *SQLStore) PutProof(ctx context.Context, p HoldOpsRaceProof) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, sqlInsertProof,
		  p.ID, p.LeaseID, p.OpsID, p.TokenID, p.GapID, p.UserID, string(p.Kind), string(p.Status),
		  p.ExpectedGen, p.ObservedGen, p.WinnerLease, p.LoserLease, p.OpenTokenCount, p.Note,
		  p.IsolationMark, p.OpenedAt, nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.EyeA, p.EyeB, p.ClearReason,
	   )
	  return err
  }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (HoldOpsRaceProof, error) {
	   if s == nil || s.db == nil {
		   return HoldOpsRaceProof{}, ErrUnavailable
	   }
	  return scanProof(s.db.QueryRowContext(ctx, sqlSelectProof, id))
   }

   func (s *SQLStore) FindProof(ctx context.Context, tokenID string, kind ProofKind) (HoldOpsRaceProof, bool, error) {
	   p, err := scanProof(s.db.QueryRowContext(ctx, sqlFindProof, tokenID, string(kind)))
	  if err == sql.ErrNoRows {
		  return HoldOpsRaceProof{}, false, nil
	   }
	   if err != nil {
		   return HoldOpsRaceProof{}, false, err
	  }
	   return p, true, nil
  }

  func (s *SQLStore) ListOpenProofs(ctx context.Context) ([]HoldOpsRaceProof, error) {
	  return s.queryProofs(ctx, sqlListOpenProofs)
   }

  func (s *SQLStore) ListProofsByToken(ctx context.Context, tokenID string) ([]HoldOpsRaceProof, error) {
	   return s.queryProofs(ctx, sqlListProofsToken, tokenID)
   }

  func (s *SQLStore) ListProofsByUser(ctx context.Context, userID string) ([]HoldOpsRaceProof, error) {
	  return s.queryProofs(ctx,
		   sqlListProofsUser, userID)
   }

  func (s *SQLStore) UpdateProof(ctx context.Context, p HoldOpsRaceProof) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpdateProof,
		   string(p.Status), p.EyeA, p.EyeB, p.ClearReason, nullTime(p.ProvenAt), nullTime(p.ClosedAt), p.ID,
	  )
	  return err
  }

   func (s *SQLStore) PutAudit(ctx context.Context, a HoldOpsAudit) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertAudit,
		   a.ID, a.OpsID, a.LeaseID, a.TokenID, a.GapID, a.Gen, a.Actor, string(a.Kind),
		  a.Outcome, a.ProofID, a.Note, a.At, a.ReplayKey,
	  )
	  return err
  }

   func (s *SQLStore) GetAuditByReplay(ctx context.Context, key string) (HoldOpsAudit, bool, error) {
	  var a HoldOpsAudit
	   var kind string
	  err := s.db.QueryRowContext(ctx, sqlSelectAuditReplay, key).Scan(
		  &a.ID, &a.OpsID, &a.LeaseID, &a.TokenID, &a.GapID, &a.Gen, &a.Actor, &kind,
		   &a.Outcome, &a.ProofID, &a.Note, &a.At, &a.ReplayKey,
	   )
	   if err == sql.ErrNoRows {
		   return HoldOpsAudit{}, false, nil
	   }
	   if err != nil {
		  return HoldOpsAudit{}, false, err
	  }
	  a.Kind = OpsKind(kind)
	   return a, true, nil
   }

  func (s *SQLStore) ListAuditByOps(ctx context.Context, opsID string) ([]HoldOpsAudit, error) {
	   rows, err := s.db.QueryContext(ctx, sqlListAuditOps, opsID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]HoldOpsAudit, 0)
	  for rows.Next() {
		   var a HoldOpsAudit
		  var kind string
		   if err := rows.Scan(&a.ID, &a.OpsID, &a.LeaseID, &a.TokenID, &a.GapID, &a.Gen, &a.Actor, &kind, &a.Outcome, &a.ProofID, &a.Note, &a.At, &a.ReplayKey); err != nil {
			   return nil, err
		   }
		   a.Kind = OpsKind(kind)
		   out = append(out, a)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) PutToken(ctx context.Context, t ShadowToken) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	   open := 0
	   if t.Open {
		   open = 1
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpsertToken, t.ID, t.UserID, t.Asset, t.Amount, t.Reason, t.Ref, t.GapID, open)
	  return err
  }

   func (s *SQLStore) GetToken(ctx context.Context, id string) (ShadowToken, error) {
	   var t ShadowToken
	  var open int
	   err := s.db.QueryRowContext(ctx, sqlSelectToken, id).Scan(&t.ID, &t.UserID, &t.Asset, &t.Amount, &t.Reason, &t.Ref, &t.GapID, &open)
	  if err == sql.ErrNoRows {
		   return ShadowToken{}, ErrTokenGone
	   }
	  if err != nil {
		   return ShadowToken{}, err
	  }
	  t.Open = open == 1
	   return t, nil
  }

   func (s *SQLStore) ListOpenTokensByGap(ctx context.Context, gapID string) ([]ShadowToken, error) {
	  return s.queryTokens(ctx, sqlListOpenTokGap, gapID)
   }

   func (s *SQLStore) ListOpenTokensByUser(ctx context.Context, userID string) ([]ShadowToken, error) {
	   return s.queryTokens(ctx, sqlListOpenTokUser, userID)
  }

   func (s *SQLStore) PutGap(ctx context.Context, g ShadowGap) error {
	   acked, closed := 0, 0
	  if g.Acked {
		   acked = 1
	   }
	   if g.Closed {
		   closed = 1
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpsertGap, g.ID, g.UserID, g.Asset, g.Amount, acked, closed, g.OpenCount)
	   return err
  }

  func (s *SQLStore) GetGap(ctx context.Context, id string) (ShadowGap, error) {
	   var g ShadowGap
	   var acked, closed int
	  err := s.db.QueryRowContext(ctx,
		  sqlSelectGap, id).Scan(&g.ID, &g.UserID, &g.Asset, &g.Amount, &acked, &closed, &g.OpenCount)
	  if err == sql.ErrNoRows {
		   return ShadowGap{}, ErrGapGone
	   }
	  if err != nil {
		  return ShadowGap{}, err
	  }
	   g.Acked = acked == 1
	   g.Closed = closed == 1
	   return g, nil
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

   func scanLease(row rowScanner) (HoldOpsLease, error) {
	  var l HoldOpsLease
	  var kind, status string
	   var bound,
		   consumed, terminal sql.NullTime
	   err := row.Scan(
		  &l.ID, &l.OpsID, &kind, &l.UserID, &l.Asset, &l.Amount, &l.TokenID, &l.GapID,
		  &l.Ref, &l.Reason, &l.Actor, &l.Nonce, &l.OpsGen, &status, &l.IsolationMark, &l.Note,
		  &l.IssuedAt, &bound, &consumed, &l.ExpiresAt, &terminal, &l.BindTicket,
	  )
	   if err != nil {
		  if err == sql.ErrNoRows {
			  return HoldOpsLease{}, ErrLeaseNotFound
		  }
		   return HoldOpsLease{}, err
	  }
	  l.Kind = OpsKind(kind)
	   l.Status = LeaseStatus(status)
	   if bound.Valid {
		  l.BoundAt = bound.Time
	   }
	  if consumed.Valid {
		  l.ConsumedAt = consumed.Time
	  }
	  if terminal.Valid {
		  l.TerminalAt = terminal.Time
	   }
	   return l, nil
   }

  func scanProof(row rowScanner) (HoldOpsRaceProof, error) {
	  var p HoldOpsRaceProof
	  var kind, status string
	  var proven, closed sql.NullTime
	   err := row.Scan(
		   &p.ID, &p.LeaseID, &p.OpsID, &p.TokenID, &p.GapID, &p.UserID, &kind, &status,
		   &p.ExpectedGen, &p.ObservedGen, &p.WinnerLease, &p.LoserLease, &p.OpenTokenCount, &p.Note,
		  &p.IsolationMark, &p.OpenedAt, &proven, &closed, &p.EyeA, &p.EyeB, &p.ClearReason,
	  )
	  if err != nil {
		   return HoldOpsRaceProof{}, err
	  }
	   p.Kind = ProofKind(kind)
	   p.Status = ProofStatus(status)
	   if proven.Valid {
		  p.ProvenAt = proven.Time
	   }
	   if closed.Valid {
		   p.ClosedAt = closed.Time
	  }
	  return p, nil
  }

  func (s *SQLStore) queryLeases(ctx context.Context, q string, args ...any) ([]HoldOpsLease, error) {
	   rows,
		   err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]HoldOpsLease, 0)
	   for rows.Next() {
		  got, err := scanLease(rows)
		   if err != nil {
			   return nil, err
		  }
		   out = append(out, got)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) queryProofs(ctx context.Context, q string, args ...any) ([]HoldOpsRaceProof, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]HoldOpsRaceProof, 0)
	  for rows.Next() {
		  got, err := scanProof(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, got)
	  }
	   return out, rows.Err()
   }

   func (s *SQLStore) queryTokens(ctx context.Context, q string, args ...any) ([]ShadowToken, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]ShadowToken, 0)
	  for rows.Next() {
		   var t ShadowToken
		   var open int
		  if err := rows.Scan(&t.ID, &t.UserID, &t.Asset, &t.Amount, &t.Reason, &t.Ref, &t.GapID, &open); err != nil {
			   return nil, err
		  }
		   t.Open = open == 1
		   out = append(out, t)
	  }
	  return out, rows.Err()
  }

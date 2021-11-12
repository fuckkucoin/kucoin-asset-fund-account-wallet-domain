// 功能：装配走 database/sql；单测默认 MemStore。本文件保证迁移列与
// 装配走 database/sql；单测默认 MemStore。本文件保证迁移列与
// persist 行结构一一对应，避免运行时列漂移。

// 作者：Hale
// 日期：2021.11.03

package scoreleasegate

import (
	"context"
	"database/sql"
	"time"
)

type SQLStore struct {
	db *sql.DB
}

  func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

   func (s *SQLStore) ping(ctx context.Context) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	   return s.db.PingContext(ctx)
  }

   func (s *SQLStore) GetGeneration(ctx context.Context) (ScoreGeneration, error) {
	  if err := s.ping(ctx); err != nil {
		  return ScoreGeneration{}, err
	   }
	   var g ScoreGeneration
	   var haltOn int
	  err := s.db.QueryRowContext(ctx, sqlSelectGen).Scan(
		   &g.ID,
		  &g.HaltGen, &haltOn, &g.PackGen, &g.Pack, &g.PackFP, &g.ModeGen, &g.Mode,
		   &g.WindowGen, &g.WindowDigest, &g.UpdatedAt,
	   )
	   if err != nil {
		   return ScoreGeneration{}, err
	   }
	   g.HaltOn = haltOn != 0
	  return g, nil
  }

  func (s *SQLStore) CASBumpHalt(ctx context.Context, expect int64, on bool, at time.Time) (ScoreGeneration, error) {
	   if err := s.ping(ctx); err != nil {
		   return ScoreGeneration{}, err
	   }
	  haltOn := 0
	   if on {
		  haltOn = 1
	   }
	  res, err := s.db.ExecContext(ctx, sqlBumpHalt, haltOn, at, expect)
	   if err != nil {
		   return ScoreGeneration{}, err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   cur, _ := s.GetGeneration(ctx)
		   return cur, ErrStaleGen
	  }
	   return s.GetGeneration(ctx)
  }

   func (s *SQLStore) CASBumpPack(ctx context.Context, expect int64, pack, fp string, at time.Time) (ScoreGeneration, error) {
	  if err := s.ping(ctx); err != nil {
		   return ScoreGeneration{}, err
	  }
	   res, err := s.db.ExecContext(ctx, sqlBumpPack, pack, fp, at, expect)
	  if err != nil {
		   return ScoreGeneration{}, err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  cur, _ := s.GetGeneration(ctx)
		   return cur, ErrStaleGen
	   }
	  return s.GetGeneration(ctx)
   }

  func (s *SQLStore) CASBumpMode(ctx context.Context, expect int64, mode string, at time.Time) (ScoreGeneration, error) {
	  if err := s.ping(ctx); err != nil {
		   return ScoreGeneration{}, err
	  }
	  res, err := s.db.ExecContext(ctx, sqlBumpMode, mode, at, expect)
	  if err != nil {
		  return ScoreGeneration{}, err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   cur, _ := s.GetGeneration(ctx)
		  return cur, ErrStaleGen
	   }
	  return s.GetGeneration(ctx)
   }

  func (s *SQLStore) CASBumpWindow(ctx context.Context, expect int64, digest string, at time.Time) (ScoreGeneration, error) {
	   if err := s.ping(ctx); err != nil {
		   return ScoreGeneration{}, err
	   }
	   res,
		   err := s.db.ExecContext(ctx, sqlBumpWindow, digest, at, expect)
	   if err != nil {
		   return ScoreGeneration{}, err
	  }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		  cur, _ := s.GetGeneration(ctx)
		   return cur, ErrStaleGen
	  }
	  return s.GetGeneration(ctx)
   }

  func (s *SQLStore) PutLease(ctx context.Context, lease ScoreLease) error {
	   if err := s.ping(ctx); err != nil {
		   return err
	  }
	  r := leaseToRow(lease)
	  _, err := s.db.ExecContext(ctx, sqlInsertLease,
		   r.ID, r.UserID, r.Chain, r.Dest, r.Asset, r.Amount, r.Kind, r.Pack, r.Mode, r.PackFP,
		   r.HaltGen, r.PackGen, r.ModeGen, r.WindowGen, r.WindowDigest,
		  r.Velocity1h,
		  r.Velocity24h, r.DestCount, r.DestCount24h,
		  r.ClusterSize, r.Correlated, r.NewDests, r.Fingerprint, r.Nonce, r.Status,
		   r.IsolationMark, r.Note, r.IssuedAt, r.BoundAt, r.ConsumedAt, r.ExpiresAt,
		   r.TerminalAt, r.BindTicket, r.EvalAllow, r.EvalReason,
	   )
	  return err
  }

  func (s *SQLStore) GetLease(ctx context.Context, id string) (ScoreLease, error) {
	   if err := s.ping(ctx); err != nil {
		  return ScoreLease{}, err
	  }
	   var r leaseRow
	  err := s.db.QueryRowContext(ctx, sqlSelectLease, id).Scan(
		   &r.ID,
		  &r.UserID, &r.Chain, &r.Dest, &r.Asset, &r.Amount, &r.Kind, &r.Pack, &r.Mode, &r.PackFP,
		   &r.HaltGen, &r.PackGen, &r.ModeGen, &r.WindowGen, &r.WindowDigest,
		   &r.Velocity1h, &r.Velocity24h, &r.DestCount, &r.DestCount24h,
		   &r.ClusterSize, &r.Correlated, &r.NewDests, &r.Fingerprint, &r.Nonce, &r.Status,
		  &r.IsolationMark, &r.Note, &r.IssuedAt, &r.BoundAt, &r.ConsumedAt, &r.ExpiresAt,
		   &r.TerminalAt, &r.BindTicket, &r.EvalAllow, &r.EvalReason,
	  )
	   if err == sql.ErrNoRows {
		   return ScoreLease{}, ErrLeaseNotFound
	   }
	   if err != nil {
		  return ScoreLease{}, err
	   }
	  return rowToLease(r), nil
  }

  func (s *SQLStore) FindLeaseByNonce(ctx context.Context, user, nonce string) (ScoreLease, bool, error) {
	   if err := s.ping(ctx); err != nil {
		  return ScoreLease{}, false, err
	  }
	   var id string
	   err := s.db.QueryRowContext(ctx, sqlSelectLeaseNonce, user, nonce).Scan(&id)
	   if err == sql.ErrNoRows {
		  return ScoreLease{}, false, nil
	  }
	  if err != nil {
		   return ScoreLease{}, false, err
	   }
	   l, err := s.GetLease(ctx, id)
	  return l, err == nil, err
  }

  func (s *SQLStore) listByIDs(ctx context.Context, q string, args ...any) ([]ScoreLease, error) {
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]ScoreLease, 0)
	   for rows.Next() {
		  var id string
		   if err := rows.Scan(&id); err != nil {
			   return nil, err
		   }
		  l, err := s.GetLease(ctx, id)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, l)
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) ListOpenLeases(ctx context.Context) ([]ScoreLease, error) {
	   if err := s.ping(ctx); err != nil {
		  return nil, err
	  }
	   return s.listByIDs(ctx,
		  sqlSelectOpenLeases)
   }

   func (s *SQLStore) ListUserOpen(ctx context.Context, user string) ([]ScoreLease, error) {
	   if err := s.ping(ctx); err != nil {
		  return nil, err
	   }
	  return s.listByIDs(ctx, sqlSelectUserOpen, user)
  }

   func (s *SQLStore) UpdateLease(ctx context.Context, lease ScoreLease) error {
	   if err := s.ping(ctx); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpdateLease,
		   string(lease.Status), lease.Note, lease.BoundAt, lease.ConsumedAt, lease.TerminalAt, lease.BindTicket, lease.ID,
	   )
	  return err
  }

  func (s *SQLStore) PutProof(ctx context.Context, p ScoreDriftProof) error {
	   if err := s.ping(ctx); err != nil {
		   return err
	   }
	  r := proofToRow(p)
	  _, err := s.db.ExecContext(ctx, sqlInsertProof,
		  r.ID, r.LeaseID, r.UserID, r.Kind, r.Status, r.ExpectedHalt, r.ObservedHalt,
		   r.ExpectedPack,
		  r.ObservedPack, r.ExpectedMode, r.ObservedMode,
		   r.ExpectedFP, r.ObservedFP, r.ExpectedWin, r.ObservedWin,
		  r.ExpectedWinGen, r.ObservedWinGen, r.Digest, r.IsolationMark, r.Note,
		  r.CloserA,
		   r.CloserB, r.CloseReason, r.OpenedAt, r.ProvenAt, r.ClosedAt,
	  )
	  return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (ScoreDriftProof, error) {
	   if err := s.ping(ctx); err != nil {
		   return ScoreDriftProof{}, err
	   }
	   var r proofRow
	  err := s.db.QueryRowContext(ctx, sqlSelectProof, id).Scan(
		   &r.ID, &r.LeaseID, &r.UserID, &r.Kind, &r.Status, &r.ExpectedHalt, &r.ObservedHalt,
		  &r.ExpectedPack, &r.ObservedPack, &r.ExpectedMode, &r.ObservedMode,
		   &r.ExpectedFP,
		   &r.ObservedFP, &r.ExpectedWin, &r.ObservedWin,
		  &r.ExpectedWinGen, &r.ObservedWinGen, &r.Digest, &r.IsolationMark, &r.Note,
		   &r.CloserA, &r.CloserB, &r.CloseReason, &r.OpenedAt, &r.ProvenAt, &r.ClosedAt,
	  )
	  if err == sql.ErrNoRows {
		  return ScoreDriftProof{}, ErrProofNotFound
	  }
	  if err != nil {
		   return ScoreDriftProof{}, err
	   }
	  return rowToProof(r), nil
   }

  func (s *SQLStore) FindProof(ctx context.Context, leaseID string, kind ProofKind, halt, packGen int64) (ScoreDriftProof, bool, error) {
	   if err := s.ping(ctx); err != nil {
		   return ScoreDriftProof{}, false, err
	  }
	   var id string
	   err := s.db.QueryRowContext(ctx,
		  sqlSelectProofFind, leaseID, string(kind), halt, packGen).Scan(&id)
	   if err == sql.ErrNoRows {
		   return ScoreDriftProof{}, false, nil
	   }
	  if err != nil {
		   return ScoreDriftProof{}, false, err
	   }
	   p, err := s.GetProof(ctx, id)
	   return p, err == nil, err
   }

  func (s *SQLStore) ListOpenProofs(ctx context.Context) ([]ScoreDriftProof, error) {
	   if err := s.ping(ctx); err != nil {
		   return nil, err
	  }
	   rows, err := s.db.QueryContext(ctx, sqlSelectOpenProofs)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]ScoreDriftProof, 0)
	  for rows.Next() {
		  var id string
		  if err := rows.Scan(&id); err != nil {
			   return nil, err
		   }
		   p, err := s.GetProof(ctx, id)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, p)
	   }
	  return out, rows.Err()
   }

   func (s *SQLStore) UpdateProof(ctx context.Context, p ScoreDriftProof) error {
	   if err := s.ping(ctx); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpdateProof,
		  string(p.Status), p.Digest, p.Note, p.CloserA, p.CloserB, p.CloseReason, p.ProvenAt, p.ClosedAt, p.ID,
	  )
	  return err
   }

   func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string) ([]ScoreDriftProof, error) {
	   all, err := s.ListOpenProofs(ctx)
	   if err != nil {
		  return nil, err
	  }
	  _ = all
	  // 开放列表不够：再按 Get 无法扫全表时由调用方走导出 SQL。此处退回开放+匹配。
	  out := make([]ScoreDriftProof, 0)
	   for _, p := range all {
		   if p.LeaseID == leaseID {
			   out = append(out, p)
		  }
	   }
	  return out, nil
  }

  func (s *SQLStore) ListProofsByUser(ctx context.Context, user string) ([]ScoreDriftProof, error) {
	   all, err := s.ListOpenProofs(ctx)
	   if err != nil {
		   return nil, err
	  }
	   out := make([]ScoreDriftProof, 0)
	  for _, p := range all {
		   if p.UserID == user {
			  out = append(out, p)
		  }
	  }
	   return out, nil
   }

  func (s *SQLStore) PutAudit(ctx context.Context, ev AuditEvent) error {
	  if err := s.ping(ctx); err != nil {
		   return err
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertAudit, ev.ID, ev.Kind, ev.LeaseID, ev.ProofID, ev.UserID, ev.Actor, ev.Note, ev.At)
	   return err
  }

  func (s *SQLStore) ListAudit(ctx context.Context, leaseID string) ([]AuditEvent, error) {
	   if err := s.ping(ctx); err != nil {
		   return nil, err
	  }
	   rows, err := s.db.QueryContext(ctx, sqlSelectAudit, leaseID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]AuditEvent, 0)
	   for rows.Next() {
		   var ev AuditEvent
		  if err := rows.Scan(&ev.ID, &ev.Kind, &ev.LeaseID, &ev.ProofID, &ev.UserID, &ev.Actor, &ev.Note, &ev.At); err != nil {
			  return nil, err
		  }
		  out = append(out, ev)
	   }
	  return out, rows.Err()
   }

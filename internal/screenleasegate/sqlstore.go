package screenleasegate

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

   func (s *SQLStore) GetGeneration(ctx context.Context) (ScreenGeneration, error) {
	   if err := s.ping(ctx); err != nil {
		   return ScreenGeneration{}, err
	  }
	   var g ScreenGeneration
	  var haltOn int
	  err := s.db.QueryRowContext(ctx, sqlSelectGen).Scan(
		  &g.ID,
		  &g.HaltGen, &haltOn, &g.PackGen, &g.Pack, &g.Mode, &g.PackFP, &g.UpdatedAt,
	  )
	  if err != nil {
		  return ScreenGeneration{}, err
	   }
	   g.HaltOn = haltOn != 0
	  return g, nil
  }

   func (s *SQLStore) CASBumpHalt(ctx context.Context, expect int64, on bool, at time.Time) (ScreenGeneration, error) {
	  if err := s.ping(ctx); err != nil {
		   return ScreenGeneration{}, err
	  }
	   haltOn := 0
	  if on {
		   haltOn = 1
	   }
	  res,
		  err := s.db.ExecContext(ctx, sqlBumpHalt, haltOn, at, expect)
	   if err != nil {
		   return ScreenGeneration{}, err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   cur, _ := s.GetGeneration(ctx)
		  return cur, ErrStaleGen
	   }
	  return s.GetGeneration(ctx)
   }

   func (s *SQLStore) CASBumpPack(ctx context.Context, expect int64, pack, mode, fp string, at time.Time) (ScreenGeneration, error) {
	  if err := s.ping(ctx); err != nil {
		   return ScreenGeneration{}, err
	   }
	   res, err := s.db.ExecContext(ctx, sqlBumpPack, pack, mode, fp, at, expect)
	  if err != nil {
		  return ScreenGeneration{}, err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   cur, _ := s.GetGeneration(ctx)
		  return cur, ErrStaleGen
	  }
	  return s.GetGeneration(ctx)
  }

   func (s *SQLStore) PutLease(ctx context.Context, lease ScreenLease) error {
	  if err := s.ping(ctx); err != nil {
		  return err
	   }
	  r := leaseToRow(lease)
	  _, err := s.db.ExecContext(ctx, sqlInsertLease,
		   r.ID, r.UserID, r.Dest, r.Asset, r.Amount, r.Kind, r.Pack, r.Mode, r.PackFP,
		  r.HaltGen, r.PackGen, r.WindowDigest, r.Velocity1h, r.Velocity24h,
		  r.DestCount, r.DestCount24h, r.Nonce, r.Status, r.Isolation, r.Note,
		  r.IssuedAt, r.BoundAt, r.ConsumedAt, r.ExpiresAt, r.TerminalAt, r.BindTicket,
	  )
	   return err
   }

  func (s *SQLStore) GetLease(ctx context.Context, id string) (ScreenLease, error) {
	   if err := s.ping(ctx); err != nil {
		   return ScreenLease{}, err
	   }
	   var r LeaseRow
	  err := s.db.QueryRowContext(ctx, sqlSelectLease, id).Scan(
		  &r.ID, &r.UserID, &r.Dest, &r.Asset, &r.Amount, &r.Kind, &r.Pack, &r.Mode, &r.PackFP,
		   &r.HaltGen, &r.PackGen, &r.WindowDigest, &r.Velocity1h, &r.Velocity24h,
		   &r.DestCount, &r.DestCount24h, &r.Nonce, &r.Status, &r.Isolation, &r.Note,
		   &r.IssuedAt, &r.BoundAt, &r.ConsumedAt, &r.ExpiresAt, &r.TerminalAt, &r.BindTicket,
	  )
	   if err == sql.ErrNoRows {
		  return ScreenLease{}, ErrLeaseNotFound
	  }
	   if err != nil {
		   return ScreenLease{}, err
	  }
	   return rowToLease(r), nil
   }

  func (s *SQLStore) FindLeaseByNonce(ctx context.Context, user, nonce string) (ScreenLease, bool, error) {
	   if err := s.ping(ctx); err != nil {
		  return ScreenLease{}, false, err
	  }
	  var id string
	   err := s.db.QueryRowContext(ctx, sqlSelectLeaseNonce, user, nonce).Scan(&id)
	   if err == sql.ErrNoRows {
		   return ScreenLease{}, false, nil
	   }
	  if err != nil {
		   return ScreenLease{}, false, err
	  }
	   l, err := s.GetLease(ctx, id)
	  return l, err == nil, err
  }

   func (s *SQLStore) ListOpenLeases(ctx context.Context) ([]ScreenLease, error) {
	  if err := s.ping(ctx); err != nil {
		  return nil, err
	  }
	  rows, err := s.db.QueryContext(ctx, sqlSelectOpenLeases)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := []ScreenLease{}
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

  func (s *SQLStore) ListUserOpen(ctx context.Context, user string) ([]ScreenLease, error) {
	  all, err := s.ListOpenLeases(ctx)
	  if err != nil {
		   return nil, err
	  }
	  out := []ScreenLease{}
	   for _, l := range all {
		   if l.UserID == user {
			   out = append(out, l)
		  }
	   }
	  return out, nil
  }

   func (s *SQLStore) UpdateLease(ctx context.Context, lease ScreenLease) error {
	  if err := s.ping(ctx); err != nil {
		  return err
	  }
	  r := leaseToRow(lease)
	   _, err := s.db.ExecContext(ctx, sqlUpdateLease, r.Status, r.Note, r.BoundAt, r.ConsumedAt, r.TerminalAt, r.BindTicket, r.ID)
	   return err
   }

   func (s *SQLStore) PutProof(ctx context.Context, p ScreenDriftProof) error {
	  if err := s.ping(ctx); err != nil {
		   return err
	  }
	   r := proofToRow(p)
	  _, err := s.db.ExecContext(ctx, sqlInsertProof,
		  r.ID, r.LeaseID, r.UserID, r.Kind, r.Status, r.ExpectedHalt, r.ObservedHalt,
		  r.ExpectedPack,
		   r.ObservedPack, r.ExpectedFP, r.ObservedFP, r.Digest,
		  r.Isolation, r.Note, r.OperatorA, r.OperatorB, r.OpenedAt, r.ProvenAt, r.ClosedAt,
	   )
	  return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (ScreenDriftProof, error) {
	  if err := s.ping(ctx); err != nil {
		  return ScreenDriftProof{}, err
	  }
	   var r ProofRow
	  err := s.db.QueryRowContext(ctx, sqlSelectProof, id).Scan(
		   &r.ID, &r.LeaseID, &r.UserID, &r.Kind, &r.Status, &r.ExpectedHalt, &r.ObservedHalt,
		  &r.ExpectedPack, &r.ObservedPack, &r.ExpectedFP, &r.ObservedFP, &r.Digest,
		   &r.Isolation, &r.Note, &r.OperatorA, &r.OperatorB, &r.OpenedAt, &r.ProvenAt, &r.ClosedAt,
	   )
	  if err == sql.ErrNoRows {
		   return ScreenDriftProof{}, ErrProofNotFound
	   }
	   if err != nil {
		  return ScreenDriftProof{}, err
	  }
	  return rowToProof(r), nil
  }

  func (s *SQLStore) FindProof(ctx context.Context, leaseID string, kind ProofKind, halt, packGen int64) (ScreenDriftProof, bool, error) {
	  _ = halt
	  _ = packGen
	  list,
		  err := s.ListProofsByLease(ctx, leaseID)
	   if err != nil {
		  return ScreenDriftProof{}, false, err
	   }
	   for _, p := range list {
		   if p.Kind == kind && p.Status != ProofClosed {
			  return p, true, nil
		  }
	  }
	  return ScreenDriftProof{}, false, nil
   }

   func (s *SQLStore) ListOpenProofs(ctx context.Context) ([]ScreenDriftProof, error) {
	  if err := s.ping(ctx); err != nil {
		  return nil, err
	   }
	   rows, err := s.db.QueryContext(ctx, `SELECT id FROM risk_screen_proofs WHERE status <> 'closed'`)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := []ScreenDriftProof{}
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

   func (s *SQLStore) UpdateProof(ctx context.Context, p ScreenDriftProof) error {
	   if err := s.ping(ctx); err != nil {
		  return err
	   }
	  r := proofToRow(p)
	  _, err := s.db.ExecContext(ctx, sqlUpdateProof, r.Status, r.Digest, r.Note, r.OperatorA, r.OperatorB, r.ProvenAt, r.ClosedAt, r.ID)
	   return err
  }

   func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string) ([]ScreenDriftProof, error) {
	  if err := s.ping(ctx); err != nil {
		   return nil, err
	  }
	   rows, err := s.db.QueryContext(ctx, `SELECT id FROM risk_screen_proofs WHERE lease_id=?`, leaseID)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  out := []ScreenDriftProof{}
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

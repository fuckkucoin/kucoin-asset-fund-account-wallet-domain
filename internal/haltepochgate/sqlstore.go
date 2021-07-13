package haltepochgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SQLStore 关系库实现。

type SQLStore struct {
	db *sql.DB
}

   func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
  }

   func (s *SQLStore) ping() error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	  }
	   return nil
   }

   func (s *SQLStore) GetEpoch(ctx context.Context, scope ScopeKey) (*HaltEpoch, error) {
	  if err := s.ping(); err != nil {
		   return nil, err
	  }
	  row := s.db.QueryRowContext(ctx, `
SELECT epoch_id, scope_key, scope, chain, asset, generation, state, prev_gen, lane_open,
       mutex_holder, mutex_kind, actor, reason, quorum_id, idem_key, window_case_id,
       created_at, updated_at, cleared_at, engaged_at
  FROM custody_halt_epochs WHERE scope_key = ?`, scope.String())
	  var e HaltEpoch
	  var cleared, engaged sql.NullTime
	   var mutexH, mutexK, quorum, idem, wcase sql.NullString
	  var lane int
	  if err := row.Scan(&e.EpochID, &e.Scope.Scope, &e.Scope.Scope, &e.Scope.Chain, &e.Scope.Asset,
		   &e.Generation, &e.State, &e.PrevGen, &lane, &mutexH, &mutexK, &e.Actor, &e.Reason,
		  &quorum, &idem, &wcase, &e.CreatedAt, &e.UpdatedAt, &cleared, &engaged); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrEpochNotFound
		  }
		  return nil, err
	  }
	  e.Scope = scope
	  e.LaneOpen = lane != 0
	  e.MutexHolder = mutexH.String
	   e.MutexKind = mutexK.String
	  e.QuorumID = quorum.String
	   e.IdemKey = idem.String
	  e.WindowCase = wcase.String
	   if cleared.Valid {
		  e.ClearedAt = cleared.Time
	  }
	   if engaged.Valid {
		   e.EngagedAt = engaged.Time
	   }
	   return &e, nil
   }

  func (s *SQLStore) PutEpoch(ctx context.Context, e HaltEpoch) error {
	  if err := s.ping(); err != nil {
		   return err
	  }
	   lane := 0
	  if e.LaneOpen {
		   lane = 1
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO custody_halt_epochs (
  epoch_id, scope_key, scope, chain, asset, generation, state, prev_gen, lane_open,
  mutex_holder, mutex_kind, actor, reason, quorum_id, idem_key, window_case_id,
  created_at, updated_at, cleared_at, engaged_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  generation=VALUES(generation), state=VALUES(state), prev_gen=VALUES(prev_gen),
  lane_open=VALUES(lane_open), mutex_holder=VALUES(mutex_holder), mutex_kind=VALUES(mutex_kind),
  actor=VALUES(actor), reason=VALUES(reason), quorum_id=VALUES(quorum_id),
  idem_key=VALUES(idem_key), window_case_id=VALUES(window_case_id),
  updated_at=VALUES(updated_at), cleared_at=VALUES(cleared_at), engaged_at=VALUES(engaged_at)`,
		  e.EpochID, e.Scope.String(), e.Scope.Scope, e.Scope.Chain, e.Scope.Asset,
		   e.Generation, string(e.State), e.PrevGen, lane,
		   nullStr(e.MutexHolder),
		   nullStr(e.MutexKind), e.Actor, e.Reason,
		  nullStr(e.QuorumID), nullStr(e.IdemKey), nullStr(e.WindowCase),
		  e.CreatedAt, e.UpdatedAt, nullTime(e.ClearedAt), nullTime(e.EngagedAt))
	  return err
   }

   func (s *SQLStore) ListEpochs(ctx context.Context, limit int) ([]HaltEpoch, error) {
	   if err := s.ping(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		  limit = 100
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT epoch_id, scope_key, scope, chain, asset, generation, state, prev_gen, lane_open,
       actor, reason, created_at, updated_at
  FROM custody_halt_epochs ORDER BY updated_at DESC LIMIT ?`, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  out := make([]HaltEpoch, 0)
	   for rows.Next() {
		   var e HaltEpoch
		   var key string
		   var lane int
		   if err := rows.Scan(&e.EpochID,
			  &key, &e.Scope.Scope, &e.Scope.Chain, &e.Scope.Asset,
			  &e.Generation, &e.State, &e.PrevGen, &lane, &e.Actor, &e.Reason, &e.CreatedAt, &e.UpdatedAt); err != nil {
			  return nil, err
		   }
		   e.LaneOpen = lane != 0
		   out = append(out, e)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) GetIntent(ctx context.Context, id string) (*FundIntent, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	   }
	   row := s.db.QueryRowContext(ctx, `
SELECT intent_id, scope_key, scope, chain, asset, lane, generation, state, account, asset_code,
       amount, idem_key, allow_ticket, proof_id, created_at, updated_at
  FROM custody_halt_intents WHERE intent_id = ?`, id)
	  in, err := scanIntent(row)
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrIntentNotFound
	  }
	  return in, err
   }

  func (s *SQLStore) GetIntentByIdem(ctx context.Context, idem string) (*FundIntent, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT intent_id, scope_key, scope, chain, asset, lane, generation, state, account, asset_code,
       amount, idem_key, allow_ticket, proof_id, created_at, updated_at
  FROM custody_halt_intents WHERE idem_key = ?`, idem)
	   in, err := scanIntent(row)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrIntentNotFound
	   }
	  return in, err
  }

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanIntent(row rowScanner) (*FundIntent, error) {
	  var in FundIntent
	   var key, assetCode, proof sql.NullString
	  var account, amount, idem, allow sql.NullString
	   if err := row.Scan(&in.IntentID, &key, &in.Scope.Scope, &in.Scope.Chain, &in.Scope.Asset,
		  &in.Lane,
		  &in.Generation, &in.State, &account, &assetCode, &amount, &idem, &allow, &proof,
		   &in.CreatedAt, &in.UpdatedAt); err != nil {
		   return nil, err
	   }
	   in.Account = account.String
	   if assetCode.Valid {
		   in.Asset = assetCode.String
	  }
	   in.Amount = amount.String
	   in.IdemKey = idem.String
	  in.AllowTick = allow.String
	   in.ProofID = proof.String
	   return &in, nil
   }

   func (s *SQLStore) PutIntent(ctx context.Context, in FundIntent) error {
	   if err := s.ping(); err != nil {
		   return err
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO custody_halt_intents (
  intent_id, scope_key, scope, chain, asset, lane, generation, state, account, asset_code,
  amount, idem_key, allow_ticket, proof_id, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), proof_id=VALUES(proof_id), updated_at=VALUES(updated_at)`,
		  in.IntentID, in.Scope.String(), in.Scope.Scope, in.Scope.Chain, in.Scope.Asset,
		   string(in.Lane),
		  in.Generation, string(in.State), nullStr(in.Account), nullStr(in.Asset),
		   nullStr(in.Amount), nullStr(in.IdemKey), nullStr(in.AllowTick), nullStr(in.ProofID),
		   in.CreatedAt, in.UpdatedAt)
	   return err
   }

   func (s *SQLStore) ListIntentsByScope(ctx context.Context, scope ScopeKey, states []IntentState, limit int) ([]FundIntent, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	   }
	  if limit <= 0 {
		   limit = 200
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT intent_id, scope_key, scope, chain, asset, lane, generation, state, account, asset_code,
       amount, idem_key, allow_ticket, proof_id, created_at, updated_at
  FROM custody_halt_intents WHERE scope_key = ? LIMIT ?`, scope.String(), limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   allow := map[IntentState]bool{}
	   for _, st := range states {
		   allow[st] = true
	  }
	   out := make([]FundIntent, 0)
	  for rows.Next() {
		   in, err := scanIntent(rows)
		   if err != nil {
			   return nil, err
		  }
		   if len(allow) > 0 && !allow[in.State] {
			  continue
		   }
		   out = append(out, *in)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) ListAllowedIntents(ctx context.Context, limit int) ([]FundIntent, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	   }
	  if limit <= 0 {
		   limit = 200
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT intent_id, scope_key, scope, chain, asset, lane, generation, state, account, asset_code,
       amount, idem_key, allow_ticket, proof_id, created_at, updated_at
  FROM custody_halt_intents WHERE state = 'allowed' LIMIT ?`, limit)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]FundIntent, 0)
	   for rows.Next() {
		  in, err := scanIntent(rows)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, *in)
	   }
	  return out, rows.Err()
   }

   func (s *SQLStore) GetProof(ctx context.Context, id string) (*SlipProof, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	  }
	  row := s.db.QueryRowContext(ctx, `
SELECT proof_id, epoch_id, scope_key, scope, chain, asset, generation, carried_gen, kind, state,
       lane, intent_id, case_id, actor, summary, fail_closed, disposition, digest,
       opened_at, proven_at, closed_at, exported_at
  FROM custody_slip_proofs WHERE proof_id = ?`, id)
	  p, err := scanProof(row)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrProofNotFound
	   }
	   return p, err
   }

   func scanProof(row rowScanner) (*SlipProof, error) {
	  var p SlipProof
	   var key string
	  var intent, caseID, actor sql.NullString
	   var proven, closed, exported sql.NullTime
	   var fail int
	  if err := row.Scan(&p.ProofID, &p.EpochID, &key, &p.Scope.Scope, &p.Scope.Chain, &p.Scope.Asset,
		   &p.Generation, &p.CarriedGen, &p.Kind, &p.State, &p.Lane, &intent, &caseID, &actor,
		   &p.Summary, &fail, &p.Disposition, &p.Digest, &p.OpenedAt, &proven, &closed, &exported); err != nil {
		   return nil, err
	  }
	   p.IntentID = intent.String
	   p.CaseID = caseID.String
	   p.Actor = actor.String
	  p.FailClosed = fail != 0
	  if proven.Valid {
		   p.ProvenAt = proven.Time
	  }
	   if closed.Valid {
		  p.ClosedAt = closed.Time
	   }
	  if exported.Valid {
		  p.ExportedAt = exported.Time
	  }
	   return &p, nil
  }

   func (s *SQLStore) PutProof(ctx context.Context, p SlipProof) error {
	  if err := s.ping(); err != nil {
		   return err
	  }
	   fail := 0
	  if p.FailClosed {
		   fail = 1
	   }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO custody_slip_proofs (
  proof_id, epoch_id, scope_key, scope, chain, asset, generation, carried_gen, kind, state,
  lane, intent_id, case_id, actor, summary, fail_closed, disposition, digest,
  opened_at, proven_at, closed_at, exported_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), digest=VALUES(digest), proven_at=VALUES(proven_at),
  closed_at=VALUES(closed_at), exported_at=VALUES(exported_at)`,
		   p.ProofID, p.EpochID, p.Scope.String(), p.Scope.Scope, p.Scope.Chain, p.Scope.Asset,
		   p.Generation,
		   p.CarriedGen, string(p.Kind), string(p.State), string(p.Lane),
		  nullStr(p.IntentID), nullStr(p.CaseID), nullStr(p.Actor), p.Summary, fail, p.Disposition, p.Digest,
		  p.OpenedAt,
		   nullTime(p.ProvenAt), nullTime(p.ClosedAt), nullTime(p.ExportedAt))
	   return err
  }

   func (s *SQLStore) ListProofs(ctx context.Context, state SlipState, limit int) ([]SlipProof, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	  }
	   if limit <= 0 {
		  limit = 200
	  }
	  q := `
SELECT proof_id, epoch_id, scope_key, scope, chain, asset, generation, carried_gen, kind, state,
       lane, intent_id, case_id, actor, summary, fail_closed, disposition, digest,
       opened_at, proven_at, closed_at, exported_at
  FROM custody_slip_proofs`
	  args := []any{}
	   if state != "" {
		   q += ` WHERE state = ?`
		  args = append(args, string(state))
	  }
	   q += ` ORDER BY opened_at DESC LIMIT ?`
	   args = append(args, limit)
	   rows,
		   err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  out := make([]SlipProof, 0)
	   for rows.Next() {
		  p, err := scanProof(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, *p)
	  }
	  return out, rows.Err()
   }

   func (s *SQLStore) ListProofsByScope(ctx context.Context, scope ScopeKey, limit int) ([]SlipProof, error) {
	  if err := s.ping(); err != nil {
		   return nil, err
	  }
	   if limit <= 0 {
		  limit = 200
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT proof_id, epoch_id, scope_key, scope, chain, asset, generation, carried_gen, kind, state,
       lane, intent_id, case_id, actor, summary, fail_closed, disposition, digest,
       opened_at, proven_at, closed_at, exported_at
  FROM custody_slip_proofs WHERE scope_key = ? LIMIT ?`, scope.String(), limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]SlipProof, 0)
	   for rows.Next() {
		   p, err := scanProof(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, *p)
	   }
	   return out, rows.Err()
   }

  func (s *SQLStore) GetCase(ctx context.Context, id string) (*ClearWindowCase, error) {
	  if err := s.ping(); err != nil {
		   return nil, err
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT case_id, epoch_id, scope_key, scope, chain, asset, clear_gen, prev_gen, state,
       revoked_intents, slip_count, actor, reason, opened_at, sealed_at
  FROM custody_clear_window_cases WHERE case_id = ?`, id)
	  var c ClearWindowCase
	  var key string
	  var sealed sql.NullTime
	   if err := row.Scan(&c.CaseID, &c.EpochID, &key, &c.Scope.Scope, &c.Scope.Chain, &c.Scope.Asset,
		  &c.ClearGen,
		  &c.PrevGen, &c.State, &c.RevokedIntents, &c.SlipCount, &c.Actor, &c.Reason,
		  &c.OpenedAt, &sealed); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrCaseNotFound
		  }
		   return nil, err
	   }
	   if sealed.Valid {
		  c.SealedAt = sealed.Time
	  }
	  return &c, nil
   }

  func (s *SQLStore) PutCase(ctx context.Context, c ClearWindowCase) error {
	  if err := s.ping(); err != nil {
		  return err
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO custody_clear_window_cases (
  case_id, epoch_id, scope_key, scope, chain, asset, clear_gen, prev_gen, state,
  revoked_intents, slip_count, actor, reason, opened_at, sealed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), revoked_intents=VALUES(revoked_intents), slip_count=VALUES(slip_count),
  sealed_at=VALUES(sealed_at)`,
		  c.CaseID, c.EpochID, c.Scope.String(), c.Scope.Scope, c.Scope.Chain, c.Scope.Asset,
		  c.ClearGen, c.PrevGen, string(c.State), c.RevokedIntents, c.SlipCount, c.Actor, c.Reason,
		   c.OpenedAt, nullTime(c.SealedAt))
	   return err
  }

   func (s *SQLStore) ListOpenCases(ctx context.Context, limit int) ([]ClearWindowCase, error) {
	  if err := s.ping(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		   limit = 100
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT case_id, epoch_id, scope_key, scope, chain, asset, clear_gen, prev_gen, state,
       revoked_intents, slip_count, actor, reason, opened_at, sealed_at
  FROM custody_clear_window_cases WHERE state <> 'sealed' LIMIT ?`, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]ClearWindowCase, 0)
	   for rows.Next() {
		   var c ClearWindowCase
		  var key string
		   var sealed sql.NullTime
		  if err := rows.Scan(&c.CaseID, &c.EpochID, &key, &c.Scope.Scope, &c.Scope.Chain, &c.Scope.Asset,
			  &c.ClearGen,
			  &c.PrevGen, &c.State, &c.RevokedIntents, &c.SlipCount, &c.Actor, &c.Reason,
			   &c.OpenedAt, &sealed); err != nil {
			  return nil, err
		  }
		  if sealed.Valid {
			  c.SealedAt = sealed.Time
		   }
		  out = append(out, c)
	   }
	  return out, rows.Err()
  }

  func (s *SQLStore) GetQuorum(ctx context.Context, id string) (*QuorumBundle, error) {
	   if err := s.ping(); err != nil {
		   return nil, err
	  }
	  row := s.db.QueryRowContext(ctx, `SELECT quorum_id, epoch_id, scope_key, threshold, votes_json, satisfied, created_at FROM custody_halt_quorum WHERE quorum_id = ?`, id)
	   return scanQuorum(row)
  }

   func (s *SQLStore) GetQuorumByEpoch(ctx context.Context, epochID string) (*QuorumBundle, error) {
	   if err := s.ping(); err != nil {
		   return nil, err
	   }
	  row := s.db.QueryRowContext(ctx, `SELECT quorum_id, epoch_id, scope_key, threshold, votes_json, satisfied, created_at FROM custody_halt_quorum WHERE epoch_id = ?`, epochID)
	   return scanQuorum(row)
  }

  func scanQuorum(row rowScanner) (*QuorumBundle, error) {
	   var q QuorumBundle
	  var key, raw string
	  var sat int
	   if err := row.Scan(&q.QuorumID, &q.EpochID, &key, &q.Threshold, &raw, &sat, &q.CreatedAt); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrQuorumDenied
		  }
		   return nil, err
	   }
	   q.Satisfied = sat != 0
	  if raw != "" {
		   _ = json.Unmarshal([]byte(raw), &q.Votes)
	   }
	   return &q, nil
   }

   func (s *SQLStore) PutQuorum(ctx context.Context, q QuorumBundle) error {
	   if err := s.ping(); err != nil {
		  return err
	   }
	   raw, _ := json.Marshal(q.Votes)
	   sat := 0
	  if q.Satisfied {
		   sat = 1
	  }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO custody_halt_quorum (quorum_id, epoch_id, scope_key, threshold, votes_json, satisfied, created_at)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE votes_json=VALUES(votes_json), satisfied=VALUES(satisfied), threshold=VALUES(threshold)`,
		   q.QuorumID, q.EpochID, q.Scope.String(), q.Threshold, string(raw), sat, q.CreatedAt)
	  return err
  }

  func (s *SQLStore) AppendAudit(ctx context.Context, e AuditEvent) error {
	   if err := s.ping(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, ` 
INSERT INTO custody_halt_epoch_audits (event_id, kind, scope_key, actor, detail, generation, occurred_at)
VALUES (?,?,?,?,?,?,?)`, e.EventID, e.Kind, e.Scope, e.Actor, e.Detail, e.Generation, e.At)
	  return err
  }

  func (s *SQLStore) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	  if err := s.ping(); err != nil {
		   return nil, err
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT event_id, kind, scope_key, actor, detail, generation, occurred_at
  FROM custody_halt_epoch_audits ORDER BY occurred_at DESC LIMIT ?`, limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]AuditEvent, 0)
	   for rows.Next() {
		   var e AuditEvent
		   if err := rows.Scan(&e.EventID, &e.Kind, &e.Scope, &e.Actor, &e.Detail, &e.Generation, &e.At); err != nil {
			   return nil, err
		  }
		   out = append(out, e)
	  }
	  return out, rows.Err()
  }

  func nullStr(s string) any {
	   if strings.TrimSpace(s) == "" {
		   return nil
	   }
	  return s
   }

   func nullTime(t time.Time) any {
	  if t.IsZero() {
		   return nil
	  }
	   return t
   }

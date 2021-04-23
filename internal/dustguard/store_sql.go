package dustguard

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// SQLStore SQL 实现，封装 *persist.SQLDB。
type SQLStore struct {
	db    *persist.SQLDB
	clock func() time.Time
}

// NewSQLStore 构造 SQL 仓储。

  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	   return &SQLStore{db: db, clock: func() time.Time { return time.Now().UTC() }}
  }

// ─── 信号 ─────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveSignal(ctx context.Context, sig DustSignal) error {
	  if sig.ID == "" {
		  sig.ID = uuid.NewString()
	  }
	  _, err := s.db.DB.ExecContext(ctx,
		  `INSERT INTO dust_signal  
		 (id,chain,address,from_addr,asset,amount,tx_hash,signal_kind,seen_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		  sig.ID,
		   sig.Chain, sig.Address, sig.FromAddr, sig.Asset,
		  sig.Amount, sig.TxHash, string(sig.SignalKind), sig.SeenAt.UTC(),
	   )
	  return err
   }

   func (s *SQLStore) ListSignals(ctx context.Context, chain, address string, since time.Time) ([]DustSignal, error) {
	  rows, err := s.db.DB.QueryContext(ctx,
		  `SELECT id,chain,address,from_addr,asset,amount,tx_hash,signal_kind,seen_at 
		 FROM dust_signal
		 WHERE chain=? AND address=? AND seen_at>=?
		 ORDER BY seen_at DESC LIMIT 1000`,
		  strings.ToUpper(chain), address, since.UTC(),
	   )
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []DustSignal
	  for rows.Next() {
		   var sig DustSignal
		   var kind string
		   var seenAt time.Time
		   if err := rows.Scan(&sig.ID, &sig.Chain, &sig.Address, &sig.FromAddr,
			   &sig.Asset, &sig.Amount, &sig.TxHash, &kind, &seenAt); err != nil {
			   return nil, err
		  }
		  sig.SignalKind = SignalKind(kind)
		   sig.SeenAt = seenAt.UTC()
		   out = append(out, sig)
	   }
	   return out, rows.Err()
  }

// ─── 分值 ─────────────────────────────────────────────────────────────────

  func (s *SQLStore) UpsertScore(ctx context.Context, score AttackScore) error {
	   if score.ID == "" {
		   score.ID = uuid.NewString()
	  }
	   sf := func(b bool) int {
		  if b {
			   return 1
		   }
		  return 0
	   }
	   _, err := s.db.DB.ExecContext(ctx,
		  `INSERT INTO dust_attack_score 
		 (id,chain,address,family,score,level,flood_count,cluster_n,surface_hit,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   score=VALUES(score), level=VALUES(level),
		   flood_count=VALUES(flood_count), cluster_n=VALUES(cluster_n),
		   surface_hit=VALUES(surface_hit), updated_at=VALUES(updated_at)`,
		   score.ID, score.Chain, score.Address, score.Family,
		   score.Score,
		  string(score.Level), score.FloodCount, score.ClusterN,
		  sf(score.SurfaceHit), score.UpdatedAt.UTC(),
	   )
	  return err
   }

   func (s *SQLStore) GetScore(ctx context.Context, chain, address string) (AttackScore, bool) {
	  row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id,chain,address,family,score,level,flood_count,cluster_n,surface_hit,updated_at
		 FROM dust_attack_score WHERE chain=? AND address=? LIMIT 1`,
		   strings.ToUpper(chain), address,
	  )
	   var sc AttackScore
	   var level string
	  var surfaceHit int
	  var updatedAt time.Time
	  if err := row.Scan(&sc.ID, &sc.Chain, &sc.Address, &sc.Family,
		  &sc.Score, &level, &sc.FloodCount, &sc.ClusterN, &surfaceHit, &updatedAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return AttackScore{}, false
		  }
		  return AttackScore{}, false
	   }
	  sc.Level = AttackLevel(level)
	   sc.SurfaceHit = surfaceHit != 0
	  sc.UpdatedAt = updatedAt.UTC()
	   return sc, true
  }

  func (s *SQLStore) ListScores(ctx context.Context, chain string, minLevel AttackLevel) ([]AttackScore, error) {
	   minScore := levelMinScore(minLevel)
	   var rows *sql.Rows
	  var err error
	   if strings.TrimSpace(chain) == "" {
		  rows, err = s.db.DB.QueryContext(ctx,
			   `SELECT id,chain,address,family,score,level,flood_count,cluster_n,surface_hit,updated_at
			 FROM dust_attack_score WHERE score>=? ORDER BY score DESC LIMIT 500`,
			  minScore,
		   )
	  } else {
		   rows, err = s.db.DB.QueryContext(ctx,
			   `SELECT id,chain,address,family,score,level,flood_count,cluster_n,surface_hit,updated_at
			 FROM dust_attack_score WHERE chain=? AND score>=? ORDER BY score DESC LIMIT 500`,
			   strings.ToUpper(chain), minScore,
		  )
	  }
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanScores(rows)
  }

   func scanScores(rows *sql.Rows) ([]AttackScore, error) {
	  var out []AttackScore
	  for rows.Next() {
		   var sc AttackScore
		  var level string
		  var surfaceHit int
		   var updatedAt time.Time
		   if err := rows.Scan(&sc.ID,
			   &sc.Chain, &sc.Address, &sc.Family,
			   &sc.Score, &level, &sc.FloodCount, &sc.ClusterN, &surfaceHit, &updatedAt); err != nil {
			  return nil, err
		   }
		   sc.Level = AttackLevel(level)
		   sc.SurfaceHit = surfaceHit != 0
		  sc.UpdatedAt = updatedAt.UTC()
		   out = append(out, sc)
	   }
	   return out, rows.Err()
   }

// ─── 隔离案 ───────────────────────────────────────────────────────────────

  func (s *SQLStore) CreateCase(ctx context.Context, c QuarantineCase) error {
	  _, err := s.db.DB.ExecContext(ctx,
		   `INSERT INTO addrpool_guard_quarantine 
		 (id,chain,address,family,user_id,state,trigger_score,trigger_level,trigger_reason,
		  forensics_until,opened_by,closed_by,rotate_plan_id,event_fact_id,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   c.ID, c.Chain, c.Address, c.Family, c.UserID, string(c.State),
		  c.TriggerScore, string(c.TriggerLevel), c.TriggerReason,
		   c.ForensicsUntil.UTC(), c.OpenedBy, c.ClosedBy,
		   c.RotatePlanID, c.EventFactID, c.CreatedAt.UTC(), c.UpdatedAt.UTC(),
	  )
	  return err
  }

  func (s *SQLStore) UpdateCase(ctx context.Context, c QuarantineCase) error {
	   res,
		  err := s.db.DB.ExecContext(ctx,
		   `UPDATE addrpool_guard_quarantine
		 SET state=?, closed_by=?, rotate_plan_id=?, event_fact_id=?, updated_at=?
		 WHERE id=?`,
		   string(c.State), c.ClosedBy, c.RotatePlanID, c.EventFactID, c.UpdatedAt.UTC(), c.ID,
	  )
	  if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrCaseNotFound
	  }
	   return nil
   }

   func (s *SQLStore) GetCase(ctx context.Context, id string) (QuarantineCase, error) {
	  row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id,chain,address,family,user_id,state,trigger_score,trigger_level,trigger_reason,
		        forensics_until,opened_by,closed_by,rotate_plan_id,event_fact_id,created_at,updated_at
		 FROM addrpool_guard_quarantine WHERE id=?`, id)
	  c, err := scanCase(row)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return QuarantineCase{}, ErrCaseNotFound
		   }
		   return QuarantineCase{}, err
	  }
	  return c, nil
   }

   func (s *SQLStore) GetActiveCase(ctx context.Context, chain, address string) (QuarantineCase, bool) {
	  row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id,chain,address,family,user_id,state,trigger_score,trigger_level,trigger_reason,
		        forensics_until,opened_by,closed_by,rotate_plan_id,event_fact_id,created_at,updated_at
		 FROM addrpool_guard_quarantine
		 WHERE chain=? AND address=? AND state IN ('open','holding')
		 ORDER BY created_at DESC LIMIT 1`,
		   strings.ToUpper(chain), address,
	   )
	   c, err := scanCase(row)
	  if err != nil {
		   return QuarantineCase{}, false
	  }
	   return c, true
   }

  func (s *SQLStore) ListCases(ctx context.Context, state QuarantineState, chain string) ([]QuarantineCase, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  var (
		   rows *sql.Rows
		  err  error
	   )
	   baseQ := `SELECT id,chain,address,family,user_id,state,trigger_score,trigger_level,trigger_reason,
	                 forensics_until,opened_by,closed_by,rotate_plan_id,event_fact_id,created_at,updated_at
	          FROM addrpool_guard_quarantine`
	  switch {
	  case state != "" && ch != "":
		  rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE state=? AND chain=? ORDER BY created_at DESC LIMIT 500`, string(state), ch)
	  case state != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE state=? ORDER BY created_at DESC LIMIT 500`, string(state))
	   case ch != "":
		  rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE chain=? ORDER BY created_at DESC LIMIT 500`, ch)
	  default:
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` ORDER BY created_at DESC LIMIT 500`)
	  }
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []QuarantineCase
	   for rows.Next() {
		   c, err := scanCaseRow(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, c)
	  }
	  return out, rows.Err()
  }

   func scanCase(row *sql.Row) (QuarantineCase, error) {
	   var c QuarantineCase
	  var state, trigLevel string
	   var forensics, created, updated time.Time
	   err := row.Scan(
		   &c.ID, &c.Chain, &c.Address, &c.Family, &c.UserID, &state,
		  &c.TriggerScore,
		  &trigLevel, &c.TriggerReason,
		   &forensics, &c.OpenedBy, &c.ClosedBy, &c.RotatePlanID, &c.EventFactID,
		  &created, &updated,
	  )
	   if err != nil {
		   return QuarantineCase{}, err
	   }
	  c.State = QuarantineState(state)
	   c.TriggerLevel = AttackLevel(trigLevel)
	  c.ForensicsUntil = forensics.UTC()
	   c.CreatedAt = created.UTC()
	   c.UpdatedAt = updated.UTC()
	  return c, nil
   }

   func scanCaseRow(rows *sql.Rows) (QuarantineCase, error) {
	   var c QuarantineCase
	  var state, trigLevel string
	   var forensics, created, updated time.Time
	  err := rows.Scan(
		  &c.ID, &c.Chain, &c.Address, &c.Family, &c.UserID, &state,
		  &c.TriggerScore, &trigLevel, &c.TriggerReason,
		  &forensics, &c.OpenedBy, &c.ClosedBy, &c.RotatePlanID, &c.EventFactID,
		   &created, &updated,
	  )
	   if err != nil {
		  return QuarantineCase{}, err
	   }
	  c.State = QuarantineState(state)
	   c.TriggerLevel = AttackLevel(trigLevel)
	   c.ForensicsUntil = forensics.UTC()
	   c.CreatedAt = created.UTC()
	   c.UpdatedAt = updated.UTC()
	   return c, nil
  }

// ─── 轮换计划 ─────────────────────────────────────────────────────────────

   func (s *SQLStore) CreatePlan(ctx context.Context, p RotatePlan) error {
	   var sealedAt *time.Time
	  if p.SealedAt != nil {
		   t := p.SealedAt.UTC()
		   sealedAt = &t
	   }
	   _, err := s.db.DB.ExecContext(ctx,
		  `INSERT INTO addrpool_guard_rotate_plan
		 (id,chain,old_address,new_address,user_id,case_id,state,trigger_kind,
		  approval_id,reason,executed_by,created_at,updated_at,sealed_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ID, p.Chain, p.OldAddress, p.NewAddress, p.UserID, p.CaseID,
		  string(p.State), string(p.Trigger), p.ApprovalID, p.Reason, p.ExecutedBy,
		   p.CreatedAt.UTC(), p.UpdatedAt.UTC(), sealedAt,
	  )
	   return err
   }

  func (s *SQLStore) UpdatePlan(ctx context.Context, p RotatePlan) error {
	  var sealedAt *time.Time
	  if p.SealedAt != nil {
		  t := p.SealedAt.UTC()
		   sealedAt = &t
	  }
	   res, err := s.db.DB.ExecContext(ctx,
		  `UPDATE addrpool_guard_rotate_plan
		 SET new_address=?, state=?, approval_id=?, executed_by=?, updated_at=?, sealed_at=?
		 WHERE id=?`,
		   p.NewAddress, string(p.State), p.ApprovalID, p.ExecutedBy, p.UpdatedAt.UTC(), sealedAt, p.ID,
	   )
	  if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrPlanNotFound
	   }
	   return nil
  }

   func (s *SQLStore) GetPlan(ctx context.Context, id string) (RotatePlan, error) {
	  row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id,chain,old_address,new_address,user_id,case_id,state,trigger_kind,
		        approval_id,reason,executed_by,created_at,updated_at,sealed_at
		 FROM addrpool_guard_rotate_plan WHERE id=?`, id)
	   var p RotatePlan
	  var state, trigger string
	   var created, updated time.Time
	   var sealedAt sql.NullTime
	  err := row.Scan(
		   &p.ID, &p.Chain, &p.OldAddress, &p.NewAddress, &p.UserID, &p.CaseID,
		  &state, &trigger, &p.ApprovalID, &p.Reason, &p.ExecutedBy,
		  &created, &updated, &sealedAt,
	   )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return RotatePlan{}, ErrPlanNotFound
		   }
		   return RotatePlan{}, err
	  }
	   p.State = RotateState(state)
	  p.Trigger = TriggerKind(trigger)
	   p.CreatedAt = created.UTC()
	  p.UpdatedAt = updated.UTC()
	  if sealedAt.Valid {
		   t := sealedAt.Time.UTC()
		   p.SealedAt = &t
	  }
	   return p, nil
  }

   func (s *SQLStore) ListPlans(ctx context.Context, state RotateState, chain string) ([]RotatePlan, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  baseQ := `SELECT id,chain,old_address,new_address,user_id,case_id,state,trigger_kind,
	                 approval_id,reason,executed_by,created_at,updated_at,sealed_at
	          FROM addrpool_guard_rotate_plan`
	  var rows *sql.Rows
	  var err error
	  switch {
	   case state != "" && ch != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE state=? AND chain=? ORDER BY created_at DESC LIMIT 500`, string(state), ch)
	  case state != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE state=? ORDER BY created_at DESC LIMIT 500`, string(state))
	   case ch != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE chain=? ORDER BY created_at DESC LIMIT 500`, ch)
	  default:
		  rows, err = s.db.DB.QueryContext(ctx, baseQ+` ORDER BY created_at DESC LIMIT 500`)
	   }
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []RotatePlan
	  for rows.Next() {
		   var p RotatePlan
		   var state2, trigger string
		   var created, updated time.Time
		   var sealedAt sql.NullTime
		  if err := rows.Scan(
			   &p.ID, &p.Chain, &p.OldAddress, &p.NewAddress, &p.UserID, &p.CaseID,
			  &state2, &trigger, &p.ApprovalID, &p.Reason, &p.ExecutedBy,
			  &created, &updated, &sealedAt,
		   ); err != nil {
			  return nil, err
		  }
		   p.State = RotateState(state2)
		   p.Trigger = TriggerKind(trigger)
		   p.CreatedAt = created.UTC()
		   p.UpdatedAt = updated.UTC()
		   if sealedAt.Valid {
			   t := sealedAt.Time.UTC()
			   p.SealedAt = &t
		  }
		   out = append(out, p)
	   }
	  return out, rows.Err()
  }

// ─── 策略 ─────────────────────────────────────────────────────────────────

   func (s *SQLStore) CreatePolicy(ctx context.Context, pol PoolGuardPolicy) error {
	   aq := func(b bool) int {
		  if b {
			  return 1
		  }
		  return 0
	  }
	   _, err := s.db.DB.ExecContext(ctx,
		  `INSERT INTO addrpool_guard_policy
		 (id,version,chain,flood_threshold,flood_window_sec,cluster_n,score_attack,score_suspect,
		  water_level,forensics_sec,auto_quarantine,auto_rotate,active,note,created_by,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   pol.ID, pol.Version, pol.Chain, pol.FloodThreshold, pol.FloodWindowSec,
		   pol.ClusterN,
		  pol.ScoreAttack, pol.ScoreSuspect, pol.WaterLevel, pol.ForensicsSec,
		  aq(pol.AutoQuarantine), aq(pol.AutoRotate), aq(pol.Active),
		  pol.Note, pol.CreatedBy, pol.CreatedAt.UTC(), pol.UpdatedAt.UTC(),
	  )
	  return err
  }

  func (s *SQLStore) UpdatePolicy(ctx context.Context, pol PoolGuardPolicy) error {
	  aq := func(b bool) int {
		   if b {
			  return 1
		   }
		  return 0
	   }
	   res, err := s.db.DB.ExecContext(ctx,
		   `UPDATE addrpool_guard_policy 
		 SET flood_threshold=?,flood_window_sec=?,cluster_n=?,score_attack=?,score_suspect=?,
		     water_level=?,forensics_sec=?,auto_quarantine=?,auto_rotate=?,active=?,note=?,updated_at=?
		 WHERE id=?`,
		   pol.FloodThreshold, pol.FloodWindowSec, pol.ClusterN, pol.ScoreAttack, pol.ScoreSuspect,
		   pol.WaterLevel, pol.ForensicsSec, aq(pol.AutoQuarantine), aq(pol.AutoRotate), aq(pol.Active),
		   pol.Note,
		  pol.UpdatedAt.UTC(), pol.ID,
	   )
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrPolicyNotFound
	   }
	   return nil
   }

  func (s *SQLStore) GetPolicy(ctx context.Context, id string) (PoolGuardPolicy, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id,version,chain,flood_threshold,flood_window_sec,cluster_n,score_attack,score_suspect,
		        water_level,forensics_sec,auto_quarantine,auto_rotate,active,note,created_by,created_at,updated_at
		 FROM addrpool_guard_policy WHERE id=?`, id)
	  return scanPolicy(row)
  }

  func (s *SQLStore) ActivePolicy(ctx context.Context, chain string) (PoolGuardPolicy, bool) {
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  // 先链级策略，代次最新
	   row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id,version,chain,flood_threshold,flood_window_sec,cluster_n,score_attack,score_suspect,
		        water_level,forensics_sec,auto_quarantine,auto_rotate,active,note,created_by,created_at,updated_at
		 FROM addrpool_guard_policy
		 WHERE chain=? AND active=1
		 ORDER BY version DESC LIMIT 1`, ch)
	   if pol, err := scanPolicy(row); err == nil {
		  return pol, true
	   }
	   // 回退全局策略
	  row = s.db.DB.QueryRowContext(ctx,
		   `SELECT id,version,chain,flood_threshold,flood_window_sec,cluster_n,score_attack,score_suspect, 
		        water_level,forensics_sec,auto_quarantine,auto_rotate,active,note,created_by,created_at,updated_at
		 FROM addrpool_guard_policy
		 WHERE chain='' AND active=1
		 ORDER BY version DESC LIMIT 1`)
	  if pol, err := scanPolicy(row); err == nil {
		   return pol, true
	   }
	   return PoolGuardPolicy{}, false
   }

  func (s *SQLStore) ListPolicies(ctx context.Context,
	   chain string) ([]PoolGuardPolicy, error) {
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	   var rows *sql.Rows
	   var err error
	   if ch == "" {
		   rows,
			   err = s.db.DB.QueryContext(ctx,
			   `SELECT id,version,chain,flood_threshold,flood_window_sec,cluster_n,score_attack,score_suspect,
			        water_level,forensics_sec,auto_quarantine,auto_rotate,active,note,created_by,created_at,updated_at
			 FROM addrpool_guard_policy ORDER BY chain, version DESC LIMIT 200`)
	  } else {
		  rows, err = s.db.DB.QueryContext(ctx,
			   `SELECT id,version,chain,flood_threshold,flood_window_sec,cluster_n,score_attack,score_suspect,
			        water_level,forensics_sec,auto_quarantine,auto_rotate,active,note,created_by,created_at,updated_at
			 FROM addrpool_guard_policy WHERE chain=? OR chain=''
			 ORDER BY chain DESC, version DESC LIMIT 200`, ch)
	  }
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []PoolGuardPolicy
	  for rows.Next() {
		   pol, err := scanPolicyRow(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, pol)
	   }
	   return out, rows.Err()
  }

   func scanPolicy(row *sql.Row) (PoolGuardPolicy, error) {
	   var pol PoolGuardPolicy
	  var aq, ar, act int
	   var created, updated time.Time
	  err := row.Scan(
		   &pol.ID, &pol.Version, &pol.Chain, &pol.FloodThreshold, &pol.FloodWindowSec,
		  &pol.ClusterN, &pol.ScoreAttack, &pol.ScoreSuspect, &pol.WaterLevel, &pol.ForensicsSec,
		   &aq, &ar, &act, &pol.Note, &pol.CreatedBy, &created, &updated,
	   )
	  if err != nil {
		   return PoolGuardPolicy{}, err
	   }
	  pol.AutoQuarantine = aq != 0
	  pol.AutoRotate = ar != 0
	  pol.Active = act != 0
	   pol.CreatedAt = created.UTC()
	   pol.UpdatedAt = updated.UTC()
	  return pol, nil
   }

  func scanPolicyRow(rows *sql.Rows) (PoolGuardPolicy, error) {
	  var pol PoolGuardPolicy
	  var aq, ar, act int
	  var created, updated time.Time
	  err := rows.Scan(
		   &pol.ID, &pol.Version, &pol.Chain, &pol.FloodThreshold, &pol.FloodWindowSec,
		  &pol.ClusterN, &pol.ScoreAttack, &pol.ScoreSuspect, &pol.WaterLevel, &pol.ForensicsSec,
		   &aq,
		   &ar, &act, &pol.Note, &pol.CreatedBy, &created, &updated,
	  )
	   if err != nil {
		  return PoolGuardPolicy{}, err
	   }
	  pol.AutoQuarantine = aq != 0
	   pol.AutoRotate = ar != 0
	  pol.Active = act != 0
	   pol.CreatedAt = created.UTC()
	   pol.UpdatedAt = updated.UTC()
	  return pol, nil
   }

// ─── 防护单 ───────────────────────────────────────────────────────────────

   func (s *SQLStore) CreateTicket(ctx context.Context, t GuardTicket) error {
	  _, err := s.db.DB.ExecContext(ctx,
		   `INSERT INTO addrpool_guard_ticket
		 (id,chain,address,user_id,tx_hash,asset,amount,case_id,state,reason,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		   t.ID, t.Chain, t.Address, t.UserID, t.TxHash, t.Asset, t.Amount,
		   t.CaseID, string(t.State), t.Reason, t.CreatedAt.UTC(), t.UpdatedAt.UTC(),
	   )
	  return err
   }

  func (s *SQLStore) UpdateTicket(ctx context.Context, t GuardTicket) error {
	   res, err := s.db.DB.ExecContext(ctx,
		   `UPDATE addrpool_guard_ticket SET state=?, updated_at=? WHERE id=?`,
		  string(t.State), t.UpdatedAt.UTC(), t.ID,
	  )
	   if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrTicketNotFound
	  }
	   return nil
   }

  func (s *SQLStore) GetTicket(ctx context.Context, id string) (GuardTicket, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id,chain,address,user_id,tx_hash,asset,amount,case_id,state,reason,created_at,updated_at
		 FROM addrpool_guard_ticket WHERE id=?`, id)
	   var t GuardTicket
	   var state string
	   var created, updated time.Time
	   if err := row.Scan(&t.ID, &t.Chain, &t.Address, &t.UserID, &t.TxHash, &t.Asset, &t.Amount,
		   &t.CaseID, &state, &t.Reason, &created, &updated); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return GuardTicket{}, ErrTicketNotFound
		   }
		  return GuardTicket{}, err
	   }
	   t.State = TicketState(state)
	   t.CreatedAt = created.UTC()
	  t.UpdatedAt = updated.UTC()
	  return t, nil
   }

   func (s *SQLStore) ListTickets(ctx context.Context, chain, address string, state TicketState) ([]GuardTicket, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	   addr := strings.TrimSpace(address)
	  baseQ := `SELECT id,chain,address,user_id,tx_hash,asset,amount,case_id,state,reason,created_at,updated_at
	          FROM addrpool_guard_ticket`
	   var rows *sql.Rows
	   var err error
	   switch {
	   case ch != "" && addr != "" && state != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE chain=? AND address=? AND state=? ORDER BY created_at DESC LIMIT 500`, ch, addr, string(state))
	  case ch != "" && state != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE chain=? AND state=? ORDER BY created_at DESC LIMIT 500`, ch, string(state))
	   case state != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE state=? ORDER BY created_at DESC LIMIT 500`, string(state))
	   default:
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` ORDER BY created_at DESC LIMIT 500`)
	   }
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []GuardTicket
	  for rows.Next() {
		   var t GuardTicket
		  var st string
		   var created, updated time.Time
		   if err := rows.Scan(&t.ID, &t.Chain, &t.Address, &t.UserID, &t.TxHash, &t.Asset, &t.Amount,
			   &t.CaseID, &st, &t.Reason, &created, &updated); err != nil {
			   return nil, err
		   }
		   t.State = TicketState(st)
		   t.CreatedAt = created.UTC()
		  t.UpdatedAt = updated.UTC()
		  out = append(out, t)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) ActiveTicket(ctx context.Context, chain, address, txHash string) (GuardTicket, bool) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	   addr := strings.TrimSpace(address)
	   hash := strings.TrimSpace(txHash)
	   var row *sql.Row
	   if hash != "" {
		   row = s.db.DB.QueryRowContext(ctx,
			  `SELECT id,chain,address,user_id,tx_hash,asset,amount,case_id,state,reason,created_at,updated_at 
			 FROM addrpool_guard_ticket
			 WHERE chain=? AND address=? AND tx_hash=? AND state IN ('open','blocking')
			 LIMIT 1`, ch, addr, hash)
	  } else {
		   row = s.db.DB.QueryRowContext(ctx,
			  `SELECT id,chain,address,user_id,tx_hash,asset,amount,case_id,state,reason,created_at,updated_at
			 FROM addrpool_guard_ticket
			 WHERE chain=? AND address=? AND state IN ('open','blocking')
			 LIMIT 1`, ch, addr)
	   }
	  var t GuardTicket
	  var st string
	   var created, updated time.Time
	   if err := row.Scan(&t.ID, &t.Chain, &t.Address, &t.UserID, &t.TxHash, &t.Asset, &t.Amount,
		  &t.CaseID, &st, &t.Reason, &created, &updated); err != nil {
		  return GuardTicket{}, false
	   }
	   t.State = TicketState(st)
	   t.CreatedAt = created.UTC()
	  t.UpdatedAt = updated.UTC()
	  return t, true
  }

// ─── 审计 ─────────────────────────────────────────────────────────────────

   func (s *SQLStore) WriteAudit(ctx context.Context, a GuardAudit) error {
	  if a.ID == "" {
		   a.ID = uuid.NewString()
	   }
	   _, err := s.db.DB.ExecContext(ctx,
		  `INSERT INTO addrpool_guard_audit
		 (id,kind,ref_id,actor,event,from_state,to_state,note,fact_id,at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		  a.ID, string(a.Kind), a.RefID, a.Actor, a.Event,
		  a.FromState, a.ToState, a.Note, a.FactID, a.At.UTC(),
	  )
	   return err
  }

   func (s *SQLStore) ListAudit(ctx context.Context, kind AuditKind, refID string) ([]GuardAudit, error) {
	   ref := strings.TrimSpace(refID)
	   baseQ := `SELECT id,kind,ref_id,actor,event,from_state,to_state,note,fact_id,at FROM addrpool_guard_audit`
	  var rows *sql.Rows
	  var err error
	   switch {
	  case kind != "" && ref != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE kind=? AND ref_id=? ORDER BY at DESC LIMIT 500`, string(kind), ref)
	   case kind != "":
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE kind=? ORDER BY at DESC LIMIT 500`, string(kind))
	  case ref != "":
		  rows, err = s.db.DB.QueryContext(ctx, baseQ+` WHERE ref_id=? ORDER BY at DESC LIMIT 500`, ref)
	  default:
		   rows, err = s.db.DB.QueryContext(ctx, baseQ+` ORDER BY at DESC LIMIT 500`)
	  }
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   var out []GuardAudit
	   for rows.Next() {
		   var a GuardAudit
		   var kind2 string
		   var at time.Time
		   if err := rows.Scan(&a.ID, &kind2, &a.RefID, &a.Actor, &a.Event,
			   &a.FromState, &a.ToState, &a.Note, &a.FactID, &at); err != nil {
			  return nil, err
		   }
		   a.Kind = AuditKind(kind2)
		   a.At = at.UTC()
		   out = append(out, a)
	   }
	   return out, rows.Err()
   }

// ─── 构建门面（自动选择内存/SQL）────────────────────────────────────────
// NewStore 按运行时是否有 SQL 选择后端。
// 必须传入 db；db 为 nil 时使用内存（仅限开发）。

  func NewStore(db *persist.SQLDB) Store {
	  if db != nil && db.DB != nil {
		  return NewSQLStore(db)
	  }
	  return NewMemStore()
   }

package destlimit

import (
	"context"
	"database/sql"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func (e *Engine) sqlOn() bool {
	  return e != nil && e.sql != nil && e.sql.DB != nil
   }

   func (e *Engine) BindSQL(db *persist.SQLDB) *Engine {
	  if e == nil || db == nil || db.DB == nil {
		  return e
	   }
	  e.mu.Lock()
	  e.sql = db
	   e.mu.Unlock()
	   if e.graph != nil {
		   e.graph.BindSQL(db)
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   _ = e.loadPolicies()
	   _ = e.loadHolds()
	   _ = e.loadOverrides()
	  return e
   }

  func (e *Engine) upsertSQL(mysqlStmt, pgStmt string, args ...any) error {
	   if !e.sqlOn() {
		   return nil
	   }
	  stmt := mysqlStmt
	  if e.sql.Dialect == persist.Postgres {
		  stmt = pgStmt
	   }
	  ctx := context.Background()
	  _, err := e.sql.Conn(ctx).ExecContext(ctx, stmt, args...)
	   return err
  }

   func (e *Engine) persistPolicy(p Policy) error {
	  en := 0
	   if p.Enabled {
		  en = 1
	  }
	  my := `INSERT INTO aa_dest_limit_policy
(policy_id, name, asset, cluster_max, user_max, dest_max, window_sec, low_mul, mid_mul, high_mul, severe_mul, version, enabled, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
name=VALUES(name), asset=VALUES(asset), cluster_max=VALUES(cluster_max), user_max=VALUES(user_max),
dest_max=VALUES(dest_max), window_sec=VALUES(window_sec), low_mul=VALUES(low_mul), mid_mul=VALUES(mid_mul),
high_mul=VALUES(high_mul), severe_mul=VALUES(severe_mul), version=VALUES(version), enabled=VALUES(enabled), updated_at=VALUES(updated_at)`
	  pg := `INSERT INTO aa_dest_limit_policy 
(policy_id, name, asset, cluster_max, user_max, dest_max, window_sec, low_mul, mid_mul, high_mul, severe_mul, version, enabled, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (policy_id) DO UPDATE SET
name=EXCLUDED.name, asset=EXCLUDED.asset, cluster_max=EXCLUDED.cluster_max, user_max=EXCLUDED.user_max,
dest_max=EXCLUDED.dest_max, window_sec=EXCLUDED.window_sec, low_mul=EXCLUDED.low_mul, mid_mul=EXCLUDED.mid_mul,
high_mul=EXCLUDED.high_mul, severe_mul=EXCLUDED.severe_mul, version=EXCLUDED.version, enabled=EXCLUDED.enabled, updated_at=EXCLUDED.updated_at`
	   return e.upsertSQL(my, pg,
		  p.ID, p.Name, p.Asset, p.ClusterMax, p.UserMax, p.DestMax, p.WindowSec,
		   p.LowMul, p.MidMul, p.HighMul, p.SevereMul, p.Version, en, p.UpdatedAt)
   }

   func (e *Engine) persistHold(h Hold) error {
	   var closed any
	   if h.ClosedAt.IsZero() {
		  closed = nil
	  } else {
		  closed = h.ClosedAt
	   }
	  my := `INSERT INTO aa_dest_limit_hold
(hold_id, user_id, asset, chain, dest, cluster_id, amount, policy_id, tier, state, request_id, limit_amt, reason, reserved_at, deadline, closed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
state=VALUES(state), reason=VALUES(reason), closed_at=VALUES(closed_at), limit_amt=VALUES(limit_amt)`
	   pg := `INSERT INTO aa_dest_limit_hold
(hold_id, user_id, asset, chain, dest, cluster_id, amount, policy_id, tier, state, request_id, limit_amt, reason, reserved_at, deadline, closed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (hold_id) DO UPDATE SET
state=EXCLUDED.state, reason=EXCLUDED.reason, closed_at=EXCLUDED.closed_at, limit_amt=EXCLUDED.limit_amt`
	   return e.upsertSQL(my, pg,
		  h.ID,
		   h.UserID, h.Asset, h.Chain, h.Dest, h.ClusterID, h.Amount, h.PolicyID, h.Tier,
		   h.State, h.RequestID, h.Limit, h.Reason, h.ReservedAt, h.Deadline, closed)
  }

   func (e *Engine) persistOverride(o Override) error {
	   var confirmed any
	   if o.ConfirmedAt.IsZero() {
		   confirmed = nil
	  } else {
		   confirmed = o.ConfirmedAt
	  }
	   my := `INSERT INTO aa_dest_limit_override
(override_id, user_id, asset, cluster_id, extra, actor_a, actor_b, reason, state, expires_at, created_at, confirmed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
actor_b=VALUES(actor_b), state=VALUES(state), confirmed_at=VALUES(confirmed_at)`
	   pg := `INSERT INTO aa_dest_limit_override
(override_id, user_id, asset, cluster_id, extra, actor_a, actor_b, reason, state, expires_at, created_at, confirmed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (override_id) DO UPDATE SET
actor_b=EXCLUDED.actor_b, state=EXCLUDED.state, confirmed_at=EXCLUDED.confirmed_at`
	  return e.upsertSQL(my, pg,
		  o.ID,
		   o.UserID, o.Asset, o.ClusterID, o.Extra, o.ActorA, o.ActorB, o.Reason, o.State, o.ExpiresAt, o.CreatedAt, confirmed)
  }

   func (e *Engine) persistAudit(a Audit) error {
	   if !e.sqlOn() {
		  return nil
	   }
	   ctx := context.Background()
	   _, err := e.sql.Conn(ctx).ExecContext(ctx, `INSERT INTO aa_dest_risk_audit
(audit_id, kind, subject, before_v, after_v, actor, detail, at_time)
VALUES (?,?,?,?,?,?,?,?)`,
		   a.ID,
		  a.Kind, a.Subject, a.Before, a.After, a.Actor, a.Detail, a.At)
	   return err
  }

   func (e *Engine) loadPolicies() error {
	  if !e.sqlOn() {
		   return nil
	   }
	   ctx := context.Background()
	   rows, err := e.sql.Conn(ctx).QueryContext(ctx, `SELECT policy_id, name, asset, cluster_max, user_max, dest_max, window_sec, low_mul, mid_mul, high_mul, severe_mul, version, enabled, updated_at FROM aa_dest_limit_policy`)
	   if err != nil {
		   return err
	  }
	  defer rows.Close()
	   next := map[string]*Policy{}
	  for rows.Next() {
		   var p Policy
		   var en int
		   if err := rows.Scan(&p.ID, &p.Name, &p.Asset, &p.ClusterMax, &p.UserMax, &p.DestMax, &p.WindowSec, &p.LowMul, &p.MidMul, &p.HighMul, &p.SevereMul, &p.Version, &en, &p.UpdatedAt); err != nil {
			   return err
		   }
		  p.Enabled = en != 0
		  cp := p
		   next[p.ID] = &cp
	   }
	   if err := rows.Err(); err != nil {
		  return err
	  }
	   if len(next) == 0 {
		   return nil
	   }
	  e.policies = next
	   return nil
  }

   func (e *Engine) loadHolds() error {
	  if !e.sqlOn() {
		  return nil
	  }
	   ctx := context.Background()
	  rows, err := e.sql.Conn(ctx).QueryContext(ctx, `SELECT hold_id, user_id, asset, chain, dest, cluster_id, amount, policy_id, tier, state, request_id, limit_amt, reason, reserved_at, deadline, closed_at FROM aa_dest_limit_hold`)
	  if err != nil {
		   return err
	  }
	   defer rows.Close()
	  holds := map[string]*Hold{}
	  byReq := map[string]string{}
	   for rows.Next() {
		  var h Hold
		  var closed sql.NullTime
		   if err := rows.Scan(&h.ID, &h.UserID, &h.Asset, &h.Chain, &h.Dest, &h.ClusterID, &h.Amount, &h.PolicyID, &h.Tier, &h.State, &h.RequestID, &h.Limit, &h.Reason, &h.ReservedAt, &h.Deadline, &closed); err != nil {
			   return err
		  }
		   if closed.Valid {
			  h.ClosedAt = closed.Time
		  }
		   cp := h
		  holds[h.ID] = &cp
		  if h.RequestID != "" {
			   byReq[h.RequestID] = h.ID
		   }
	  }
	  if err := rows.Err(); err != nil {
		  return err
	   }
	   if len(holds) == 0 {
		   return nil
	  }
	  e.holds = holds
	   e.byReq = byReq
	  return nil
   }

  func (e *Engine) loadOverrides() error {
	   if !e.sqlOn() {
		   return nil
	   }
	   ctx := context.Background()
	  rows, err := e.sql.Conn(ctx).QueryContext(ctx, `SELECT override_id, user_id, asset, cluster_id, extra, actor_a, actor_b, reason, state, expires_at, created_at, confirmed_at FROM aa_dest_limit_override`)
	   if err != nil {
		   return err
	   }
	   defer rows.Close()
	   next := map[string]*Override{}
	   for rows.Next() {
		  var o Override
		   var confirmed sql.NullTime
		   if err := rows.Scan(&o.ID, &o.UserID, &o.Asset, &o.ClusterID, &o.Extra, &o.ActorA, &o.ActorB, &o.Reason, &o.State, &o.ExpiresAt, &o.CreatedAt, &confirmed); err != nil {
			   return err
		   }
		   if confirmed.Valid {
			  o.ConfirmedAt = confirmed.Time
		   }
		  cp := o
		   next[o.ID] = &cp
	  }
	   if err := rows.Err(); err != nil {
		  return err
	   }
	   if len(next) == 0 {
		   return nil
	   }
	  e.overrides = next
	  return nil
  }

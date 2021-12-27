package wdrelease

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func (d *Desk) sqlOn() bool {
	   return d != nil && d.sql != nil && d.sql.DB != nil
   }

   func (d *Desk) putSQL(ctx context.Context, t *Ticket) error {
	  if !d.sqlOn() {
		   return nil
	  }
	  return d.sql.InTx(ctx, func(ctx context.Context) error {
		   if err := d.upsertTicket(ctx, t); err != nil {
			   return err
		  }
		   q := d.sql.Conn(ctx)
		   if _, err := q.ExecContext(ctx, `DELETE FROM wd_release_segment WHERE ticket_id=?`, t.ID); err != nil {
			   return err
		   }
		  if _, err := q.ExecContext(ctx, `DELETE FROM wd_release_step WHERE ticket_id=?`, t.ID); err != nil {
			  return err
		  }
		   if _, err := q.ExecContext(ctx, `DELETE FROM wd_release_audit WHERE ticket_id=?`, t.ID); err != nil {
			  return err
		   }
		   for _, seg := range t.Segments {
			  if _, err := q.ExecContext(ctx, `INSERT INTO wd_release_segment
(ticket_id, seq, bps, amount, cap, need_level, state, authorized_by, at)
VALUES (?,?,?,?,?,?,?,?,?)`,
				  t.ID, seg.Seq, seg.Bps, seg.Amount, seg.Cap, seg.Need, seg.State, seg.By, nullTime(seg.At)); err != nil {
				  return err
			  }
		   }
		  for _, st := range t.Steps {
			   if _, err := q.ExecContext(ctx, `INSERT INTO wd_release_step
(ticket_id, seq, kind, level, actor, role, code, note, at)
VALUES (?,?,?,?,?,?,?,?,?)`,
				   t.ID, st.Seq, st.Kind, st.Level, st.Actor, st.Role, st.Code, st.Note, st.At); err != nil {
				  return err
			  }
		  }
		  for _, a := range d.audits[t.ID] {
			  if _, err := q.ExecContext(ctx, `INSERT INTO wd_release_audit 
(ticket_id, seq, event, state, actor, code, prev_hash, hash, at)
VALUES (?,?,?,?,?,?,?,?,?)`,
				  a.TicketID, a.Seq, a.Event, a.State, a.Actor, a.Code, a.Prev, a.Hash, a.At); err != nil {
				  return err
			  }
		  }
		   return nil
	  })
   }

   func (d *Desk) upsertTicket(ctx context.Context, t *Ticket) error {
	   q := d.sql.Conn(ctx)
	   deadline := any(nil)
	  if !t.Deadline.IsZero() {
		  deadline = t.Deadline
	  }
	  base := `INSERT INTO wd_release_ticket
(id, idem, fingerprint, user_id, asset, amount, chain, dest, dest_class, risk, policy_ver, state, depth, base_depth, approvals, skip, timeout_action, sla_sec, deadline, applicant, code, note, authorized, frozen, residual, escalated, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  args := []any{
		   t.ID,
		   t.Idem, t.FP, t.UserID, t.Asset, t.Amount, t.Chain, t.Dest, t.DestClass, t.Risk, t.PolicyVer,
		   t.State, t.Depth, t.BaseDepth, t.Approvals, boolInt(t.Skip), t.Timeout, t.SLASec, deadline, t.Applicant,
		   t.Code, t.Note, t.Authorized, t.Frozen, t.Residual, t.Escalated, t.CreatedAt, t.UpdatedAt,
	   }
	   if d.sql.Dialect == persist.Postgres {
		   base += ` ON CONFLICT (id) DO UPDATE SET
idem=EXCLUDED.idem, fingerprint=EXCLUDED.fingerprint, state=EXCLUDED.state, depth=EXCLUDED.depth,
approvals=EXCLUDED.approvals, timeout_action=EXCLUDED.timeout_action, deadline=EXCLUDED.deadline,
code=EXCLUDED.code, note=EXCLUDED.note, authorized=EXCLUDED.authorized, frozen=EXCLUDED.frozen,
residual=EXCLUDED.residual, escalated=EXCLUDED.escalated, updated_at=EXCLUDED.updated_at`
	   } else {
		  base += ` ON DUPLICATE KEY UPDATE
state=VALUES(state), depth=VALUES(depth), approvals=VALUES(approvals), timeout_action=VALUES(timeout_action),
deadline=VALUES(deadline), code=VALUES(code), note=VALUES(note), authorized=VALUES(authorized),
frozen=VALUES(frozen), residual=VALUES(residual), escalated=VALUES(escalated), updated_at=VALUES(updated_at)`
	   }
	  _, err := q.ExecContext(ctx, base, args...)
	  return err
  }

   func boolInt(v bool) int {
	  if v {
		  return 1
	  }
	  return 0
  }

  func nullTime(t time.Time) any {
	   if t.IsZero() {
		   return nil
	   }
	  return t
  }

// PutPolicyRow 记录当前生效代次，供迁移后的库。
  func (d *Desk) PutPolicyRow(ctx context.Context) error {
	  if !d.sqlOn() {
		   return nil
	  }
	  d.mu.Lock()
	  id := d.activeID
	   d.mu.Unlock()
	   q := d.sql.Conn(ctx)
	   stmt := `INSERT INTO wd_release_policy (version, active, created_at) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE active=VALUES(active)`
	   if d.sql.Dialect == persist.Postgres {
		   stmt = `INSERT INTO wd_release_policy (version, active, created_at) VALUES (?,?,?) 
ON CONFLICT (version) DO UPDATE SET active=EXCLUDED.active`
	   }
	  _, err := q.ExecContext(ctx, stmt, id, 1, d.now())
	   return err
   }

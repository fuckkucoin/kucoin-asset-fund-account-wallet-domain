package attribrt

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/attribrt/collide"
	"account-assets.kucoin.net/funding/internal/persist"
)

const claimantSep = "\x1f"

// BindSQL 挂库。内存结论仍然是进程内权威。
// 碰撞案会回填；库没挂上或表还没建时不把缺表当成放行。
   func (e *Engine) BindSQL(db *persist.SQLDB) {
	   if e == nil || db == nil {
		   return
	   }
	  e.mu.Lock()
	  e.sql = db
	   e.mu.Unlock()
	  e.loadCollisions(db)
   }

  func (e *Engine) persist(d Decision, env Envelope) {
	   if e == nil {
		  return
	  }
	  e.mu.Lock()
	   db := e.sql
	  e.mu.Unlock()
	   if db == nil || db.DB == nil {
		   return
	  }
	   ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	   defer cancel()
	   credit := 0
	  if d.CreditIntent {
		   credit = 1
	   }
	   _, _ = db.DB.ExecContext(ctx, ` 
INSERT INTO aa_attrib_decision
  (id, chain, tx_hash, output_index, asset, user_id, disposition, credit_intent, confidence, policy_version, family, reasons, envelope_id, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   d.ID, d.Chain, d.TxHash, d.OutputIndex, d.Asset, d.UserID, d.Disposition, credit, d.Confidence,
		   d.PolicyVersion, d.Family, strings.Join(d.Reasons, ","), env.ID, d.At.UTC())
	   c2 := 0
	  if env.CreditIntent {
		  c2 = 1
	  }
	   _, _ = db.DB.ExecContext(ctx, ` 
INSERT INTO aa_attrib_envelope
  (id, decision_id, chain, tx_hash, output_index, prev_hash, payload_hash, credit_intent, disposition, policy_version, reasons, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		  env.ID,
		  env.DecisionID, env.Chain, env.TxHash, env.OutputIndex, env.PrevHash, env.PayloadHash,
		  c2, env.Disposition, env.PolicyVersion, env.Reasons, d.At.UTC())
   }

   func (e *Engine) saveCollision(c collide.Case) {
	   if e == nil || c.ID == "" || c.Fold == "" {
		  return
	  }
	   e.mu.Lock()
	  db := e.sql
	  e.mu.Unlock()
	   if db == nil || db.DB == nil {
		   return
	  }
	  at := c.Updated
	  if at.IsZero() {
		  at = time.Now().UTC()
	  }
	   ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	  defer cancel()
	  _, _ = db.DB.ExecContext(ctx, `
INSERT INTO aa_attrib_collision
  (id, chain, namespace, fold_key, state, origin_user, winner, claimants, reason, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), origin_user=VALUES(origin_user), winner=VALUES(winner),
  claimants=VALUES(claimants), reason=VALUES(reason), updated_at=VALUES(updated_at),
  namespace=VALUES(namespace)`,
		   c.ID, c.Chain, c.Namespace, c.Fold, string(c.State), c.Origin, c.Winner,
		   joinClaimants(c.Claimants), clipRunes(c.Reason, 255), at.UTC())
   }

   func (e *Engine) loadCollisions(db *persist.SQLDB) {
	   if e == nil || e.book == nil || db == nil || db.DB == nil {
		   return
	  }
	  ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	   defer cancel()
	  rows, err := db.DB.QueryContext(ctx, ` 
SELECT id, chain, namespace, fold_key, state, origin_user, winner, claimants, reason, updated_at
FROM aa_attrib_collision`)
	   if err != nil {
		  return
	   }
	  defer rows.Close()
	  for rows.Next() {
		   var c collide.Case
		  var state, claimants string
		   if err := rows.Scan(&c.ID, &c.Chain, &c.Namespace, &c.Fold, &state, &c.Origin, &c.Winner, &claimants, &c.Reason, &c.Updated); err != nil {
			  continue
		  }
		  c.State = collide.State(state)
		  c.Claimants = splitClaimants(claimants)
		  e.book.Restore(c)
	   }
  }

   func joinClaimants(xs []string) string {
	  return strings.Join(xs,
		   claimantSep)
  }

   func splitClaimants(s string) []string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return nil
	   }
	  parts := strings.Split(s,
		   claimantSep)
	  out := make([]string, 0, len(parts))
	  for _, p := range parts {
		   p = strings.TrimSpace(p)
		  if p != "" {
			   out = append(out, p)
		   }
	   }
	   return out
   }

   func clipRunes(s string, n int) string {
	  r := []rune(s)
	  if len(r) <= n {
		   return s
	   }
	   return string(r[:n])
   }

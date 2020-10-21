// 功能：对账漂移阻断持久化。
// 把 DriftGuard 阻断写入 aa_ledger_drift_block，多实例共享熔断视图。Clear 必须带 actor/note 审计；未清阻断接着拦新冻。只改内存 map 不落库会导致滚动发布后熔断丢失。

// 作者：Kinsley
// 日期：2020.09.30

package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DriftSQL 漂移阻断仓库。
type DriftSQL struct {
	db *sql.DB
}

  func NewDriftSQL(db *sql.DB) *DriftSQL { return &DriftSQL{db: db} }

// PersistBlock 记录用户级阻断。
   func (d *DriftSQL) PersistBlock(ctx context.Context, userID, reason, reportJSON string) error {
	   if d == nil || d.db == nil {
		  return fmt.Errorf("drift sql unavailable")
	   }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return fmt.Errorf("%w: user_id", ErrInvalidAddress)
	   }
	  _, err := d.db.ExecContext(ctx, `
INSERT INTO aa_ledger_drift_block (user_id, reason, report_json, blocked_at)
VALUES (?, ?, ?, ?)`, userID, strings.TrimSpace(reason), reportJSON, time.Now().UTC())
	  return err
   }

// LoadOpen 装载未解除阻断到内存 DriftGuard。

  func (d *DriftSQL) LoadOpen(ctx context.Context, g *DriftGuard) (int, error) {
	   if d == nil || d.db == nil || g == nil {
		   return 0, fmt.Errorf("drift sql unavailable")
	  }
	   rows, err := d.db.QueryContext(ctx, `
SELECT user_id, reason FROM aa_ledger_drift_block WHERE cleared_at IS NULL`)
	  if err != nil {
		   return 0, err
	  }
	   defer rows.Close()
	   n := 0
	   for rows.Next() {
		   var uid, reason string
		   if err := rows.Scan(&uid, &reason); err != nil {
			  return n, err
		  }
		  g.Block(uid, reason)
		   n++
	  }
	  return n, rows.Err()
   }

// Clear 运维解封并写审计字段。

   func (d *DriftSQL) Clear(ctx context.Context, userID, actor, note string) error {
	  if d == nil || d.db == nil {
		   return fmt.Errorf("drift sql unavailable")
	  }
	  userID = strings.TrimSpace(userID)
	  actor = strings.TrimSpace(actor)
	  if userID == "" || actor == "" {
		  return fmt.Errorf("%w: user_id/actor", ErrInvalidAddress)
	   }
	   res, err := d.db.ExecContext(ctx, ` 
UPDATE aa_ledger_drift_block
SET cleared_at=?, clear_actor=?, clear_note=?
WHERE user_id=? AND cleared_at IS NULL`,
		   time.Now().UTC(), actor, strings.TrimSpace(note), userID)
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return fmt.Errorf("drift block not open for %s", userID)
	   }
	   return nil
  }

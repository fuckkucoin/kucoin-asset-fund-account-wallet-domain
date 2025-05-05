package withdraw

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FreezeEventWriter 冻结跃迁审计。

type FreezeEventWriter struct {
	db *sql.DB
}

  func NewFreezeEventWriter(db *sql.DB) *FreezeEventWriter {
	   return &FreezeEventWriter{db: db}
   }

// Write 落一条 from→to。

  func (w *FreezeEventWriter) Write(ctx context.Context, transferID, userID string, from, to FreezeState, actor, detail string) error {
	  if w == nil || w.db == nil {
		   return fmt.Errorf("withdraw freeze event writer unavailable")
	  }
	  transferID = strings.TrimSpace(transferID)
	  userID = strings.TrimSpace(userID)
	  if transferID == "" || userID == "" {
		   return fmt.Errorf("transfer_id/user_id required")
	  }
	  _, err := w.db.ExecContext(ctx, `
INSERT INTO aa_withdraw_freeze_event (transfer_id, user_id, from_state, to_state, actor, detail, at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		  transferID, userID, string(from), string(to), strings.TrimSpace(actor), strings.TrimSpace(detail), time.Now().UTC())
	  return err
   }

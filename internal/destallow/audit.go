package destallow

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const kindDestAudit = "destallow.audit"

// AuditRec 白名单运营审计条。

type AuditRec struct {
	ID    string `json:"id"`
	Actor string `json:"actor"`
	Action  string `json:"action"`
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Label string `json:"label,omitempty"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

  func (b *Book) noteAudit(actor, action, userID, chain, addr, label, detail string) {
	   if b == nil || b.kv == nil {
		  return
	   }
	  rec := AuditRec{
		   ID: uuid.NewString(),
		   Actor: strings.TrimSpace(actor), Action: strings.TrimSpace(action),
		   UserID: strings.TrimSpace(userID), Chain: strings.ToUpper(strings.TrimSpace(chain)),
		   Address: strings.TrimSpace(addr), Label: strings.TrimSpace(label), Detail: strings.TrimSpace(detail),
		  At: time.Now().UTC(),
	  }
	  if rec.Actor == "" {
		  rec.Actor = rec.UserID
	  }
	  body, err := json.Marshal(rec)
	  if err != nil {
		   return
	   }
	   key := rec.At.Format("20060102150405") + "|" + rec.ID
	  _, _, _ = persist.GetOrInit(context.Background(), b.kv, kindDestAudit, key, body)
  }

// ListAudit 运营回放白名单审计。user 空则全台；硬顶 200。
  func (b *Book) ListAudit(ctx context.Context, userID, chain string, limit int) ([]AuditRec, error) {
	   if b == nil || b.kv == nil {
		   return []AuditRec{}, nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  limit = validate.Limit(limit, 50, 200)
	  rows, err := b.kv.List(ctx, kindDestAudit, 400)
	   if err != nil {
		   return nil, err
	  }
	  userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  out := make([]AuditRec, 0, limit)
	   for i := len(rows) - 1; i >= 0; i-- {
		   var rec AuditRec
		   if json.Unmarshal(rows[i].Body, &rec) != nil {
			  continue
		  }
		   if userID != "" && rec.UserID != userID {
			  continue
		   }
		  if chain != "" && rec.Chain != chain {
			   continue
		  }
		  out = append(out, rec)
		  if len(out) >= limit {
			  break
		  }
	   }
	   return out, nil
  }

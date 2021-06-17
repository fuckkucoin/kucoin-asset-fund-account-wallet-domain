package fundpref

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

  func (b *Book) noteAudit(ctx context.Context, actor, action, userID, before string, after Preference) {
	  if b == nil {
		   return
	  }
	  afterBody, err := json.Marshal(after)
	  if err != nil {
		  return
	   }
	   rec := AuditRec{
		   ID: uuid.NewString(), UserID: userID, Actor: strings.TrimSpace(actor),
		   Action: strings.TrimSpace(action),
		  Before: before, After: string(afterBody),
		  At: b.now(),
	  }
	  if rec.Actor == "" {
		   rec.Actor = userID
	  }
	  body, err := json.Marshal(rec)
	  if err != nil {
		  return
	  }
	   key := rec.At.Format("20060102150405") + "|" + rec.ID
	   if b.kvOn() {
		   _, _, _ = persist.GetOrInit(ctx, b.kv, kindPrefAudit, key, body)
	   }
	  if b.sqlOn() {
		   _ = sqlrepo.NewFundPref(b.sql).PutAudit(ctx, sqlrepo.FundPrefAudit{
			   ID: rec.ID, UserID: rec.UserID, Actor: rec.Actor, Action: rec.Action,
			  Before: rec.Before, After: rec.After, At: rec.At,
		   })
	   }
   }

// ListAudit 回放偏好审计。user 空则全台；硬顶 200。
   func (b *Book) ListAudit(ctx context.Context, userID string, limit int) ([]AuditRec, error) {
	   if b == nil {
		  return []AuditRec{}, nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   limit = validate.Limit(limit, 50, 200)
	   userID = strings.TrimSpace(userID)
	  if b.sqlOn() {
		  rows, err := sqlrepo.NewFundPref(b.sql).ListAudit(ctx, userID, limit)
		   if err != nil {
			  return nil, fmt.Errorf("%w: %v", ErrStore, err)
		   }
		  out := make([]AuditRec, 0, len(rows))
		  for _, r := range rows {
			  out = append(out, AuditRec{
				   ID:     r.ID,
				  UserID: r.UserID, Actor: r.Actor, Action: r.Action,
				  Before: r.Before, After: r.After, At: r.At,
			   })
		   }
		  return out, nil
	  }
	   if !b.kvOn() {
		  return []AuditRec{}, nil
	  }
	   rows, err := b.kv.List(ctx, kindPrefAudit, 400)
	  if err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrStore, err)
	   }
	   out := make([]AuditRec, 0, limit)
	   for i := len(rows) - 1; i >= 0; i-- {
		   var rec AuditRec
		  if json.Unmarshal(rows[i].Body, &rec) != nil {
			  continue
		  }
		   if userID != "" && rec.UserID != userID {
			   continue
		  }
		  out = append(out, rec)
		  if len(out) >= limit {
			  break
		   }
	  }
	   return out, nil
  }

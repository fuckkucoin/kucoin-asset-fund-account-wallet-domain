package fundpref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
)

// KV 键空间与 fundsurf 冻结/工单隔离。kind 须短于 aa_kv.kind VARCHAR(32)。
const (
	kindPrefRow = "fundpref.row"
	kindPrefAudit = "fundpref.audit"
)

type Book struct {
	kv  persist.KV
	sql *persist.SQLDB
	clock func() time.Time
}

   func New() *Book {
	  return &Book{clock: func() time.Time { return time.Now().UTC() }}
   }

  func (b *Book) BindKV(kv persist.KV) *Book {
	  if b != nil && kv != nil {
		  b.kv = kv
	  }
	  return b
   }

  func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
	  }
	  return b
  }

  func (b *Book) now() time.Time {
	  if b != nil && b.clock != nil {
		   return b.clock()
	   }
	   return time.Now().UTC()
   }

  func (b *Book) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
  }

  func (b *Book) kvOn() bool {
	   return b != nil && b.kv != nil
  }

// Get 读用户偏好。无行回 Defaults；载荷损坏或非法返回 ErrCorrupt。
   func (b *Book) Get(ctx context.Context, userID string) (Preference, error) {
	  uid, err := validate.UserID(userID)
	  if err != nil {
		   return Preference{}, ErrUser
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  if b.sqlOn() {
		  row, ok, err := sqlrepo.NewFundPref(b.sql).Get(ctx, uid)
		  if err != nil {
			  return Preference{}, fmt.Errorf("%w: %v", ErrStore, err)
		   }
		  if ok {
			   return decodeStored(row.Body)
		   }
	  }
	   if b.kvOn() {
		   row, err := b.kv.Get(ctx, kindPrefRow, uid)
		   if err == nil {
			  return decodeStored(row.Body)
		   }
		   if !errors.Is(err, persist.ErrNotFound) {
			   return Preference{}, fmt.Errorf("%w: %v", ErrStore, err)
		  }
	   }
	  return Defaults(uid), nil
   }

// Put 校验后落 SQL/KV，并追加审计。允许用合法新体修复损坏旧行。
   func (b *Book) Put(ctx context.Context,
	  actor, userID string, req WriteReq) (Preference, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return Preference{}, ErrUser
	   }
	  if strings.TrimSpace(actor) == "" {
		  actor = uid
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  base, beforeRaw := b.loadBase(ctx, uid)
	  next := mergeWrite(base, req)
	  next.UserID = uid
	  if err := Validate(&next); err != nil {
		   return Preference{}, err
	   }
	   next.UpdatedAt = b.now()
	   next.Version = base.Version + 1
	   if err := b.persistRow(ctx, next); err != nil {
		   return Preference{}, err
	   }
	   b.noteAudit(ctx, actor, "put", uid, beforeRaw, next)
	   return next, nil
   }

   func (b *Book) loadBase(ctx context.Context, uid string) (Preference, string) {
	   raw, ok := b.loadRaw(ctx, uid)
	  if !ok {
		   return Defaults(uid), ""
	   }
	   p, err := decodeStored(raw)
	  if err != nil {
		  return Defaults(uid), string(raw)
	   }
	  return p, string(raw)
  }

  func (b *Book) loadRaw(ctx context.Context, uid string) ([]byte, bool) {
	   if b.sqlOn() {
		   row,
			   ok, err := sqlrepo.NewFundPref(b.sql).Get(ctx, uid)
		   if err == nil && ok {
			   return row.Body, true
		  }
	   }
	   if b.kvOn() {
		   row, err := b.kv.Get(ctx, kindPrefRow, uid)
		  if err == nil {
			  return row.Body, true
		   }
	   }
	  return nil, false
   }

  func decodeStored(body []byte) (Preference, error) {
	   if len(body) == 0 {
		   return Preference{}, ErrCorrupt
	  }
	  var p Preference
	  if err := json.Unmarshal(body, &p); err != nil {
		  return Preference{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	   }
	  if err := Validate(&p); err != nil {
		  return Preference{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	  }
	  return p, nil
  }

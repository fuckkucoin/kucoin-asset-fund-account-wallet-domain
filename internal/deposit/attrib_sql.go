// 充值入账域，覆盖归因、扫链、挂单与核验闸。

// Author: Kael
// Date: 2020.10.12

package deposit

import (
	"context"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// BindSQL 归因表挂 SQL；Hydrate 回内存。
// 必须 BindSQL；OpenMem 不绑。
// Hydrate 失败不得假装已持久化。
   func (a *Attributor) BindSQL(db *persist.SQLDB) *Attributor {
	   if a != nil {
		  a.sql = db
		  _ = a.Hydrate(context.Background())
	  }
	   return a
  }

  func (a *Attributor) sqlOn() bool {
	   return a != nil && a.sql != nil && a.sql.DB != nil
  }

// Hydrate 从 aa_deposit_attribs 回填。
   func (a *Attributor) Hydrate(ctx context.Context) error {
	   if a == nil || !a.sqlOn() {
		   return nil
	   }
	  rows, err := sqlrepo.NewDepAttribs(a.sql).List(ctx)
	   if err != nil {
		   return err
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  for _, row := range rows {
		   kind := AttribKind(row.Kind)
		   a.by[attribKey(row.Chain, kind, row.MemoKey)] = Binding{
			  UserID: row.UserID, Chain: row.Chain, Kind: kind, Key: row.MemoKey,
		  }
	  }
	   return nil
  }

   func (a *Attributor) persistLocked(b Binding) error {
	   if !a.sqlOn() {
		   return nil
	   }
	  return sqlrepo.NewDepAttribs(a.sql).Put(context.Background(), sqlrepo.DepAttrib{
		   UserID: b.UserID, Chain: b.Chain, Kind: string(b.Kind), MemoKey: b.Key,
		  BoundAt: time.Now().UTC(),
	  })
   }

// BindMemoKey MemoBinder 适配（addrpool.SharedMemoPool.BindUser）。
  func (a *Attributor) BindMemoKey(userID, chain, kind, key string) error {
	   k, err := ParseAttribKind(kind)
	  if err != nil {
		  return err
	  }
	  _, err = a.BindUnique(userID, chain, k, key)
	  return err
  }

// memoBinder 适配 Attributor → addrpool.MemoBinder。

type memoBinder struct{ a *Attributor }

  func (m memoBinder) BindUnique(userID, chain, kind, key string) error {
	  if m.a == nil {
		  return errAttrUnavailable
	  }
	  return m.a.BindMemoKey(userID, chain, kind, key)
  }

  func AsMemoBinder(a *Attributor) addrpool.MemoBinder {
	   return memoBinder{a: a}
  }

var errAttrUnavailable = errors.New("deposit attributor unavailable")

// LookupSQL 内存未命中时回落 SQL（跨实例）。

  func (a *Attributor) LookupSQL(chain string, kind AttribKind, key string) (Binding, bool) {
	   if b, ok := a.Lookup(chain, kind, key); ok {
		  return b, true
	   }
	  if !a.sqlOn() {
		   return Binding{}, false
	  }
	   row, err := sqlrepo.NewDepAttribs(a.sql).Get(context.Background(), chain, string(kind), key)
	  if err != nil {
		  if errors.Is(err, sqlrepo.ErrNotFound) {
			   return Binding{}, false
		  }
		   return Binding{}, false
	   }
	  b := Binding{UserID: row.UserID, Chain: row.Chain, Kind: AttribKind(row.Kind), Key: row.MemoKey}
	   a.mu.Lock()
	   a.by[attribKey(b.Chain, b.Kind, b.Key)] = b
	   a.mu.Unlock()
	   return b, true
  }

// ResolveObserved 扫链观察备注自动归因。
// 先 memo，再 tag；皆空则失败。
// 不得猜用户；SQL 未绑时仅内存。

   func (a *Attributor) ResolveObserved(chain, memo, tag string) (Binding, error) {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   memo = strings.TrimSpace(memo)
	  tag = strings.TrimSpace(tag)
	   if memo != "" {
		  if b, ok := a.LookupSQL(chain, AttribMemo, memo); ok {
			   return b, nil
		  }
		  if b, ok := a.LookupSQL(chain, AttribTag, memo); ok {
			   return b, nil
		  }
	  }
	  if tag != "" {
		   if b, ok := a.LookupSQL(chain, AttribTag, tag); ok {
			  return b, nil
		   }
		  if b, ok := a.LookupSQL(chain, AttribMemo, tag); ok {
			   return b, nil
		   }
	  }
	  return Binding{}, errors.New("deposit not attributed from observed tag")
  }

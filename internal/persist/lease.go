// @FileName: lease.go
// @Summary: persist/lease.go
// @Author: Kinsley
// @Created: 2020.09.03

package persist

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const kindLease = "lease"

// Lease 工人租约。同一 name 只能被一个 holder 持有；过期后别人才能抢走。
type Lease struct {
	Name string `json:"name"`
	Holder string `json:"holder"`
	ExpiresAt time.Time `json:"expires_at"`
	Fence     Fence     `json:"fence"`
}

type Leases struct {
	kv    KV
	clock func() time.Time
}

   func NewLeases(kv KV) *Leases {
	   if kv == nil {
		  kv = NewMemKV()
	   }
	  return &Leases{kv: kv, clock: func() time.Time { return time.Now().UTC() }}
   }

  func (l *Leases) WithClock(c func() time.Time) *Leases {
	   if l != nil && c != nil {
		  l.clock = c
	  }
	  return l
   }

   // Acquire 抢租约。已有未过期且 holder 不同则冲突。自己续租必须带当前围栏。

  func (l *Leases) Acquire(ctx context.Context, name, holder string, ttl time.Duration) (Lease, error) {
	   if l == nil || l.kv == nil {
		   return Lease{}, ErrTxClosed
	   }
	  name = strings.TrimSpace(name)
	   holder = strings.TrimSpace(holder)
	  if name == "" || holder == "" {
		   return Lease{}, ErrConflict
	   }
	  if ttl <= 0 {
		   ttl = 8 * time.Second
	   }
	   var out Lease
	  err := l.kv.InTx(ctx, func(ctx context.Context) error {
		  now := l.clock()
		   row,
			   err := l.kv.Get(ctx, kindLease, name)
		   if err != nil && err != ErrNotFound {
			  return err
		   }
		   exp := now.Add(ttl)
		   if err == ErrNotFound {
			   ls := Lease{Name: name, Holder: holder, ExpiresAt: exp}
			   body, merr := json.Marshal(ls)
			   if merr != nil {
				   return merr
			   }
			  if err := l.kv.Put(ctx, Row{Kind: kindLease, Key: name, Body: body}, Fence{}); err != nil {
				   return err
			   }
			  got, gerr := l.kv.Get(ctx, kindLease, name)
			  if gerr != nil {
				   return gerr
			   }
			  ls.Fence = got.Fence
			  out = ls
			  return nil
		   }
		  var cur Lease
		   if err := json.Unmarshal(row.Body, &cur); err != nil {
			  return ErrConflict
		   }
		  if now.Before(cur.ExpiresAt) && cur.Holder != holder {
			  return ErrConflict
		   }
		   cur.Holder = holder
		  cur.ExpiresAt = exp
		   body, merr := json.Marshal(cur)
		   if merr != nil {
			   return merr
		  }
		   row.Body = body
		   if err := l.kv.Put(ctx, row, row.Fence); err != nil {
			   return err
		  }
		   got, gerr := l.kv.Get(ctx, kindLease, name)
		  if gerr != nil {
			   return gerr
		  }
		   cur.Fence = got.Fence
		   out = cur
		   return nil
	   })
	  return out, err
  }

// Register 写入或条件续持；同 key 不同 holder 且未过期返回 ErrConflict。

  func (l *Leases) Register(ctx context.Context, key, holder string, ttl time.Duration) (Lease, error) {
	  return l.Acquire(ctx,
		   key, holder, ttl)
  }

// Get 仅返回未过期租约。

   func (l *Leases) Get(ctx context.Context, key string) (Lease, error) {
	   ls, err := l.Holder(ctx, key)
	   if err != nil {
		  return Lease{}, err
	   }
	  now := time.Now().UTC()
	  if l != nil && l.clock != nil {
		   now = l.clock()
	  }
	   if !now.Before(ls.ExpiresAt) {
		  return Lease{}, ErrNotFound
	   }
	   return ls, nil
  }

// Renew 由当前 holder 延长 TTL。

  func (l *Leases) Renew(ctx context.Context, key, holder string, ttl time.Duration) (Lease, error) {
	  cur, err := l.Get(ctx, key)
	  if err != nil {
		  return Lease{}, err
	   }
	  if strings.TrimSpace(cur.Holder) != strings.TrimSpace(holder) {
		   return Lease{}, ErrConflict
	  }
	   return l.Acquire(ctx, key, holder, ttl)
   }

// ListActive 扫描 kind=lease，过滤已过期行。
   func (l *Leases) ListActive(ctx context.Context, limit int) ([]Lease, error) {
	   if l == nil || l.kv == nil {
		  return nil, ErrTxClosed
	   }
	   if limit <= 0 {
		   limit = 64
	  }
	  rows, err := l.kv.List(ctx, kindLease, limit)
	   if err != nil {
		  return nil, err
	   }
	  now := time.Now().UTC()
	   if l.clock != nil {
		  now = l.clock()
	  }
	  out := make([]Lease, 0, len(rows))
	  for _, row := range rows {
		   var ls Lease
		   if err := json.Unmarshal(row.Body, &ls); err != nil {
			   continue
		   }
		  ls.Fence = row.Fence
		   if now.Before(ls.ExpiresAt) {
			  out = append(out, ls)
		  }
	   }
	   return out, nil
  }

   func (l *Leases) Holder(ctx context.Context, name string) (Lease, error) {
	  row, err := l.kv.Get(ctx, kindLease, strings.TrimSpace(name))
	  if err != nil {
		  return Lease{}, err
	   }
	  var ls Lease
	   if err := json.Unmarshal(row.Body, &ls); err != nil {
		   return Lease{}, ErrConflict
	   }
	  ls.Fence = row.Fence
	   return ls, nil
  }

// Release 只有当前 holder 能退租。别人的 token 当过期围栏。

   func (l *Leases) Release(ctx context.Context, name, holder string) error {
	   return l.kv.InTx(ctx, func(ctx context.Context) error {
		  row, err := l.kv.Get(ctx, kindLease, strings.TrimSpace(name))
		   if err != nil {
			  return err
		  }
		   var cur Lease
		  if err := json.Unmarshal(row.Body, &cur); err != nil {
			  return ErrConflict
		  }
		   if cur.Holder != strings.TrimSpace(holder) {
			  return ErrStaleFence
		   }
		   return l.kv.Delete(ctx, kindLease, name, row.Fence)
	  })
   }

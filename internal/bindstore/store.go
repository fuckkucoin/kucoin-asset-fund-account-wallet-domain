package bindstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Binding struct {
	UserID  string    `json:"user_id"`
	Chain   string    `json:"chain"`
	Family  string    `json:"family"`
	Address string    `json:"address"`
	BoundAt time.Time `json:"bound_at"`
	Cooldown time.Time `json:"reuse_after"`
	Allocated bool `json:"allocated"`
	Fence     persist.Fence
}

type Store struct {
	kv persist.KV
	sql   *persist.SQLDB
	reuse time.Duration
	clock func() time.Time
}

   func New(kv persist.KV, reuse time.Duration) *Store {
	  if kv == nil {
		   kv = persist.NewMemKV()
	   }
	  if reuse <= 0 {
		   reuse = 24 * time.Hour
	   }
	   return &Store{kv: kv, reuse: reuse, clock: func() time.Time { return time.Now().UTC() }}
   }

   func NewMem() *Store { return New(nil, 0) }

const (
	kindBind = "bind"
	kindAddr = "bind-addr"
)

  func family(chain string) string {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   if wallet.IsEVM(chain) {
		   return "EVM"
	  }
	  return chain
   }

  func bindKey(userID, chain string) string {
	   return strings.TrimSpace(userID) + "|" + family(chain)
  }

  func (s *Store) loadBind(ctx context.Context, userID, chain string) (Binding, persist.Fence, error) {
	   if s.sqlOn() {
		   got, err := s.loadSQL(ctx, userID, chain)
		   if err == nil {
			   return got, got.Fence, nil
		  }
		  if !ignoreSQLMiss(err) {
			  return Binding{},
				  persist.Fence{}, err
		   }
	  }
	  row, err := s.kv.Get(ctx, kindBind, bindKey(userID, chain))
	  if err != nil {
		  return Binding{},
			  persist.Fence{}, err
	  }
	   var b Binding
	  if err := json.Unmarshal(row.Body, &b); err != nil {
		  return Binding{},
			  persist.Fence{}, err
	   }
	  b.Fence = row.Fence
	   return b, row.Fence, nil
  }

// Allocate 同一用户同家族复用已绑地址。地址已被别人占用则失败，不得串户。

  func (s *Store) Allocate(ctx context.Context, userID, chain, addr string) (Binding, error) {
	  userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   addr = strings.TrimSpace(addr)
	   if userID == "" || chain == "" || addr == "" {
		   return Binding{}, persist.ErrConflict
	   }
	   var out Binding
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, fence, err := s.loadBind(ctx, userID, chain)
		  if err == nil && cur.Allocated {
			   out = cur
			  return nil
		   }
		   expected := persist.Fence{}
		   if err == nil {
			   expected = fence
		  } else if !errors.Is(err, persist.ErrNotFound) {
			  return err
		  }
		  if owner, oerr := s.kv.Get(ctx, kindAddr, strings.ToLower(addr)); oerr == nil {
			   if string(owner.Body) != bindKey(userID, chain) {
				  return persist.ErrConflict
			   }
		  } else if !errors.Is(oerr, persist.ErrNotFound) {
			   return oerr
		   }
		   now := s.clock()
		   b := Binding{
			   UserID: userID, Chain: chain, Family: family(chain), Address: addr,
			   BoundAt: now, Cooldown: now.Add(s.reuse), Allocated: true,
		   }
		  body, err := json.Marshal(b)
		  if err != nil {
			  return err
		   }
		   if err := s.persistSQL(ctx, b); err != nil {
			   return err
		   }
		   if err := s.kv.Put(ctx, persist.Row{Kind: kindBind, Key: bindKey(userID, chain), Body: body}, expected); err != nil {
			  return err
		  }
		   if err := s.kv.Put(ctx, persist.Row{Kind: kindAddr, Key: strings.ToLower(addr), Body: []byte(bindKey(userID, chain))}, persist.Fence{}); err != nil && !errors.Is(err, persist.ErrStaleFence) {
			   // 地址索引已在则必须是本人，上面已经核过。
			  if !errors.Is(err,
				   persist.ErrConflict) {
				   return err
			  }
		   }
		   got, _, err := s.loadBind(ctx, userID, chain)
		  if err != nil {
			  return err
		  }
		   out = got
		  return nil
	   })
	   return out, err
  }

// Release 冷却未满不得解绑，避免刚分出去的地址立刻给别人。

   func (s *Store) Release(ctx context.Context, userID, chain string) error {
	   return s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, fence, err := s.loadBind(ctx, userID, chain)
		  if err != nil {
			   if errors.Is(err,
				  persist.ErrNotFound) {
				   return nil
			   }
			   return err
		   }
		   if !cur.Allocated {
			  return nil
		   }
		   if s.clock().Before(cur.Cooldown) {
			  return persist.ErrConflict
		  }
		   cur.Allocated = false
		   body, err := json.Marshal(cur)
		  if err != nil {
			   return err
		  }
		   if err := s.persistSQL(ctx, cur); err != nil {
			   return err
		  }
		   if err := s.kv.Put(ctx, persist.Row{Kind: kindBind, Key: bindKey(userID, chain), Body: body}, fence); err != nil {
			  return err
		  }
		   addrRow, aerr := s.kv.Get(ctx, kindAddr, strings.ToLower(cur.Address))
		   if aerr == nil {
			  _ = s.kv.Delete(ctx, kindAddr, strings.ToLower(cur.Address), addrRow.Fence)
		  }
		  return nil
	  })
   }

  func (s *Store) Lookup(ctx context.Context, userID, chain string) (Binding, bool) {
	  b, _, err := s.loadBind(ctx, userID, chain)
	   if err != nil || !b.Allocated {
		   return Binding{}, false
	  }
	  return b, true
   }

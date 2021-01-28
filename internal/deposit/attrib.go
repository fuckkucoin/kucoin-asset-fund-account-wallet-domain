// @FileName: attrib.go
// @Summary: 充值入账域之充值归因。
// @Author: Cason
// @Created: 2021.01.07

package deposit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type AttribKind string

const (
	AttribMemo AttribKind = "memo"
	AttribTag AttribKind = "tag"
	AttribUTXO AttribKind = "utxo"
	AttribAddr AttribKind = "address"
)

type Binding struct {
	UserID string     `json:"user_id"`
	Chain  string     `json:"chain"`
	Kind   AttribKind `json:"kind"`
	Key    string     `json:"key"`
}

type Attributor struct {
	mu sync.Mutex
	by map[string]Binding
	dust string
	sql *persist.SQLDB
}

  func NewAttributor() *Attributor {
	  return &Attributor{by: map[string]Binding{}, dust: "0.00000001"}
  }

  func attribKey(chain string, kind AttribKind, key string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + string(kind) + "|" + strings.TrimSpace(key)
   }

  func (a *Attributor) Bind(userID, chain string, kind AttribKind, key string) (Binding, error) {
	  if a == nil {
		   return Binding{}, fmt.Errorf("deposit attributor unavailable")
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return Binding{}, err
	  }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return Binding{}, err
	   }
	  key = strings.TrimSpace(key)
	   if key == "" {
		   return Binding{}, fmt.Errorf("attribution key required")
	   }
	  if len(key) > 128 {
		  return Binding{}, fmt.Errorf("attribution key too long")
	   }
	  if kind == "" {
		  kind = AttribMemo
	  }
	  b := Binding{UserID: uid, Chain: ch, Kind: kind, Key: key}
	  a.mu.Lock()
	  defer a.mu.Unlock()
	   k := attribKey(ch, kind, key)
	   if cur, ok := a.by[k]; ok && cur.UserID != uid {
		  return Binding{}, fmt.Errorf("attribution key already bound")
	   }
	  a.by[k] = b
	  if err := a.persistLocked(b); err != nil {
		   delete(a.by, k)
		  return Binding{}, err
	  }
	  return b, nil
  }

  func (a *Attributor) Lookup(chain string, kind AttribKind, key string) (Binding, bool) {
	  if a == nil {
		  return Binding{}, false
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  b, ok := a.by[attribKey(chain, kind, key)]
	  return b, ok
   }

// Resolve 按 memo/tag/utxo 认领。都没有则退回地址绑定。空 key 不得猜用户。

   func (a *Attributor) Resolve(chain, memo, tag, utxo, addrUser string) (Binding, error) {
	   if memo != "" {
		  if b, ok := a.Lookup(chain, AttribMemo, memo); ok {
			   noteAttrib(b, false)
			   return b, nil
		   }
	  }
	   if tag != "" {
		  if b, ok := a.Lookup(chain, AttribTag, tag); ok {
			   noteAttrib(b, false)
			  return b, nil
		  }
	   }
	  if utxo != "" {
		  if b, ok := a.Lookup(chain, AttribUTXO, utxo); ok {
			   noteAttrib(b, false)
			  return b, nil
		   }
	  }
	   if addrUser != "" {
		   b := Binding{UserID: addrUser,
			   Chain: chain, Kind: AttribAddr}
		  noteAttrib(b, false)
		   return b, nil
	   }
	  noteAttrib(Binding{Chain: chain}, true)
	   return Binding{}, fmt.Errorf("deposit not attributed")
   }

  func noteAttrib(b Binding, noise bool) {
	   ingest.Try(context.Background(), ingest.DepositDecision(b.UserID, b.Chain, string(b.Kind), b.Key, "", "0", "", "deposit-desk", noise, time.Now().UTC()))
  }

   func (a *Attributor) SetDust(s string) error {
	  if _, err := money.Parse(s); err != nil {
		   if _, err2 := money.ParseBook(s); err2 != nil {
			   return err
		   }
	   }
	   a.mu.Lock()
	   a.dust = s
	   a.mu.Unlock()
	   return nil
  }

// DustReject 小于尘阈值不得开入账。尘只能并进下一笔，不能单独 credit。
  func (a *Attributor) DustReject(amount string) error {
	   got, err := money.Parse(amount)
	   if err != nil {
		  return err
	   }
	   if got.Dust() {
		   return fmt.Errorf("deposit dust")
	  }
	  a.mu.Lock()
	  floor := a.dust
	  a.mu.Unlock()
	  min, err := money.ParseBook(orZero(floor))
	   if err != nil {
		  return err
	   }
	   if got.Cmp(min) < 0 {
		   return fmt.Errorf("deposit below dust floor %s", floor)
	   }
	   return nil
  }

   func orZero(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	   }
	  return s
   }

   func (s *Service) Attributor() *Attributor {
	  if s == nil {
		   return NewAttributor()
	   }
	   if s.attr == nil {
		  s.attr = NewAttributor()
	  }
	   return s.attr
   }

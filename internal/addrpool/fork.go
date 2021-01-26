package addrpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

const kindForkSignal = "addrpool.fork.signal"

var ErrForkHeight = errors.New("addrpool fork height invalid")

   func init() {
	  apierr.Register(ErrForkHeight, http.StatusBadRequest)
   }

// ForkSignal 分叉高度信号桩。

type ForkSignal struct {
	Chain  string `json:"chain"`
	Height int64  `json:"height"`
	Reason string `json:"reason,omitempty"`
	Reassign bool `json:"reassign,omitempty"`
	Actor string    `json:"actor,omitempty"`
	At    time.Time `json:"at"`
}

// ForkHit 受影响绑定。
type ForkHit struct {
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Moved   bool   `json:"moved"`
	NewAddr string `json:"new_address,omitempty"`
	Error string `json:"error,omitempty"`
}

// ForkReport 分叉处理报告。
type ForkReport struct {
	Signal ForkSignal `json:"signal"`
	Hits  []ForkHit `json:"hits"`
	Total int       `json:"total"`
	Moved int       `json:"moved"`
}

   func (p *Pool) listChainAll(chain string) []Binding {
	   if p == nil {
		  return nil
	  }
	   want := family(chain)
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  out := make([]Binding, 0)
	   for _, b := range p.byKey {
		   if b == nil {
			   continue
		   }
		   if family(b.Chain) == want || strings.EqualFold(b.Chain, chain) {
			   out = append(out, *b)
		   }
	   }
	  return out
  }

  func putForkSignal(ctx context.Context, kv persist.KV, sig ForkSignal) error {
	  if kv == nil {
		   return nil
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   body, err := json.Marshal(sig)
	  if err != nil {
		  return err
	  }
	  key := strings.ToUpper(strings.TrimSpace(sig.Chain))
	   _, existed, err := persist.GetOrInit(ctx, kv, kindForkSignal, key, body)
	   if err != nil {
		   return err
	   }
	   if existed {
		   return persist.Replace(ctx,
			  kv, kindForkSignal, key, body)
	  }
	   return nil
  }

// ApplyForkSignal 记录分叉高度；reassign 时对占用做 Release+Assign 桩。

  func ApplyForkSignal(ctx context.Context, pool *Pool, shared *SharedMemoPool, binder MemoBinder, rec *RecycleBook, marks *MarkBook, kv persist.KV, sig ForkSignal) (ForkReport, error) {
	   ch,
		   err := validate.Chain(sig.Chain)
	  if err != nil {
		  return ForkReport{}, err
	  }
	  if sig.Height <= 0 {
		   return ForkReport{}, ErrForkHeight
	   }
	  if wallet.RequiresFamMemo(ch) {
		   return ForkReport{}, ErrSharedRelease
	   }
	  now := time.Now().UTC()
	  sig.Chain, sig.At = ch, now
	  if strings.TrimSpace(sig.Reason) == "" {
		  sig.Reason = "fork_height"
	   }
	  if err := putForkSignal(ctx, kv, sig); err != nil {
		  return ForkReport{}, err
	  }
	  rep := ForkReport{Signal: sig,
		   Hits: []ForkHit{}}
	   if pool == nil {
		   return rep, nil
	  }
	   for _, b := range pool.listChainAll(ch) {
		   hit := ForkHit{UserID: b.UserID, Chain: b.Chain, Address: b.Address}
		  if marks != nil && marks.Blocked(b.Address) {
			  hit.Error = ErrCompromised.Error()
			   rep.Hits = append(rep.Hits, hit)
			   continue
		   }
		  if !sig.Reassign {
			   rep.Hits = append(rep.Hits, hit)
			  continue
		  }
		  if rec != nil {
			   _, _ = rec.Mark(ctx, b.UserID, b.Chain, b.Address)
		   }
		  if err := pool.Release(b.UserID, b.Chain); err != nil {
			   hit.Error = err.Error()
			   rep.Hits = append(rep.Hits, hit)
			  continue
		  }
		  view, aerr := Assign(pool, shared, binder, AssignReq{UserID: b.UserID, Chain: b.Chain})
		   if aerr != nil {
			  hit.Error = aerr.Error()
			  rep.Hits = append(rep.Hits, hit)
			  continue
		   }
		   hit.Moved, hit.NewAddr = true, view.Address
		  rep.Moved++
		   rep.Hits = append(rep.Hits, hit)
	  }
	  rep.Total = len(rep.Hits)
	   return rep, nil
   }

// LastForkSignal 读最近分叉桩。
  func LastForkSignal(ctx context.Context, kv persist.KV, chain string) (ForkSignal, error) {
	   if kv == nil {
		   return ForkSignal{}, fmt.Errorf("addrpool fork store unavailable")
	   }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return ForkSignal{}, err
	  }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   row,
		   err := kv.Get(ctx, kindForkSignal, ch)
	   if err != nil {
		   return ForkSignal{}, err
	   }
	  var sig ForkSignal
	   if json.Unmarshal(row.Body, &sig) != nil {
		   return ForkSignal{}, ErrForkHeight
	   }
	  return sig, nil
   }

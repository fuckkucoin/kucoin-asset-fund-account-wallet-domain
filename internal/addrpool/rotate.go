package addrpool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

const kindPoolMark = "addrpool.mark"

var (
	ErrCompromised = errors.New("deposit address compromised")
	ErrNotMarked = errors.New("deposit address not marked")
)

  func init() {
	  apierr.Register(ErrCompromised, http.StatusConflict)
	   apierr.Register(ErrNotMarked, http.StatusNotFound)
   }

// AddrMark 黑洞或失陷标记。

type AddrMark struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
	Reason string `json:"reason"`
	Kind   string `json:"kind"`
	Actor string `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// MarkBook 失陷/黑洞簿。

type MarkBook struct {
	mu   sync.RWMutex
	rows map[string]AddrMark
	kv   persist.KV
}

   func NewMarkBook(kv persist.KV) *MarkBook {
	  return &MarkBook{rows: map[string]AddrMark{}, kv: kv}
  }

  func markKey(addr string) string {
	  return strings.ToLower(strings.TrimSpace(addr))
  }

   func (m *MarkBook) Blocked(addr string) bool {
	   if m == nil {
		  return false
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  _, ok := m.rows[markKey(addr)]
	   return ok
   }

  func (m *MarkBook) Reload(ctx context.Context) error {
	   if m == nil || m.kv == nil {
		   return nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  rows,
		   err := m.kv.List(ctx, kindPoolMark, 400)
	  if err != nil {
		   return err
	  }
	   next := map[string]AddrMark{}
	   for _, row := range rows {
		  var rec AddrMark
		   if json.Unmarshal(row.Body, &rec) != nil {
			  continue
		   }
		  next[markKey(rec.Address)] = rec
	   }
	  m.mu.Lock()
	   m.rows = next
	  m.mu.Unlock()
	   return nil
  }

   func (m *MarkBook) Get(addr string) (AddrMark, bool) {
	  if m == nil {
		   return AddrMark{}, false
	  }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  got, ok := m.rows[markKey(addr)]
	   return got, ok
  }

  func (m *MarkBook) Mark(ctx context.Context, chain, addr, kind, reason, actor string) (AddrMark, error) {
	   if m == nil {
		   return AddrMark{}, fmtErr("addrpool mark book unavailable")
	  }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		  return AddrMark{}, err
	  }
	  a, err := validate.Address(ch, addr)
	  if err != nil {
		  return AddrMark{}, err
	   }
	   kind = strings.ToLower(strings.TrimSpace(kind))
	   if kind != "blackhole" && kind != "compromise" {
		   kind = "compromise"
	   }
	   rec := AddrMark{
		   Address: a, Chain: ch, Kind: kind, Reason: strings.TrimSpace(reason),
		  Actor: strings.TrimSpace(actor), CreatedAt: time.Now().UTC(),
	  }
	   m.mu.Lock()
	   if m.rows == nil {
		  m.rows = map[string]AddrMark{}
	  }
	   m.rows[markKey(a)] = rec
	  m.mu.Unlock()
	  metrics.FundGates.WithLabelValues("addrpool", kind).Inc()
	   if m.kv == nil {
		   return rec, nil
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  body, err := json.Marshal(rec)
	  if err != nil {
		   return AddrMark{}, err
	   }
	  _, existed, err := persist.GetOrInit(ctx, m.kv, kindPoolMark, markKey(a), body)
	   if err != nil {
		  return AddrMark{}, err
	  }
	  if existed {
		   if err := persist.Replace(ctx, m.kv, kindPoolMark, markKey(a), body); err != nil {
			  return AddrMark{}, err
		   }
	   }
	  return rec, nil
   }

  func fmtErr(s string) error { return errors.New(s) }

// RotateCompromised 标记后释放旧绑并领新址。
   func RotateCompromised(ctx context.Context, pool *Pool, shared *SharedMemoPool, binder MemoBinder, rec *RecycleBook, marks *MarkBook, userID, chain, addr, kind, reason, actor string) (AssignedView, AddrMark, error) {
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return AssignedView{}, AddrMark{}, err
	  }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		   return AssignedView{}, AddrMark{}, err
	   }
	   if wallet.RequiresFamMemo(ch) {
		  return AssignedView{},
			  AddrMark{}, ErrSharedRelease
	   }
	   if marks == nil {
		   return AssignedView{}, AddrMark{}, fmtErr("addrpool mark book unavailable")
	  }
	  mark, err := marks.Mark(ctx, ch, addr, kind, reason, actor)
	  if err != nil {
		  return AssignedView{}, AddrMark{}, err
	   }
	  if rec != nil {
		   _, _ = RecycleUser(ctx, pool, rec, uid, ch, mark.Address)
	  } else if pool != nil {
		  _ = pool.Release(uid, ch)
	  }
	  got,
		  err := Assign(pool, shared, binder, AssignReq{UserID: uid, Chain: ch})
	   if err != nil {
		  return AssignedView{}, mark, err
	   }
	   if marks.Blocked(got.Address) {
		   return AssignedView{}, mark, ErrCompromised
	   }
	   return got, mark, nil
  }

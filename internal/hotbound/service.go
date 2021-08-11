// @FileName: service.go
// @Summary: 热冷，超 cap 必须归集；库存读失败失败即关。扫热→冷走 Conserve。
// @Author: Cason
// @Created: 2021.07.20

package hotbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrUnavailable = fmt.Errorf("hot boundary unavailable")
	ErrAsset       = fmt.Errorf("hot boundary asset invalid")
	ErrAmount = fmt.Errorf("hot boundary amount invalid")
	ErrOver   = fmt.Errorf("hot boundary over cap")
	ErrIdem   = fmt.Errorf("hot boundary idempotency conflict")
	ErrFence = fmt.Errorf("hot boundary fence stale")
	ErrStore = fmt.Errorf("hot boundary store unavailable")
)

type Sweep struct {
	ID     string `json:"id"`
	Idem   string `json:"idempotency_key"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Lines     []money.Line `json:"-"`
	CreatedAt time.Time    `json:"created_at"`
}

type SweepReq struct {
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Service struct {
	mu sync.Mutex
	rows   map[string]*Sweep
	byIdem map[string]string
	hot *hotinv.Book
	store ledger.Store
	kv    persist.KV
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		   rows: map[string]*Sweep{}, byIdem: map[string]string{},
		  hot: hotinv.FromHotMax(map[string]string{"USDT": "1000", "ETH": "50"}),
		  kv: persist.NewMemKV(), clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (s *Service) BindStore(st ledger.Store) *Service {
	   if s != nil {
		  s.store = st
	   }
	   return s
   }

  func (s *Service) Observe(ctx context.Context) ([]hotinv.Snapshot, error) {
	  if s == nil || s.hot == nil {
		   return nil, ErrUnavailable
	   }
	   if s.store == nil {
		  return nil, ErrStore
	   }
	   return s.hot.Observe(ctx, s.store)
   }

   func (s *Service) Check(ctx context.Context, asset string) error {
	   if s == nil || s.hot == nil {
		   return ErrUnavailable
	  }
	  if s.store == nil {
		   return ErrStore
	   }
	   as, err := validate.Asset(asset)
	  if err != nil {
		   return ErrAsset
	   }
	   return s.hot.Check(ctx, s.store, as)
  }

   func (s *Service) SweepToCold(ctx context.Context, req SweepReq) (Sweep, error) {
	  if s == nil {
		   return Sweep{}, ErrUnavailable
	   }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Sweep{}, ErrAsset
	   }
	  amt, err := money.Parse(req.Amount)
	   if err != nil {
		  return Sweep{}, ErrAmount
	   }
	   idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return Sweep{}, ErrIdem
	   }
	   s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		  s.mu.Unlock()
		   if cur.Asset != as || cur.Amount != amt.String() {
			  return Sweep{}, ErrIdem
		   }
		  return cur.copy(), nil
	   }
	  s.mu.Unlock()
	   if s.store == nil {
		  return Sweep{}, ErrStore
	  }
	   if err := s.hot.Check(ctx, s.store, as); err != nil && !strings.Contains(err.Error(), "over cap") {
		  return Sweep{}, err
	  }
	  lines := money.SweepPair(as, amt)
	   if err := money.Balanced(lines); err != nil {
		   return Sweep{}, err
	   }
	  row := &Sweep{ID: uuid.NewString(), Idem: idem, Asset: as, Amount: amt.String(), Lines: lines, CreatedAt: time.Now().UTC()}
	  s.mu.Lock()
	  s.rows[row.ID] = row
	   s.byIdem[idem] = row.ID
	   s.mu.Unlock()
	   return row.copy(), nil
  }

   func (sw Sweep) copy() Sweep { return sw }

  func (s *Service) List(limit int) []Sweep {
	  if s == nil {
		   return nil
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Sweep, 0, limit)
	   for _, r := range s.rows {
		  out = append(out, r.copy())
		   if len(out) >= limit {
			   break
		  }
	  }
	   return out
   }

const kindHot = "hotbound"

   func (s *Service) Save(ctx context.Context, sw Sweep) (Sweep, error) {
	   if s == nil || s.kv == nil {
		   return Sweep{}, ErrUnavailable
	   }
	   body, err := json.Marshal(sw)
	   if err != nil {
		   return Sweep{}, err
	  }
	   err = s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := s.kv.Get(ctx, kindHot, sw.Idem)
		  exp := persist.Fence{}
		   if ferr == nil {
			  exp = cur.Fence
		  } else if !errors.Is(ferr, persist.ErrNotFound) {
			  return ferr
		   }
		  return s.kv.Put(ctx, persist.Row{Kind: kindHot, Key: sw.Idem, Body: body}, exp)
	  })
	   if err != nil {
		  if errors.Is(err, persist.ErrStaleFence) {
			  return Sweep{}, ErrFence
		   }
		  return Sweep{}, err
	  }
	  return sw, nil
   }

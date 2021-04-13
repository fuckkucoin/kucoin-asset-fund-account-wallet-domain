// @FileName: service.go
// @Summary: 入金认领：memo/tag/utxo/地址归因，尘拒，确认数不够失败即关，幂等 credit。
// @Author: Finn
// @Created: 2021.03.31

package depclaim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

var (
	ErrUnavailable = fmt.Errorf("deposit claim unavailable")
	ErrUser = fmt.Errorf("deposit claim user invalid")
	ErrAttrib = fmt.Errorf("deposit claim not attributed")
	ErrDust    = fmt.Errorf("deposit claim dust")
	ErrConfirm = fmt.Errorf("deposit claim confirmations")
	ErrIdem    = fmt.Errorf("deposit claim idempotency conflict")
	ErrAmount  = fmt.Errorf("deposit claim amount invalid")
	ErrFence   = fmt.Errorf("deposit claim fence stale")
	ErrSecond  = fmt.Errorf("deposit claim second credit refused")
	ErrNetwork = fmt.Errorf("deposit claim wrong network")
)

type Claim struct {
	ID     string `json:"id"`
	Idem   string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	TxHash string `json:"tx_hash"`
	Attrib deposit.Binding   `json:"attrib"`
	Sight  addrpool.Sighting `json:"sight"`
	Lines  []money.Line      `json:"-"`
	Fence  persist.Fence     `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type ClaimReq struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount  string `json:"amount"`
	TxHash  string `json:"tx_hash"`
	Address string `json:"address"`
	Memo string `json:"memo"`
	Tag  string `json:"tag"`
	UTXO string `json:"utxo"`
	Confirms       int    `json:"confirmations"`
	IdempotencyKey string `json:"idempotency_key"`
	Network        string `json:"network"`
	OutputIndex    int    `json:"output_index"`
}

type Service struct {
	mu     sync.Mutex
	rows   map[string]*Claim
	byIdem map[string]string
	byOut map[string]string
	holds []Recovery
	attr *deposit.Attributor
	obs  *addrpool.Observer
	kv   persist.KV
	clock func() time.Time
}

  func New() *Service {
	  return &Service{
		   rows: map[string]*Claim{}, byIdem: map[string]string{}, byOut: map[string]string{},
		  attr: deposit.NewAttributor(), obs: addrpool.NewObserver(),
		   kv: persist.NewMemKV(),
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

  func (s *Service) Attributor() *deposit.Attributor { return s.attr }

   func (s *Service) Observer() *addrpool.Observer { return s.obs }

  func (s *Service) Claim(ctx context.Context, req ClaimReq) (Claim, error) {
	   if s == nil {
		   return Claim{}, ErrUnavailable
	   }
	  ch, err := validate.Chain(req.Chain)
	  if err != nil {
		  return Claim{}, err
	   }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return Claim{}, err
	   }
	   amt, err := money.Parse(req.Amount)
	  if err != nil {
		   return Claim{}, ErrAmount
	  }
	   if err := s.attr.DustReject(req.Amount); err != nil {
		  return Claim{}, ErrDust
	   }
	   idem, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		   return Claim{}, ErrIdem
	   }
	  hash, err := validate.TxHash(req.TxHash)
	   if err != nil {
		  return Claim{}, err
	   }
	  sight, err := s.obs.Observe(ch, req.Address, as, amt.String(), hash, req.Confirms)
	   if err != nil {
		   return Claim{}, err
	  }
	  if !sight.Ready {
		  return Claim{}, ErrConfirm
	  }
	   if deposit.WrongNetwork(ch, as, req.Network) {
		  s.land(req, deposit.KindWrongNetEvent, "wrong network")
		  return Claim{}, ErrNetwork
	   }
	   if strings.TrimSpace(req.Memo) == "" && strings.TrimSpace(req.Tag) == "" && strings.TrimSpace(req.UTXO) == "" {
		  s.land(req, deposit.KindUnattribEvent, "unattributed")
		   return Claim{}, ErrAttrib
	  }
	   b, err := s.attr.Resolve(ch, req.Memo, req.Tag, req.UTXO, "")
	   if err != nil {
		  s.land(req, deposit.KindWrongMemoEvent, err.Error())
		  return Claim{}, ErrAttrib
	  }
	   bound,
		  err := s.obs.BindUser(ch, hash, b.UserID)
	  if err != nil {
		   return Claim{}, err
	  }
	  lines := money.CreditPair(as, amt)
	  if err := money.Balanced(lines); err != nil {
		  return Claim{}, err
	  }
	  outKey := ch + "|" + hash + "|" + strconv.Itoa(req.OutputIndex)
	  s.mu.Lock()
	   if prev, ok := s.byOut[outKey]; ok {
		  cur := s.rows[prev]
		   s.mu.Unlock()
		  if cur != nil && cur.Idem == idem {
			  return cur.copy(), nil
		   }
		   s.land(req, deposit.KindDupOutput, "duplicate output")
		   return Claim{}, ErrSecond
	   }
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		  s.mu.Unlock()
		  if cur.UserID != b.UserID || cur.Amount != amt.String() || cur.TxHash != hash {
			  return Claim{}, ErrIdem
		  }
		  return cur.copy(), nil
	   }
	   row := &Claim{
		  ID: uuid.NewString(), Idem: idem, UserID: b.UserID, Chain: ch, Asset: as,
		  Amount: amt.String(), TxHash: hash, Attrib: b, Sight: bound, Lines: lines, CreatedAt: s.now(),
	  }
	   s.rows[row.ID] = row
	   s.byIdem[idem] = row.ID
	   s.byOut[outKey] = row.ID
	  s.mu.Unlock()
	   _ = ctx
	  return row.copy(), nil
  }

  func (c Claim) copy() Claim { return c }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
   }

   func (s *Service) List(userID string, limit int) []Claim {
	   if s == nil {
		   return nil
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Claim, 0, limit)
	   for _, r := range s.rows {
		  if userID != "" && r.UserID != userID {
			   continue
		  }
		   out = append(out, r.copy())
		   if len(out) >= limit {
			  break
		   }
	   }
	  return out
  }

const kindClaim = "depclaim"

  func (s *Service) Save(ctx context.Context, c Claim) (Claim, error) {
	   if s == nil || s.kv == nil {
		  return Claim{}, ErrUnavailable
	   }
	   body, err := json.Marshal(c)
	   if err != nil {
		   return Claim{}, err
	  }
	   err = s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := s.kv.Get(ctx, kindClaim, c.Idem)
		   exp := persist.Fence{}
		   if ferr == nil {
			   exp = cur.Fence
		   } else if !errors.Is(ferr, persist.ErrNotFound) {
			  return ferr
		  }
		  return s.kv.Put(ctx, persist.Row{Kind: kindClaim, Key: c.Idem, Body: body}, exp)
	  })
	   if err != nil {
		  if errors.Is(err, persist.ErrStaleFence) {
			   return Claim{}, ErrFence
		   }
		   return Claim{}, err
	  }
	   got, err := s.kv.Get(ctx, kindClaim, c.Idem)
	  if err != nil {
		   return Claim{}, err
	  }
	   var out Claim
	   if err := json.Unmarshal(got.Body, &out); err != nil {
		  return Claim{}, err
	   }
	  out.Fence = got.Fence
	   return out, nil
   }

   func (s *Service) Get(id string) (Claim,
	  error) {
	   if s == nil {
		  return Claim{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   row, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		  return Claim{}, ErrUnavailable
	  }
	  return row.copy(), nil
  }

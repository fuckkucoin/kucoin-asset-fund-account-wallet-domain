// @FileName: service.go
// @Summary: 领域服务：业务用例编排与校验。
// @Author: Cason
// @Created: 2021.09.21

package p2pnet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Service struct {
	mu     sync.Mutex
	rows   map[string]*Transfer
	byIdem map[string]string
	risk   *Screen
	vel    *Velocity
	led    *ledger.Service
	window time.Duration
	clock  func() time.Time
}

  func New() *Service {
	  return &Service{
		  rows: map[string]*Transfer{}, byIdem: map[string]string{},
		   risk: NewScreen(), vel: NewVelocity(), window: DefaultWindow,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (s *Service) BindLedger(led *ledger.Service) *Service {
	   if s != nil {
		  s.led = led
	   }
	   return s
   }

  func (s *Service) Risk() *Screen {
	  if s == nil {
		   return NewScreen()
	   }
	  return s.risk
  }

  func (s *Service) Velocity() *Velocity {
	  if s == nil {
		   return NewVelocity()
	   }
	  return s.vel
  }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	   return s.clock()
  }

// Send 资金账用户对用户。先过风控和流速，再记 SpotSend。
   func (s *Service) Send(ctx context.Context, req SendReq) (Transfer, error) {
	  if s == nil {
		  return Transfer{}, ErrUnavailable
	  }
	  from, err := validate.UserID(req.From)
	  if err != nil {
		  return Transfer{}, ErrUser
	  }
	   to, err := validate.UserID(req.To)
	   if err != nil {
		   return Transfer{}, ErrDest
	   }
	  asset,
		   err := validate.Asset(req.Asset)
	   if err != nil {
		   return Transfer{}, ErrAsset
	   }
	  amt, err := money.Parse(req.Amount)
	  if err != nil {
		  return Transfer{}, ErrAmount
	  }
	  idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		  return Transfer{}, ErrIdem
	  }
	   s.mu.Lock()
	  if id, ok := s.byIdem[idem]; ok {
		   cur := s.rows[id]
		  s.mu.Unlock()
		   if cur.From != from || cur.To != to || cur.Asset != asset || cur.Amount != amt.String() {
			   return Transfer{}, ErrIdem
		  }
		   return cur.Copy(), nil
	  }
	  s.mu.Unlock()
	   flags, err := s.risk.Check(from, to, asset, amt.String())
	   if err != nil {
		  return Transfer{}, err
	   }
	   if err := s.vel.Reserve(from, amt.String()); err != nil {
		  return Transfer{}, err
	  }
	   if s.led == nil {
		   return Transfer{}, ErrLedger
	  }
	  tr, err := s.led.Create(ctx, ledger.CreateReq{
		  UserID:    from,
		   Direction: ledger.SpotSend, Asset: asset,
		  Amount: amt.Dec(), ToAddress: to, IdempotencyKey: "p2p:" + idem, WaitConfirm: true,
	   })
	  if err != nil {
		   return Transfer{}, fmt.Errorf("%w: %v", ErrLedger, err)
	   }
	  now := s.now()
	   row := &Transfer{
		  ID: uuid.NewString(), IdempotencyKey: idem, From: from, To: to,
		   Asset:  asset,
		  Amount: amt.String(), LedgerID: tr.ID, State: StateSent,
		  Flags: flags, CreatedAt: now, UpdatedAt: now,
	  }
	  s.mu.Lock()
	   s.rows[row.ID] = row
	   s.byIdem[idem] = row.ID
	   s.mu.Unlock()
	   metrics.P2PSend.WithLabelValues("sent").Inc()
	  return row.Copy(), nil
   }

// Reverse 窗口内由对手原路退回。过窗拒绝，不得当已冲正。

  func (s *Service) Reverse(ctx context.Context, id, actor, reason string) (Transfer, error) {
	  if s == nil {
		   return Transfer{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		  return Transfer{}, ErrActor
	  }
	  reason, err := validate.Reason(reason)
	   if err != nil {
		   return Transfer{}, ErrReason
	   }
	  s.mu.Lock()
	  cur, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		  s.mu.Unlock()
		  return Transfer{}, ErrNotFound
	  }
	  if cur.State != StateSent {
		  s.mu.Unlock()
		  return cur.Copy(), ErrState
	   }
	  if !s.inWindow(cur.CreatedAt) {
		  s.mu.Unlock()
		   return cur.Copy(), ErrWindow
	  }
	  cp := cur.Copy()
	   s.mu.Unlock()
	   if s.led == nil {
		   return Transfer{}, ErrLedger
	  }
	   amt,
		  err := decimal.NewFromString(cp.Amount)
	  if err != nil {
		  return Transfer{}, ErrAmount
	  }
	   tr, err := s.led.Create(ctx, ledger.CreateReq{
		   UserID: cp.To, Direction: ledger.SpotSend, Asset: cp.Asset,
		  Amount: amt, ToAddress: cp.From, IdempotencyKey: "p2p-rev:" + cp.ID, WaitConfirm: true,
	   })
	   if err != nil {
		  return Transfer{}, fmt.Errorf("%w: %v", ErrLedger, err)
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur = s.rows[cp.ID]
	   cur.State = StateReversed
	  cur.ReverseID = tr.ID
	   cur.Reason = reason
	   cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
  }

  func (s *Service) Get(id string) (Transfer, error) {
	  if s == nil {
		   return Transfer{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		  return Transfer{}, ErrNotFound
	  }
	   return cur.Copy(), nil
  }

   func (s *Service) List(userID string, limit int) []Transfer {
	   if s == nil {
		  return nil
	   }
	  userID = strings.TrimSpace(userID)
	  if limit <= 0 {
		  limit = 50
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Transfer, 0, limit)
	  for _, r := range s.rows {
		   if userID != "" && r.From != userID && r.To != userID {
			  continue
		  }
		   out = append(out, r.Copy())
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out
   }

  func (s *Service) dumpLocked() []Transfer {
	  out := make([]Transfer, 0, len(s.rows))
	  for _, r := range s.rows {
		  out = append(out, r.Copy())
	   }
	  return out
   }

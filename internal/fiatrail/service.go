package fiatrail

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
	rows   map[string]*Booking
	byIdem map[string]string
	kyc    *Gate
	psp    *PSP
	limits *Limits
	led   *ledger.Service
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		  rows: map[string]*Booking{}, byIdem: map[string]string{},
		   kyc: NewGate(),
		   psp: NewPSP(), limits: NewLimits(),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (s *Service) BindLedger(led *ledger.Service) *Service {
	   if s != nil {
		  s.led = led
	  }
	   return s
   }

   func (s *Service) KYC() *Gate {
	  if s == nil {
		  return NewGate()
	  }
	  return s.kyc
   }

  func (s *Service) PSP() *PSP {
	   if s == nil {
		  return NewPSP()
	  }
	  return s.psp
  }

   func (s *Service) Limits() *Limits {
	  if s == nil {
		  return NewLimits()
	   }
	   return s.limits
  }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	   return s.clock()
  }

// Book 只落预约。KYC / PSP / 入账都要另走，避免未过闸就动账。
  func (s *Service) Book(req BookReq) (Booking, error) {
	  if s == nil {
		   return Booking{}, ErrUnavailable
	  }
	  uid,
		  err := validate.UserID(req.UserID)
	  if err != nil {
		  return Booking{}, ErrUser
	  }
	   kind, err := ParseKind(string(req.Kind))
	  if err != nil {
		   return Booking{}, err
	   }
	  asset, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Booking{}, ErrAsset
	  }
	  amt, err := money.Parse(req.Amount)
	  if err != nil {
		   return Booking{}, ErrAmount
	   }
	   cur := strings.ToUpper(strings.TrimSpace(req.Currency))
	   if cur == "" {
		   cur = "USD"
	  }
	  if _, err := validate.Asset(cur); err != nil {
		  return Booking{}, ErrCurrency
	   }
	  idem, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Booking{}, ErrIdem
	  }
	  card := strings.TrimSpace(req.CardRef)
	   if err := s.limits.Check(uid, amt.String(), card, kind); err != nil {
		   return Booking{}, err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if id, ok := s.byIdem[idem]; ok {
		   cur := s.rows[id]
		   if cur.UserID != uid || cur.Kind != kind || cur.Asset != asset || cur.Amount != amt.String() {
			   return Booking{}, ErrIdem
		   }
		   return cur.Copy(), nil
	   }
	  now := s.now()
	  b := &Booking{
		  ID: uuid.NewString(), IdempotencyKey: idem, UserID: uid, Kind: kind,
		  Asset: asset, Amount: amt.String(), Currency: cur, CardRef: card,
		  State: StateBooked, CreatedAt: now, UpdatedAt: now,
	  }
	   s.rows[b.ID] = b
	   s.byIdem[idem] = b.ID
	   return b.Copy(), nil
  }

   func (s *Service) Get(id string) (Booking, error) {
	  if s == nil {
		  return Booking{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  b, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Booking{}, ErrBooking
	   }
	  return b.Copy(), nil
  }

  func (s *Service) List(userID string, limit int) []Booking {
	   if s == nil {
		   return nil
	  }
	   userID = strings.TrimSpace(userID)
	   if limit <= 0 {
		  limit = 50
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Booking, 0, limit)
	  for _, b := range s.rows {
		  if userID != "" && b.UserID != userID {
			  continue
		  }
		  out = append(out, b.Copy())
		   if len(out) >= limit {
			  break
		  }
	  }
	  return out
  }

// ScreenKYC 预约过闸。无档或档位不够记失败，不得授权。

  func (s *Service) ScreenKYC(id string) (Booking, error) {
	   if s == nil {
		  return Booking{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   b, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		   s.mu.Unlock()
		  return Booking{}, ErrBooking
	  }
	  if err := CanAdvance(b.State, StateKYC); err != nil {
		   s.mu.Unlock()
		  return b.Copy(), err
	  }
	  kind, amount := b.Kind, b.Amount
	   uid := b.UserID
	   s.mu.Unlock()
	   p, err := s.kyc.Check(uid, kind, amount)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   b = s.rows[id]
	   if b == nil {
		   return Booking{}, ErrBooking
	  }
	   if err != nil {
		   b.State = StateFailed
		   b.FailReason = err.Error()
		   b.UpdatedAt = s.now()
		   return b.Copy(), err
	  }
	  b.State = StateKYC
	   b.KYCTier = p.Tier
	   b.UpdatedAt = s.now()
	  return b.Copy(), nil
  }

// Authorize 通道预授权。通道挂起失败即关。
   func (s *Service) Authorize(id string) (Booking, error) {
	   if s == nil {
		   return Booking{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  b, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  s.mu.Unlock()
		   return Booking{}, ErrBooking
	  }
	   if err := CanAdvance(b.State, StateAuthorized); err != nil {
		  s.mu.Unlock()
		   return b.Copy(), err
	  }
	  kind, amount, cur := b.Kind, b.Amount, b.Currency
	   s.mu.Unlock()
	  in, err := s.psp.Authorize(id, kind, amount, cur)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  b = s.rows[id]
	   if b == nil {
		   return Booking{}, ErrBooking
	   }
	   if err != nil {
		  b.State = StateFailed
		   b.FailReason = err.Error()
		   b.UpdatedAt = s.now()
		  return b.Copy(), err
	   }
	  b.State = StateAuthorized
	  b.PSPIntent = in.ID
	   b.UpdatedAt = s.now()
	  return b.Copy(), nil
  }

// Settle 捕获 PSP 并记内部流水。入金走 FiatOnramp，出金走 FiatOfframp。
  func (s *Service) Settle(ctx context.Context, id, actor string) (Booking, error) {
	   if s == nil {
		   return Booking{}, ErrUnavailable
	   }
	  actor,
		  err := validate.Actor(actor)
	  if err != nil {
		  return Booking{}, ErrActor
	  }
	   s.mu.Lock()
	   b, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		  s.mu.Unlock()
		  return Booking{}, ErrBooking
	  }
	  if err := CanAdvance(b.State, StateSettled); err != nil {
		   s.mu.Unlock()
		  return b.Copy(), err
	   }
	  cp := b.Copy()
	   s.mu.Unlock()
	   if _, err := s.psp.Capture(cp.PSPIntent); err != nil {
		  return s.fail(id, err)
	  }
	   tr, err := s.postLedger(ctx, cp)
	  if err != nil {
		  _, _ = s.psp.Refund(cp.PSPIntent)
		  return s.fail(id, fmt.Errorf("%w: %v", ErrLedger, err))
	  }
	   s.limits.Record(cp.UserID, cp.Amount, cp.Kind)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   b = s.rows[id]
	  b.State = StateSettled
	   b.LedgerID = tr.ID
	  b.Actor = actor
	   b.UpdatedAt = s.now()
	  metrics.FiatRail.WithLabelValues(string(b.Kind), string(b.State)).Inc()
	  return b.Copy(), nil
   }

   func (s *Service) postLedger(ctx context.Context, b Booking) (*ledger.Transfer, error) {
	  if s.led == nil {
		  return nil, ErrLedger
	  }
	  amt, err := decimal.NewFromString(b.Amount)
	   if err != nil {
		  return nil, ErrAmount
	  }
	   dir := ledger.FiatOnramp
	  if b.Kind == KindOfframp {
		   dir = ledger.FiatOfframp
	  }
	  return s.led.Create(ctx, ledger.CreateReq{
		  UserID: b.UserID, Direction: dir, Asset: b.Asset, Amount: amt,
		   IdempotencyKey: "fiat:" + b.ID, WaitConfirm: true,
	  })
   }

// Refund 未入账只作废授权；已入账再反记一笔并对 PSP 退款。
  func (s *Service) Refund(ctx context.Context, id, actor, reason string) (Booking, error) {
	   if s == nil {
		   return Booking{}, ErrUnavailable
	   }
	  actor, err := validate.Actor(actor)
	   if err != nil {
		  return Booking{}, ErrActor
	   }
	   reason, err = validate.Reason(reason)
	  if err != nil {
		  return Booking{}, ErrReason
	  }
	   s.mu.Lock()
	  b, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		   s.mu.Unlock()
		   return Booking{}, ErrBooking
	   }
	   if err := CanAdvance(b.State, StateRefunded); err != nil {
		  s.mu.Unlock()
		  return b.Copy(), err
	  }
	  cp := b.Copy()
	  s.mu.Unlock()
	  var refundID string
	  switch cp.State {
	  case StateAuthorized:
		   if _, err := s.psp.Void(cp.PSPIntent); err != nil {
			   return s.fail(id, err)
		  }
	   case StateSettled:
		  if _, err := s.psp.Refund(cp.PSPIntent); err != nil {
			   return Booking{}, fmt.Errorf("%w: %v", ErrRefund, err)
		   }
		  rev := cp
		   if cp.Kind == KindOnramp {
			   rev.Kind = KindOfframp
		   } else {
			  rev.Kind = KindOnramp
		   }
		  rev.ID = cp.ID + ":refund"
		   tr, err := s.postLedger(ctx, rev)
		   if err != nil {
			   return Booking{}, fmt.Errorf("%w: %v", ErrRefund, err)
		   }
		   refundID = tr.ID
	  default:
		   return Booking{}, ErrRefund
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  b = s.rows[id]
	   b.State = StateRefunded
	   b.Actor = actor
	  b.Reason = reason
	  b.RefundLedger = refundID
	  b.UpdatedAt = s.now()
	  return b.Copy(), nil
  }

  func (s *Service) Fail(id, reason string) (Booking, error) {
	   return s.fail(id, fmt.Errorf("%s", strings.TrimSpace(reason)))
   }

   func (s *Service) fail(id string, cause error) (Booking, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   b, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		  return Booking{}, ErrBooking
	  }
	  if b.State.Terminal() {
		  return b.Copy(), cause
	  }
	  if err := CanAdvance(b.State, StateFailed); err != nil {
		   return b.Copy(), err
	  }
	   b.State = StateFailed
	   if cause != nil {
		  b.FailReason = cause.Error()
	   }
	   b.UpdatedAt = s.now()
	   return b.Copy(), cause
   }

  func (s *Service) dumpLocked() []Booking {
	  out := make([]Booking, 0, len(s.rows))
	   for _, b := range s.rows {
		  out = append(out, b.Copy())
	   }
	   return out
   }

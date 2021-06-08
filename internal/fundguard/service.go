// @FileName: service.go
// @Summary: 资金护栏与限流服务实现。
// @Author: CainReed
// @Created: 2021.06.07

package fundguard

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Service struct {
	mu sync.Mutex
	halt bool
	seen map[string]HashClaim
	snaps []Bundle
	led   *ledger.Service
	clock func() time.Time
	leaser  LeaseService
	proofer ProofService
}

   func New() *Service {
	   return &Service{seen: map[string]HashClaim{}, clock: func() time.Time { return time.Now().UTC() }}
   }

  func (s *Service) BindLedger(l *ledger.Service) *Service {
	   if s != nil {
		   s.led = l
	   }
	   return s
   }

  func (s *Service) SetHalt(on bool) {
	  if s == nil {
		   return
	   }
	  s.mu.Lock()
	   s.halt = on
	  s.mu.Unlock()
   }

   func (s *Service) Halted() bool {
	  if s == nil {
		  return false
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.halt
   }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
  }

   func hashKey(chain, hash string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(hash))
   }

   func requireActor(s string) (string, error) {
	   a, err := validate.Actor(s)
	   if err != nil {
		  return "", ErrActor
	  }
	   return a, nil
  }

  func (s *Service) Snapshot(ctx context.Context, userID, asset string) (Bundle, error) {
	  if s == nil {
		  return Bundle{}, ErrUnavailable
	   }
	   if s.Halted() {
		  return Bundle{}, ErrHalt
	  }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return Bundle{}, ErrUser
	  }
	   as, err := validate.Asset(asset)
	  if err != nil {
		  return Bundle{}, ErrAsset
	  }
	  if s.led == nil {
		   return Bundle{}, ErrUnavailable
	  }
	  rows, err := s.led.Accounts(ctx, uid)
	  if err != nil {
		   return Bundle{}, err
	  }
	  out := Bundle{UserID: uid, Asset: as, At: s.now()}
	   sum := money.Zero()
	   for _, r := range rows {
		   if !strings.EqualFold(r.Asset,
			  as) {
			   continue
		  }
		  avail, err := money.ParseBook(orZ(r.Available))
		  if err != nil {
			  return Bundle{}, ErrAmount
		  }
		  fr, err := money.ParseBook(orZ(r.Frozen))
		  if err != nil {
			  return Bundle{}, ErrAmount
		  }
		  tot, err := avail.Add(fr)
		  if err != nil {
			   return Bundle{}, err
		  }
		   next, err := sum.Add(tot)
		  if err != nil {
			  return Bundle{}, err
		   }
		   sum = next
		   out.Rows = append(out.Rows, Snap{
			  UserID: uid, Book: r.Subject, Asset: as,
			  Available: avail.String(), Frozen: fr.String(),
		  })
	   }
	  out.Total = sum.String()
	  s.mu.Lock()
	   s.snaps = append(s.snaps, out.Copy())
	   s.mu.Unlock()
	   return out.Copy(), nil
   }

   func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		   return "0"
	   }
	   return s
   }

  func (s *Service) Conserve(before, after Bundle) error {
	   if before.Asset != after.Asset || before.UserID != after.UserID {
		   return ErrSnap
	  }
	   a, err := money.ParseBook(orZ(before.Total))
	  if err != nil {
		  return ErrAmount
	   }
	  b,
		  err := money.ParseBook(orZ(after.Total))
	   if err != nil {
		   return ErrAmount
	  }
	   if err := money.Conserve(a, b); err != nil {
		   return ErrConserve
	   }
	   return nil
   }

   func (s *Service) ClaimHash(chain, hash, idem string) (HashClaim, error) {
	  if s == nil {
		  return HashClaim{}, ErrUnavailable
	   }
	  if s.Halted() {
		  return HashClaim{}, ErrHalt
	   }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		  return HashClaim{}, ErrHash
	  }
	  h, err := validate.TxHash(hash)
	  if err != nil {
		  return HashClaim{}, ErrHash
	  }
	  id, err := validate.Idem(idem)
	   if err != nil {
		  return HashClaim{}, ErrHash
	   }
	   k := hashKey(ch, h)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if cur, ok := s.seen[k]; ok {
		   if cur.Idem == id {
			  return cur, nil
		   }
		   return HashClaim{}, ErrDup
	  }
	   row := HashClaim{Chain: ch, TxHash: h, Idem: id, At: s.now()}
	   s.seen[k] = row
	   return row, nil
  }

   func (s *Service) RefuseFrozen(ctx context.Context, userID string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	   if s.Halted() {
		   return ErrHalt
	  }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return ErrUser
	  }
	   if s.led == nil {
		  return ErrUnavailable
	   }
	  if err := s.led.CheckUser(ctx, uid); err != nil {
		  return ErrFrozen
	  }
	   return nil
  }

   func (s *Service) CheckPartial(p Partial) error {
	   if p.Submitted+p.Skipped+p.Failed != p.Total {
		   return ErrPartial
	   }
	   if p.Total < 0 || p.Submitted < 0 || p.Skipped < 0 || p.Failed < 0 {
		   return ErrPartial
	   }
	   return nil
   }

   func (s *Service) Gate(ctx context.Context, req Check) (Verdict, error) {
	   // 护栏入口：halt → 账户冻结 → 链上 hash 占用 → 账本快照；任一步失败即关。
	   if _, err := requireActor(req.Actor); err != nil {
		  return Verdict{}, err
	  }
	  if err := s.RefuseFrozen(ctx, req.UserID); err != nil {
		  return Verdict{Reason: err.Error()}, err
	  }
	  if req.TxHash != "" {
		   if _, err := s.ClaimHash(req.Chain, req.TxHash, req.Idem); err != nil {
			  return Verdict{Reason: err.Error()}, err
		  }
	  }
	  snap, err := s.Snapshot(ctx, req.UserID, req.Asset)
	   if err != nil {
		   return Verdict{Reason: err.Error()}, err
	   }
	   return Verdict{Allow: true,
		   Total: snap.Total}, nil
  }

   func (s *Service) Snaps() []Bundle {
	  if s == nil {
		  return nil
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Bundle, 0, len(s.snaps))
	  for _, b := range s.snaps {
		  out = append(out, b.Copy())
	  }
	  return out
   }

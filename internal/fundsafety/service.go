package fundsafety

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Service struct {
	mu     sync.Mutex
	halt   bool
	seen   map[string]CreditClaim
	fences map[string]persist.Fence
	batches  map[string]*BatchRun
	credits  map[string]CreditOutbox
	confirms map[string]ConfirmWin
	led      *ledger.Service
	dep      *deposit.Service
	clock     func() time.Time
	haltView  HaltViewer
	reconView ReconViewer
	laneView LaneViewer
	// creditGenNotifiers 入金贷记出箱世代翻转通知器列表（由 BindCreditGenNotifier 注册）。
	creditGenNotifiers []CreditGenNotifier
	// creditEnqueueHook EnqueueCredit 成功后的世代绑定钩子（由 BindCreditEnqueueHook 注册）。
	creditEnqueueHook CreditEnqueueHook
}

  func New() *Service {
	   return &Service{
		  seen:   map[string]CreditClaim{},
		  fences: map[string]persist.Fence{},
		   batches: map[string]*BatchRun{},
		  credits: map[string]CreditOutbox{},
		   confirms: map[string]ConfirmWin{},
		  clock:    func() time.Time { return time.Now().UTC() },
	  }
   }

  func (s *Service) BindLedger(l *ledger.Service) *Service {
	   if s != nil {
		  s.led = l
	  }
	   return s
   }

  func (s *Service) BindDeposit(d *deposit.Service) *Service {
	  if s != nil {
		  s.dep = d
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

   func (s *Service) RefuseNegative(amount string) error {
	  raw := strings.TrimSpace(amount)
	   if strings.HasPrefix(raw, "-") {
		   return ErrNegative
	  }
	   amt, err := money.Parse(raw)
	   if err != nil {
		  if errors.Is(err, money.ErrNotPositive) {
			  return ErrNegative
		   }
		   return ErrAmount
	  }
	   if !amt.IsPositive() {
		  return ErrNegative
	  }
	   return nil
   }

  func rejectSci(s string) error {
	   for _, r := range s {
		  if r == 'e' || r == 'E' || unicode.IsLetter(r) {
			  return ErrAmount
		  }
	   }
	   return nil
  }

  func (s *Service) CheckPartial(p Partial) error {
	  for _, v := range []string{p.Requested, p.Submitted, p.Skipped, p.Failed} {
		   if strings.TrimSpace(v) == "" {
			  continue
		  }
		  if err := rejectSci(v); err != nil {
			  return err
		   }
	  }
	  req, err := money.ParseBook(orZ(p.Requested))
	  if err != nil {
		   return ErrAmount
	   }
	  sub, err := money.ParseBook(orZ(p.Submitted))
	   if err != nil {
		   return ErrAmount
	   }
	   sk, err := money.ParseBook(orZ(p.Skipped))
	   if err != nil {
		   return ErrAmount
	   }
	   fl, err := money.ParseBook(orZ(p.Failed))
	  if err != nil {
		   return ErrAmount
	  }
	  right, err := sub.Add(sk)
	   if err != nil {
		  return err
	  }
	   right, err = right.Add(fl)
	   if err != nil {
		   return err
	  }
	   if err := money.Conserve(req, right); err != nil {
		   return ErrPartial
	   }
	   return nil
  }

   func orZ(v string) string {
	  if strings.TrimSpace(v) == "" {
		  return "0"
	  }
	   return v
  }

  func (s *Service) ClaimHash(chain, hash, idem, user, asset string) (CreditClaim, error) {
	   if s == nil {
		   return CreditClaim{}, ErrUnavailable
	  }
	  if s.Halted() {
		   return CreditClaim{}, ErrHalt
	   }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		  return CreditClaim{}, ErrHash
	  }
	  h, err := validate.TxHash(hash)
	  if err != nil {
		   return CreditClaim{}, ErrHash
	   }
	   id, err := validate.Idem(idem)
	   if err != nil {
		  return CreditClaim{}, ErrHash
	  }
	   uid, err := validate.UserID(user)
	   if err != nil {
		  return CreditClaim{}, ErrUser
	   }
	   as, err := validate.Asset(asset)
	  if err != nil {
		  return CreditClaim{}, ErrAsset
	  }
	  if err := s.GuardBoardLane(context.Background(), "deposit", uid, ch, as); err != nil {
		   return CreditClaim{}, err
	   }
	   k := hashKey(ch, h)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if cur, ok := s.seen[k]; ok {
		  if cur.Idem == id && cur.UserID == uid {
			  return cur, nil
		  }
		   return CreditClaim{}, ErrDup
	   }
	  tok := persist.NewToken()
	   row := CreditClaim{Chain: ch, TxHash: h, Idem: id, UserID: uid, Asset: as, Epoch: 1, Token: tok, At: s.now()}
	  s.seen[k] = row
	  s.fences[k] = persist.Fence{Epoch: 1, Token: tok}
	   return row, nil
   }

  func (s *Service) RefuseFrozen(ctx context.Context, userID string) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   if err := s.GuardCombo(ctx, userID); err != nil {
		  return err
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

  func (s *Service) ObserveOnce(ctx context.Context, req deposit.ObserveReq) (*ledger.Transfer, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if s.Halted() {
		   return nil, ErrHalt
	   }
	  if err := s.GuardBoardLane(ctx, "deposit", req.UserID, req.Chain, req.Asset); err != nil {
		  return nil, err
	   }
	  if err := s.RefuseFrozen(ctx, req.UserID); err != nil {
		  return nil, err
	   }
	  claim, err := s.ClaimHash(req.Chain, req.TxHash, req.IdempotencyKey, req.UserID, req.Asset)
	  if err != nil {
		   return nil, err
	  }
	   if s.dep == nil {
		  return nil, ErrUnavailable
	  }
	  tr, err := s.dep.Observe(ctx, req)
	   if err != nil {
		  if errors.Is(err, ledger.ErrDuplicateHash) {
			  return nil, ErrDup
		  }
		  return nil, err
	   }
	   if tr != nil {
		   s.mu.Lock()
		  claim.Ledger = tr.ID
		   s.seen[hashKey(claim.Chain, claim.TxHash)] = claim
		   s.mu.Unlock()
	  }
	   return tr, nil
  }

   func (s *Service) ReorgThenReplay(ctx context.Context, id string, req deposit.ObserveReq) (ReorgReplay, error) {
	   if s == nil {
		  return ReorgReplay{}, ErrUnavailable
	  }
	  if err := s.GuardBoardLane(ctx, "deposit", req.UserID, req.Chain, req.Asset); err != nil {
		  return ReorgReplay{}, err
	   }
	   out := ReorgReplay{TransferID: strings.TrimSpace(id)}
	  if s.dep == nil || s.led == nil {
		   return out, ErrUnavailable
	  }
	  if err := s.dep.Unwind(ctx, id); err != nil {
		   out.Reason = err.Error()
		  return out, ErrReorg
	  }
	   out.Unwound = true
	   tr, err := s.dep.Observe(ctx, req)
	   if err != nil {
		   if errors.Is(err, ledger.ErrDuplicateHash) {
			  out.Reason = ErrDup.Error()
			   return out, ErrDup
		  }
		  out.Reason = err.Error()
		   return out, err
	  }
	   out.Replayed = tr != nil
	   if tr != nil && tr.ID != id {
		   out.Second = true
		   return out, ErrDup
	   }
	  return out, nil
  }

   func (s *Service) Claims() []CreditClaim {
	   if s == nil {
		  return nil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]CreditClaim,
		   0, len(s.seen))
	   for _, v := range s.seen {
		  out = append(out, v)
	  }
	  return out
   }

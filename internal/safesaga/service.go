package safesaga

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu   sync.Mutex
	halt bool
	runs map[string]*Run
	led   *ledger.Service
	pair  *ha.Pair
	pipe  *signpipe.Pipe
	clock func() time.Time
}

  func New() *Service {
	   return &Service{
		   runs: map[string]*Run{},
		  pipe: signpipe.New(0),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Service) BindLedger(l *ledger.Service) *Service {
	   if s != nil {
		  s.led = l
	  }
	   return s
   }

  func (s *Service) BindPair(p *ha.Pair) *Service {
	  if s != nil && p != nil {
		   s.pair = p
	   }
	  return s
  }

   func (s *Service) BindPipe(p *signpipe.Pipe) *Service {
	  if s != nil && p != nil {
		  s.pipe = p
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

   func (s *Service) requireWrite() error {
	   if s.pair == nil {
		   return nil
	   }
	  if s.pair.SplitBrain() {
		   return ErrSplit
	  }
	  if err := s.pair.RequireWrite(); err != nil {
		   return ErrSplit
	  }
	  return nil
   }

   func (s *Service) Open(user, asset, amount, idem string) (Run, error) {
	  if s == nil {
		   return Run{}, ErrUnavailable
	  }
	  if s.Halted() {
		  return Run{}, ErrHalt
	  }
	  uid, err := validate.UserID(user)
	  if err != nil {
		   return Run{}, ErrUser
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return Run{}, ErrAsset
	   }
	   if _, err := money.Parse(amount); err != nil {
		   return Run{}, ErrAmount
	   }
	   idk, err := validate.Idem(idem)
	  if err != nil {
		   return Run{}, ErrIdem
	  }
	   if err := s.requireWrite(); err != nil {
		  return Run{}, err
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, r := range s.runs {
		  if r.Idem == idk {
			   cp := *r
			  return cp, nil
		  }
	   }
	  row := &Run{
		   ID:     uuid.NewString(),
		  UserID: uid, Asset: as, Amount: amount, Idem: idk,
		   Step: StepOpen, Epoch: 1, Token: persist.NewToken(), At: s.now(),
	   }
	   s.runs[row.ID] = row
	   return *row, nil
   }

   func (s *Service) checkFenceLocked(r *Run, epoch int64, token string) error {
	  if r.Epoch != epoch || r.Token == "" || r.Token != token {
		  return ErrFence
	  }
	  return nil
  }

   func (s *Service) bumpLocked(r *Run, step Step, note string) {
	  r.Step = step
	  r.Epoch++
	   r.Token = persist.NewToken()
	   r.Note = note
	   r.At = s.now()
   }

   func (s *Service) Advance(ctx context.Context, id, actor string, epoch int64, token string) (Run, error) {
	  if s == nil {
		  return Run{}, ErrUnavailable
	   }
	   if s.Halted() {
		   return Run{}, ErrHalt
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return Run{}, ErrActor
	   }
	   if err := s.requireWrite(); err != nil {
		   return Run{}, err
	  }
	  s.mu.Lock()
	  r, ok := s.runs[strings.TrimSpace(id)]
	   if !ok {
		   s.mu.Unlock()
		  return Run{}, ErrUnavailable
	  }
	   if err := s.checkFenceLocked(r, epoch, token); err != nil {
		  s.mu.Unlock()
		  return Run{}, err
	  }
	  step := r.Step
	   uid, asset, amt, idem := r.UserID, r.Asset, r.Amount, r.Idem
	   s.mu.Unlock()
	   switch step {
	   case StepOpen:
		  if s.led != nil {
			  if err := s.led.CheckUser(ctx, uid); err != nil {
				   return s.abort(id, err.Error())
			   }
		   }
		  return s.commitStep(id, StepFreeze, "")
	  case StepFreeze:
		   if s.led != nil {
			   if _, err := s.led.Create(ctx, ledger.CreateReq{
				  UserID: uid, Direction: ledger.FundingToTrading, Asset: asset,
				  Amount: decimal.RequireFromString(amt), IdempotencyKey: idem + "-move", WaitConfirm: true,
			   }); err != nil {
				   return s.compensate(ctx, id, err.Error())
			   }
		  }
		  return s.commitStep(id, StepMove, "")
	  case StepMove:
		  if s.pipe != nil {
			   job, err := s.pipe.Open(actor, idem, "ETH", "0x"+strings.Repeat("ab", 20), amt)
			  if err != nil {
				   return s.compensate(ctx, id, err.Error())
			   }
			   _ = job
		   }
		  return s.commitStep(id, StepSign, "")
	   case StepSign:
		   return s.commitStep(id, StepCommit, "")
	   case StepCommit:
		   return s.commitStep(id, StepDone, "")
	   default:
		  return Run{}, ErrStep
	  }
   }

   func (s *Service) commitStep(id string, step Step, note string) (Run, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   r := s.runs[id]
	   if r == nil {
		  return Run{}, ErrUnavailable
	  }
	   s.bumpLocked(r, step, note)
	  return *r, nil
   }

   func (s *Service) abort(id, note string) (Run, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  r := s.runs[id]
	  if r == nil {
		   return Run{}, ErrUnavailable
	   }
	  s.bumpLocked(r, StepAborted, note)
	   return *r, ErrStep
   }

  func (s *Service) compensate(ctx context.Context, id, note string) (Run, error) {
	   s.mu.Lock()
	   r := s.runs[id]
	  if r == nil {
		  s.mu.Unlock()
		  return Run{}, ErrUnavailable
	   }
	  s.bumpLocked(r, StepCompensate, note)
	   cp := *r
	  s.mu.Unlock()
	  _ = ctx
	  return cp, ErrCompensate
   }

   func (s *Service) SignFailThenRetry(id, reqActor, apprActor, signActor string, signer signpipe.Signer) error {
	  if s == nil || s.pipe == nil {
		   return ErrUnavailable
	   }
	  if err := s.requireWrite(); err != nil {
		  return err
	  }
	  s.mu.Lock()
	  r := s.runs[strings.TrimSpace(id)]
	   s.mu.Unlock()
	   if r == nil {
		  return ErrUnavailable
	   }
	  job, err := s.pipe.Open(reqActor, r.Idem, "ETH", "0x"+strings.Repeat("cd", 20), r.Amount)
	  if err != nil {
		  return err
	   }
	   job, err = s.pipe.Approve(job.ID, apprActor, job.Fence.Epoch, job.Fence.Token, job.MovementID, job.Chain, job.To, job.Amount)
	   if err != nil {
		  return err
	  }
	   fail := func(string) (string, error) { return "", ErrSign }
	  if _, err := s.pipe.Sign(job.ID, signActor, job.Fence.Epoch, job.Fence.Token, fail); err == nil {
		  return ErrSign
	  }
	   failed, err := s.pipe.Get(job.ID)
	  if err != nil {
		   return err
	   }
	  if _, err := s.pipe.RetrySign(failed.ID, signActor, failed.Fence.Epoch, failed.Fence.Token, signer); err != nil {
		  return err
	  }
	  return nil
  }

   func (s *Service) Get(id string) (Run, error) {
	  if s == nil {
		  return Run{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  r := s.runs[strings.TrimSpace(id)]
	   if r == nil {
		  return Run{}, ErrUnavailable
	  }
	  return *r, nil
  }

  func (s *Service) List() []Run {
	  if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Run,
		   0, len(s.runs))
	  for _, r := range s.runs {
		  out = append(out, *r)
	   }
	   return out
  }

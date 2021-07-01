package fundsafety

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

  func parseLineAmt(raw string) (money.Amount, error) {
	   raw = strings.TrimSpace(raw)
	   if err := rejectSci(raw); err != nil {
		  return money.Zero(), err
	  }
	   amt, err := money.ParseBook(orZ(raw))
	   if err != nil {
		  return money.Zero(), ErrAmount
	   }
	   if amt.IsNegative() {
		   return money.Zero(), ErrNegative
	   }
	   return amt, nil
   }

  func (s *Service) OpenBatch(user, asset, requested string) (BatchRun, error) {
	   if s == nil {
		  return BatchRun{},
			  ErrUnavailable
	   }
	  if err := s.GuardBoardLane(context.Background(), "withdraw", user, "", ""); err != nil {
		  return BatchRun{}, err
	   }
	  uid, err := validate.UserID(user)
	   if err != nil {
		   return BatchRun{}, ErrUser
	   }
	   as, err := validate.Asset(asset)
	  if err != nil {
		   return BatchRun{}, ErrAsset
	   }
	  req,
		  err := parseLineAmt(requested)
	  if err != nil {
		  return BatchRun{}, err
	   }
	   if !req.IsPositive() {
		  return BatchRun{}, ErrNegative
	   }
	   row := &BatchRun{
		  ID: uuid.NewString(), UserID: uid, Asset: as, Requested: req.String(),
		   Submitted: "0", Skipped: "0", Failed: "0",
		   Epoch: 1, Token: persist.NewToken(), At: s.now(),
	  }
	   s.mu.Lock()
	   s.batches[row.ID] = row
	   s.mu.Unlock()
	   return *row, nil
  }

  func (s *Service) batchLocked(id string) (*BatchRun, error) {
	  r := s.batches[strings.TrimSpace(id)]
	   if r == nil {
		   return nil, ErrBatch
	   }
	   return r, nil
   }

  func addBook(cur, delta string) (string, error) {
	  a, err := money.ParseBook(orZ(cur))
	  if err != nil {
		   return "", ErrAmount
	   }
	   b, err := money.ParseBook(orZ(delta))
	   if err != nil {
		  return "", ErrAmount
	   }
	   sum, err := a.Add(b)
	  if err != nil {
		   return "", err
	  }
	  return sum.String(), nil
  }

// AppendLine 按围栏追加一行。关批后拒。累加后必须仍能 Conserve。

  func (s *Service) AppendLine(id, kind, amount, note string, epoch int64, token string) (BatchRun, error) {
	   if s == nil {
		  return BatchRun{}, ErrUnavailable
	  }
	   if err := s.GuardBoardLane(context.Background(), "sweep", "", "", ""); err != nil {
		  return BatchRun{}, err
	   }
	   kind = strings.ToLower(strings.TrimSpace(kind))
	   switch kind {
	  case "submit", "skip", "fail":
	  default:
		  return BatchRun{}, ErrBatch
	   }
	  amt, err := parseLineAmt(amount)
	   if err != nil {
		  return BatchRun{}, err
	   }
	   if !amt.IsPositive() {
		  return BatchRun{}, ErrNegative
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   r, err := s.batchLocked(id)
	   if err != nil {
		   return BatchRun{}, err
	  }
	   if r.Closed {
		  return BatchRun{}, ErrClosed
	  }
	  if r.Epoch != epoch || r.Token == "" || r.Token != token {
		  return BatchRun{}, ErrFence
	  }
	  switch kind {
	   case "submit":
		  r.Submitted, err = addBook(r.Submitted, amt.String())
	  case "skip":
		   r.Skipped, err = addBook(r.Skipped, amt.String())
	   case "fail":
		   r.Failed, err = addBook(r.Failed, amt.String())
	   }
	  if err != nil {
		   return BatchRun{}, err
	   }
	   r.Lines = append(r.Lines, BatchLine{Kind: kind, Amount: amt.String(), Note: strings.TrimSpace(note)})
	  r.Epoch++
	   r.Token = persist.NewToken()
	  r.At = s.now()
	  if err := s.checkPartialLocked(r); err != nil && err != ErrPartial {
		   return BatchRun{}, err
	   }
	   return *r, nil
  }

  func (s *Service) checkPartialLocked(r *BatchRun) error {
	   return s.CheckPartial(Partial{
		  Requested: r.Requested, Submitted: r.Submitted, Skipped: r.Skipped, Failed: r.Failed,
	   })
   }

// CloseBatch 关批时必须 Conserve。未配平失败即关，围栏作废。

  func (s *Service) CloseBatch(id string, epoch int64, token string) (BatchRun, error) {
	  if s == nil {
		   return BatchRun{},
			  ErrUnavailable
	  }
	  if err := s.GuardBoardLane(context.Background(), "sweep", "", "", ""); err != nil {
		  return BatchRun{}, err
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   r, err := s.batchLocked(id)
	  if err != nil {
		  return BatchRun{}, err
	   }
	   if r.Closed {
		   return *r, nil
	  }
	  if r.Epoch != epoch || r.Token == "" || r.Token != token {
		  return BatchRun{}, ErrFence
	   }
	   if err := s.checkPartialLocked(r); err != nil {
		   return BatchRun{}, err
	  }
	   r.Closed = true
	   r.Epoch++
	  r.Token = persist.NewToken()
	   r.At = s.now()
	  return *r, nil
  }

  func (s *Service) GetBatch(id string) (BatchRun, error) {
	  if s == nil {
		  return BatchRun{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   r, err := s.batchLocked(id)
	  if err != nil {
		   return BatchRun{}, err
	   }
	  return *r, nil
  }

  func (s *Service) Batches() []BatchRun {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]BatchRun,
		  0, len(s.batches))
	   for _, r := range s.batches {
		  if r != nil {
			  out = append(out, *r)
		   }
	   }
	   return out
  }

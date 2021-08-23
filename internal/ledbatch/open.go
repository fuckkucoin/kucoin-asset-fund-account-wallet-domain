package ledbatch

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

  func (s *Service) Open(req OpenReq) (Batch, error) {
	   if s == nil {
		   return Batch{}, ErrUnavailable
	   }
	  act, err := requireActor(req.Actor)
	   if err != nil {
		  return Batch{}, err
	   }
	  idem, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Batch{}, ErrIdem
	  }
	  if len(req.Items) == 0 || len(req.Items) > 64 {
		  return Batch{}, ErrEmpty
	  }
	   items := make([]Item, 0, len(req.Items))
	   reqAmt := money.Zero()
	  for i, it := range req.Items {
		  row, err := shapeHop(it, i+1)
		  if err != nil {
			   return Batch{}, err
		   }
		  amt, err := money.Parse(row.Amount)
		   if err != nil {
			   return Batch{}, ErrAmount
		   }
		  next, err := reqAmt.Add(amt)
		   if err != nil {
			   return Batch{}, err
		  }
		  reqAmt = next
		  items = append(items, row)
	  }
	   s.mu.Lock()
	  if id, ok := s.byIdem[idem]; ok {
		   cur := s.rows[id]
		  s.mu.Unlock()
		   if cur.Requested != reqAmt.String() || cur.Actor != act {
			  return Batch{}, ErrIdem
		   }
		   return cur.Copy(), nil
	   }
	  now := s.now()
	   row := &Batch{
		  ID: uuid.NewString(), IdempotencyKey: idem, Actor: act, Status: StatusOpen,
		   Requested: reqAmt.String(), Submitted: "0", Skipped: "0", Failed: "0",
		   Items: items, CreatedAt: now, UpdatedAt: now,
	   }
	  s.rows[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	  return row.Copy(), nil
  }

  func shapeHop(it Item, seq int) (Item, error) {
	   uid, err := validate.UserID(it.UserID)
	  if err != nil {
		   return Item{}, ErrUser
	  }
	  as, err := validate.Asset(it.Asset)
	  if err != nil {
		  return Item{}, ErrAsset
	  }
	  amt, err := money.Parse(it.Amount)
	  if err != nil {
		   return Item{}, ErrAmount
	   }
	   if _, err := parseDir(it.Direction); err != nil {
		   return Item{}, err
	   }
	   idem, err := validate.Idem(it.IdempotencyKey)
	   if err != nil {
		  return Item{}, ErrIdem
	  }
	  dir := strings.ToLower(strings.TrimSpace(it.Direction))
	   if dir == "" {
		  dir = string(ledger.FundingToTrading)
	   }
	   return Item{
		   Seq: seq, UserID: uid, Direction: dir, Asset: as,
		  Amount: amt.String(), IdempotencyKey: idem, Status: ItemPending,
	  }, nil
  }

   func parseDir(s string) (ledger.Direction, error) {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case "", string(ledger.FundingToTrading):
		   return ledger.FundingToTrading, nil
	  case string(ledger.TradingToFunding):
		  return ledger.TradingToFunding,
			  nil
	  case string(ledger.FundingToMargin):
		   return ledger.FundingToMargin, nil
	   case string(ledger.MarginToFunding):
		  return ledger.MarginToFunding, nil
	   default:
		  return "", ErrDir
	  }
  }

  func (s *Service) List(limit int) []Batch {
	  if s == nil {
		   return nil
	   }
	  if limit <= 0 {
		   limit = 50
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Batch, 0, limit)
	  for _, r := range s.rows {
		   out = append(out, r.Copy())
		  if len(out) >= limit {
			   break
		  }
	   }
	   return out
  }

   func (s *Service) Cancel(id, actor string) (Batch, error) {
	  if s == nil {
		  return Batch{}, ErrUnavailable
	   }
	  if _, err := requireActor(actor); err != nil {
		   return Batch{}, err
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur,
		   ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Batch{}, ErrBatch
	   }
	   if cur.Status != StatusOpen {
		  return cur.Copy(), ErrState
	  }
	   cur.Status = StatusCanceled
	  cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
   }

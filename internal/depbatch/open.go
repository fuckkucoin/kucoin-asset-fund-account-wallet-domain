package depbatch

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
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
	   seen := map[string]struct{}{}
	  for i, it := range req.Items {
		   row, err := shapeObs(it, i+1)
		   if err != nil {
			  return Batch{}, err
		   }
		  k := hashKey(row.Chain, row.TxHash)
		   if _, ok := seen[k]; ok {
			   return Batch{}, ErrDup
		  }
		  seen[k] = struct{}{}
		  items = append(items, row)
	  }
	   s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		   cur := s.rows[id]
		  s.mu.Unlock()
		  if cur.Count != len(items) || cur.Actor != act {
			   return Batch{}, ErrIdem
		  }
		  return cur.Copy(), nil
	   }
	   now := s.now()
	   row := &Batch{
		   ID: uuid.NewString(), IdempotencyKey: idem, Actor: act, Status: StatusOpen,
		  Count: len(items), Items: items, CreatedAt: now, UpdatedAt: now,
	  }
	  s.rows[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	  return row.Copy(), nil
  }

  func shapeObs(it Item, seq int) (Item, error) {
	  uid, err := validate.UserID(it.UserID)
	   if err != nil {
		  return Item{}, ErrUser
	  }
	  as, err := validate.Asset(it.Asset)
	   if err != nil {
		   return Item{}, ErrAsset
	  }
	   ch, err := validate.Chain(it.Chain)
	   if err != nil {
		   return Item{}, ErrChain
	  }
	  hash, err := validate.TxHash(it.TxHash)
	   if err != nil {
		  return Item{}, ErrHash
	   }
	  idem, err := validate.Idem(it.IdempotencyKey)
	  if err != nil {
		  return Item{}, ErrIdem
	  }
	  return Item{
		  Seq: seq, UserID: uid, Chain: ch, TxHash: hash, Asset: as,
		   IdempotencyKey: idem, Status: ItemPending,
	   }, nil
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
	  cur, ok := s.rows[strings.TrimSpace(id)]
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

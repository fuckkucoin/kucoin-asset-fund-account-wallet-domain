package stablebr

import (
	"context"
	"strings"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Mint 证明齐全后从 system.bridge 入用户资金账。两边金额必须 Conserve。

   func (s *Service) Mint(ctx context.Context, req MintReq) (Journal, error) {
	  return s.post(ctx, KindMint, req.UserID, req.Asset, req.Amount, req.Actor, req.IdempotencyKey, req.MintLeaseID)
  }

   func (s *Service) Redeem(ctx context.Context, req RedeemReq) (Journal, error) {
	  return s.post(ctx, KindRedeem, req.UserID, req.Asset, req.Amount, req.Actor, req.IdempotencyKey, req.MintLeaseID)
  }

  func (s *Service) post(ctx context.Context, kind Kind, userID, asset, amount, actor, idemS, leaseID string) (Journal, error) {
	   if s == nil {
		  return Journal{}, ErrUnavailable
	  }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return Journal{}, ErrUser
	   }
	   if uid == "system" {
		  return Journal{}, ErrUser
	   }
	  as, err := allowedAsset(asset)
	   if err != nil {
		   return Journal{}, err
	  }
	   amt, err := money.Parse(amount)
	  if err != nil {
		  return Journal{}, ErrAmount
	   }
	   act, err := validate.Actor(actor)
	  if err != nil {
		   return Journal{}, ErrActor
	   }
	   idem, err := validate.Idem(idemS)
	  if err != nil {
		  return Journal{}, ErrIdem
	   }
	   s.mu.Lock()
	  if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		  s.mu.Unlock()
		   if cur.Status == StatusHalf {
			   return Journal{}, ErrHalfOpen
		  }
		  if cur.UserID != uid || cur.Amount != amt.String() || cur.Kind != kind {
			   return Journal{}, ErrIdem
		  }
		  return cur.Copy(), nil
	  }
	   s.mu.Unlock()
	  if s.guard != nil {
		   if err := s.guard.BeforePost(ctx, kind, as, uid, amt.String(), act, idem, leaseID); err != nil {
			  return Journal{}, err
		   }
	   } else if s.journalFence && strings.TrimSpace(leaseID) == "" {
		   return Journal{}, ErrLease
	  }
	  s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		   s.mu.Unlock()
		   if cur.Status == StatusHalf {
			  return Journal{}, ErrHalfOpen
		  }
		  return cur.Copy(), nil
	  }
	   if err := s.gateLocked(kind, as); err != nil {
		  s.mu.Unlock()
		   return Journal{}, err
	   }
	  bridge, userBook := split(kind, amt)
	  if err := money.Conserve(bridge, userBook); err != nil {
		  s.mu.Unlock()
		   return Journal{}, ErrConserve
	  }
	   row := &Journal{
		  ID: uuid.NewString(), IdempotencyKey: idem, Kind: kind, UserID: uid,
		   Asset:  as,
		  Amount: amt.String(), Bridge: bridge.String(), UserBook: userBook.String(),
		  Status: StatusPosted, MintLeaseID: strings.TrimSpace(leaseID), Actor: act, At: s.now(),
	  }
	  s.rows[row.ID] = row
	   s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	  if s.led != nil || s.ledErr != nil {
		   var (
			  tr *ledger.Transfer
			  lerr error
		   )
		   if s.ledErr != nil {
			  lerr = s.ledErr
		  } else {
			  dir := ledger.FiatOnramp
			   if kind == KindRedeem {
				   dir = ledger.FiatOfframp
			   }
			   tr, lerr = s.led.Create(ctx, ledger.CreateReq{
				  UserID: uid, Direction: dir, Asset: as, Amount: amt.Dec(), IdempotencyKey: idem,
			  })
		   }
		  if lerr != nil {
			   if s.journalFence {
				   s.mu.Lock()
				  row.Status = StatusHalf
				  s.mu.Unlock()
				  if s.guard != nil {
					  _ = s.guard.OnLedgerFail(ctx, row.Copy(), lerr)
				  }
				   return row.Copy(), lerr
			  }
			   s.mu.Lock()
			   delete(s.rows, row.ID)
			  delete(s.byIdem, idem)
			   s.mu.Unlock()
			   return Journal{}, lerr
		  }
		   s.mu.Lock()
		   row.LedgerID = tr.ID
		  s.mu.Unlock()
	  }
	   if s.guard != nil {
		  _ = s.guard.OnPosted(ctx, row.Copy())
	  }
	  return row.Copy(), nil
   }

   func split(kind Kind, amt money.Amount) (bridge money.Amount, user money.Amount) {
	   // 铸：桥出 = 用户入；赎：用户出 = 桥入。两边都是同一金额。
	   _ = kind
	  return amt, amt
  }

  func (s *Service) Get(id string) (Journal, error) {
	  if s == nil {
		   return Journal{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.rows[id]
	   if !ok {
		   return Journal{}, ErrNotFound
	  }
	  return cur.Copy(), nil
   }

  func (s *Service) List(userID string, limit int) []Journal {
	  if s == nil {
		  return nil
	   }
	   if limit <= 0 {
		   limit = 50
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Journal, 0, limit)
	  for _, r := range s.rows {
		   if userID != "" && r.UserID != userID {
			  continue
		  }
		   out = append(out, r.Copy())
		   if len(out) >= limit {
			   break
		   }
	  }
	   return out
  }

  func (s *Service) Sum(asset string) (string, error) {
	   if s == nil {
		   return "", ErrUnavailable
	  }
	   as := ""
	  if asset != "" {
		  var err error
		  as, err = allowedAsset(asset)
		   if err != nil {
			   return "", err
		   }
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   sum := money.Zero()
	   for _, r := range s.rows {
		  if r.Status != StatusPosted {
			   continue
		  }
		   if as != "" && r.Asset != as {
			  continue
		   }
		  amt, err := money.ParseBook(r.Amount)
		   if err != nil {
			  return "", err
		   }
		  if r.Kind == KindRedeem {
			  next, err := sum.Sub(amt)
			   if err != nil {
				   // 净赎回大于净铸，仍记下绝对值方向，调用方看符号没有——用 ParseBook 0 夹住。
				  sum = money.Zero()
				  continue
			   }
			   sum = next
			   continue
		   }
		  next, err := sum.Add(amt)
		   if err != nil {
			   return "", err
		   }
		   sum = next
	   }
	  return sum.String(), nil
   }

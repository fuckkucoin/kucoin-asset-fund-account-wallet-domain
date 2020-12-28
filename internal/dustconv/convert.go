package dustconv

import (
	"strings"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Convert 只转低于 floor 的余额。USDT 入账 = 数量 * 牌价，必须 Conserve。

  func (s *Service) Convert(req ConvertReq) (Convert, error) {
	  if s == nil {
		  return Convert{}, ErrUnavailable
	  }
	   uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return Convert{}, ErrUser
	   }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		   return Convert{}, ErrActor
	   }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Convert{}, ErrAsset
	   }
	  if as == "USDT" {
		  return Convert{}, ErrAsset
	   }
	  amt, err := money.Parse(req.Amount)
	  if err != nil {
		   return Convert{}, ErrAmount
	   }
	   idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return Convert{}, ErrIdem
	   }
	  s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		   s.mu.Unlock()
		   if cur.UserID != uid || cur.Amount != amt.String() || cur.Asset != as {
			  return Convert{}, ErrIdem
		   }
		  return cur.Copy(), nil
	   }
	   pol := s.policy[as]
	   if pol == nil || !pol.Enabled {
		  s.mu.Unlock()
		  return Convert{}, ErrPolicy
	  }
	   floor, err := money.Parse(pol.Floor)
	   if err != nil {
		   s.mu.Unlock()
		   return Convert{}, ErrFloor
	   }
	   minC, err := money.Parse(pol.MinConvert)
	   if err != nil {
		   s.mu.Unlock()
		   return Convert{}, ErrFloor
	  }
	  if amt.Cmp(floor) >= 0 {
		   s.mu.Unlock()
		   return Convert{}, ErrNotDust
	   }
	   if amt.Cmp(minC) < 0 {
		  s.mu.Unlock()
		  return Convert{}, ErrPolicy
	   }
	   rt := s.rates[as]
	   if rt == nil {
		   s.mu.Unlock()
		   return Convert{}, ErrRate
	  }
	   px, err := money.Parse(rt.USDT)
	   if err != nil {
		   s.mu.Unlock()
		  return Convert{}, ErrRate
	   }
	   usdt := money.FromDecimal(amt.Dec().Mul(px.Dec()).Round(18))
	   if !usdt.IsPositive() {
		  s.mu.Unlock()
		  return Convert{}, ErrAmount
	  }
	   want := money.FromDecimal(amt.Dec().Mul(px.Dec()).Round(18))
	   if err := money.Conserve(usdt, want); err != nil {
		   s.mu.Unlock()
		  return Convert{}, ErrConserve
	  }
	   gate := s.leaseGate
	   leaseID := strings.TrimSpace(req.DustQuoteLeaseID)
	  s.mu.Unlock()
	  if gate != nil {
		  if leaseID == "" {
			  return Convert{}, ErrIdem
		  }
		   if err := gate.AssertBeforeConvert(nil, leaseID, uid, as, amt.String(), actor); err != nil {
			  return Convert{}, err
		  }
	  }
	  s.mu.Lock()
	  if err := s.subLocked(uid, as, amt); err != nil {
		  s.mu.Unlock()
		  return Convert{}, err
	  }
	  if err := s.addLocked(uid, "USDT", usdt); err != nil {
		  _ = s.addLocked(uid, as, amt)
		  s.mu.Unlock()
		  return Convert{}, err
	   }
	  row := &Convert{
		   ID: uuid.NewString(),
		   IdempotencyKey: idem, UserID: uid, Asset: as,
		   Amount: amt.String(), USDT: usdt.String(), Rate: px.String(), Actor: actor, At: s.now(),
	  }
	  s.rows[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	  if gate != nil {
		  if err := gate.SettleAfterConvert(nil, leaseID, row.ID, usdt.String(), actor); err != nil {
			   s.mu.Lock()
			  _ = s.addLocked(uid, as, amt)
			   _ = s.subLocked(uid, "USDT", usdt)
			   delete(s.rows, row.ID)
			   delete(s.byIdem, idem)
			   s.mu.Unlock()
			   _ = gate.FailClosed(nil, leaseID, actor, "settle refused, ledger rolled back")
			  return Convert{}, ErrConserve
		  }
	   }
	   return row.Copy(), nil
  }

   func (s *Service) Scan(userID string) []Book {
	   if s == nil {
		   return nil
	  }
	   userID = strings.TrimSpace(userID)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Book, 0)
	   for _, b := range s.books {
		   if userID != "" && b.UserID != userID {
			  continue
		  }
		   if b.Asset == "USDT" {
			  continue
		   }
		  pol := s.policy[b.Asset]
		  if pol == nil || !pol.Enabled {
			  continue
		  }
		  got,
			  err := money.ParseBook(orZ(b.Available))
		  if err != nil || !got.IsPositive() {
			  continue
		   }
		  floor, err := money.Parse(pol.Floor)
		   if err != nil {
			   continue
		  }
		   if got.Cmp(floor) < 0 {
			   out = append(out, b.Copy())
		   }
	  }
	  return out
  }

   func (s *Service) List(userID string, limit int) []Convert {
	  if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		  limit = 50
	   }
	   userID = strings.TrimSpace(userID)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Convert, 0, limit)
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

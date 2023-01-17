package wdrollback

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

type hold struct {
	UserID string
	Asset string
	Transfer string
	Protected money.Amount
}

  func posKey(user, asset string) string {
	  return strings.TrimSpace(user) + "|" + strings.TrimSpace(asset)
  }

// SeedIfAbsent 首次建账。已有账不覆盖，避免幂等重入把已回滚余额刷回旧快照。
   func (s *Service) SeedIfAbsent(user, asset, available, frozen, fee, occupancy string) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  user = strings.TrimSpace(user)
	  asset = strings.TrimSpace(asset)
	  if user == "" || asset == "" {
		  return ErrAmount
	   }
	   av, err := parseBook(available)
	  if err != nil {
		   return err
	   }
	  fr, err := parseBook(frozen)
	   if err != nil {
		  return err
	   }
	   fe, err := parseBook(fee)
	   if err != nil {
		  return err
	  }
	  ocRaw := strings.TrimSpace(occupancy)
	  if ocRaw == "" {
		  ocRaw = fr.String()
	   }
	   oc, err := parseBook(ocRaw)
	  if err != nil {
		  return err
	   }
	  eq, err := av.Add(fr)
	   if err != nil {
		   return err
	   }
	  eq, err = eq.Add(fe)
	  if err != nil {
		   return err
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   k := posKey(user, asset)
	   if _, ok := s.pos[k]; ok {
		  return nil
	   }
	  s.pos[k] = &Position{
		  UserID: user, Asset: asset,
		   Available: av, Frozen: fr, FeeReserved: fe, Occupancy: oc,
		   Available0: av, Frozen0: fr, Fee0: fe, Equity0: eq,
	   }
	   return nil
   }

// Protect 登记一笔出金里不能解冻的已成交或在途金额。
// 功能：部分成交时把已成交份额钉在冻结里。
// 同一出金单重复登记必须是同一金额，否则视为冲突。
// 不变量：该用户该资产的保护量合计不得超过当前冻结。

  func (s *Service) Protect(transfer, user, asset string, amt money.Amount) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   transfer = strings.TrimSpace(transfer)
	  user = strings.TrimSpace(user)
	   asset = strings.TrimSpace(asset)
	  if transfer == "" || user == "" || asset == "" || amt.IsNegative() {
		   return ErrAmount
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.protect(transfer,
		   user, asset, amt)
   }

   func (s *Service) protect(transfer, user, asset string, amt money.Amount) error {
	  if old, ok := s.holds[transfer]; ok {
		   if old.UserID == user && old.Asset == asset && old.Protected.Equal(amt) {
			   return nil
		  }
		   return ErrIdemConflict
	  }
	   p := s.pos[posKey(user, asset)]
	   if p == nil {
		   return ErrPosition
	   }
	   s.holds[transfer] = &hold{UserID: user,
		   Asset: asset, Transfer: transfer, Protected: amt}
	  if s.protected(user, asset).Cmp(p.Frozen) > 0 {
		   delete(s.holds, transfer)
		  return ErrAmount
	   }
	  return nil
  }

   func (s *Service) protected(user, asset string) money.Amount {
	   sum := money.Zero()
	  for _, h := range s.holds {
		   if h.UserID != user || h.Asset != asset {
			  continue
		  }
		   next, err := sum.Add(h.Protected)
		   if err != nil {
			   return sum
		  }
		   sum = next
	   }
	  return sum
  }

   func (s *Service) releasable(user, asset string) (money.Amount, error) {
	   p := s.pos[posKey(user, asset)]
	   if p == nil {
		  return money.Zero(), ErrPosition
	   }
	   return p.Frozen.Sub(s.protected(user, asset))
  }

   func (s *Service) unfreeze(p *Position, amt money.Amount) error {
	   if !amt.IsPositive() {
		   return ErrAmount
	   }
	   rel, err := s.releasable(p.UserID, p.Asset)
	   if err != nil {
		  return err
	  }
	   if amt.Cmp(rel) > 0 {
		  return ErrFilledProtected
	   }
	  nextFrozen, err := p.Frozen.Sub(amt)
	  if err != nil {
		  return ErrNegative
	   }
	   nextAvail, err := p.Available.Add(amt)
	  if err != nil {
		   return err
	  }
	  p.Frozen = nextFrozen
	  p.Available = nextAvail
	   if p.Occupancy.Cmp(p.Frozen) > 0 {
		   p.Occupancy = p.Frozen
	   }
	   return nil
   }

   func (s *Service) releaseFee(p *Position, amt money.Amount) error {
	   if !amt.IsPositive() {
		  return ErrAmount
	   }
	  nextFee, err := p.FeeReserved.Sub(amt)
	  if err != nil {
		   return ErrNegative
	  }
	  nextAvail,
		  err := p.Available.Add(amt)
	  if err != nil {
		  return err
	  }
	   p.FeeReserved = nextFee
	   p.Available = nextAvail
	  return nil
   }

  func (s *Service) releaseOccupancy(p *Position, amt money.Amount) error {
	  if !amt.IsPositive() {
		   return ErrAmount
	   }
	   next, err := p.Occupancy.Sub(amt)
	  if err != nil {
		  return ErrNegative
	  }
	  floor := s.protected(p.UserID,
		  p.Asset)
	  if next.Cmp(floor) < 0 {
		  return ErrFilledProtected
	   }
	   p.Occupancy = next
	   return nil
  }

  func parseBook(s string) (money.Amount, error) {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return money.Zero(), nil
	   }
	  if strings.ContainsAny(s, "eE") {
		  return money.Zero(), ErrAmount
	  }
	  a, err := money.ParseBook(s)
	   if err != nil {
		  return money.Zero(), ErrAmount
	   }
	  return a, nil
  }

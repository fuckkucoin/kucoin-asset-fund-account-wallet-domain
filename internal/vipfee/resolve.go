package vipfee

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Resolve 按钉档或 30 日量选档，再取该档 maker/taker/返佣/出金 BPS。
// 资产表缺行回落到 *。kyc0 钉档不会被量抬上去。

   func (s *Service) Resolve(req ResolveReq) (Effective, error) {
	   if s == nil {
		   return Effective{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return Effective{}, ErrUser
	   }
	   as,
		  err := validate.Asset(req.Asset)
	  if err != nil {
		   return Effective{}, ErrAsset
	   }
	  if strings.TrimSpace(req.Notional) != "" {
		   if _, err := money.Parse(req.Notional); err != nil {
			   return Effective{}, ErrAmount
		   }
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   vol, err := s.windowLocked(uid, "")
	   if err != nil {
		  return Effective{}, err
	   }
	  tier, src := s.pickLocked(uid, vol)
	   out := Effective{UserID: uid, Tier: tier, Asset: as, Source: src, Volume: vol.String()}
	   maker, err := s.getLocked(tier, KindMaker, as)
	   if err != nil {
		   return Effective{}, err
	  }
	   taker, err := s.getLocked(tier, KindTaker, as)
	  if err != nil {
		  return Effective{}, err
	   }
	  reb, err := s.getLocked(tier, KindRebate, as)
	  if err != nil {
		   return Effective{}, err
	  }
	  wd, err := s.getLocked(tier, KindWithdraw, as)
	  if err != nil {
		   return Effective{}, err
	   }
	   out.MakerBPS = maker.BPS
	  out.TakerBPS = taker.BPS
	   out.MakerRebateBPS = reb.BPS
	  out.TakerRebateBPS = 0
	   out.WithdrawBPS = wd.BPS
	   return out, nil
  }

  func (s *Service) pickLocked(userID string, vol money.Amount) (Tier, string) {
	   if pin, ok := s.pins[userID]; ok {
		  return pin.Tier, "pin"
	  }
	  plus,
		  _ := parseBook(s.promo[TierVIPPlus])
	  vip, _ := parseBook(s.promo[TierVIP])
	   if plus.IsPositive() && vol.Cmp(plus) >= 0 {
		   return TierVIPPlus, "volume"
	  }
	  if vip.IsPositive() && vol.Cmp(vip) >= 0 {
		  return TierVIP, "volume"
	  }
	   return TierStandard, "default"
  }

  func (s *Service) Pin(req PinReq) (Pin, error) {
	   if s == nil {
		   return Pin{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return Pin{}, ErrUser
	   }
	  actor, err := validate.Actor(req.Actor)
	  if err != nil {
		  return Pin{}, ErrActor
	  }
	   tier, err := ParseTier(req.Tier)
	   if err != nil {
		   return Pin{}, err
	  }
	   row := Pin{UserID: uid, Tier: tier, Actor: actor, Note: strings.TrimSpace(req.Note), At: s.now()}
	   s.mu.Lock()
	  prev, had := s.pins[uid]
	   s.pins[uid] = row
	   s.mu.Unlock()
	   if err := s.persistPin(row); err != nil {
		  s.mu.Lock()
		  if had {
			  s.pins[uid] = prev
		  } else {
			   delete(s.pins, uid)
		  }
		   s.mu.Unlock()
		  return Pin{}, err
	   }
	  return row.Copy(), nil
  }

  func (s *Service) ClearPin(userID, actor string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return ErrUser
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	  }
	   s.mu.Lock()
	  prev, ok := s.pins[uid]
	   if !ok {
		   s.mu.Unlock()
		  return ErrPin
	  }
	   delete(s.pins, uid)
	   s.mu.Unlock()
	  if err := s.persistClearPin(uid); err != nil {
		  s.mu.Lock()
		  s.pins[uid] = prev
		  s.mu.Unlock()
		   return err
	   }
	  return nil
  }

   func (s *Service) PinOf(userID string) (Pin, error) {
	  if s == nil {
		  return Pin{}, ErrUnavailable
	  }
	   uid,
		  err := validate.UserID(userID)
	   if err != nil {
		   return Pin{}, ErrUser
	   }
	   if s.sqlOn() {
		   got, err := sqlrepo.NewVIPFee(s.sql).GetPin(context.Background(), uid)
		  if err == nil {
			   return Pin{UserID: got.UserID, Tier: Tier(got.Tier), Actor: got.Actor, Note: got.Note, At: got.At}, nil
		  }
		  if !errors.Is(err, sqlrepo.ErrNotFound) {
			  return Pin{}, ErrUnavailable
		   }
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  row, ok := s.pins[uid]
	   if !ok {
		   return Pin{}, ErrPin
	  }
	  return row.Copy(), nil
   }

package earn

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// RedeemEarlySettled 提前赎回；本金全额出理财账后从资金账扣罚金。
// fee+net=本金；EarnRedeem(全额) 后 FeeDebit→system.fee；释 holdcat。
// 未开 EarlyBPS 拒；锁定期已满走普通 Redeem；罚金扣款失败则仓位已赎回须人工补扣/RetryEarlyFee。

  func (s *Service) RedeemEarlySettled(ctx context.Context, req EarlyReq) (Position, Penalty, error) {
	  if s == nil {
		  return Position{}, Penalty{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return Position{},
			  Penalty{}, err
	  }
	   key, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Position{}, Penalty{}, err
	  }
	  p, ok := s.book.Get(req.PositionID)
	  if !ok || p.UserID != uid {
		  return Position{}, Penalty{}, ErrNotFound
	   }
	  // 已赎回且留有罚金：运营补扣路径（复用 redeem-early 面，不新开公开口）。
	   if p.Status == StatusRedeemed && strings.TrimSpace(p.Penalty) != "" {
		  _, err := s.RetryEarlyFee(ctx, uid, p.ID, key)
		   pen := Penalty{PositionID: p.ID, Fee: p.Penalty, Gross: p.Principal}
		   if err != nil {
			   return p, pen, err
		   }
		   return p, pen, nil
	   }
	   prod, err := s.cat.Get(p.ProductID)
	  if err != nil {
		  return Position{}, Penalty{}, err
	  }
	  now := s.clock()
	   if !p.Locked(now) {
		   got, err := s.Redeem(ctx, RedeemReq{UserID: uid, PositionID: p.ID, IdempotencyKey: key})
		  return got, Penalty{}, err
	  }
	  if prod.EarlyBPS <= 0 {
		  return Position{}, Penalty{}, ErrNoEarly
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   if un, err := p.Unclaimed(); err == nil && un.IsPositive() {
		   if _, err := s.PostInterest(ctx, uid, p.ID, key+":int"); err != nil {
			  return Position{}, Penalty{}, err
		  }
		   if fresh, ok := s.book.Get(p.ID); ok {
			   p = fresh
		   }
	  }
	   pen, err := ComputePenalty(p, prod)
	  if err != nil {
		  return Position{}, Penalty{}, err
	  }
	  if _, err := money.Parse(pen.Net); err != nil {
		  return Position{}, Penalty{}, fmt.Errorf("early redeem net invalid")
	  }
	   feeAmt, err := money.Parse(pen.Fee)
	   if err != nil {
		   return Position{}, Penalty{}, err
	  }
	   // 跳过锁定期闸，全额出理财账。
	   p.UnlockAt = now
	  p.Status = StatusOpen
	  p.Penalty = pen.Fee
	  if err := s.book.Put(p); err != nil {
		  return Position{}, pen, err
	  }
	   if s.led != nil && feeAmt.IsPositive() {
		  if _, err := s.debitEarlyFee(ctx, uid, p.Asset, pen.Fee, key); err != nil {
			  return Position{}, pen, fmt.Errorf("early fee debit: %w", err)
		   }
	   }
	   got,
		  err := s.Redeem(ctx, RedeemReq{UserID: uid, PositionID: p.ID, IdempotencyKey: key})
	  if err != nil {
		   return got, pen, err
	  }
	  got.Penalty = pen.Fee
	  if err := s.book.Put(got); err != nil {
		  return got, pen, err
	  }
	   if s.holds != nil {
		   if _, err := s.holds.ReleaseByRef(ctx, uid, holdcat.ReasonEarnHold, got.ID); err != nil {
			   return got, pen, err
		   }
	  }
	   if got.AutoSubscribe && strings.EqualFold(prod.Category, CatFlexible) {
		   if _, err := s.Subscribe(ctx, SubscribeReq{
			  UserID: uid, ProductID: got.ProductID, Asset: got.Asset, Amount: pen.Net,
			  IdempotencyKey: key + ":autosub", AutoSubscribe: true,
		  }); err != nil {
			   return got, pen, err
		   }
	  }
	  return got, pen, nil
   }

// debitEarlyFee 资金账 FeeDebit→system.fee（幂等键 :early-fee）。
   func (s *Service) debitEarlyFee(ctx context.Context, uid, asset, fee, key string) (*ledger.Transfer, error) {
	  feeDec, err := ledger.ParseAmount(fee)
	  if err != nil {
		  return nil, err
	  }
	   if !feeDec.IsPositive() {
		   return nil, nil
	  }
	   return s.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.FeeDebit, Asset: asset,
		  Amount: feeDec, IdempotencyKey: key + ":early-fee",
	  })
   }

// RetryEarlyFee 仓位已赎回但罚金扣款失败后的运营补扣（journal 路径）。
// 读 position.Penalty；复用原幂等键 :early-fee；走 ledger.FeeDebit。
// 未赎回/无罚金拒；不得经 bridge；Create 幂等不得二次扣。
  func (s *Service) RetryEarlyFee(ctx context.Context, userID, positionID, idempotencyKey string) (*ledger.Transfer, error) {
	  if s == nil || s.led == nil {
		  return nil, ErrUnavailable
	   }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		   return nil, err
	  }
	   key, err := validate.Idem(idempotencyKey)
	  if err != nil {
		  return nil, err
	  }
	  p, ok := s.book.Get(positionID)
	   if !ok || p.UserID != uid {
		   return nil, ErrNotFound
	   }
	   if p.Status != StatusRedeemed {
		   return nil, fmt.Errorf("early fee retry requires redeemed position")
	   }
	  fee := strings.TrimSpace(p.Penalty)
	   if fee == "" {
		  return nil, fmt.Errorf("early fee retry requires position penalty")
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   tr, err := s.debitEarlyFee(ctx, uid, p.Asset, fee, key)
	   if err != nil {
		  return tr, fmt.Errorf("early fee retry: %w", err)
	  }
	   return tr, nil
   }

// ReverseEarlyFee 运营冲正提前赎回罚金 FeeDebit（仓位已赎回场景）。
// 仅 ledger.ReverseFee；不回滚 EarnRedeem 本金。
// fee 单不存在/非 credited 上浮；不得经 bridge 补铸。
   func (s *Service) ReverseEarlyFee(ctx context.Context, feeTransferID string) error {
	   if s == nil || s.led == nil {
		   return ErrUnavailable
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  return s.led.ReverseFee(ctx, strings.TrimSpace(feeTransferID))
   }

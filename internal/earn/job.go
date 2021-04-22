package earn

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/validate"
)

type JobReport struct {
	Users    int `json:"users"`
	Accruals int `json:"accruals"`
	Errs     int `json:"errors"`
}

// Tick 给所有持仓用户计提一天。单用户失败不拖死整轮。
  func (s *Service) Tick(ctx context.Context) JobReport {
	   _ = ctx
	   out := JobReport{}
	   if s == nil {
		   return out
	  }
	   for _, uid := range s.book.Users() {
		   out.Users++
		   rows, err := s.AccrueUser(uid)
		  if err != nil {
			  out.Errs++
			  continue
		  }
		   out.Accruals += len(rows)
	   }
	  return out
  }

type EarlyReq struct {
	UserID string
	PositionID     string
	IdempotencyKey string
}

// RedeemEarly 锁定期未满时按 EarlyBPS 切罚金后赎回本金。产品未开 early 仍拒。
// 绑定账本时走 RedeemEarlySettled（全额出理财账后扣罚金）。
   func (s *Service) RedeemEarly(ctx context.Context, req EarlyReq) (Position, Penalty, error) {
	   if s != nil && s.led != nil {
		  return s.RedeemEarlySettled(ctx, req)
	  }
	  if s == nil {
		   return Position{}, Penalty{}, ErrUnavailable
	   }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return Position{}, Penalty{}, err
	  }
	  p, ok := s.book.Get(req.PositionID)
	  if !ok || p.UserID != uid {
		   return Position{}, Penalty{}, ErrNotFound
	  }
	   prod,
		   err := s.cat.Get(p.ProductID)
	   if err != nil {
		  return Position{}, Penalty{}, err
	  }
	   now := s.clock()
	   if !p.Locked(now) {
		  got,
			  err := s.Redeem(ctx, RedeemReq{UserID: uid, PositionID: p.ID, IdempotencyKey: req.IdempotencyKey})
		   return got, Penalty{}, err
	   }
	  if prod.EarlyBPS <= 0 {
		  return Position{},
			  Penalty{}, ErrNoEarly
	  }
	  pen, err := ComputePenalty(p, prod)
	   if err != nil {
		  return Position{}, Penalty{}, err
	  }
	  p.UnlockAt = now
	   p.Status = StatusOpen
	  p.Penalty = pen.Fee
	   if err := s.book.Put(p); err != nil {
		   return Position{}, Penalty{}, err
	   }
	   got,
		  err := s.Redeem(ctx, RedeemReq{UserID: uid, PositionID: p.ID, IdempotencyKey: req.IdempotencyKey})
	  if err != nil {
		   return got, pen, err
	  }
	  got.Penalty = pen.Fee
	  if err := s.book.Put(got); err != nil {
		  return got, pen, err
	  }
	   return got, pen, nil
   }

   func (s *Service) JobString() string {
	  if s == nil {
		   return "earn-job<nil>"
	  }
	   return fmt.Sprintf("earn-job users=%d", len(s.book.Users()))
   }

package earn

import (
	"context"
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Post struct {
	PositionID string `json:"position_id"`
	Amount     string `json:"amount"`
	TransferID string `json:"transfer_id,omitempty"`
}

// PostInterest 把已计提未领利息从 system.bridge 入资金账（FiatOnramp）。
// 金额必须来自仓位 Unclaimed，不得用调用方另造的数。入账后记 claimed。
   func (s *Service) PostInterest(ctx context.Context, userID, positionID, idem string) (Post, error) {
	  if s == nil || s.led == nil {
		  return Post{}, ErrUnavailable
	   }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return Post{}, err
	   }
	  key, err := validate.Idem(idem)
	   if err != nil {
		  return Post{}, err
		  return Post{PositionID: p.ID, Amount: orZero(p.Claimed), TransferID: rec.ResultID}, nil
	   }
	  un, err := p.Unclaimed()
	  if err != nil {
		   return Post{}, err
	  }
	  if !un.IsPositive() {
		  return Post{PositionID: p.ID, Amount: "0"}, nil
	   }
	  if _, err := money.Parse(un.String()); err != nil {
		   // 极小正数过不了 Parse 时仍允许 ParseBook 过的应付，但不得入账幽灵尘。
		   if un.Dust() {
			  return Post{PositionID: p.ID, Amount: "0"}, nil
		   }
		  return Post{}, err
	  }
	  payload := command.HashPayload("earn_int", uid, p.ID, un.String())
	  rec, existed, err := s.cmds.Begin(key, "earn_interest", payload)
	  if err != nil && !errors.Is(err, command.ErrInFlight) {
		  return Post{}, err
	  }
	  if existed && rec != nil && rec.Status == command.StatusSucceeded && rec.ResultID != "" {
		  return Post{PositionID: p.ID, Amount: un.String(), TransferID: rec.ResultID}, nil
	   }
	   dec, err := ledger.ParseAmount(un.String())
	   if err != nil {
		   s.cmds.Fail(key, err)
		   return Post{}, err
	   }
	  ledKey := "earn-int:" + p.ID + ":" + un.String()
	  tr, err := s.led.Create(ctx, ledger.CreateReq{
		   UserID:    uid,
		  Direction: ledger.EarnInterest, Asset: p.Asset,
		   Amount: dec, IdempotencyKey: ledKey,
	  })
	   if err != nil {
		   s.cmds.Fail(key, err)
		   return Post{}, err
	  }
	   next, cl, err := ClaimAccrued(p, s.clock())
	   if err != nil {
		  s.cmds.Fail(key, err)
		  return Post{}, err
	   }
	  if cl.Amount != un.String() {
		  s.cmds.Fail(key, fmt.Errorf("claimed %s want %s", cl.Amount, un.String()))
		  return Post{}, fmt.Errorf("claimed drifted")
	  }
	  if err := s.book.Put(next); err != nil {
		  s.cmds.Fail(key, err)
		  return Post{}, err
	  }
	  s.cmds.Succeed(key, tr.ID)
	  return Post{PositionID: p.ID,
		   Amount: cl.Amount, TransferID: tr.ID}, nil
  }

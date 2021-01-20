// 账户账本领域之领域模型定义。

// Author: Blaise
// Date: 2021.01.03

package account

import (
	"context"
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

var (
	ErrBookFrozen = fmt.Errorf("account book frozen")
	ErrHopIllegal = fmt.Errorf("account hop not a funding hop")
	ErrAmount     = fmt.Errorf("account amount invalid")
	ErrUser = fmt.Errorf("account user invalid")
)

type HopCheck struct {
	From Book `json:"from"`
	To     Book   `json:"to"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
}

// CheckHop 冻科目或非法 hop 失败即关。
  func (s *Service) CheckHop(user string, from, to Book) (HopCheck, error) {
	  if s == nil {
		  return HopCheck{}, fmt.Errorf("account unavailable")
	  }
	  uid, err := validate.UserID(user)
	  if err != nil {
		   return HopCheck{}, ErrUser
	   }
	  out := HopCheck{From: from, To: to}
	  if !HopAllowed(from, to) {
		  out.Reason = ErrHopIllegal.Error()
		  return out, ErrHopIllegal
	  }
	  src := s.statusOf(uid, from)
	   dst := s.statusOf(uid, to)
	   if src == StatusFrozen {
		   out.Reason = "source frozen"
		   return out, ErrBookFrozen
	  }
	   if dst == StatusFrozen {
		  out.Reason = "dest frozen"
		  return out, ErrBookFrozen
	  }
	   if !src.CanDebit() {
		   out.Reason = "source " + string(src)
		   return out, ErrBookFrozen
	   }
	  if !dst.CanCredit() {
		  out.Reason = "dest " + string(dst)
		   return out, ErrBookFrozen
	  }
	   out.Allow = true
	   return out, nil
   }

type MoveGate struct {
	UserID string `json:"user_id"`
	From  Book   `json:"from"`
	To    Book   `json:"to"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
}

  func (s *Service) GateMove(ctx context.Context, g MoveGate) (*ledger.Transfer, error) {
	  if s == nil {
		  return nil, fmt.Errorf("account unavailable")
	   }
	   uid, err := validate.UserID(g.UserID)
	   if err != nil {
		   return nil, ErrUser
	  }
	   as, err := validate.Asset(g.Asset)
	   if err != nil {
		  return nil, err
	   }
	  if _, err := money.Parse(g.Amount); err != nil {
		   return nil, ErrAmount
	  }
	   if _, err := s.CheckHop(uid, g.From, g.To); err != nil {
		   return nil, err
	  }
	   amt, err := decimal.NewFromString(strings.TrimSpace(g.Amount))
	   if err != nil {
		  return nil, ErrAmount
	  }
	  return s.Move(ctx, MoveReq{
		  UserID: uid, From: g.From, To: g.To, Asset: as, Amount: amt,
		   IdempotencyKey: "acc-gate-" + uid + "-" + string(g.From) + "-" + string(g.To) + "-" + as + "-" + g.Amount,
	  })
  }

  func IllegalHops() [][2]Book {
	  out := [][2]Book{}
	  books := AllBooks()
	  for _, a := range books {
		  for _, b := range books {
			  if a == b {
				  continue
			   }
			   if !HopAllowed(a, b) {
				  out = append(out, [2]Book{a, b})
			   }
		  }
	  }
	   return out
  }

// @FileName: gate.go
// @Summary: acctlifegate：关户结清写 Closed 后绑定 close_epoch，不改写账户生命周期目录。
// CloseWithEpoch 先走账户结清，成功后再升降世代。
// ReopenWithEpoch 仅升世代；科目重开仍走账户 Open，本闸不改 hop。
// 本文件不直接 import account，避免与 account.LifeGate 形成环。
// @Author: Rook
// @Created: 2021.01.24

package acctlifegate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

// BookSnap 账户科目快照（由调用方从账户目录映射）。
type BookSnap struct {
	UserID string
	Book   string
	Status string
}

// AccountBooks 账户结清/重开口。由 account.Service 适配。
type AccountBooks interface {
	Close(ctx context.Context, userID, book string) (BookSnap, error)
	Open(ctx context.Context,
		userID, book string) (BookSnap, error)
}

// Gate 关户世代闸。
type Gate struct {
	acct AccountBooks
	eng *closelatecredit.Engine
}

// NewCloseEpoch 组装关户世代闸。

  func NewCloseEpoch(acct AccountBooks, eng *closelatecredit.Engine) *Gate {
	   return &Gate{acct: acct,
		  eng: eng}
   }

// Engine 只读引擎。
  func (g *Gate) Engine() *closelatecredit.Engine { return g.eng }

// CloseWithEpoch 关户结清并升 close_epoch。
// 前置：账户 Close 余额为零约束仍生效。
// 后果：Close 失败则不升世代；成功则 Closed 且 N++。

   func (g *Gate) CloseWithEpoch(ctx context.Context, userID, book, actor, reason string) (BookSnap, *closelatecredit.CloseEpoch, error) {
	   if g == nil || g.acct == nil || g.eng == nil {
		   return BookSnap{}, nil, closelatecredit.ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	  st, err := g.acct.Close(ctx, userID, book)
	  if err != nil {
		  return BookSnap{}, nil, err
	   }
	   ep, err := g.eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{
		   UserID: userID, Book: book, Actor: actor, Reason: reason,
	  })
	   if err != nil {
		   return st, nil, err
	  }
	   return st, ep, nil
  }

// ReopenWithEpoch 科目 Open 后升世代。旧 epoch 贷记不得入新活户。
  func (g *Gate) ReopenWithEpoch(ctx context.Context, userID, book, actor, reason string) (BookSnap, *closelatecredit.CloseEpoch, error) {
	   if g == nil || g.acct == nil || g.eng == nil {
		  return BookSnap{}, nil, closelatecredit.ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	  st, err := g.acct.Open(ctx, userID, book)
	  if err != nil {
		   return BookSnap{}, nil, err
	  }
	  ep, err := g.eng.ReopenWithEpoch(ctx, closelatecredit.ReopenRequest{
		  UserID: userID,
		  Book:   book, Actor: actor, Reason: reason,
	  })
	  if err != nil {
		  return st, nil, err
	  }
	  return st, ep, nil
  }

// CreditAssert 转发贷记断言（只读账户状态，不写账）。

  func (g *Gate) CreditAssert(ctx context.Context, req closelatecredit.CreditAssertRequest) (*closelatecredit.GateResult, error) {
	  if g == nil || g.eng == nil {
		  return nil, closelatecredit.ErrUnavailable
	  }
	  return g.eng.CreditAssert(ctx,
		  req)
   }

// CurrentEpoch 只读世代。
   func (g *Gate) CurrentEpoch(ctx context.Context, userID, book string) (*closelatecredit.CloseEpoch, error) {
	   if g == nil || g.eng == nil {
		   return nil,
			  closelatecredit.ErrUnavailable
	   }
	   return g.eng.CurrentEpoch(ctx, userID, book)
   }
// temporary guard removed after the production check

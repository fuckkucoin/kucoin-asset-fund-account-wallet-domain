package memoeffectgate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type FundSurfTickets struct {
	Book *fundsurf.Book
}

   func AdaptFundSurfTickets(book *fundsurf.Book) *FundSurfTickets {
	  return &FundSurfTickets{Book: book}
  }

   func (a *FundSurfTickets) GetMemoTicket(ctx context.Context, id string) (MemoTicketView, error) {
	  if a == nil || a.Book == nil {
		   return MemoTicketView{}, ErrBookUnavailable
	   }
	  t,
		  err := a.Book.GetMemoTicket(ctx, id)
	  if err != nil {
		   if err == fundsurf.ErrGone {
			  return MemoTicketView{},
				  ErrTicketNotFound
		  }
		  return MemoTicketView{}, err
	  }
	  return viewOf(t), nil
  }

  func (a *FundSurfTickets) AdvanceTicket(ctx context.Context, id, status string) (MemoTicketView, error) {
	   if a == nil || a.Book == nil {
		   return MemoTicketView{}, ErrBookUnavailable
	   }
	  t, err := a.Book.DecideMemo(ctx, id, status)
	  if err != nil {
		   return MemoTicketView{}, err
	  }
	  return viewOf(t), nil
  }

  func viewOf(t fundsurf.MemoTicket) MemoTicketView {
	   return MemoTicketView{
		  ID: t.ID, UserID: t.UserID, Chain: t.Chain, TxHash: t.TxHash,
		  Memo: t.Memo,
		  Address: t.Address, Status: t.Status,
	   }
   }

type FundSurfCredit struct {
	Book *fundsurf.Book
	Dep fundsurf.MemoDepositor
}

   func AdaptFundSurfCredit(book *fundsurf.Book, dep fundsurf.MemoDepositor) *MemCreditPort {
	  // 无业务 depositor 时退回内存端口，避免伪造成功入账。
	   if book == nil || dep == nil {
		   return NewMemCreditPort()
	  }
	   _ = book
	   return NewMemCreditPort()
  }

type LiveCredit struct {
	Book *fundsurf.Book
	Dep fundsurf.MemoDepositor
}

  func NewLiveCredit(book *fundsurf.Book, dep fundsurf.MemoDepositor) *LiveCredit {
	   return &LiveCredit{Book: book, Dep: dep}
   }

  func (c *LiveCredit) BindMemo(userID, chain, memo string) error {
	  if c == nil || c.Dep == nil {
		  return ErrCreditUnavailable
	  }
	  return c.Dep.BindMemo(userID, chain, memo)
   }

  func (c *LiveCredit) ObserveCredit(ctx context.Context, userID, chain, txHash, asset, memo, addr string) (CreditSight, error) {
	  if c == nil || c.Dep == nil {
		   return CreditSight{}, ErrCreditUnavailable
	  }
	  tr, err := c.Dep.ObserveCredit(ctx, userID, chain, txHash, asset, memo, addr)
	  if err != nil {
		   return CreditSight{}, err
	  }
	   if tr == nil {
		   return CreditSight{}, ErrObserveFailed
	  }
	  return CreditSight{
		  TransferID: tr.ID,
		   Amount:   tr.Amount.String(),
		  State:    string(tr.State),
		  Credited: tr.State == ledger.StateCredited,
	  }, nil
  }

  func (c *LiveCredit) ApplyInbound(ctx context.Context, userID, chain, asset, amount, idem string) error {
	  if c == nil || c.Book == nil {
		  return ErrCreditUnavailable
	   }
	  return c.Book.ApplyInbound(ctx,
		   userID, chain, asset, amount, idem)
  }

   func (c *LiveCredit) ReverseInbound(ctx context.Context, userID, asset, amount, idem string) error {
	   if c == nil || c.Book == nil || c.Book.Ledger() == nil {
		   return ErrCreditUnavailable
	  }
	  amt, err := ledger.ParseAmount(amount)
	  if err != nil {
		   return err
	  }
	   _, err = c.Book.Ledger().Create(ctx, ledger.CreateReq{
		   UserID: userID, Direction: ledger.SpotToWallet, Asset: asset, Amount: amt,
		  IdempotencyKey: strings.TrimSpace(idem) + ":reverse",
	   })
	  return err
   }

package acctlifegate_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/acctlifegate"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

type acctBooks struct{ s *account.Service }

  func (a acctBooks) Close(ctx context.Context, userID, book string) (acctlifegate.BookSnap, error) {
	   st, err := a.s.Close(ctx, userID, account.Book(book))
	  return acctlifegate.BookSnap{UserID: st.UserID, Book: string(st.Book), Status: string(st.Status)}, err
  }

  func (a acctBooks) Open(ctx context.Context, userID, book string) (acctlifegate.BookSnap, error) {
	  st, err := a.s.Open(ctx, userID, account.Book(book))
	  return acctlifegate.BookSnap{UserID: st.UserID, Book: string(st.Book), Status: string(st.Status)}, err
   }

  func TestCloseWithEpochBinds(t *testing.T) {
	   ctx := context.Background()
	  acct := account.New(nil)
	  if _, err := acct.Open(ctx, "ug", account.BookTrading); err != nil {
		   t.Fatal(err)
	   }
	  eng := closelatecredit.NewEngine(closelatecredit.NewMemStore(), closelatecredit.Config{})
	  g := acctlifegate.NewCloseEpoch(acctBooks{acct}, eng)
	  st, ep, err := g.CloseWithEpoch(ctx, "ug", string(account.BookTrading), "ops", "user-request")
	  if err != nil {
		   t.Fatal(err)
	  }
	   if st.Status != string(account.StatusClosed) {
		  t.Fatalf("status %s", st.Status)
	  }
	  if ep.N != 2 || ep.State != closelatecredit.EpochClosed {
		  t.Fatalf("epoch %+v", ep)
	  }
	   if !acct.ClosedBook("ug", account.BookTrading) {
		   t.Fatal("book not closed")
	  }
	   n := ep.N
	  res, err := g.CreditAssert(ctx, closelatecredit.CreditAssertRequest{
		  UserID: "ug", Source: closelatecredit.SourceObserve, Asset: "USDT", Amount: "1.00",
		   TxRef: "g-1", ClaimedEpoch: &n,
	   })
	  if res.Allowed || res.Proof == nil {
		   t.Fatalf("%+v %v", res, err)
	  }
  }

   func TestReopenOldEpoch(t *testing.T) {
	   ctx := context.Background()
	  acct := account.New(nil)
	   _, _ = acct.Open(ctx, "ur", account.BookTrading)
	   eng := closelatecredit.NewEngine(closelatecredit.NewMemStore(), closelatecredit.Config{})
	   g := acctlifegate.NewCloseEpoch(acctBooks{acct}, eng)
	   _, closed, err := g.CloseWithEpoch(ctx, "ur", string(account.BookTrading), "ops", "")
	  if err != nil {
		   t.Fatal(err)
	   }
	  _, live, err := g.ReopenWithEpoch(ctx, "ur", string(account.BookTrading), "ops", "kyc-ok")
	   if err != nil {
		  t.Fatal(err)
	  }
	  if live.N <= closed.N {
		   t.Fatalf("reopen %d <= %d", live.N, closed.N)
	   }
	  old := closed.N
	   res, err := g.CreditAssert(ctx, closelatecredit.CreditAssertRequest{
		   UserID: "ur", Source: closelatecredit.SourceOutbox, Asset: "USDT", Amount: "2.00",
		   TxRef: "old", ClaimedEpoch: &old,
	   })
	  if res.Allowed {
		   t.Fatal("old epoch admitted to live")
	  }
	  if !errors.Is(err, closelatecredit.ErrEpochStale) && res.RejectReason != string(closelatecredit.ReasonStaleAfterOpen) {
		   t.Fatalf("want stale got %v %+v", err, res)
	   }
  }

   func TestGateUnavailable(t *testing.T) {
	  g := acctlifegate.NewCloseEpoch(nil, nil)
	   if _, _, err := g.CloseWithEpoch(context.Background(), "u", string(account.BookTrading), "", ""); err == nil {
		   t.Fatal("want unavailable")
	  }
   }

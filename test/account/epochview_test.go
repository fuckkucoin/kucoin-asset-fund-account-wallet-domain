package account_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/account"
)

   func TestBookStatusReadOnly(t *testing.T) {
	  svc := account.New(nil)
	  ctx := context.Background()
	  if _, err := svc.Open(ctx, "uv", account.BookTrading); err != nil {
		  t.Fatal(err)
	   }
	   if !svc.CanCreditBook("uv", account.BookTrading) {
		   t.Fatal("open should credit")
	  }
	   if _, err := svc.Close(ctx, "uv", account.BookTrading); err != nil {
		  t.Fatal(err)
	  }
	  if !svc.ClosedBook("uv", account.BookTrading) {
		   t.Fatal("closed")
	   }
	  if svc.CanCreditBook("uv", account.BookTrading) {
		   t.Fatal("closed must not credit")
	  }
	  ev := svc.LifecycleEvents("uv", 10)
	   if len(ev) == 0 {
		  t.Fatal("events")
	   }
	  if _, err := svc.ListBooks(ctx, "uv"); err != nil {
		  t.Fatal(err)
	  }
  }

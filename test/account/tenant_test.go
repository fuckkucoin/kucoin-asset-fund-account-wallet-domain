package account_test

import (
	"context"
	"errors"
	"testing"

	"account-assets.kucoin.net/funding/internal/account"
)

func TestTenantPinsUser(t *testing.T) {
	svc := account.New(nil)
	ctxA := account.WithTenant(context.Background(), "tenant-a")
	if _, err := svc.Open(ctxA, "u1", account.BookTrading); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctxA, "u1", account.BookFunding); err != nil {
		t.Fatal(err)
	}
	ctxB := account.WithTenant(context.Background(), "tenant-b")
	_, err := svc.Freeze(ctxB, "u1", account.BookTrading, true)
	if !errors.Is(err, account.ErrTenant) {
		t.Fatalf("cross tenant err = %v", err)
	}
	_, err = svc.Open(context.Background(), "u1", account.BookEarn)
	if !errors.Is(err, account.ErrTenant) {
		t.Fatalf("missing tenant err = %v", err)
	}
}

func TestEmptyTenantStaysSingleTenant(t *testing.T) {
	svc := account.New(nil)
	if _, err := svc.Open(context.Background(), "u2", account.BookTrading); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(context.Background(), "u2", account.BookFunding); err != nil {
		t.Fatal(err)
	}
}

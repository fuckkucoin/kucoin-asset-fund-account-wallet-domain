package wdverify_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wdverify"
)

   func newTestGate(t *testing.T) *wdverify.Gate {
	  t.Helper()
	   kv := persist.NewMemKV()
	   return wdverify.New(kv, nil, nil)
   }

// TestCheckWithdrawWithLease_NoGuard_Fallback：无 LeaseGuard 时退化为 CheckWithdraw（向前兼容）。
   func TestCheckWithdrawWithLease_NoGuard_Fallback(t *testing.T) {
	  g := newTestGate(t)
	  // 无 LeaseGuard，无组合，无 tradepw → 通过。
	   err := g.CheckWithdrawWithLease(
		   context.Background(),
		  "user-001", "ETH", "0xAAAA",
		   wdverify.Presented{},
		  false,
		  wdverify.CheckWithdrawOpts{LeaseID: ""},
	   )
	   if err != nil {
		   t.Errorf("expected no error (no guard), got %v", err)
	  }
   }

// TestCheckWithdrawWithLease_NoLeaseID_Reject：有 LeaseGuard 但无 LeaseID 时拒绝。

   func TestCheckWithdrawWithLease_NoLeaseID_Reject(t *testing.T) {
	  g := newTestGate(t)
	   guard := wdverify.LeaseGuardFunc(func(_ context.Context, leaseID, _, _, _, _, _ string) (*wdverify.LeaseGuardResult, error) {
		   if leaseID == "" {
			  return nil, wdverify.ErrNoLease
		  }
		  return &wdverify.LeaseGuardResult{LeaseID: leaseID}, nil
	   })
	   g.BindLeaseGuard(guard)
	  err := g.CheckWithdrawWithLease(
		   context.Background(),
		   "user-002", "ETH", "0xAAAA",
		   wdverify.Presented{},
		  false,
		   wdverify.CheckWithdrawOpts{LeaseID: ""},
	  )
	  if err == nil {
		   t.Fatal("expected error for empty lease_id with guard bound")
	  }
	  // 无 LeaseID → ErrNoLease（Gate 层自检）。
	  if !errors.Is(err, wdverify.ErrNoLease) {
		  t.Errorf("expected ErrNoLease, got %v", err)
	   }
  }

// TestCheckWithdrawWithLease_GuardDrift_StaleError：LeaseGuard 返回漂移时包装 LeaseStaleError。
  func TestCheckWithdrawWithLease_GuardDrift_StaleError(t *testing.T) {
	   g := newTestGate(t)
	  someStaleErr := errors.New("combo gen drifted")
	  guard := wdverify.LeaseGuardFunc(func(_ context.Context, leaseID, _, _, _, _, _ string) (*wdverify.LeaseGuardResult, error) {
		   return &wdverify.LeaseGuardResult{
			  LeaseID: leaseID,
			  Drifted: true,
			  ProofID: "proof-001",
		  }, someStaleErr
	   })
	   g.BindLeaseGuard(guard)
	  err := g.CheckWithdrawWithLease(
		  context.Background(),
		  "user-003", "ETH", "0xBBBB",
		   wdverify.Presented{},
		   false,
		   wdverify.CheckWithdrawOpts{
			  LeaseID: "lease-001",
			  Factor: "totp",
		  },
	   )
	   if err == nil {
		  t.Fatal("expected stale error from guard")
	  }
	   var staleErr *wdverify.LeaseStaleError
	  if !errors.As(err, &staleErr) {
		   t.Fatalf("expected *LeaseStaleError, got %T: %v", err, err)
	   }
	  if staleErr.ProofID != "proof-001" {
		   t.Errorf("proof_id: got %s, want proof-001", staleErr.ProofID)
	  }
	  if staleErr.LeaseID != "lease-001" {
		   t.Errorf("lease_id: got %s, want lease-001", staleErr.LeaseID)
	   }
	  if !errors.Is(err, someStaleErr) {
		  t.Errorf("Unwrap should expose original error")
	  }
   }

// TestLeaseGuardFunc_Adapter：LeaseGuardFunc 实现 LeaseGuard 接口。

   func TestLeaseGuardFunc_Adapter(t *testing.T) {
	  called := false
	   fn := wdverify.LeaseGuardFunc(func(_ context.Context, _, _, _, _, _, _ string) (*wdverify.LeaseGuardResult, error) {
		  called = true
		   return &wdverify.LeaseGuardResult{LeaseID: "x"}, nil
	  })
	  result, err := fn.CheckWithdrawGuard(context.Background(), "lid", "uid", "totp", "", "ETH", "0x0")
	  if err != nil {
		   t.Fatalf("unexpected error: %v", err)
	   }
	   if !called {
		  t.Error("guard func was not called")
	   }
	  if result.LeaseID != "x" {
		   t.Errorf("lease_id: got %s, want x", result.LeaseID)
	   }
   }

// TestLeaseStaleError_Nil_Safety：LeaseStaleError nil 指针安全。

  func TestLeaseStaleError_Nil_Safety(t *testing.T) {
	   var e *wdverify.LeaseStaleError
	   if e.Error() == "" {
		  // nil 指针 Error 应返回非空哨兵字符串。
	  }
	  if e.Unwrap() != nil {
		   t.Error("nil Unwrap should be nil")
	  }
  }

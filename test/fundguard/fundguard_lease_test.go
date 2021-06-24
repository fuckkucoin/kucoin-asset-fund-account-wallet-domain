package fundguard_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fundguard"
)

// ---- 辅助 ----

   func newFGSvc() *fundguard.Service {
	   return fundguard.New()
   }

// ---- 1 — CheckPartialAmt 计数守恒 ----

type partialAmtCase struct {
	name string
	p            fundguard.PartialAmt
	wantErr      error
	wantCountOK  bool
	wantAmountOK bool
}

   func TestCheckPartialAmt_FundGuard_TableDriven(t *testing.T) {
	   svc := newFGSvc()
	  cases := []partialAmtCase{
		   {
			   name: "both_count_and_amount_ok",
			   p: fundguard.PartialAmt{
				  Submitted: 5, Skipped: 3, Failed: 2, Total: 10,
				  SubmittedAmt: "500", SkippedAmt: "300", FailedAmt: "200",
				   ExpectedTotal: "1000",
			  },
			   wantErr: nil, wantCountOK: true, wantAmountOK: true,
		   },
		  {
			  name: "count_fail",
			   p: fundguard.PartialAmt{
				   Submitted: 5, Skipped: 3, Failed: 1, Total: 10, // 9 != 10
				   SubmittedAmt: "500", SkippedAmt: "300", FailedAmt: "100",
				   ExpectedTotal: "900",
			  },
			  wantErr: fundguard.ErrPartial, wantCountOK: false, wantAmountOK: false,
		  },
		   {
			  name: "count_ok_amount_fail",
			  p: fundguard.PartialAmt{
				  Submitted: 5, Skipped: 3, Failed: 2, Total: 10,
				   SubmittedAmt: "500", SkippedAmt: "300", FailedAmt: "199",
				   ExpectedTotal: "1000", // actual = 999 ≠ 1000
			   },
			   wantErr: fundguard.ErrPartialAmt, wantCountOK: true, wantAmountOK: false,
		   },
		   {
			   name: "negative_count",
			   p: fundguard.PartialAmt{
				  Submitted: -1, Skipped: 0, Failed: 0, Total: -1,
			  },
			   wantErr:     fundguard.ErrPartial,
			   wantCountOK: false,
		   },
	   }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			   res,
				   err := svc.CheckPartialAmt(tc.p)
			  if err != tc.wantErr {
				  t.Fatalf("CheckPartialAmt error: got %v, want %v", err, tc.wantErr)
			  }
			  if res.CountOK != tc.wantCountOK {
				  t.Fatalf("CountOK: got %v, want %v", res.CountOK, tc.wantCountOK)
			  }
			   if tc.wantErr == nil && res.AmountOK != tc.wantAmountOK {
				   t.Fatalf("AmountOK: got %v, want %v", res.AmountOK, tc.wantAmountOK)
			   }
		   })
	  }
   }

// ---- 2 — BundleHash 幂等性 ----

   func TestBundleHash_Idempotent(t *testing.T) {
	  b := fundguard.Bundle{
		  UserID: "user-alice-001",
		  Asset: "USDT",
		  Total: "1000",
		  At: time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC),
		   Rows: []fundguard.Snap{
			   {UserID: "user-alice-001", Book: "main", Asset: "USDT", Available: "800", Frozen: "200"},
		   },
	   }
	  h1 := fundguard.BundleHash(b)
	  h2 := fundguard.BundleHash(b)
	   if h1 != h2 {
		   t.Fatalf("BundleHash not idempotent: %s != %s", h1, h2)
	  }
	   if h1 == "" {
		   t.Fatal("BundleHash returned empty string")
	  }
   }

// ---- 3 — BundleHash 对不同内容不等 ----

  func TestBundleHash_DifferentContent(t *testing.T) {
	   b1 := fundguard.Bundle{
		   UserID: "user-alice-001", Asset: "USDT", Total: "1000",
		   At: time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC),
		   Rows: []fundguard.Snap{
			   {UserID: "user-alice-001", Book: "main", Asset: "USDT", Available: "800", Frozen: "200"},
		  },
	   }
	  b2 := fundguard.Bundle{
		  UserID: "user-alice-001", Asset: "USDT", Total: "1001", // 不同 Total
		  At: time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC),
		  Rows: []fundguard.Snap{
			   {UserID: "user-alice-001", Book: "main", Asset: "USDT", Available: "801", Frozen: "200"},
		  },
	   }
	   if fundguard.BundleHash(b1) == fundguard.BundleHash(b2) {
		   t.Fatal("BundleHash collision on different bundles")
	  }
  }

// ---- 4 — ConserveWithLease（无 leaser 注入时退化到原 Conserve）----

   func TestConserveWithLease_NoLeaser_Degraded(t *testing.T) {
	  svc := newFGSvc() // leaser 未注入
	   before := fundguard.Bundle{UserID: "u1", Asset: "USDT", Total: "100"}
	   after := fundguard.Bundle{UserID: "u1", Asset: "USDT", Total: "100"}
	   err := svc.ConserveWithLease(context.Background(), before, after, "", "")
	  if err != nil {
		   t.Fatalf("ConserveWithLease without leaser should succeed: %v", err)
	  }
   }

  func TestConserveWithLease_NoLeaser_Imbalance(t *testing.T) {
	   svc := newFGSvc()
	  before := fundguard.Bundle{UserID: "u1", Asset: "USDT", Total: "100"}
	   after := fundguard.Bundle{UserID: "u1", Asset: "USDT", Total: "99"}
	  err := svc.ConserveWithLease(context.Background(), before, after, "", "")
	  if err != fundguard.ErrConserve {
		  t.Fatalf("expected ErrConserve, got %v", err)
	  }
   }

// ---- 5 — ConserveWithProof（无 proofer 注入时退化到 Conserve）----

  func TestConserveWithProof_NoProofer_Degraded(t *testing.T) {
	   svc := newFGSvc()
	  before := fundguard.Bundle{UserID: "u1", Asset: "BTC", Total: "1"}
	   after := fundguard.Bundle{UserID: "u1", Asset: "BTC", Total: "1"}
	   err := svc.ConserveWithProof(context.Background(), before, after, "", "")
	   if err != nil {
		  t.Fatalf("ConserveWithProof without proofer should succeed: %v", err)
	   }
  }

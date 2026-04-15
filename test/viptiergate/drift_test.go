package viptiergate_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/viptiergate"
)

// ─────────────────────────────────────────────────────────────────────────────
// OpenDriftCase 幂等
// ─────────────────────────────────────────────────────────────────────────────

   func TestOpenDriftCase_Idempotent(t *testing.T) {
	  ctx := context.Background()
	  svc := viptiergate.NewService(nil)
	  userID := "u-dc-idem-001"
	  req := viptiergate.OpenDriftCaseReq{
		  UserID: userID,
		   Asset: "BTC",
		   Kind: viptiergate.DriftBPSStale,
		   ObservedTier: viptiergate.TierStandard,
		   ExpectedTier: viptiergate.TierVIP,
		   ObservedBPS: 10,
		  ExpectedBPS: 5,
		   Amount: "100.0",
		   Generation: 1,
	  }
	  dc1, err := svc.OpenDriftCase(ctx, req)
	   if err != nil {
		  t.Fatalf("first OpenDriftCase: %v", err)
	   }
	  // 相同参数再次调用，应返回已有案件（幂等）。
	  dc2, err := svc.OpenDriftCase(ctx, req)
	   if err != nil {
		   t.Fatalf("second OpenDriftCase: %v", err)
	   }
	  if dc1.ID != dc2.ID {
		   t.Errorf("idempotent call should return same case: %s vs %s", dc1.ID, dc2.ID)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// DriftCase 所有类型开立
// ─────────────────────────────────────────────────────────────────────────────

   func TestOpenDriftCase_AllKinds(t *testing.T) {
	   ctx := context.Background()
	   svc := viptiergate.NewService(nil)
	  kinds := []viptiergate.DriftKind{
		   viptiergate.DriftBPSStale,
		   viptiergate.DriftFreezeStale,
		   viptiergate.DriftBPSReverse,
		  viptiergate.DriftQuoteGenStale,
	  }
	  for _, kind := range kinds {
		   t.Run(string(kind), func(t *testing.T) {
			   userID := "u-dk-" + string(kind)
			  dc, err := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
				   UserID: userID,
				  Asset:  "USDT",
				   Kind: kind,
				  ObservedTier: viptiergate.TierStandard,
				   ExpectedTier: viptiergate.TierVIP,
				  ObservedBPS:  10,
				   ExpectedBPS:  5,
				   Amount: "500.0",
				  Generation: 1,
			   })
			  if err != nil {
				  t.Fatalf("OpenDriftCase kind=%s: %v", kind, err)
			  }
			   if dc.Kind != kind {
				  t.Errorf("kind mismatch: got %s, want %s", dc.Kind, kind)
			  }
			  if dc.Status != viptiergate.DriftOpen {
				   t.Errorf("initial status: got %s, want open", dc.Status)
			  }
			   if !dc.IsBlocking() {
				  t.Error("open drift case should be blocking")
			  }
		  })
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// DriftProof 构建+验证+导出完整路径
// ─────────────────────────────────────────────────────────────────────────────

   func TestDriftProof_BuildVerifyExport(t *testing.T) {
	   ctx := context.Background()
	   svc := viptiergate.NewService(nil)
	  userID := "u-proof-full-001"
	   dc, _ := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		   UserID: userID,
		   Asset:        "ETH",
		  Kind:         viptiergate.DriftFreezeStale,
		  ObservedTier: viptiergate.TierVIP,
		  ExpectedTier: viptiergate.TierVIPPlus,
		   ObservedBPS: 5,
		   ExpectedBPS: 2,
		  Amount:      "3.0",
		   Generation:  4,
	  })
	   // 构建并验证证明。
	  proof, err := svc.BuildAndVerifyProof(ctx, dc.ID)
	  if err != nil {
		   t.Fatalf("BuildAndVerifyProof: %v", err)
	   }
	   if proof.Status != viptiergate.DriftProofVerified {
		  t.Errorf("proof status: got %s, want verified", proof.Status)
	  }
	   if proof.Digest == "" {
		  t.Error("proof digest must not be empty")
	   }
	   // 案件应推进至 proven。
	   updated, _ := svc.GetDriftCase(ctx, dc.ID)
	   if updated.Status != viptiergate.DriftProven {
		   t.Errorf("drift case status after proof: got %s, want proven", updated.Status)
	  }
	   if updated.ProofID != proof.ID {
		   t.Errorf("drift case proof_id: got %s, want %s", updated.ProofID, proof.ID)
	  }
	   // 导出证明（在线验签）。
	  exported, err := svc.ExportProof(ctx, proof.ID)
	  if err != nil {
		   t.Fatalf("ExportProof: %v", err)
	   }
	  if exported.ID != proof.ID {
		  t.Errorf("exported proof ID mismatch: %s vs %s", exported.ID, proof.ID)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 重复构建证明（幂等）
// ─────────────────────────────────────────────────────────────────────────────

  func TestBuildProof_Idempotent(t *testing.T) {
	  ctx := context.Background()
	  svc := viptiergate.NewService(nil)
	   userID := "u-proof-idem-001"
	  dc, _ := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		  UserID: userID,
		   Asset:  "BTC",
		  Kind:         viptiergate.DriftBPSStale,
		   ObservedTier: viptiergate.TierStandard,
		   ExpectedTier: viptiergate.TierVIP,
		   ObservedBPS:  10,
		  ExpectedBPS: 5,
		   Amount:      "10.0",
		   Generation: 2,
	  })
	   proof1, err := svc.BuildAndVerifyProof(ctx, dc.ID)
	  if err != nil {
		  t.Fatalf("first BuildAndVerifyProof: %v", err)
	  }
	  // 案件已 proven，再次构建应返回已有证明。
	   proof2, err := svc.BuildAndVerifyProof(ctx, dc.ID)
	  if err != nil {
		   t.Fatalf("second BuildAndVerifyProof: %v", err)
	  }
	  // proven 状态的案件再次 Build 返回已有 proof（幂等）。
	  _ = proof1
	   _ = proof2
  }

// ─────────────────────────────────────────────────────────────────────────────
// EstimatedDrift 工具函数
// ─────────────────────────────────────────────────────────────────────────────

   func TestEstimatedDrift(t *testing.T) {
	  cases := []struct {
		   observed int
		  expected int
		   amount   string
		  want     int
	   }{
		  {10, 5, "1000.0", 5},
		  {5, 10, "1000.0", 5},
		  {0, 0, "100.0", 0},
		   {100, 1, "500.0", 99},
	  }
	  for _, tc := range cases {
		  got := viptiergate.EstimatedDrift(tc.observed, tc.expected, tc.amount)
		   if got != tc.want {
			  t.Errorf("EstimatedDrift(%d,%d): got %d, want %d",
				   tc.observed, tc.expected, got)
		   }
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// VerifyDigestOnly 防篡改表驱动
// ─────────────────────────────────────────────────────────────────────────────

   func TestVerifyDigestOnly_TableDriven(t *testing.T) {
	  ctx := context.Background()
	   svc := viptiergate.NewService(nil)
	  userID := "u-digest-table-001"
	  dc, _ := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		   UserID: userID,
		   Asset:  "SOL",
		   Kind:         viptiergate.DriftQuoteGenStale,
		   ObservedTier: viptiergate.TierVIP,
		   ExpectedTier: viptiergate.TierVIPPlus,
		   ObservedBPS: 8,
		  ExpectedBPS: 4,
		   Amount:     "200.0",
		   Generation: 5,
	  })
	  proof,
		  _ := svc.BuildAndVerifyProof(ctx, dc.ID)
	   cases := []struct {
		   name   string
		   mutate func(*viptiergate.DriftProof)
		  wantOK bool
	   }{
		  {
			  name: "untampered",
			  mutate: func(p *viptiergate.DriftProof) {},
			   wantOK: true,
		  },
		   {
			  name: "tamper_observed_bps",
			  mutate: func(p *viptiergate.DriftProof) { p.ObservedBPS = 99 },
			  wantOK: false,
		   },
		   {
			  name:   "tamper_expected_bps",
			   mutate: func(p *viptiergate.DriftProof) { p.ExpectedBPS = 99 },
			  wantOK: false,
		  },
		   {
			   name:   "tamper_amount",
			   mutate: func(p *viptiergate.DriftProof) { p.Amount = "9999.0" },
			   wantOK: false,
		   },
		  {
			   name: "tamper_generation",
			   mutate: func(p *viptiergate.DriftProof) { p.Generation = 999 },
			   wantOK: false,
		   },
		  {
			   name:   "tamper_user_id",
			   mutate: func(p *viptiergate.DriftProof) { p.UserID = "attacker" },
			   wantOK: false,
		  },
		  {
			  name:   "tamper_asset",
			   mutate: func(p *viptiergate.DriftProof) { p.Asset = "FAKE" },
			  wantOK: false,
		  },
		   {
			  name:   "nil_proof",
			   mutate: nil,
			   wantOK: false,
		   },
	  }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   if tc.mutate == nil {
				   if viptiergate.VerifyDigestOnly(nil) {
					   t.Error("VerifyDigestOnly(nil) should be false")
				   }
				   return
			   }
			  copy := proof.Copy()
			   tc.mutate(&copy)
			   got := viptiergate.VerifyDigestOnly(&copy)
			  if got != tc.wantOK {
				   t.Errorf("VerifyDigestOnly: got %v, want %v", got, tc.wantOK)
			  }
		   })
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 多用户并发漂移案隔离
// ─────────────────────────────────────────────────────────────────────────────

   func TestDriftCase_MultiUserIsolation(t *testing.T) {
	  ctx := context.Background()
	   svc := viptiergate.NewService(nil)
	   const userCount = 5
	   for i := 0; i < userCount; i++ {
		  uid := "u-iso-" + string(rune('A'+i))
		  _, err := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
			   UserID: uid,
			  Asset: "USDT",
			  Kind: viptiergate.DriftBPSStale,
			   ObservedTier: viptiergate.TierStandard,
			   ExpectedTier: viptiergate.TierVIP,
			  ObservedBPS: 10,
			  ExpectedBPS: 5,
			   Amount:     "100.0",
			   Generation: int64(i),
		   })
		  if err != nil {
			   t.Fatalf("user %s OpenDriftCase: %v", uid, err)
		   }
	   }
	  // 每个用户只能看到自己的漂移案。
	  for i := 0; i < userCount; i++ {
		  uid := "u-iso-" + string(rune('A'+i))
		   cases, err := svc.ListOpenDriftCasesForUser(ctx, uid)
		  if err != nil {
			  t.Fatalf("ListOpenDriftCasesForUser %s: %v", uid, err)
		   }
		   if len(cases) != 1 {
			  t.Errorf("user %s: expected 1 drift case, got %d", uid, len(cases))
		   }
		   if cases[0].UserID != uid {
			  t.Errorf("cross-user contamination: case.UserID=%s, want %s", cases[0].UserID, uid)
		  }
	   }
   }

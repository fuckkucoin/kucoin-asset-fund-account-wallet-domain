package haltclear_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/haltclear"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ---- 辅助 -----------------------------------------------------------

  func newGate() *custodyhalt.Gate {
	   kv := persist.NewMemKV()
	   return custodyhalt.New(kv)
   }

   func newDetector(gate *custodyhalt.Gate, diffCount int, diffKeys []string) *haltclear.FalseClearDetector {
	  caseRepo := haltclear.NewMemCaseRepo()
	  diffQuery := &haltclear.StaticDiffQuerier{Count: diffCount, Keys: diffKeys}
	   return haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
		   haltclear.ScopeTarget{Scope: "global"},
	   )
  }

// ---- Detect 检测逻辑表驱动 ----------------------------------------

var detectTable = []struct {
	name string
	haltOn bool // 熔断是否开启
	openDiffs int // 未关差异数
	existingOpen bool // 是否已有 open 案例
	wantMisclear bool // 是否应产出新案例
	wantExisting bool
}{
	{"halted no detect", true, 5, false, false, false},
	{"no diffs no detect", false, 0, false, false, false},
	{"open diffs detect", false, 3, false, true, false},
	{"diffs but existing case", false, 3, true, false, true},
	{"halted even with diffs", true, 3, false, false, false},
}

   func TestDetect(t *testing.T) {
	  for _, tc := range detectTable {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   gate := newGate()
			  ctx := context.Background()
			   if tc.haltOn {
				  if err := gate.Engage(ctx, "system", "test"); err != nil {
					   t.Fatal(err)
				   }
			  }
			   caseRepo := haltclear.NewMemCaseRepo()
			   diffQuery := &haltclear.StaticDiffQuerier{Count: tc.openDiffs}
			   if tc.existingOpen {
				  now := func() interface{} { return nil }
				  _ = now
				   // 插入一条已有的 open 案例。
				  _ = caseRepo.Create(ctx, &haltclear.MisClearCase{
					  MisclearID: "existing-mc",
					   Scope:      "global",
					   State:   haltclear.MisClearStateOpen,
					  Trigger: haltclear.TriggerOpenDiffScan,
					   OpenDiffCount: tc.openDiffs,
				   })
			  }
			   detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
				  haltclear.ScopeTarget{Scope: "global"},
			  )
			   results := detector.Detect(ctx)
			  if len(results) == 0 {
				   t.Fatal("expected at least one result")
			  }
			   r := results[0]
			   if r.Error != nil {
				   t.Fatalf("unexpected error: %v", r.Error)
			   }
			  if tc.wantMisclear && r.MisclearID == "" {
				  t.Error("expected misclear_id to be set")
			  }
			  if !tc.wantMisclear && r.MisclearID != "" {
				   t.Errorf("unexpected misclear_id=%s", r.MisclearID)
			  }
			  if tc.wantExisting && !r.Existing {
				   t.Error("expected existing=true")
			  }
		  })
	  }
   }

// TestDetectNoDuplicateCase 同一作用域不产出重复 open 案例。

   func TestDetectNoDuplicateCase(t *testing.T) {
	  gate := newGate()
	  ctx := context.Background()
	   caseRepo := haltclear.NewMemCaseRepo()
	   diffQuery := &haltclear.StaticDiffQuerier{Count: 3, Keys: []string{"key-1", "key-2"}}
	   detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
		   haltclear.ScopeTarget{Scope: "global"},
	  )
	  // 第一次检测产出案例。
	   results1 := detector.Detect(ctx)
	   if len(results1) == 0 || results1[0].MisclearID == "" {
		  t.Fatal("first detect should produce a misclear case")
	   }
	   // 第二次检测不应重复产出。
	  results2 := detector.Detect(ctx)
	  if len(results2) == 0 {
		  t.Fatal("expected result")
	  }
	  if results2[0].MisclearID != "" {
		   t.Error("second detect should not produce a new misclear case")
	  }
	  if !results2[0].Existing {
		  t.Error("second detect should mark existing=true")
	   }
   }

// TestDetectDiffKeys 差异工单 key 最多截断为 32 条。
  func TestDetectDiffKeys(t *testing.T) {
	   gate := newGate()
	   ctx := context.Background()
	   caseRepo := haltclear.NewMemCaseRepo()
	  keys := make([]string, 50)
	  for i := range keys {
		   keys[i] = fmt.Sprintf("key-%d", i)
	  }
	  diffQuery := &haltclear.StaticDiffQuerier{Count: 50,
		  Keys: keys}
	   detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
		   haltclear.ScopeTarget{Scope: "global"},
	  )
	  results := detector.Detect(ctx)
	   if len(results) == 0 || results[0].MisclearID == "" {
		  t.Fatal("expected misclear case")
	   }
	   mc, err := caseRepo.Get(ctx, results[0].MisclearID)
	  if err != nil {
		  t.Fatal(err)
	  }
	   if len(mc.OpenDiffKeys) > 32 {
		   t.Errorf("open_diff_keys=%d should be truncated to 32", len(mc.OpenDiffKeys))
	   }
  }

// TestDetectFromForcedAudit ForcedClear 后 open_diff_count>0 产出 MisClearCase。
   func TestDetectFromForcedAudit(t *testing.T) {
	   gate := newGate()
	   ctx := context.Background()
	   caseRepo := haltclear.NewMemCaseRepo()
	   diffQuery := haltclear.NoopDiffQuerier{}
	  detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
		  haltclear.ScopeTarget{Scope: "global"},
	   )
	   mc, err := detector.DetectFromForcedAudit(ctx,
		   "fa:test-001", "global", "", "", 3, []string{"k1", "k2", "k3"})
	  if err != nil {
		   t.Fatalf("detect: %v", err)
	   }
	  if mc == nil {
		   t.Fatal("expected MisClearCase")
	   }
	   if mc.Trigger != haltclear.TriggerForcedClear {
		  t.Errorf("trigger=%s want forced_clear", mc.Trigger)
	  }
	  if mc.ForcedAuditID != "fa:test-001" {
		  t.Errorf("forced_audit_id=%s want fa:test-001", mc.ForcedAuditID)
	  }
	  if mc.OpenDiffCount != 3 {
		   t.Errorf("open_diff_count=%d want 3", mc.OpenDiffCount)
	   }
  }

// TestDetectFromForcedAudit_ZeroDiffs 无差异时不产出案例。
   func TestDetectFromForcedAudit_ZeroDiffs(t *testing.T) {
	   gate := newGate()
	  caseRepo := haltclear.NewMemCaseRepo()
	  diffQuery := haltclear.NoopDiffQuerier{}
	  detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo)
	   mc, err := detector.DetectFromForcedAudit(context.Background(), "fa:002", "global", "", "", 0, nil)
	  if err != nil {
		  t.Fatalf("unexpected error: %v", err)
	  }
	   if mc != nil {
		  t.Error("expected no misclear case for zero diffs")
	  }
  }

// TestDetectDiffQueryError 差异查询失败时 DetectResult 携带错误。
   func TestDetectDiffQueryError(t *testing.T) {
	  gate := newGate()
	  ctx := context.Background()
	  caseRepo := haltclear.NewMemCaseRepo()
	   diffQuery := &haltclear.StaticDiffQuerier{Err: errors.New("recon db unavailable")}
	  detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
		  haltclear.ScopeTarget{Scope: "global"},
	  )
	  results := detector.Detect(ctx)
	  if len(results) == 0 {
		   t.Fatal("expected result")
	  }
	   if results[0].Error == nil {
		  t.Error("expected error in result")
	   }
	  if results[0].MisclearID != "" {
		  t.Error("should not produce misclear case on query error")
	   }
   }

// TestDetectMultipleScopes 多个作用域各自检测。
  func TestDetectMultipleScopes(t *testing.T) {
	  gate := newGate()
	  ctx := context.Background()
	   caseRepo := haltclear.NewMemCaseRepo()
	   diffQuery := &haltclear.StaticDiffQuerier{Count: 2}
	  detector := haltclear.NewFalseClearDetector(gate,
		  diffQuery, caseRepo,
		   haltclear.ScopeTarget{Scope: "global"},
		   haltclear.ScopeTarget{Scope: "chain", Chain: "ETH"},
		  haltclear.ScopeTarget{Scope: "asset", Asset: "USDT"},
	  )
	  results := detector.Detect(ctx)
	   if len(results) != 3 {
		   t.Errorf("expected 3 results got %d", len(results))
	  }
	   newCount := 0
	  for _, r := range results {
		  if r.Error != nil {
			   t.Errorf("scope=%s err=%v", r.Scope, r.Error)
		  }
		   if r.MisclearID != "" {
			  newCount++
		  }
	   }
	   if newCount != 3 {
		   t.Errorf("expected 3 new misclear cases got %d", newCount)
	   }
  }

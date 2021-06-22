package flow

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrongcredit"
)

  func newTestOrch() *wrongcredit.Orchestrator {
	  disc := wrongcredit.NewDiscovery()
	   mutex := wrongcredit.NewCreditMutex()
	   return wrongcredit.NewOrchestrator(disc, mutex)
  }

   func TestWrongCreditOpenAndIdempotent(t *testing.T) {
	   o := newTestOrch()
	  req := wrongcredit.OpenReq{
		   IdempotencyKey: "idem-wc-001",
		  UserID:         "user-10001",
		  CreditRef: "xfer-wc-001",
		   Asset:       "USDT",
		  CreditedAmt: "200",
		   OpenedBy:    "cs-ops",
		   Note:        "suspected duplicate credit",
	   }
	   c1, err := o.Open(req)
	   if err != nil {
		  t.Fatalf("open: %v", err)
	   }
	  if c1.Step != wrongcredit.StepOpen {
		  t.Fatalf("step after open: want open, got %s", c1.Step)
	   }
	  if c1.Phase != wrongcredit.PhaseInvestigating {
		  t.Fatalf("phase after open: want investigating, got %s", c1.Phase)
	   }
	   // 幂等：相同参数重开返回原始案件
	  c2, err := o.Open(req)
	   if err != nil {
		   t.Fatalf("idempotent open: %v", err)
	   }
	  if c1.ID != c2.ID {
		   t.Fatalf("idempotent open must return same case: %s vs %s", c1.ID, c2.ID)
	   }
   }

   func TestWrongCreditDuplicateCreditRefRejected(t *testing.T) {
	   o := newTestOrch()
	  req1 := wrongcredit.OpenReq{
		  IdempotencyKey: "idem-wc-dup-1",
		  UserID: "user-10002",
		  CreditRef:   "xfer-wc-dup",
		  Asset:       "USDT",
		   CreditedAmt: "100",
		  OpenedBy:    "cs-ops",
		   Note:        "first case",
	  }
	  if _, err := o.Open(req1); err != nil {
		  t.Fatalf("open first: %v", err)
	  }
	   // 不同幂等键但相同 CreditRef，且第一案件未关闭 → 拒绝
	   req2 := wrongcredit.OpenReq{
		  IdempotencyKey: "idem-wc-dup-2",
		   UserID:         "user-10002",
		   CreditRef:   "xfer-wc-dup",
		  Asset:       "USDT",
		  CreditedAmt: "100",
		   OpenedBy: "cs-ops-2",
		   Note:     "duplicate attempt",
	  }
	   _, err := o.Open(req2)
	   if !errors.Is(err, wrongcredit.ErrDuplicate) {
		  t.Fatalf("expected ErrDuplicate, got %v", err)
	  }
  }

  func TestWrongCreditOpenScoreWaive(t *testing.T) {
	  o := newTestOrch()
	   c, err := o.Open(wrongcredit.OpenReq{
		  IdempotencyKey: "idem-wc-waive",
		   UserID:         "user-10003",
		  CreditRef: "xfer-wc-waive",
		  Asset: "BTC",
		  CreditedAmt: "0.01",
		   OpenedBy: "cs-ops",
	   })
	  if err != nil {
		   t.Fatalf("open: %v", err)
	  }
	   // 附加证据（desk_report 类型，手工举报）
	   _, err = o.AttachEvidence(wrongcredit.AttachEvidenceReq{
		  CaseID: c.ID,
		   Source: wrongcredit.SourceDeskReport,
		   Kind: "manual_review",
		   Note: "user confirmed wrong credit",
		  SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		   Score:  0.8,
		  Actor:  "cs-ops",
	  })
	  if err != nil {
		  t.Fatalf("attach evidence: %v", err)
	  }
	  // 分值（无 usdRate 时跳过层级更新，保持 T1）
	  scored, err := o.Score(c.ID, "cs-ops", "")
	  if err != nil {
		   t.Fatalf("score: %v", err)
	  }
	  if scored.Step != wrongcredit.StepScored {
		  t.Fatalf("step after score: want scored, got %s", scored.Step)
	   }
	  // Waive 豁免（scored 状态可豁免）
	   waived, err := o.Waive(context.Background(), wrongcredit.WaiveReq{
		   CaseID: c.ID,
		   Actor:  "senior-ops",
		   Note:       "amount too small to pursue",
		  FenceEpoch: scored.FenceEpoch,
		  FenceToken: scored.FenceToken,
	  })
	   if err != nil {
		   t.Fatalf("waive: %v", err)
	   }
	  if waived.Step != wrongcredit.StepWaived {
		  t.Fatalf("step after waive: want waived, got %s", waived.Step)
	   }
	   if !waived.Phase.Terminal() {
		   t.Fatalf("phase after waive should be terminal, got %s", waived.Phase)
	  }
  }

   func TestWrongCreditWaivedIsTerminal(t *testing.T) {
	  o := newTestOrch()
	  c, _ := o.Open(wrongcredit.OpenReq{
		  IdempotencyKey: "idem-wc-term",
		  UserID:         "user-10004",
		   CreditRef:      "xfer-wc-term",
		   Asset:          "USDT",
		  CreditedAmt:    "5",
		   OpenedBy: "cs-ops",
	   })
	   // Score 使案件进入 scored 状态
	   scored, _ := o.Score(c.ID, "cs-ops", "")
	   // Waive
	   waived, _ := o.Waive(context.Background(), wrongcredit.WaiveReq{
		   CaseID: c.ID,
		   Actor: "cs-ops",
		   Note:  "waived by ops",
		   FenceEpoch: scored.FenceEpoch,
		  FenceToken: scored.FenceToken,
	   })
	   // 终态案件再次 Score 应拒绝
	  _, err := o.Score(waived.ID, "cs-ops", "")
	   if !errors.Is(err, wrongcredit.ErrTerminal) {
		  t.Fatalf("expected ErrTerminal on waived case, got %v", err)
	   }
   }

  func TestWrongCreditMutexPreventsDoubleReversal(t *testing.T) {
	   mutex := wrongcredit.NewCreditMutex()
	  // wrong-credit 先声明占用
	   if err := mutex.ClaimWrongCredit("xfer-wc-mutex", "case-001"); err != nil {
		   t.Fatalf("ClaimWrongCredit: %v", err)
	   }
	   // reorg 路径尝试同一 CreditRef 应被拒绝（wrong-credit 已占用 → ErrMutexConflict）
	  err := mutex.ClaimReorg("xfer-wc-mutex", "reorg-batch-001")
	   if !errors.Is(err, wrongcredit.ErrMutexConflict) {
		  t.Fatalf("expected ErrMutexConflict, got %v", err)
	   }
	  // 释放后 reorg 可正常声明
	   mutex.Release("xfer-wc-mutex", "case-001")
	  if err := mutex.ClaimReorg("xfer-wc-mutex", "reorg-batch-001"); err != nil {
		  t.Fatalf("ClaimReorg after release: %v", err)
	   }
  }

  func TestWrongCreditStaleScoreRejected(t *testing.T) {
	   o := newTestOrch()
	   c, err := o.Open(wrongcredit.OpenReq{
		  IdempotencyKey: "idem-wc-stale",
		   UserID:         "user-10005",
		  CreditRef:      "xfer-wc-stale",
		  Asset:          "USDT",
		  CreditedAmt:    "10",
		  OpenedBy: "cs-ops",
	   })
	  if err != nil {
		   t.Fatalf("open: %v", err)
	   }
	   scored, err := o.Score(c.ID, "cs-ops", "")
	  if err != nil {
		   t.Fatalf("score: %v", err)
	   }
	   // Waive 使用旧 fence（epoch+1 后过期）
	   _, err = o.Waive(context.Background(), wrongcredit.WaiveReq{
		  CaseID: c.ID,
		   Actor: "cs-ops",
		   Note:  "stale fence attempt",
		   FenceEpoch: scored.FenceEpoch - 1, // stale
		   FenceToken: "wrong-token",
	   })
	   if !errors.Is(err, wrongcredit.ErrFence) {
		  t.Fatalf("expected ErrFence for stale fence, got %v", err)
	  }
  }

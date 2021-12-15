package trexecgate_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/trexecgate"
)

// ──────────────────────────────────────────────
// Mock treasury.Service 的 Plan 路径
// ──────────────────────────────────────────────
// mockInventoryForTreasury 实现 hotinv.Book 的 Observe 接口（用于 treasury.Service.Plan）。
// 注意：treasury.New 接受 *hotinv.Book，不接受接口；所以 PlanAndRecord 需要直接
// 构造 SweepPlan 并调用 RecordPlans，再手动调用 BindLease。
// 此处改为 LeaseAwareService 的内部逻辑，通过 stubLeaseAwareSvc 注入预设计划。
// stubLeaseAwareSvc 为提供可注入计划的集成服务包装。

type stubLeaseAwareSvc struct {
	realSvc *trexecgate.LeaseAwareService
	// fixedPlans 若非空，则跳过 treasury.Service.Plan 直接使用这些计划。
	plans []treasury.SweepPlan
}

// planAndRecordWithPlans 使用预设 SweepPlan 绕过 Plan（Observe），直接走 Record+BindLease。
   func (s *stubLeaseAwareSvc) planAndRecordWithPlans(ctx context.Context, st ledger.Store, plans []treasury.SweepPlan) ([]trexecgate.ExecWithLease, error) {
	  svc := s.realSvc.Underlying()
	  eng := s.realSvc.Engine()
	  out := make([]trexecgate.ExecWithLease,
		   0, len(plans))
	   for _, plan := range plans {
		   exec, err := svc.Exec().Record(plan)
		  if err != nil {
			  return out, err
		  }
		  planID := plan.Asset + ":" + exec.ID
		   bindRes, bindErr := eng.BindLease(ctx, st, trexecgate.BindInput{
			   PlanID: planID,
			  ExecID: exec.ID,
			   Asset:  plan.Asset,
			   Amount: plan.Amount,
		   })
		   if bindErr != nil {
			  _, _ = svc.Exec().MarkFailed(exec.ID, "lease bind failed", exec.Fence.Epoch, exec.Fence.Token)
			  return out, bindErr
		  }
		  out = append(out, trexecgate.ExecWithLease{
			   Exec:  exec,
			  Lease: bindRes.Lease,
			  PlanID: planID,
		  })
	   }
	  return out, nil
   }

// ──────────────────────────────────────────────
// 构造辅助
// ──────────────────────────────────────────────

   func newLeaseAwareSvc(hotAvails map[string]string) (*stubLeaseAwareSvc, *trexecgate.MemLeaseRepo, *trexecgate.MemDriftProofRepo) {
	   inv := &mockHotInv{snaps: hotAvails}
	  leases := trexecgate.NewMemLeaseRepo()
	  proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	  eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog())
	  // treasury.Service 需要 *hotinv.Book；此处用实际的 hotinv.FromHotMax，
	   // 但 PlanSweeps 会用 Observe，中通过直接传 plan 绕过。
	  svc := treasury.New(hotinv.FromHotMax(map[string]string{
		  "USDT": "500000", "BTC": "80", "ETH": "800",
	  }), nil)
	   las := trexecgate.NewLeaseAwareService(svc, eng)
	  stub := &stubLeaseAwareSvc{realSvc: las}
	   return stub, leases, proofs
   }

// ──────────────────────────────────────────────
// 主路径：Record + BindLease + MarkSentWithGate + Ack
// ──────────────────────────────────────────────

  func TestLeaseAwareSvc_FullPath(t *testing.T) {
	  ctx := context.Background()
	   stub, _, _ := newLeaseAwareSvc(map[string]string{"USDT": "480000"})
	   plans := []treasury.SweepPlan{
		   {Asset: "USDT", Amount: "280000", From: "hot", To: "cold", Reason: "above high band", Level: "above_high"},
	   }
	   results, err := stub.planAndRecordWithPlans(ctx, nil, plans)
	   if err != nil {
		   t.Fatalf("planAndRecordWithPlans: %v", err)
	   }
	   if len(results) != 1 {
		  t.Fatalf("expected 1 result, got %d", len(results))
	  }
	  r := results[0]
	  if r.Exec.ID == "" {
		  t.Error("exec.id empty")
	  }
	   if r.Lease == nil {
		  t.Fatal("lease nil after record")
	  }
	  if r.Lease.LeaseID == "" {
		  t.Error("lease_id empty")
	   }
	  if r.Lease.ExecID != r.Exec.ID {
		   t.Errorf("lease.exec_id=%s != exec.id=%s", r.Lease.ExecID, r.Exec.ID)
	  }
	   if r.Lease.State != trexecgate.LeaseBound {
		   t.Errorf("lease state: got %s, want bound", r.Lease.State)
	   }
	   // MarkSentWithGate 通过（热账未变）。
	  eng := stub.realSvc.Engine()
	   rr, err := eng.Recheck(ctx, nil, r.Lease.LeaseID)
	  if err != nil || !rr.Passed {
		   t.Fatalf("Recheck: %v / passed=%v", err, rr.Passed)
	  }
	  sentLease, gateErr := eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		  LeaseID: r.Lease.LeaseID,
		  ExecID:  r.Exec.ID,
	   })
	   if gateErr != nil {
		  t.Fatalf("MarkSentGate: %v", gateErr)
	   }
	   if sentLease.State != trexecgate.LeaseSent {
		   t.Errorf("sent lease state: got %s, want sent", sentLease.State)
	  }
   }

// ──────────────────────────────────────────────
// 漂移拒绝路径：Record 后热账跌破 target
// ──────────────────────────────────────────────

   func TestLeaseAwareSvc_DriftRejectPath(t *testing.T) {
	   ctx := context.Background()
	   inv := &mockHotInv{snaps: map[string]string{"USDT": "480000"}}
	  leases := trexecgate.NewMemLeaseRepo()
	   proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	   eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog())
	  svc := treasury.New(hotinv.FromHotMax(map[string]string{"USDT": "500000"}), nil)
	   // Record 一条计划。
	  plan := treasury.SweepPlan{Asset: "USDT", Amount: "280000", From: "hot", To: "cold"}
	   exec, err := svc.Exec().Record(plan)
	   if err != nil {
		  t.Fatalf("Record: %v", err)
	   }
	  // BindLease（快照 = 480000）。
	  bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		   PlanID: "USDT:" + exec.ID,
		  ExecID: exec.ID,
		   Asset: "USDT",
		  Amount: plan.Amount,
	   })
	  if err != nil {
		   t.Fatalf("BindLease: %v", err)
	   }
	  leaseID := bindRes.Lease.LeaseID
	   // 压低热账至低于 target=200000。
	  inv.snaps["USDT"] = "150000"
	   // Recheck 须拒绝。
	   rr, recheckErr := eng.Recheck(ctx, nil, leaseID)
	  if !errors.Is(recheckErr, trexecgate.ErrDriftExceeded) {
		   t.Fatalf("Recheck: expected ErrDriftExceeded, got %v", recheckErr)
	  }
	   if rr.Proof == nil || rr.Proof.Decision != "blocked" {
		   t.Error("expected blocked proof")
	   }
	   // MarkSentGate 须拒绝（租约已 blocked）。
	  _, msErr := eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		   LeaseID: leaseID,
		  ExecID: exec.ID,
	   })
	   if msErr == nil {
		  t.Error("MarkSentGate: expected error, got nil")
	   }
	   // 验证 Exec 状态仍为 planned（未被 MarkSent 推进）。
	  got, ok := svc.Exec().Get(exec.ID)
	   if !ok {
		   t.Fatal("Exec not found")
	  }
	  if got.Status != treasury.ExecPlanned {
		  t.Errorf("exec status: got %s, want planned", got.Status)
	   }
  }

// ──────────────────────────────────────────────
// MarkSentWithGate 集成：通过场景
// ──────────────────────────────────────────────

  func TestLeaseAwareSvc_MarkSentWithGate_Pass(t *testing.T) {
	  ctx := context.Background()
	  inv := &mockHotInv{snaps: map[string]string{"BTC": "75"}}
	   leases := trexecgate.NewMemLeaseRepo()
	   proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	   eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog())
	  svc := treasury.New(hotinv.FromHotMax(map[string]string{"BTC": "80"}), nil)
	  las := trexecgate.NewLeaseAwareService(svc,
		   eng)
	  plan := treasury.SweepPlan{Asset: "BTC", Amount: "50", From: "hot", To: "cold"}
	   exec, _ := svc.Exec().Record(plan)
	   bindRes, _ := eng.BindLease(ctx, nil, trexecgate.BindInput{
		   PlanID: "BTC:" + exec.ID,
		   ExecID: exec.ID,
		   Asset:  "BTC",
		   Amount: "50",
	   })
	  leaseID := bindRes.Lease.LeaseID
	  // MarkSentWithGate（hot 未变，复核通过）。
	   gotExec, gotLease, err := las.MarkSentWithGate(ctx, nil, exec.ID, leaseID, "0xabcdef1234", exec.Fence.Epoch, exec.Fence.Token)
	   if err != nil {
		   t.Fatalf("MarkSentWithGate: %v", err)
	  }
	   if gotExec.Status != treasury.ExecSent {
		   t.Errorf("exec status: got %s, want sent", gotExec.Status)
	  }
	  if gotLease == nil || gotLease.State != trexecgate.LeaseSent {
		  t.Errorf("lease state: got %v, want sent", gotLease)
	   }
   }

// ──────────────────────────────────────────────
// MarkSentWithGate 集成：漂移拒绝场景
// ──────────────────────────────────────────────

   func TestLeaseAwareSvc_MarkSentWithGate_DriftReject(t *testing.T) {
	   ctx := context.Background()
	   inv := &mockHotInv{snaps: map[string]string{"ETH": "750"}}
	  leases := trexecgate.NewMemLeaseRepo()
	  proofs := trexecgate.NewMemDriftProofRepo()
	   gens := trexecgate.NewMemGenRepo()
	   eng := trexecgate.NewEngineWithObs(leases,
		   proofs, gens, inv, treasury.DefaultCatalog())
	   svc := treasury.New(hotinv.FromHotMax(map[string]string{"ETH": "800"}), nil)
	   las := trexecgate.NewLeaseAwareService(svc, eng)
	  plan := treasury.SweepPlan{Asset: "ETH", Amount: "500", From: "hot", To: "cold"}
	   exec, _ := svc.Exec().Record(plan)
	   bindRes, _ := eng.BindLease(ctx, nil, trexecgate.BindInput{
		  PlanID: "ETH:" + exec.ID,
		   ExecID: exec.ID,
		   Asset: "ETH",
		   Amount: "500",
	  })
	   leaseID := bindRes.Lease.LeaseID
	  // 压低热账到低于 band.Target=200。
	   inv.snaps["ETH"] = "100"
	   _, _, err := las.MarkSentWithGate(ctx, nil, exec.ID, leaseID, "0xabcdef1234", exec.Fence.Epoch, exec.Fence.Token)
	  if err == nil {
		  t.Fatal("MarkSentWithGate: expected error after drift, got nil")
	  }
	  if !errors.Is(err, trexecgate.ErrDriftExceeded) {
		   t.Errorf("expected ErrDriftExceeded, got: %v", err)
	   }
   }

// ──────────────────────────────────────────────
// AckWithLease 集成
// ──────────────────────────────────────────────

  func TestLeaseAwareSvc_AckWithLease(t *testing.T) {
	   ctx := context.Background()
	  inv := &mockHotInv{snaps: map[string]string{"BTC": "75"}}
	  leases := trexecgate.NewMemLeaseRepo()
	  proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	   eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog())
	   svc := treasury.New(hotinv.FromHotMax(map[string]string{"BTC": "80"}), nil)
	  las := trexecgate.NewLeaseAwareService(svc,
		   eng)
	  plan := treasury.SweepPlan{Asset: "BTC", Amount: "50", From: "hot", To: "cold"}
	   exec, _ := svc.Exec().Record(plan)
	  bindRes, _ := eng.BindLease(ctx, nil, trexecgate.BindInput{
		   PlanID: "BTC:" + exec.ID,
		  ExecID: exec.ID,
		   Asset:  "BTC",
		  Amount: "50",
	  })
	  leaseID := bindRes.Lease.LeaseID
	   // MarkSentWithGate 通过。
	   gotExec, gotLease, err := las.MarkSentWithGate(ctx, nil, exec.ID, leaseID, "0xabcdef1234", exec.Fence.Epoch, exec.Fence.Token)
	   if err != nil {
		  t.Fatalf("MarkSentWithGate: %v", err)
	  }
	   // AckWithLease。
	  ackedExec, ackedLease, err := las.AckWithLease(ctx, gotExec.ID, leaseID, "0xabcdef1234", gotExec.Fence.Epoch, gotExec.Fence.Token)
	  if err != nil {
		   t.Fatalf("AckWithLease: %v", err)
	  }
	  if ackedExec.Status != treasury.ExecAcked {
		   t.Errorf("exec acked status: got %s", ackedExec.Status)
	   }
	  _ = gotLease
	  if ackedLease == nil || ackedLease.State != trexecgate.LeaseAcked {
		  t.Errorf("lease acked state: got %v", ackedLease)
	   }
   }

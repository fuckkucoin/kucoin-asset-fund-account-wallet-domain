package inflightgengate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
	"account-assets.kucoin.net/funding/internal/inflightgengate"
)

// ─────────────────────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────────────────────

   func makeGate(t *testing.T) *inflightgengate.Gate {
	  t.Helper()
	  return inflightgengate.NewGate(
		   destlife.NewMemGenLeaseStore(),
		  destlife.NewMemRevokeGenStore(),
		   destlife.NewMemDriftCaseStore(),
		   destlife.NewMemProofStore(),
		  inflightgengate.GateConfig{
			   LeaseTTL: 4 * time.Hour,
			  Actor:    "test",
		   },
	   )
  }

// ─────────────────────────────────────────────────────────────────────────────
// 正常路径
// ─────────────────────────────────────────────────────────────────────────────

type normalPathCase struct {
	name    string
	entryID string
	userID  string
	chain      string
	address    string
	movementID string
}

   func TestNormalPath(t *testing.T) {
	  cases := []normalPathCase{
		  {
			   name: "usdt_eth_normal",
			  entryID: "entry-eth-001",
			  userID: "user-10001",
			  chain:  "ETH",
			   address:    "0xabcdef1234567890abcdef1234567890abcdef12",
			   movementID: "mov-00001",
		   },
		  {
			   name:    "btc_normal",
			   entryID: "entry-btc-001",
			  userID:  "user-10002",
			  chain:   "BTC",
			  address: "1A1zP1eP5QGefi2DMPTfTL5SLmv7Divf NA",
			  movementID: "mov-00002",
		  },
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  gate := makeGate(t)
			  ctx := context.Background()
			  // AcquireRef
			  acq, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
				   EntryID: tc.entryID,
				  UserID:  tc.userID,
				  Chain:   tc.chain,
				   Address:    tc.address,
				  MovementID: tc.movementID,
			  })
			  if err != nil {
				   t.Fatalf("AcquireRef: %v", err)
			   }
			  if acq.Idempotent {
				   t.Error("first Acquire must not be idempotent")
			   }
			   if acq.State != destlife.RefAcquired {
				  t.Errorf("expected acquired, got %s", acq.State)
			   }
			   // BindRef
			  bound, err := gate.BindRef(ctx, acq.LeaseID)
			  if err != nil {
				  t.Fatalf("BindRef: %v", err)
			   }
			  if bound.State != destlife.RefBound {
				  t.Errorf("expected bound, got %s", bound.State)
			  }
			   // VerifyBroadcast
			   vr, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
				  LeaseID:       acq.LeaseID,
				  MovementID:    tc.movementID,
				  BroadcastTxID: "tx-abc123",
			   })
			  if err != nil {
				   t.Fatalf("VerifyBroadcast: %v", err)
			   }
			   if !vr.OK {
				   t.Errorf("VerifyBroadcast: expected OK, reason=%s", vr.RejectReason)
			   }
			  // ReleaseRef
			  rel, err := gate.ReleaseRef(ctx, acq.LeaseID, "test-actor")
			   if err != nil {
				   t.Fatalf("ReleaseRef: %v", err)
			  }
			  if rel.State != destlife.RefReleased {
				  t.Errorf("expected released, got %s", rel.State)
			   }
		  })
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 幂等
// ─────────────────────────────────────────────────────────────────────────────

  func TestAcquireRefIdempotent(t *testing.T) {
	   gate := makeGate(t)
	   ctx := context.Background()
	  in := inflightgengate.AcquireRefInput{
		   EntryID: "entry-idem-001",
		   UserID:  "user-20001",
		  Chain:      "ETH",
		   Address:    "0xaaa",
		   MovementID: "mov-idem-001",
	  }
	  first, err := gate.AcquireRef(ctx, in)
	  if err != nil {
		   t.Fatalf("first AcquireRef: %v", err)
	  }
	  if first.Idempotent {
		   t.Error("first Acquire must not be idempotent")
	   }
	  second, err := gate.AcquireRef(ctx, in)
	   if err != nil {
		   t.Fatalf("second AcquireRef: %v", err)
	  }
	   if !second.Idempotent {
		   t.Error("second Acquire with same movement_id must be idempotent")
	  }
	   if second.LeaseID != first.LeaseID {
		  t.Errorf("idempotent Acquire must return same lease_id: got %s want %s", second.LeaseID, first.LeaseID)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 吊销与广播互斥
// ─────────────────────────────────────────────────────────────────────────────

   func TestBeginRevokingWithActiveRefDeferredAndBroadcastReject(t *testing.T) {
	  gate := makeGate(t)
	  ctx := context.Background()
	   entryID := "entry-revoke-001"
	  movID := "mov-revoke-001"
	  // AcquireRef
	   acq, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		  EntryID: entryID,
		   UserID: "user-30001",
		   Chain:  "ETH",
		  Address:    "0xbbb",
		   MovementID: movID,
	   })
	  if err != nil {
		   t.Fatalf("AcquireRef: %v", err)
	  }
	   // BindRef 必须在 BeginRevoking 之前调用，以捕获旧 gen（0）
	  // 这样 BeginRevoking 抬升 gen 后，BoundRevokeGen(0) != CurrentRevokeGen(1) → VerifyBroadcast 失败
	  _, err = gate.BindRef(ctx, acq.LeaseID)
	  if err != nil {
		  t.Fatalf("BindRef before BeginRevoking: %v", err)
	   }
	   // BeginRevoking：有活跃引用，应进入 revoking（Deferred=true），gen 抬升 0→1
	  rev, err := gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{
		  EntryID: entryID,
		   Actor:   "ops-admin",
	  })
	  if err != nil {
		  t.Fatalf("BeginRevoking: %v", err)
	   }
	   if !rev.Deferred {
		  t.Error("expected Deferred=true with active refs")
	  }
	  if rev.State != destlife.EntryRevoking {
		  t.Errorf("expected revoking, got %s", rev.State)
	  }
	  if rev.ActiveRefs != 1 {
		  t.Errorf("expected 1 active ref, got %d", rev.ActiveRefs)
	  }
	   // VerifyBroadcast：BoundRevokeGen(0) != CurrentRevokeGen(1)，应拒绝并产出 DriftCase
	   vr, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
		   LeaseID:    acq.LeaseID,
		  MovementID: movID,
	   })
	   if err == nil {
		  t.Error("expected VerifyBroadcast to fail after BeginRevoking")
	  }
	  if vr.OK {
		  t.Error("VerifyBroadcast.OK must be false after BeginRevoking with gen mismatch")
	  }
	  if !errors.Is(err, destlife.ErrStaleGen) && !errors.Is(err, destlife.ErrEntryRevoking) && !errors.Is(err, destlife.ErrEntryRevoked) {
		   t.Errorf("expected ErrStaleGen/ErrEntryRevoking/ErrEntryRevoked, got %v", err)
	  }
	   // 验证 DriftCase 已产出
	  dc, dcErr := gate.GetOpenDriftCase(ctx, entryID)
	   if dcErr != nil {
		   t.Fatalf("expected open drift case, got: %v", dcErr)
	   }
	   if dc.EntryID != entryID {
		  t.Errorf("drift case entry mismatch: got %s want %s", dc.EntryID, entryID)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// ReleaseRef 后旧 gen 广播失败
// ─────────────────────────────────────────────────────────────────────────────

   func TestBroadcastAfterReleaseReject(t *testing.T) {
	  gate := makeGate(t)
	  ctx := context.Background()
	   entryID := "entry-release-001"
	  movID := "mov-release-001"
	   acq, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		  EntryID: entryID,
		  UserID:  "user-40001",
		  Chain: "ETH",
		  Address:    "0xccc",
		  MovementID: movID,
	   })
	  if err != nil {
		  t.Fatalf("AcquireRef: %v", err)
	  }
	   _, err = gate.BindRef(ctx, acq.LeaseID)
	  if err != nil {
		   t.Fatalf("BindRef: %v", err)
	  }
	  // ReleaseRef
	  if _, err := gate.ReleaseRef(ctx, acq.LeaseID, "test"); err != nil {
		   t.Fatalf("ReleaseRef: %v", err)
	   }
	  // 持旧 lease 再次广播 → 必须失败
	  vr, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
		   LeaseID:    acq.LeaseID,
		  MovementID: movID,
	   })
	   if err == nil {
		  t.Error("expected VerifyBroadcast to fail after ReleaseRef")
	  }
	  if vr.OK {
		   t.Error("VerifyBroadcast.OK must be false after ReleaseRef")
	  }
	  if !errors.Is(err, destlife.ErrStaleGen) {
		  t.Errorf("expected ErrStaleGen, got %v", err)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 无活跃引用 BeginRevoking → ForceRevoke
// ─────────────────────────────────────────────────────────────────────────────

   func TestBeginRevokingNoActiveRefsForceRevoke(t *testing.T) {
	  gate := makeGate(t)
	   ctx := context.Background()
	   entryID := "entry-force-001"
	   // 先初始化条目（通过 GetOrInit）
	   _, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		   EntryID: entryID,
		  UserID: "user-50001",
		  Chain:      "BTC",
		   Address:    "1Force",
		  MovementID: "mov-force-init",
	   })
	  if err != nil {
		  t.Fatalf("AcquireRef init: %v", err)
	   }
	   // ReleaseRef，令活跃引用归零
	   ref, _ := gate.GetRefByMovement(ctx, entryID, "mov-force-init")
	   if _, err := gate.ReleaseRef(ctx, ref.LeaseID, "test"); err != nil {
		  t.Fatalf("ReleaseRef: %v", err)
	   }
	  // BeginRevoking：无活跃引用，直接 ForceRevoke
	   rev, err := gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{
		  EntryID: entryID,
		  Actor:   "ops",
	   })
	  if err != nil {
		  t.Fatalf("BeginRevoking: %v", err)
	  }
	  if rev.Deferred {
		  t.Error("expected Deferred=false when no active refs")
	  }
	  if rev.State != destlife.EntryRevoked {
		   t.Errorf("expected revoked, got %s", rev.State)
	  }
	  // 此后 AcquireRef 应失败
	   _, err = gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		   EntryID: entryID,
		  UserID:  "user-50001",
		   Chain:      "BTC",
		   Address:    "1Force",
		  MovementID: "mov-force-002",
	  })
	   if !errors.Is(err, destlife.ErrEntryRevoked) {
		  t.Errorf("expected ErrEntryRevoked after ForceRevoke, got %v", err)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// ReleaseRef 使 revoking 条目自动完成吊销
// ─────────────────────────────────────────────────────────────────────────────

   func TestReleaseRefCompletesRevoke(t *testing.T) {
	   gate := makeGate(t)
	   ctx := context.Background()
	  entryID := "entry-complete-001"
	  movID := "mov-complete-001"
	   acq, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		   EntryID: entryID,
		   UserID: "user-60001",
		  Chain:      "ETH",
		  Address:    "0xddd",
		   MovementID: movID,
	   })
	  if err != nil {
		   t.Fatalf("AcquireRef: %v", err)
	   }
	  // BeginRevoking，进入 revoking
	   rev, err := gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{
		   EntryID: entryID,
		  Actor:   "ops",
	   })
	   if err != nil {
		   t.Fatalf("BeginRevoking: %v", err)
	   }
	   if !rev.Deferred {
		   t.Error("expected Deferred=true")
	   }
	  // ReleaseRef：引用归零，条目自动 revoked
	   if _, err := gate.ReleaseRef(ctx, acq.LeaseID, "test"); err != nil {
		  t.Fatalf("ReleaseRef: %v", err)
	   }
	   // 验证条目已 revoked
	  rg, err := gate.GetRevokeGen(ctx, entryID)
	   if err != nil {
		   t.Fatalf("GetRevokeGen: %v", err)
	  }
	  if rg.State != destlife.EntryRevoked {
		  t.Errorf("expected revoked after all refs released, got %s", rg.State)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// DriftCase 阻断广播路径
// ─────────────────────────────────────────────────────────────────────────────

   func TestDriftCaseBlocksBroadcast(t *testing.T) {
	  gate := makeGate(t)
	   ctx := context.Background()
	  entryID := "entry-block-001"
	  movID1 := "mov-block-001"
	  movID2 := "mov-block-002"
	  // AcquireRef → BindRef（先绑定，捕获旧 gen）→ BeginRevoking（产出 DriftCase）
	  acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		   EntryID: entryID,
		  UserID:  "user-70001",
		   Chain:   "ETH",
		  Address: "0xeee",
		   MovementID: movID1,
	   })
	  // BindRef 在 BeginRevoking 之前，捕获 gen=0
	  _, _ = gate.BindRef(ctx, acq.LeaseID)
	  // BeginRevoking：gen 0→1
	   gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{ // nolint
		  EntryID: entryID,
		   Actor:   "ops",
	  })
	  // VerifyBroadcast：BoundRevokeGen(0) != CurrentRevokeGen(1) → 失败，产出 DriftCase
	   vr, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
		  LeaseID: acq.LeaseID,
		   MovementID: movID1,
	   })
	   if err == nil || vr.OK {
		  t.Fatalf("expected first VerifyBroadcast to fail and produce DriftCase")
	   }
	   // 验证 DriftCase 已产出
	   _, dcErr := gate.GetOpenDriftCase(ctx, entryID)
	  if dcErr != nil {
		  t.Fatalf("expected open drift case after first failed VerifyBroadcast: %v", dcErr)
	   }
	   // 新引用（不同 movement）尝试广播 → 被 DriftCase 阻断
	  acq2, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		  EntryID: entryID,
		   UserID:  "user-70001",
		  Chain: "ETH",
		  Address: "0xeee",
		   MovementID: movID2,
	   })
	  if err != nil {
		   // entryID 处于 revoking 或 revoked，Acquire 可能失败，属于正常拒绝
		   if errors.Is(err, destlife.ErrEntryRevoked) || errors.Is(err, destlife.ErrEntryRevoking) {
			  return
		  }
		   t.Fatalf("AcquireRef for second movement: %v", err)
	  }
	   _, _ = gate.BindRef(ctx, acq2.LeaseID)
	  _, err = gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
		  LeaseID:    acq2.LeaseID,
		  MovementID: movID2,
	   })
	   if err == nil {
		  t.Error("expected VerifyBroadcast to fail due to open DriftCase")
	   }
	  if !errors.Is(err, destlife.ErrDriftBlocked) &&
		   !errors.Is(err,
			   destlife.ErrStaleGen) &&
		  !errors.Is(err, destlife.ErrEntryRevoked) {
		   t.Errorf("expected ErrDriftBlocked/ErrStaleGen/ErrEntryRevoked, got %v", err)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// DriftCase 四眼收口
// ─────────────────────────────────────────────────────────────────────────────

  func TestDriftCaseClose(t *testing.T) {
	  gate := makeGate(t)
	   ctx := context.Background()
	  entryID := "entry-close-001"
	  movID := "mov-close-001"
	   acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		   EntryID: entryID,
		   UserID: "user-80001",
		  Chain:  "ETH",
		   Address:    "0xfff",
		  MovementID: movID,
	   })
	  // BindRef 在 BeginRevoking 之前，捕获 gen=0
	   _, _ = gate.BindRef(ctx, acq.LeaseID)
	   gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{ // nolint
		  EntryID: entryID,
		  Actor:   "ops",
	   })
	  // VerifyBroadcast 产出 DriftCase
	   gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{ // nolint
		   LeaseID:    acq.LeaseID,
		   MovementID: movID,
	   })
	   // 获取 open 案例
	   dc, err := gate.GetOpenDriftCase(ctx, entryID)
	   if err != nil {
		  t.Fatalf("GetOpenDriftCase: %v", err)
	   }
	  // 四眼收口
	  closed, err := gate.CloseDriftCase(ctx, dc.CaseID, "ops-admin", "confirmed: stale gen, address already revoked")
	   if err != nil {
		  t.Fatalf("CloseDriftCase: %v", err)
	   }
	  if closed.State != destlife.DriftCaseClosed {
		   t.Errorf("expected closed, got %s", closed.State)
	  }
	   if closed.ClosedBy != "ops-admin" {
		  t.Errorf("expected closed_by=ops-admin, got %s", closed.ClosedBy)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// VerifyBroadcast 证明幂等
// ─────────────────────────────────────────────────────────────────────────────

   func TestVerifyBroadcastProofIdempotent(t *testing.T) {
	   gate := makeGate(t)
	  ctx := context.Background()
	  entryID := "entry-proof-001"
	  movID := "mov-proof-001"
	   acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		   EntryID: entryID,
		   UserID: "user-90001",
		  Chain:      "ETH",
		   Address:    "0x123",
		  MovementID: movID,
	   })
	  gate.BindRef(ctx, acq.LeaseID) // nolint
	  // 第一次 VerifyBroadcast
	  vr1, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
		  LeaseID: acq.LeaseID,
		   MovementID:    movID,
		  BroadcastTxID: "tx-001",
	  })
	   if err != nil {
		   t.Fatalf("first VerifyBroadcast: %v", err)
	   }
	   if !vr1.OK {
		  t.Error("first VerifyBroadcast must succeed")
	  }
	   // 第二次 VerifyBroadcast（幂等证明复用）
	   vr2, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
		   LeaseID:       acq.LeaseID,
		  MovementID:    movID,
		   BroadcastTxID: "tx-001",
	  })
	  if err != nil {
		   t.Fatalf("second VerifyBroadcast (idempotent): %v", err)
	   }
	   if !vr2.OK {
		   t.Errorf("second VerifyBroadcast must also succeed (idempotent): reason=%s", vr2.RejectReason)
	  }
	   // 验证证明存在
	  proof, err := gate.GetProof(ctx, acq.LeaseID, movID)
	   if err != nil {
		  t.Fatalf("GetProof: %v", err)
	  }
	   if proof.LeaseID != acq.LeaseID {
		  t.Errorf("proof lease mismatch: got %s want %s", proof.LeaseID, acq.LeaseID)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 条目已 revoked 时 AcquireRef 拒绝
// ─────────────────────────────────────────────────────────────────────────────

   func TestAcquireRefOnRevokedEntryReject(t *testing.T) {
	   gate := makeGate(t)
	   ctx := context.Background()
	   entryID := "entry-revoked-002"
	  // 先 Acquire 再 Release 再 ForceRevoke
	   acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		  EntryID: entryID,
		  UserID:     "user-revoked",
		   Chain:      "ETH",
		  Address:    "0xrev",
		  MovementID: "mov-rev-init",
	  })
	  gate.ReleaseRef(ctx, acq.LeaseID, "test")                   // nolint
	  gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{ // nolint
		   EntryID: entryID,
		  Actor:   "ops",
	   })
	   _, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
		  EntryID: entryID,
		   UserID: "user-revoked",
		   Chain: "ETH",
		  Address:    "0xrev",
		   MovementID: "mov-rev-new",
	  })
	   if !errors.Is(err, destlife.ErrEntryRevoked) {
		  t.Errorf("expected ErrEntryRevoked, got %v", err)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 表驱动：BeginRevoking 场景矩阵
// ─────────────────────────────────────────────────────────────────────────────

type revokeCase struct {
	name      string
	setupRefs int
	wantDeferred bool
	wantState destlife.EntryState
}

  func TestBeginRevokingMatrix(t *testing.T) {
	   cases := []revokeCase{
		  {name: "no_refs_force_revoke", setupRefs: 0, wantDeferred: false, wantState: destlife.EntryRevoked},
		   {name: "one_ref_deferred", setupRefs: 1, wantDeferred: true, wantState: destlife.EntryRevoking},
		   {name: "three_refs_deferred", setupRefs: 3, wantDeferred: true, wantState: destlife.EntryRevoking},
	   }
	   for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   gate := makeGate(t)
			  ctx := context.Background()
			   entryID := "entry-matrix-" + tc.name
			   var leaseIDs []string
			  for i := 0; i < tc.setupRefs; i++ {
				   acq, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
					  EntryID: entryID,
					  UserID:  "user-matrix",
					  Chain:   "ETH",
					   Address:    "0xmatrix",
					  MovementID: "mov-matrix-" + tc.name + "-" + string(rune('a'+i)),
				  })
				  if err != nil {
					   t.Fatalf("AcquireRef %d: %v", i, err)
				   }
				   leaseIDs = append(leaseIDs, acq.LeaseID)
			  }
			  _ = leaseIDs
			  rev, err := gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{
				  EntryID: entryID,
				  UserID: "user-matrix",
				  Chain:   "ETH",
				   Address: "0xmatrix",
				  Actor: "ops",
			   })
			  if err != nil {
				   t.Fatalf("BeginRevoking: %v", err)
			  }
			  if rev.Deferred != tc.wantDeferred {
				   t.Errorf("Deferred: got %v want %v", rev.Deferred, tc.wantDeferred)
			  }
			  if rev.State != tc.wantState {
				   t.Errorf("State: got %s want %s", rev.State, tc.wantState)
			  }
		  })
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 表驱动：VerifyBroadcast 场景矩阵
// ─────────────────────────────────────────────────────────────────────────────

type broadcastCase struct {
	name string
	setup      func(t *testing.T, gate *inflightgengate.Gate, ctx context.Context) string // 返回 leaseID
	movementID string
	wantOK     bool
	wantErr    error
}

  func TestVerifyBroadcastMatrix(t *testing.T) {
	  cases := []broadcastCase{
		   {
			   name: "normal_bound",
			   setup: func(t *testing.T, gate *inflightgengate.Gate, ctx context.Context) string {
				  acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
					  EntryID: "e-bcast-ok", UserID: "u1", Chain: "ETH", Address: "0x1", MovementID: "m-bcast-ok",
				  })
				  gate.BindRef(ctx, acq.LeaseID) // nolint
				  return acq.LeaseID
			  },
			  movementID: "m-bcast-ok",
			   wantOK: true,
		   },
		   {
			   name: "released_lease",
			   setup: func(t *testing.T, gate *inflightgengate.Gate, ctx context.Context) string {
				   acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
					   EntryID: "e-bcast-rel", UserID: "u1", Chain: "ETH", Address: "0x2", MovementID: "m-bcast-rel",
				  })
				   gate.BindRef(ctx, acq.LeaseID)         // nolint
				  gate.ReleaseRef(ctx, acq.LeaseID, "t") // nolint
				  return acq.LeaseID
			   },
			   movementID: "m-bcast-rel",
			   wantOK:     false,
			   wantErr:    destlife.ErrStaleGen,
		  },
		   {
			   name: "revoked_entry",
			  setup: func(t *testing.T, gate *inflightgengate.Gate, ctx context.Context) string {
				  acq, _ := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
					  EntryID: "e-bcast-rev", UserID: "u1", Chain: "ETH", Address: "0x3", MovementID: "m-bcast-rev",
				   })
				   gate.BindRef(ctx, acq.LeaseID) // nolint
				  gate.ReleaseRef(ctx, acq.LeaseID, "t") // nolint
				   gate.BeginRevoking(ctx, inflightgengate.BeginRevokingInput{EntryID: "e-bcast-rev", Actor: "ops"}) // nolint
				   // 再签发一个新引用，但条目已 revoked
				  acq2, err := gate.AcquireRef(ctx, inflightgengate.AcquireRefInput{
					  EntryID: "e-bcast-rev", UserID: "u1", Chain: "ETH", Address: "0x3", MovementID: "m-bcast-rev2",
				  })
				   if err != nil {
					  return ""
				   }
				  gate.BindRef(ctx, acq2.LeaseID) // nolint
				  return acq2.LeaseID
			   },
			  movementID: "m-bcast-rev2",
			  wantOK:     false,
			  wantErr: destlife.ErrEntryRevoked,
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   gate := makeGate(t)
			   ctx := context.Background()
			   leaseID := tc.setup(t, gate, ctx)
			   if leaseID == "" {
				  t.Skip("setup returned empty leaseID (entry already revoked)")
				   return
			  }
			   vr, err := gate.VerifyBroadcast(ctx, inflightgengate.VerifyBroadcastInput{
				   LeaseID:    leaseID,
				   MovementID: tc.movementID,
			  })
			   if tc.wantOK {
				  if err != nil {
					  t.Errorf("expected no error, got %v", err)
				   }
				  if !vr.OK {
					   t.Errorf("expected OK=true, reason=%s", vr.RejectReason)
				   }
			  } else {
				   if err == nil {
					  t.Error("expected error, got nil")
				  }
				  if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					  t.Errorf("expected %v, got %v", tc.wantErr, err)
				   }
			   }
		   })
	  }
   }
// temporary guard removed after the production check

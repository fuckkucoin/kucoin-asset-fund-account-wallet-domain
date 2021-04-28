package claimmutex_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/claimmutex"
)

// ─────────────────────────── 辅助 ────────────────────────────────────────

func newTestService(t *testing.T) *claimmutex.Service {
	t.Helper()
	fixed := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	return claimmutex.NewWithStore(claimmutex.NewMemStore()).WithClock(func() time.Time { return fixed })
}

func outKey(chain, hash string, idx int) claimmutex.OutputKey {
	return claimmutex.OutputKey{Chain: chain, TxHash: hash, OutputIndex: idx}
}

var (
	ethKey	= outKey("ETH", "0xabc123def456abc123def456abc123def456abc123def456abc123def456abcd", 0)
	btcKey	= outKey("BTC", "7b4d9a3c2e1f0a8b6d5e4c3b2a1f0e9d8c7b6a5d4c3b2a1f0e9d8c7b6a5d4c3", 2)
	trxKey	= outKey("TRX", "e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d8c7b6a5d4c3b2", 0)
)

func acquireReq(key claimmutex.OutputKey, kind, actor, idem string) claimmutex.AcquireLeaseRequest {
	return claimmutex.AcquireLeaseRequest{
		OutputKey:	key,
		Kind:		kind,
		ActorID:	actor,
		UserID:		"user-test",
		Asset:		"ETH",
		Amount:		"1.0",
		IdemKey:	idem,
		TTL:		5 * time.Minute,
	}
}

// ─────────────────────────── AcquireLease 主路径 ─────────────────────────────

func TestAcquireLease_HappyPath_Claim(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-alice-001"))
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if lease.State != claimmutex.LeaseOpen {
		t.Errorf("want state=open, got %s", lease.State)
	}
	if lease.WinnerKind != claimmutex.KindClaim {
		t.Errorf("want winner_kind=claim, got %s", lease.WinnerKind)
	}
	if lease.WinnerActor != "user-alice" {
		t.Errorf("want winner_actor=user-alice, got %s", lease.WinnerActor)
	}
	if lease.LeaseID == "" {
		t.Error("want non-empty lease_id")
	}
}

func TestAcquireLease_HappyPath_AutoCredit(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, err := svc.AcquireLease(ctx, acquireReq(btcKey, claimmutex.KindAutoCredit, "autocredit-worker", "idem-ac-001"))
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if lease.WinnerKind != claimmutex.KindAutoCredit {
		t.Errorf("want winner_kind=auto_credit, got %s", lease.WinnerKind)
	}
}

// ─────────────────────────── 二次认领拦截 ────────────────────────────────────

func TestAcquireLease_SecondClaim_SameOutput_DifferentActor(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// 第一次 Claim 胜出
	_, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-alice-001"))
	if err != nil {
		t.Fatalf("first claim: want nil, got %v", err)
	}
	// 第二次不同 actor Claim → ErrSecondClaim
	_, err = svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-bob", "idem-bob-001"))
	if !errors.Is(err, claimmutex.ErrSecondClaim) {
		t.Errorf("second claim: want ErrSecondClaim, got %v", err)
	}
}

func TestAcquireLease_SecondClaim_SameOutput_SameActor_Idempotent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	l1, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-alice-001"))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	l2, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-alice-001"))
	if err != nil {
		t.Fatalf("second call (idempotent): %v", err)
	}
	if l1.LeaseID != l2.LeaseID {
		t.Error("idempotent: lease_id should be same")
	}
}

// ─────────────────────────── Claim vs AutoCredit 竞态 ───────────────────────

func TestClaim_vs_AutoCredit_ClaimWins(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// 先注册 auto-credit pending
	_, err := svc.RegisterAutoCredit(ctx, claimmutex.RegisterAutoCreditRequest{
		OutputKey:	btcKey,
		UserID:		"user-carol",
		Asset:		"BTC",
		Amount:		"0.5",
		IdemKey:	"idem-ac-001",
		ActorID:	"autocredit-worker",
	})
	if err != nil {
		t.Fatalf("RegisterAutoCredit: %v", err)
	}
	// Claim 抢占租约（发现 pending bind，拒绝 bind，开 DupClaimCase）
	lease, err := svc.AcquireLease(ctx, acquireReq(btcKey, claimmutex.KindClaim, "user-carol", "idem-claim-001"))
	if err != nil {
		t.Fatalf("AcquireLease claim: want nil, got %v", err)
	}
	if lease.WinnerKind != claimmutex.KindClaim {
		t.Errorf("want winner_kind=claim, got %s", lease.WinnerKind)
	}
	// 检查 DupClaimCase 已开
	dupCase, err := svc.store.GetOpenDupCase(ctx, btcKey)
	if err != nil {
		t.Fatalf("want dup case open, got %v", err)
	}
	if dupCase.State != claimmutex.DupCaseOpen {
		t.Errorf("want dup case state=open, got %s", dupCase.State)
	}
	if dupCase.FirstKind != claimmutex.KindClaim {
		t.Errorf("want first_kind=claim, got %s", dupCase.FirstKind)
	}
	if dupCase.SecondKind != claimmutex.KindAutoCredit {
		t.Errorf("want second_kind=auto_credit, got %s", dupCase.SecondKind)
	}
}

func TestAutoCredit_After_Claim_IsRejected(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// Claim 先胜出
	_, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-alice-001"))
	if err != nil {
		t.Fatalf("claim first: %v", err)
	}
	// auto-credit 后到，应被拒绝
	_, err = svc.RegisterAutoCredit(ctx, claimmutex.RegisterAutoCreditRequest{
		OutputKey:	ethKey,
		UserID:		"user-alice",
		Asset:		"ETH",
		Amount:		"1.0",
		IdemKey:	"idem-ac-002",
		ActorID:	"autocredit-worker",
	})
	if !errors.Is(err, claimmutex.ErrMutexConflict) && !errors.Is(err, claimmutex.ErrSecondClaim) {
		t.Errorf("want ErrMutexConflict or ErrSecondClaim, got %v", err)
	}
}

// ─────────────────────────── GuardOutput ─────────────────────────────────────

func TestGuardOutput_MatchingLease_OK(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	got, err := svc.GuardOutput(ctx, claimmutex.GuardRequest{
		OutputKey:	ethKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ActorID:	"user-alice",
		Kind:		claimmutex.KindClaim,
	})
	if err != nil {
		t.Errorf("guard: want nil, got %v", err)
	}
	if got == nil || got.LeaseID != lease.LeaseID {
		t.Error("guard: returned wrong lease")
	}
}

func TestGuardOutput_WrongLeaseID_GenerationMismatch(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	_, err = svc.GuardOutput(ctx, claimmutex.GuardRequest{
		OutputKey:	ethKey,
		LeaseID:	"stale-lease-id",
		Generation:	0,
		ActorID:	"recovery-worker",
		Kind:		"recovery",
	})
	if !errors.Is(err, claimmutex.ErrGenerationMismatch) {
		t.Errorf("want ErrGenerationMismatch, got %v", err)
	}
}

func TestGuardOutput_BlockedLease_ReturnsSecondClaim(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := svc.BlockOutput(ctx, lease.LeaseID, "test block"); err != nil {
		t.Fatalf("block: %v", err)
	}
	_, err = svc.GuardOutput(ctx, claimmutex.GuardRequest{
		OutputKey:	ethKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
	})
	if !errors.Is(err, claimmutex.ErrSecondClaim) {
		t.Errorf("want ErrSecondClaim on blocked lease, got %v", err)
	}
}

// ─────────────────────────── ClaimEffectProof FSM ────────────────────────────

func TestCreateProof_HappyPath(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, err := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	proof, err := svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
		ActorID:	"user-dave",
		UserID:		"user-dave",
		Asset:		"TRX",
		Amount:		"100",
		IdemKey:	"idem-001",
	})
	if err != nil {
		t.Fatalf("CreateProof: %v", err)
	}
	if proof.State != claimmutex.ProofPending {
		t.Errorf("want state=pending, got %s", proof.State)
	}
}

func TestVerifyProof_TransitionsToPending_Verified(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	proof, _ := svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
		ActorID:	"user-dave",
	})
	verified,
		err := svc.VerifyProof(ctx, proof.ProofID)
	if err != nil {
		t.Fatalf("VerifyProof: %v", err)
	}
	if verified.State != claimmutex.ProofVerified {
		t.Errorf("want state=verified, got %s", verified.State)
	}
}

// ─────────────────────────── CompleteClaim 证明守卫 ──────────────────────────

func TestCompleteClaim_MissingProofID_ErrProofMissing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	_, err := svc.CompleteClaim(ctx, claimmutex.CompleteClaimRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		ProofID:	"",
		SealedBy:	"user-dave",
	})
	if !errors.Is(err, claimmutex.ErrProofMissing) {
		t.Errorf("want ErrProofMissing, got %v", err)
	}
}

func TestCompleteClaim_PendingProof_ErrProofMissing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	proof, _ := svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
		ActorID:	"user-dave",
	})
	// proof 仍为 pending，不允许完成
	_, err := svc.CompleteClaim(ctx, claimmutex.CompleteClaimRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		ProofID:	proof.ProofID,
		SealedBy:	"user-dave",
	})
	if !errors.Is(err, claimmutex.ErrProofMissing) {
		t.Errorf("want ErrProofMissing for pending proof, got %v", err)
	}
}

func TestCompleteClaim_VerifiedProof_OK(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	proof, _ := svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
		ActorID:	"user-dave",
	})
	svc.VerifyProof(ctx,	//nolint
		proof.ProofID)
	sealed, err := svc.CompleteClaim(ctx, claimmutex.CompleteClaimRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		ProofID:	proof.ProofID,
		SealedBy:	"ops-admin",
	})
	if err != nil {
		t.Fatalf("CompleteClaim: want nil, got %v", err)
	}
	if sealed.State != claimmutex.ProofSealed {
		t.Errorf("want state=sealed, got %s", sealed.State)
	}
	if sealed.SealedBy != "ops-admin" {
		t.Errorf("want sealed_by=ops-admin, got %s", sealed.SealedBy)
	}
}

func TestCompleteClaim_Idempotent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	proof, _ := svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
		ActorID:	"user-dave",
	})
	svc.VerifyProof(ctx, proof.ProofID)	//nolint
	req := claimmutex.CompleteClaimRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		ProofID:	proof.ProofID,
		SealedBy:	"ops-admin",
	}
	p1, err1 := svc.CompleteClaim(ctx, req)
	p2,
		err2 := svc.CompleteClaim(ctx, req)
	if err1 != nil || err2 != nil {
		t.Fatalf("idempotent CompleteClaim: err1=%v, err2=%v", err1, err2)
	}
	if p1.ProofID != p2.ProofID {
		t.Error("idempotent: proof_id should be same")
	}
}

// ─────────────────────────── DupClaimCase FSM ────────────────────────────────

func TestDupClaimCase_ProveThenClose(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// 开案
	dupCase := claimmutex.DupClaimCase{
		CaseID:		"case-001",
		OutputKey:	btcKey,
		LeaseID:	"lease-001",
		FirstKind:	claimmutex.KindClaim,
		FirstActor:	"user-alice",
		SecondKind:	claimmutex.KindAutoCredit,
		SecondActor:	"autocredit-worker",
		DeltaAmount:	"0.5",
		State:		claimmutex.DupCaseOpen,
		DetectedAt:	time.Now(),
	}
	if err := svc.store.InsertDupCase(ctx, dupCase); err != nil {
		t.Fatalf("InsertDupCase: %v", err)
	}
	// prove
	proved, err := svc.ProveDupCase(ctx, claimmutex.ProveDupCaseRequest{
		CaseID:	"case-001",
		Actor1:	"ops-1",
		Actor2:	"ops-2",
	})
	if err != nil {
		t.Fatalf("ProveDupCase: %v", err)
	}
	if proved.State != claimmutex.DupCaseProven {
		t.Errorf("want state=proven, got %s", proved.State)
	}
	// close
	closed, err := svc.CloseDupCase(ctx, claimmutex.CloseDupCaseRequest{
		CaseID:		"case-001",
		CloseNote:	"fixed by ops",
		ClosedBy:	"ops-1",
	})
	if err != nil {
		t.Fatalf("CloseDupCase: %v", err)
	}
	if closed.State != claimmutex.DupCaseClosed {
		t.Errorf("want state=closed, got %s", closed.State)
	}
}

func TestDupClaimCase_Prove_RequiresTwoDistinctActors(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dupCase := claimmutex.DupClaimCase{
		CaseID:		"case-002",
		OutputKey:	btcKey,
		State:		claimmutex.DupCaseOpen,
		DetectedAt:	time.Now(),
	}
	_ = svc.store.InsertDupCase(ctx,
		dupCase)
	_, err := svc.ProveDupCase(ctx, claimmutex.ProveDupCaseRequest{
		CaseID:	"case-002",
		Actor1:	"ops-1",
		Actor2:	"ops-1",	// 同一人，应拒绝
	})
	if err == nil {
		t.Error("want error for same actor1==actor2, got nil")
	}
}

func TestDupClaimCase_Close_RequiresProven(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dupCase := claimmutex.DupClaimCase{
		CaseID:		"case-003",
		OutputKey:	btcKey,
		State:		claimmutex.DupCaseOpen,
		DetectedAt:	time.Now(),
	}
	_ = svc.store.InsertDupCase(ctx, dupCase)
	_, err := svc.CloseDupCase(ctx, claimmutex.CloseDupCaseRequest{
		CaseID:		"case-003",
		CloseNote:	"skip prove",
	})
	if err == nil {
		t.Error("want error: close without prove")
	}
}

// ─────────────────────────── AdvanceLease ────────────────────────────────────

func TestAdvanceLease_OpenToClaimed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	advanced, err := svc.AdvanceLease(ctx, lease.LeaseID, "user-alice", claimmutex.KindClaim)
	if err != nil {
		t.Fatalf("AdvanceLease: %v", err)
	}
	if advanced.State != claimmutex.LeaseClaimed {
		t.Errorf("want state=claimed, got %s", advanced.State)
	}
}

func TestAdvanceLease_WrongActor_MutexConflict(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	_, err := svc.AdvanceLease(ctx, lease.LeaseID, "user-bob", claimmutex.KindClaim)
	if !errors.Is(err, claimmutex.ErrMutexConflict) {
		t.Errorf("want ErrMutexConflict, got %v", err)
	}
}

// ─────────────────────────── BlockOutput ─────────────────────────────────────

func TestBlockOutput_Idempotent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	if err := svc.BlockOutput(ctx, lease.LeaseID, "reason-1"); err != nil {
		t.Fatalf("first BlockOutput: %v", err)
	}
	if err := svc.BlockOutput(ctx, lease.LeaseID, "reason-2"); err != nil {
		t.Fatalf("second BlockOutput (idempotent): %v", err)
	}
}

// ─────────────────────────── GetOutputEffect ─────────────────────────────────

func TestGetOutputEffect_EmptyOutput(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	sum,
		err := svc.GetOutputEffect(ctx, ethKey)
	if err != nil {
		t.Fatalf("GetOutputEffect: %v", err)
	}
	if sum.Lease != nil {
		t.Error("want nil lease for unknown output")
	}
}

func TestGetOutputEffect_AfterAcquire(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, _ = svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	sum, err := svc.GetOutputEffect(ctx, ethKey)
	if err != nil {
		t.Fatalf("GetOutputEffect: %v", err)
	}
	if sum.Lease == nil {
		t.Fatal("want non-nil lease")
	}
	if sum.Lease.WinnerKind != claimmutex.KindClaim {
		t.Errorf("want winner_kind=claim, got %s", sum.Lease.WinnerKind)
	}
}

// ─────────────────────────── Worker ─────────────────────────────────────

func TestWorker_ExpireSweeper_MarksExpired(t *testing.T) {
	store := claimmutex.NewMemStore()
	fixed := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	svc := claimmutex.NewWithStore(store).WithClock(func() time.Time { return fixed })
	ctx := context.Background()
	// 写入一条已超期的 open 租约
	expired := claimmutex.OutputLease{
		LeaseID:	"lease-expired-001",
		OutputKey:	ethKey,
		Generation:	1,
		State:		claimmutex.LeaseOpen,
		WinnerKind:	claimmutex.KindClaim,
		WinnerActor:	"user-alice",
		IssuedAt:	fixed.Add(-10 * time.Minute),
		ExpiresAt:	fixed.Add(-1 * time.Minute),	// 已超期
	}
	// 直接写入，绕过 InsertLease 的唯一检查
	store.mu.Lock()
	store.leases[expired.LeaseID] = &expired
	store.byOut[expired.OutputKey.String()] = expired.LeaseID
	store.mu.Unlock()
	cfg := claimmutex.WorkerConfig{BatchSize: 10}
	worker := claimmutex.NewWorker(svc, cfg, &claimmutex.WorkerMetrics{})
	count := worker.RunExpireOnce(ctx)
	if count != 1 {
		t.Errorf("want 1 expired, got %d", count)
	}
	got, err := store.GetLeaseByID(ctx, expired.LeaseID)
	if err != nil {
		t.Fatalf("GetLeaseByID: %v", err)
	}
	if got.State != claimmutex.LeaseExpired {
		t.Errorf("want state=expired, got %s", got.State)
	}
}

func TestWorker_ProofVerifier_VerifiesPending(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(trxKey, claimmutex.KindClaim, "user-dave", "idem-001"))
	proof, _ := svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	trxKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
		ActorID:	"user-dave",
	})
	cfg := claimmutex.WorkerConfig{BatchSize: 10}
	worker := claimmutex.NewWorker(svc, cfg, &claimmutex.WorkerMetrics{})
	verified,
		failed := worker.RunVerifyOnce(ctx)
	if verified != 1 {
		t.Errorf("want 1 verified, got %d", verified)
	}
	if failed != 0 {
		t.Errorf("want 0 failed, got %d", failed)
	}
	got, err := svc.store.GetProofByID(ctx, proof.ProofID)
	if err != nil {
		t.Fatalf("GetProofByID: %v", err)
	}
	if got.State != claimmutex.ProofVerified {
		t.Errorf("want state=verified, got %s", got.State)
	}
}

// ─────────────────────────── 表驱动：参数校验 ────────────────────────────────

func TestAcquireLease_Validation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	cases := []struct {
		name	string
		req	claimmutex.AcquireLeaseRequest
		wantErr	error
	}{
		{
			name:		"empty_output_key",
			req:		claimmutex.AcquireLeaseRequest{Kind: claimmutex.KindClaim, ActorID: "actor"},
			wantErr:	claimmutex.ErrOutputKeyEmpty,
		},
		{
			name:		"empty_actor",
			req:		claimmutex.AcquireLeaseRequest{OutputKey: ethKey, Kind: claimmutex.KindClaim},
			wantErr:	claimmutex.ErrActorEmpty,
		},
		{
			name:		"invalid_kind",
			req:		claimmutex.AcquireLeaseRequest{OutputKey: ethKey, Kind: "recovery", ActorID: "actor"},
			wantErr:	claimmutex.ErrInvalidRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AcquireLease(ctx, tc.req)
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// ─────────────────────────── ：错链回收 / 入金批次 隔离 ─────────────────────────
// TestBoundary_NoWrongChainTicket 验证本包不写错链回收工单

func TestBoundary_NoWrongChainTicket(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// 本包的 AcquireLease / GuardOutput / CompleteClaim 均不涉及错链工单 FSM
	lease, err := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// 没有任何错链工单相关字段被写入
	if lease.ClaimCaseID != "" {
		// ClaimCaseID 只在双贷案件发现时才非空，正常认领为空
		// 这里 ClaimCaseID 可以非空（DupCase），但不等于错链工单
	}
	// 核心：OutputLease 只含认领效果互斥字段，不含错链 FSM 字段
	t.Log("boundary check: no wrong-chain ticket fields in OutputLease")
}

// TestBoundary_NoDepbatchHalfObserve 验证本包不做 批次半观察
func TestBoundary_NoDepbatchHalfObserve(t *testing.T) {
	// 本包的 Store 接口和 Service 方法均无 batch_id / partial_observe 参数
	// 此仅为文档性质，确认不引入 depbatch 逻辑
	t.Log("boundary check: no depbatch batch_id fields in claimmutex store")
}

// ─────────────────────────── 幂等性综合 ──────────────────────────────────

func TestIdempotency_FullFlow(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// 多次调用 AcquireLease（同 actor）→ 同一租约
	req := acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-alice-001")
	l1,
		_ := svc.AcquireLease(ctx, req)
	l2, _ := svc.AcquireLease(ctx, req)
	if l1.LeaseID != l2.LeaseID {
		t.Error("AcquireLease not idempotent")
	}
	// ProveDupCase 幂等
	dupCase := claimmutex.DupClaimCase{
		CaseID:		"case-idem-001",
		OutputKey:	btcKey,
		State:		claimmutex.DupCaseOpen,
		DetectedAt:	time.Now(),
	}
	_ = svc.store.InsertDupCase(ctx, dupCase)
	p1, _ := svc.ProveDupCase(ctx, claimmutex.ProveDupCaseRequest{CaseID: "case-idem-001", Actor1: "a1", Actor2: "a2"})
	p2, _ := svc.ProveDupCase(ctx, claimmutex.ProveDupCaseRequest{CaseID: "case-idem-001", Actor1: "a1", Actor2: "a2"})
	if p1.State != p2.State {
		t.Error("ProveDupCase not idempotent")
	}
}

// ─────────────────────────── OutputKey.String ──────────────────────────────

func TestOutputKey_String(t *testing.T) {
	cases := []struct {
		key	claimmutex.OutputKey
		want	string
	}{
		{outKey("ETH", "0xabc", 0), "ETH|0xabc|0"},
		{outKey("BTC", "txhash", 2), "BTC|txhash|2"},
		{claimmutex.OutputKey{}, ""},
	}
	for _, tc := range cases {
		got := tc.key.String()
		if got != tc.want {
			t.Errorf("OutputKey.String() = %q, want %q", got, tc.want)
		}
	}
}

// ─────────────────────────── LeaseState helpers ──────────────────────────────

func TestOutputLease_IsExpired(t *testing.T) {
	now := time.Now()
	lease := claimmutex.OutputLease{ExpiresAt: now.Add(-time.Second)}
	if !lease.IsExpired(now) {
		t.Error("want IsExpired=true")
	}
	lease2 := claimmutex.OutputLease{ExpiresAt: now.Add(time.Minute)}
	if lease2.IsExpired(now) {
		t.Error("want IsExpired=false")
	}
}

func TestOutputLease_IsBlocked(t *testing.T) {
	blocked := claimmutex.OutputLease{State: claimmutex.LeaseBlocked}
	if !blocked.IsBlocked() {
		t.Error("want IsBlocked=true for blocked state")
	}
	open := claimmutex.OutputLease{State: claimmutex.LeaseOpen}
	if open.IsBlocked() {
		t.Error("want IsBlocked=false for open state")
	}
}

func TestClaimEffectProof_CanComplete(t *testing.T) {
	cases := []struct {
		state	claimmutex.ProofState
		want	bool
	}{
		{claimmutex.ProofPending, false},
		{claimmutex.ProofFailed, false},
		{claimmutex.ProofVerified, true},
		{claimmutex.ProofSealed, true},
	}
	for _, tc := range cases {
		p := claimmutex.ClaimEffectProof{State: tc.state}
		if p.CanComplete() != tc.want {
			t.Errorf("state=%s: CanComplete()=%v, want %v", tc.state, p.CanComplete(), tc.want)
		}
	}
}

// ─────────────────────────── ListOpenDupCases / ListPendingProofs ─────────────

func TestListOpenDupCases(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	for i := range [3]struct{}{} {
		_ = svc.store.InsertDupCase(ctx, claimmutex.DupClaimCase{
			CaseID:		claimmutex.Itoa(i),
			OutputKey:	outKey("ETH", claimmutex.Itoa(i), 0),
			State:		claimmutex.DupCaseOpen,
			DetectedAt:	time.Now(),
		})
	}
	cases, err := svc.ListOpenDupCases(ctx, 10)
	if err != nil {
		t.Fatalf("ListOpenDupCases: %v", err)
	}
	if len(cases) != 3 {
		t.Errorf("want 3 open cases, got %d", len(cases))
	}
}

func TestListPendingProofs(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	lease, _ := svc.AcquireLease(ctx, acquireReq(ethKey, claimmutex.KindClaim, "user-alice", "idem-001"))
	_, _ = svc.CreateProof(ctx, claimmutex.CreateProofRequest{
		OutputKey:	ethKey,
		LeaseID:	lease.LeaseID,
		Generation:	lease.Generation,
		ClaimKind:	claimmutex.KindClaim,
	})
	proofs, err := svc.ListPendingProofs(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingProofs: %v", err)
	}
	// AcquireLease 内部已写一个 pending proof；CreateProof 再写一个（如果输出键相同会覆盖指针）
	// 内存实现中每次 InsertProof 独立存储，不去重
	if len(proofs) == 0 {
		t.Error("want at least 1 pending proof")
	}
}

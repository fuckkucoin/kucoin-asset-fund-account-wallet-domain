package fundintgate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fundint"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fundintgate"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fundintgate")
}

func loadGolden(t *testing.T, name string) map[string]any {
	t.Helper()
	b,
		err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func newTestSvc(sender *fundintgate.RecordingSender,) *fundintgate.Service {
	if sender == nil {
		sender = &fundintgate.RecordingSender{ID: "xfer-1"}
	}
	return fundintgate.New(fundintgate.StaticResolver{ToUserID: "u-to", Channel: "uid"}, fundintgate.StaticGate{}, sender,
	)
}

func issueOK(t *testing.T, svc *fundintgate.Service,) fundintgate.IntGateLease {
	t.Helper()
	lease, err := svc.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
		UserID:	"u-from", ToUID: "u-to", Asset: "USDT", Amount: "10",
		IdempotencyKey:	"idem-" + time.Now().Format("150405.000"),
	})
	if err != nil {
		t.Fatalf("CheckWithLease: %v", err)
	}
	if lease.State != fundintgate.LeaseGated {
		t.Fatalf("state=%s", lease.State)
	}
	if lease.LeaseID == "" {
		t.Fatal("empty lease id")
	}
	return lease
}

func TestFSMTable(t *testing.T) {
	cases := []struct {
		from	fundintgate.LeaseState
		to	fundintgate.LeaseState
		ok	bool
	}{
		{fundintgate.LeaseGated, fundintgate.LeaseBound, true},
		{fundintgate.LeaseGated, fundintgate.LeaseExpired, true},
		{fundintgate.LeaseGated, fundintgate.LeaseRevoked, true},
		{fundintgate.LeaseGated, fundintgate.LeaseSent, false},
		{fundintgate.LeaseBound, fundintgate.LeaseSent, true},
		{fundintgate.LeaseBound, fundintgate.LeaseExpired, true},
		{fundintgate.LeaseBound, fundintgate.LeaseRevoked, true},
		{fundintgate.LeaseSent, fundintgate.LeaseRevoked, false},
		{fundintgate.LeaseExpired, fundintgate.LeaseGated, false},
		{fundintgate.LeaseRevoked, fundintgate.LeaseBound, false},
	}
	for _, tc := range cases {
		got := fundintgate.CanTransit(tc.from, tc.to)
		if got != tc.ok {
			t.Fatalf("%s→%s got %v want %v", tc.from, tc.to, got, tc.ok)
		}
	}
	if _, ok := fundintgate.TransitionTable()[fundintgate.LeaseGated]; !ok {
		t.Fatal("transition table missing gated")
	}
}

func TestCheckThenHaltThenSend_Golden(t *testing.T) {
	_ = loadGolden(t, "golden_halt_after_pass.json")
	sender := &fundintgate.RecordingSender{ID: "xfer-halt"}
	svc := newTestSvc(sender)
	lease := issueOK(t, svc)
	if _, n, err := svc.HaltGate(context.Background(), "ops", "drill"); err != nil {
		t.Fatal(err)
	} else if n < 1 {
		t.Fatalf("revoked=%d", n)
	}
	_, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease.LeaseID, UserID: "u-from", ToUID: "u-to",
		Asset:	"USDT", Amount: "10", IdempotencyKey: "send-halt",
	})
	if !errors.Is(err, fundintgate.ErrGateHalted) && !errors.Is(err, fundintgate.ErrLeaseRevoked) {
		t.Fatalf("err=%v", err)
	}
	if sender.Calls != 0 {
		t.Fatalf("spot send calls=%d", sender.Calls)
	}
	proofs, err := svc.store.ListProofs(context.Background(), fundintgate.ExportProofFilter{LeaseID: lease.LeaseID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(proofs) == 0 {
		// Halt 泵已吊销，Send 走 revoked 证明
		proofs, _ = svc.store.ListProofs(context.Background(), fundintgate.ExportProofFilter{Limit: 20})
	}
	found := false
	for _, p := range proofs {
		if p.Kind == fundintgate.DriftHalt || p.Kind == fundintgate.DriftRevoked {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected halt/revoked proof, got %+v", proofs)
	}
}

func TestImplSwapInvalidatesLease_Golden(t *testing.T) {
	_ = loadGolden(t, "golden_impl_swap.json")
	sender := &fundintgate.RecordingSender{}
	svc := newTestSvc(sender)
	lease := issueOK(t, svc)
	if _, _, err := svc.SwapGateImpl(context.Background(), "fundint.GateFunc.v2-halt-aware", "ops"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease.LeaseID, UserID: "u-from", Asset: "USDT", Amount: "10", IdempotencyKey: "s",
	})
	if !errors.Is(err, fundintgate.ErrGateImplSwap) && !errors.Is(err, fundintgate.ErrLeaseRevoked) {
		t.Fatalf("err=%v", err)
	}
	if sender.Calls != 0 {
		t.Fatal("spot send must not run")
	}
}

func TestForbidRaiseInvalidatesLease_Golden(t *testing.T) {
	_ = loadGolden(t, "golden_forbid_raise.json")
	sender := &fundintgate.RecordingSender{}
	svc := newTestSvc(sender)
	lease := issueOK(t, svc)
	if _, _, err := svc.RaiseForbidWithdraw(context.Background(), "ops"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease.LeaseID, UserID: "u-from", Asset: "USDT", Amount: "10", IdempotencyKey: "s",
	})
	if !errors.Is(err, fundintgate.ErrForbidWithdrawRaised) && !errors.Is(err, fundintgate.ErrLeaseRevoked) {
		t.Fatalf("err=%v", err)
	}
	if sender.Calls != 0 {
		t.Fatal("spot send must not run")
	}
}

func TestMissingAndExpiredFailClosed_Golden(t *testing.T) {
	_ = loadGolden(t, "golden_missing_and_expired.json")
	t.Run("missing", func(t *testing.T) {
		sender := &fundintgate.RecordingSender{}
		svc := newTestSvc(sender)
		_, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
			UserID:	"u-from", Asset: "USDT", Amount: "10", IdempotencyKey: "x",
		})
		if !errors.Is(err, fundintgate.ErrNoLease) {
			t.Fatalf("err=%v", err)
		}
		if sender.Calls != 0 {
			t.Fatal("calls")
		}
		proofs, _ := svc.store.ListProofs(context.Background(), fundintgate.ExportProofFilter{Kind: fundintgate.DriftNoLease, Limit: 10})
		if len(proofs) == 0 {
			t.Fatal("expected no_lease proof")
		}
	})
	t.Run("expired", func(t *testing.T) {
		sender := &fundintgate.RecordingSender{}
		now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
		svc := newTestSvc(sender).WithClock(func() time.Time { return now })
		lease, err := svc.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
			UserID:	"u-from", ToUID: "u-to", Asset: "USDT", Amount: "10", TTLSeconds: 1, IdempotencyKey: "ttl-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(5 * time.Second)
		n, err := fundintgate.NewExpireWorker(svc, time.Second).RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n < 1 {
			t.Fatalf("expire n=%d", n)
		}
		_, err = svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
			LeaseID:	lease.LeaseID, UserID: "u-from", Asset: "USDT", Amount: "10", IdempotencyKey: "late",
		})
		if !errors.Is(err, fundintgate.ErrLeaseExpired) {
			t.Fatalf("err=%v", err)
		}
		if sender.Calls != 0 {
			t.Fatal("calls")
		}
	})
}

func TestHappyBindAndIdempotentSend_Golden(t *testing.T) {
	_ = loadGolden(t, "golden_happy_bind.json")
	sender := &fundintgate.RecordingSender{ID: "xfer-ok"}
	svc := newTestSvc(sender)
	lease := issueOK(t, svc)
	res, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease.LeaseID, UserID: "u-from", ToUID: "u-to",
		Asset:	"USDT", Amount: "10", IdempotencyKey: "send-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Lease.State != fundintgate.LeaseSent || res.TransferID != "xfer-ok" {
		t.Fatalf("res=%+v", res)
	}
	res2, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease.LeaseID, UserID: "u-from", ToUID: "u-to",
		Asset:	"USDT", Amount: "10", IdempotencyKey: "send-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sender.Calls != 1 {
		t.Fatalf("calls=%d", sender.Calls)
	}
	if res2.TransferID != "xfer-ok" {
		t.Fatal("idem mismatch")
	}
}

func TestMinAmountAndResolveReject(t *testing.T) {
	t.Run("min", func(t *testing.T) {
		svc := newTestSvc(&fundintgate.RecordingSender{}).BindMinAmount("20")
		_, err := svc.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
			UserID:	"u-from", ToUID: "u-to", Asset: "USDT", Amount: "10",
		})
		if !errors.Is(err, fundintgate.ErrMinAmount) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("resolve", func(t *testing.T) {
		svc := fundintgate.New(fundintgate.StaticResolver{Err: errors.New("unknown")}, fundintgate.StaticGate{}, &fundintgate.RecordingSender{})
		_, err := svc.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
			UserID:	"u-from", ToUID: "ghost", Asset: "USDT", Amount: "10",
		})
		if !errors.Is(err, fundintgate.ErrResolveFail) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("self", func(t *testing.T) {
		svc := fundintgate.New(fundintgate.StaticResolver{ToUserID: "u-from"}, fundintgate.StaticGate{}, &fundintgate.RecordingSender{})
		_, err := svc.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
			UserID:	"u-from", ToUID: "u-from", Asset: "USDT", Amount: "10",
		})
		if !errors.Is(err, fundintgate.ErrSelfTransfer) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("gate deny", func(t *testing.T) {
		svc := fundintgate.New(fundintgate.StaticResolver{ToUserID: "u-to"}, fundintgate.StaticGate{Err: errors.New("cool")}, &fundintgate.RecordingSender{})
		_, err := svc.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
			UserID:	"u-from", ToUID: "u-to", Asset: "USDT", Amount: "10",
		})
		if !errors.Is(err, fundintgate.ErrGateDenied) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestAmountAndResolveDrift(t *testing.T) {
	sender := &fundintgate.RecordingSender{}
	svc := newTestSvc(sender)
	lease := issueOK(t, svc)
	_, err := svc.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease.LeaseID, UserID: "u-from", Asset: "BTC", Amount: "10", IdempotencyKey: "d1",
	})
	if !errors.Is(err, fundintgate.ErrAmountDrift) {
		t.Fatalf("err=%v", err)
	}
	svc.resolver = fundintgate.StaticResolver{ToUserID: "other"}
	lease2 := issueOK(t, newTestSvc(sender))
	// 恢复并针对 lease2 换解析
	svc2 := newTestSvc(sender)
	lease2,
		_ = svc2.CheckWithLease(context.Background(), fundintgate.CheckWithLeaseReq{
		UserID:	"u-from", ToUID: "u-to", Asset: "USDT", Amount: "11", IdempotencyKey: "d2",
	})
	svc2.resolver = fundintgate.StaticResolver{ToUserID: "hijack"}
	_, err = svc2.SendAssert(context.Background(), fundintgate.SendAssertReq{
		LeaseID:	lease2.LeaseID, UserID: "u-from", ToUID: "u-to", Asset: "USDT", Amount: "11", IdempotencyKey: "d3",
	})
	if !errors.Is(err, fundintgate.ErrResolveDrift) {
		t.Fatalf("err=%v", err)
	}
	if sender.Calls != 0 {
		t.Fatal("calls")
	}
}

func TestWorkersPatrolAndExport(t *testing.T) {
	svc := newTestSvc(&fundintgate.RecordingSender{})
	_ = issueOK(t, svc)
	if _, _, err := svc.HaltGate(context.Background(), "ops", "halt"); err != nil {
		t.Fatal(err)
	}
	n, err := fundintgate.NewUnconsumedPatrolWorker(svc, time.Second).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = n
	list, err := fundintgate.NewProofExportWorker(svc, time.Second).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = list
	w := fundintgate.NewHaltFlipRevokeWorker(svc, nil)
	if _, err := w.RunEvent(context.Background(), fundintgate.GateFlipEvent{NewGen: 99, Reason: "halt"}); err != nil {
		t.Fatal(err)
	}
}

func TestOpsRevokeAndAudit(t *testing.T) {
	svc := newTestSvc(&fundintgate.RecordingSender{})
	lease := issueOK(t, svc)
	got, err := svc.OpsRevoke(context.Background(), fundintgate.OpsRevokeReq{LeaseID: lease.LeaseID, Reason: "manual", Actor: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != fundintgate.LeaseRevoked {
		t.Fatalf("state=%s", got.State)
	}
	aud, err := svc.ListAudit(context.Background(), lease.LeaseID, 50)
	if err != nil || len(aud) == 0 {
		t.Fatalf("audit=%v err=%v", aud, err)
	}
}

func TestFingerprintStable(t *testing.T) {
	a := fundintgate.GateFingerprintOf("impl-a", false, false, fundintgate.DefaultPolicySalt, 1)
	b := fundintgate.GateFingerprintOf("impl-a", false, false, fundintgate.DefaultPolicySalt, 1)
	c := fundintgate.GateFingerprintOf("impl-b", false, false, fundintgate.DefaultPolicySalt, 1)
	if a != b || a == c {
		t.Fatalf("fp a=%s b=%s c=%s", a, b, c)
	}
	amt, err := fundintgate.CanonicalAmount("10")
	if err != nil || amt != "10.000000000000000000" {
		t.Fatalf("amt=%s err=%v", amt, err)
	}
	if !fundintgate.SurfaceEquals(fundintgate.GateSurface{Gen: 1}, fundintgate.GateSurface{Gen: 1}) {
		t.Fatal("equals")
	}
	if fundintgate.ClassifyFlipReason("impl_swap:x") != fundintgate.DriftImplSwap {
		t.Fatal("classify")
	}
}

func TestFundintLeaseEngineBind(t *testing.T) {
	sender := &fundintgate.RecordingSender{ID: "via-fi"}
	svc := newTestSvc(sender)
	// fundint 未挂 ledger 时 Send 不可用；仅验证 IssueLease / AssertSend
	eng := fundintgate.Engine{Svc: svc}
	id, err := eng.CheckPass(context.Background(), fundint.Req{
		UserID:	"u-from", ToUID: "u-to", Asset: "USDT",
		Amount:	decimal.RequireFromString("10"), IdempotencyKey: "fi-1",
	})
	if err != nil || id == "" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	if err := eng.AssertSend(context.Background(), id, fundint.Req{
		UserID:	"u-from", ToUID: "u-to", Asset: "USDT",
		Amount:	decimal.RequireFromString("10"), IdempotencyKey: "fi-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.AssertSend(context.Background(), "", fundint.Req{}); !errors.Is(err, fundintgate.ErrNoLease) {
		t.Fatalf("err=%v", err)
	}
}

func TestMemStoreRoundTrip(t *testing.T) {
	m := fundintgate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	lease := fundintgate.IntGateLease{LeaseID: "L1", State: fundintgate.LeaseGated, IssuedAt: now, TTLSeconds: 60, IdempotencyKey: "k", CreatedAt: now, UpdatedAt: now}
	if err := m.SaveLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetLease(ctx, "L1")
	if err != nil || got.LeaseID != "L1" {
		t.Fatal(err)
	}
	ex, ok, err := m.GetLeaseByIdempotencyKey(ctx, "k")
	if err != nil || !ok || ex.LeaseID != "L1" {
		t.Fatal(err)
	}
	if err := m.SaveProof(ctx, fundintgate.IntGateDriftProof{ProofID: "P1", Kind: fundintgate.DriftNoLease, CreatedAt: now, DetectedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetProof(ctx, "P1"); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveGen(ctx, fundintgate.GateGeneration{ID: "G1", Scope: fundintgate.ScopeGlobal, Gen: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetGen(ctx, fundintgate.ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	_ = m.SaveAudit(ctx, fundintgate.IntGateAudit{AuditID: "A1", LeaseID: "L1", Action: fundintgate.AuditIssue, CreatedAt: now})
	if list, _ := m.ListAuditByLease(ctx, "L1", 10); len(list) != 1 {
		t.Fatal(list)
	}
}

func TestDirectoryResolverMultiID(t *testing.T) {
	// 无 Directory 时失败
	r := fundintgate.DirectoryResolver{}
	if _, err := r.Resolve(context.Background(), "a", "", ""); !errors.Is(err, fundintgate.ErrResolveFail) {
		t.Fatalf("err=%v", err)
	}
	mr := fundintgate.MapResolver{Users: map[string]string{"alice": "U1", "a@x": "U1"}}
	res, err := mr.Resolve(context.Background(), "alice", "", "")
	if err != nil || res.ToUserID != "U1" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestPolicyEncode(t *testing.T) {
	ev := fundintgate.PolicyEvent{Kind: fundintgate.PolicyHalt, Actor: "ops", Reason: "drill", NextGen: 2, Halted: true, At: time.Now().UTC()}
	raw := fundintgate.EncodePolicy(ev)
	back, err := fundintgate.DecodePolicy(raw)
	if err != nil || back.Kind != fundintgate.PolicyHalt {
		t.Fatalf("%v %v", back, err)
	}
	if _, err := fundintgate.DefaultTTLForSurface(fundintgate.GateSurface{Halted: true}); !errors.Is(err, fundintgate.ErrGateHalted) {
		t.Fatalf("err=%v", err)
	}
}

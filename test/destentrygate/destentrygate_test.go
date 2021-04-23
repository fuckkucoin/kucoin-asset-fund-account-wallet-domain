package destentrygate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/destentrygate"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "destentrygate")
}

func loadGolden(t *testing.T, name string) destentrygate.GoldenTrace {
	t.Helper()
	b,
		err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	var g destentrygate.GoldenTrace
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func assertGolden(t *testing.T, got destentrygate.GoldenTrace, name string) {
	t.Helper()
	want := loadGolden(t, name)
	if got.Scenario != want.Scenario || got.IssueOK != want.IssueOK || got.WithdrawOK != want.WithdrawOK {
		t.Fatalf("golden %s core mismatch got=%+v want=%+v", name, got, want)
	}
	if want.RejectReason != "" && got.RejectReason != want.RejectReason {
		t.Fatalf("golden %s reject got=%q want=%q", name, got.RejectReason, want.RejectReason)
	}
	if want.LeaseState != "" && got.LeaseState != want.LeaseState {
		t.Fatalf("golden %s state got=%s want=%s", name, got.LeaseState, want.LeaseState)
	}
	for _, k := range want.ProofKinds {
		found := false
		for _, g := range got.ProofKinds {
			if g == k {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("golden %s missing proof kind %s in %v", name, k, got.ProofKinds)
		}
	}
}

func liveStrict(user, chain, addr string) destentrygate.LiveEntry {
	return destentrygate.LiveEntry{
		EntryID: destentrygate.DeriveEntryID(user,
			chain, addr),
		UserID:	user, Chain: chain, Address: addr,
		Mode:	destentrygate.ModeStrict, Live: true, CooldownFull: true,
	}
}

func newSvc(surf *destentrygate.MemorySurface, sink *destentrygate.RecordingSink,) *destentrygate.Service {
	if surf == nil {
		surf = destentrygate.NewMemorySurface(true)
	}
	if sink == nil {
		sink = &destentrygate.RecordingSink{Ref: "wd-1"}
	}
	return destentrygate.New(surf, sink)
}

func TestFSMTable(t *testing.T) {
	cases := []struct {
		from	destentrygate.LeaseState
		to	destentrygate.LeaseState
		ok	bool
	}{
		{destentrygate.LeaseIssued, destentrygate.LeaseBound, true},
		{destentrygate.LeaseIssued, destentrygate.LeaseExpired, true},
		{destentrygate.LeaseIssued, destentrygate.LeaseRevoked, true},
		{destentrygate.LeaseIssued, destentrygate.LeaseConsumed, false},
		{destentrygate.LeaseBound, destentrygate.LeaseConsumed, true},
		{destentrygate.LeaseBound, destentrygate.LeaseExpired, true},
		{destentrygate.LeaseBound, destentrygate.LeaseRevoked, true},
		{destentrygate.LeaseBound, destentrygate.LeaseIssued, false},
		{destentrygate.LeaseConsumed, destentrygate.LeaseRevoked, false},
		{destentrygate.LeaseExpired, destentrygate.LeaseBound, false},
		{destentrygate.LeaseRevoked, destentrygate.LeaseIssued, false},
	}
	for _, tc := range cases {
		if destentrygate.CanTransit(tc.from, tc.to) != tc.ok {
			t.Fatalf("%s→%s want %v", tc.from, tc.to, tc.ok)
		}
	}
	if !destentrygate.IsTerminalLease(destentrygate.LeaseConsumed) || destentrygate.IsTerminalLease(destentrygate.LeaseIssued) {
		t.Fatal("terminal")
	}
	tab := destentrygate.TransitionTable()
	if len(tab[destentrygate.LeaseIssued]) != 3 {
		t.Fatalf("issued edges %d", len(tab[destentrygate.LeaseIssued]))
	}
}

func TestHappyBindConsume(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xabc")
	surf.Put(e)
	sink := &destentrygate.RecordingSink{Ref: "wd-ok"}
	svc := newSvc(surf, sink)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-h1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if lease.LeaseID == "" || lease.State != destentrygate.LeaseIssued || lease.EntryGen != 1 {
		t.Fatalf("lease %+v", lease)
	}
	again, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-h1",
	})
	if err != nil || again.LeaseID != lease.LeaseID {
		t.Fatalf("idempotent %v %s", err, again.LeaseID)
	}
	res, err := svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		LeaseID:	lease.LeaseID, UserID: e.UserID, Chain: e.Chain, Address: e.Address, Consume: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Consumed || res.WithdrawRef != "wd-ok" || sink.Calls != 1 {
		t.Fatalf("res %+v calls=%d", res, sink.Calls)
	}
}

func TestTempExpireOldLeaseReject(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xtemp")
	e.Mode = destentrygate.ModeTemp
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-temp",
	})
	if err != nil {
		t.Fatal(err)
	}
	dead := e
	dead.Live = false
	dead.Reason = "temp_expired"
	surf.Put(dead)
	surf.ExpiredT = []destentrygate.LiveEntry{dead}
	n, err := svc.OnTempExpired(context.Background(), dead, "test")
	if err != nil || n < 1 {
		t.Fatalf("revoke n=%d err=%v", n, err)
	}
	_, werr := svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		LeaseID:	lease.LeaseID, UserID: e.UserID, Chain: e.Chain, Address: e.Address,
	})
	if werr == nil {
		t.Fatal("expected reject")
	}
	gotL, _ := svc.GetLease(context.Background(), lease.LeaseID)
	proofs, _ := svc.ListProofs(context.Background(), destentrygate.ExportProofFilter{Limit: 20})
	trace := destentrygate.BuildGoldenTrace("temp_expire_old_lease_withdraw", true, false, gotL, proofs, werr)
	assertGolden(t, trace, "golden_temp_expire_reject.json")
}

func TestBreakglassReclaimReject(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xglass")
	e.Mode = destentrygate.ModeBreakglass
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-glass",
	})
	if err != nil {
		t.Fatal(err)
	}
	dead := e
	dead.Live = false
	dead.Reason = "breakglass_reclaimed"
	surf.Put(dead)
	n, err := svc.OnBreakglassReclaim(context.Background(), dead, "ops")
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, werr := svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		LeaseID:	lease.LeaseID,
		UserID:		e.UserID, Chain: e.Chain, Address: e.Address,
	})
	if werr == nil {
		t.Fatal("expected reject")
	}
	gotL, _ := svc.GetLease(context.Background(), lease.LeaseID)
	proofs, _ := svc.ListProofs(context.Background(), destentrygate.ExportProofFilter{Limit: 20})
	trace := destentrygate.BuildGoldenTrace("breakglass_reclaim_rejects_withdraw", true, false, gotL, proofs, werr)
	assertGolden(t, trace, "golden_breakglass_reclaim.json")
}

func TestRevokeEntryGenDrift(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xrev")
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-rev",
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.OnEntryRevoke(context.Background(), e, "ops")
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, werr := svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		LeaseID:	lease.LeaseID, UserID: e.UserID, Chain: e.Chain, Address: e.Address,
	})
	if !errors.Is(werr, destentrygate.ErrLeaseRevoked) {
		t.Fatalf("want revoked got %v", werr)
	}
	gotL, _ := svc.GetLease(context.Background(), lease.LeaseID)
	proofs, _ := svc.ListProofs(context.Background(), destentrygate.ExportProofFilter{Limit: 20})
	trace := destentrygate.BuildGoldenTrace("revoke_entry_gen_drift", true, false, gotL, proofs, werr)
	assertGolden(t, trace, "golden_revoke_gen_drift.json")
	g,
		err := svc.Store().GetGen(context.Background(), e.EntryID)
	if err != nil || g.Gen < 2 {
		t.Fatalf("gen %+v %v", g, err)
	}
}

func TestWhitelistOnlyMissingLease(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	svc := newSvc(surf, nil)
	_, err := svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		UserID:	"u1", Chain: "ETH", Address: "0xmiss",
	})
	if !errors.Is(err, destentrygate.ErrWhitelistOnly) {
		t.Fatalf("got %v", err)
	}
	proofs, _ := svc.ListProofs(context.Background(), destentrygate.ExportProofFilter{Limit: 10})
	trace := destentrygate.BuildGoldenTrace("whitelist_only_missing_lease", false, false, destentrygate.DestEntryLease{}, proofs, err)
	assertGolden(t, trace, "golden_missing_lease_failclosed.json")
}

func TestRestoreCooldownRenewDenied(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xcool")
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-cool",
	})
	if err != nil {
		t.Fatal(err)
	}
	gap := e
	gap.Live = false
	gap.CooldownFull = false
	gap.Reason = "cooldown"
	surf.Put(gap)
	_, rerr := svc.RenewLease(context.Background(), destentrygate.RenewLeaseReq{LeaseID: lease.LeaseID, Actor: "restore"})
	if !errors.Is(rerr, destentrygate.ErrRenewDenied) {
		t.Fatalf("got %v", rerr)
	}
	gotL, _ := svc.GetLease(context.Background(), lease.LeaseID)
	proofs, _ := svc.ListProofs(context.Background(), destentrygate.ExportProofFilter{Limit: 10})
	trace := destentrygate.BuildGoldenTrace("restore_cooldown_not_full_renew", true, false, gotL, proofs, rerr)
	assertGolden(t, trace, "golden_restore_cooldown.json")
}

func TestCooldownBlocksIssue(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xgap")
	e.Live = false
	e.CooldownFull = false
	e.Reason = "cooldown"
	surf.Put(e)
	svc := newSvc(surf, nil)
	_, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-gap",
	})
	if !errors.Is(err, destentrygate.ErrCooldownNotFull) {
		t.Fatalf("got %v", err)
	}
}

func TestAddrDrift(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xone")
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-addr",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		LeaseID:	lease.LeaseID, UserID: "u1", Chain: "ETH", Address: "0xtwo",
	})
	if !errors.Is(err, destentrygate.ErrAddrDrift) {
		t.Fatalf("got %v", err)
	}
}

func TestExpireWorkerTTL(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xttl")
	surf.Put(e)
	svc := newSvc(surf, nil).WithClock(func() time.Time { return now })
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, TTLSeconds: 1, IdempotencyKey: "idem-ttl",
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	n, err := destentrygate.NewExpireWorker(svc, time.Second).RunOnce(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, err = svc.AssertLease(context.Background(), destentrygate.AssertLeaseReq{
		LeaseID:	lease.LeaseID, UserID: e.UserID, Chain: e.Chain, Address: e.Address,
	})
	if !errors.Is(err, destentrygate.ErrLeaseExpired) {
		t.Fatalf("got %v", err)
	}
}

func TestWorkersScanAndPump(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xscan")
	e.Mode = destentrygate.ModeTemp
	surf.Put(e)
	svc := newSvc(surf, nil)
	_, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-scan",
	})
	if err != nil {
		t.Fatal(err)
	}
	dead := e
	dead.Live = false
	dead.Reason = "temp_expired"
	surf.ExpiredT = []destentrygate.LiveEntry{dead}
	n, err := destentrygate.NewTempExpireScanWorker(svc, time.Second).RunOnce(context.Background())
	if err != nil || n < 1 {
		t.Fatalf("temp scan n=%d err=%v", n, err)
	}
	surf.ExpiredG = []destentrygate.LiveEntry{{
		EntryID:	destentrygate.DeriveEntryID("u2", "ETH", "0xg"),
		UserID:		"u2", Chain: "ETH", Address: "0xg", Mode: destentrygate.ModeBreakglass, Reason: "breakglass_reclaimed",
	}}
	if _, err := destentrygate.NewBreakglassReclaimWorker(svc, time.Second).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	evs := make(chan destentrygate.LifecycleEvent, 1)
	pump := destentrygate.NewRevokePumpWorker(svc, evs)
	e3 := liveStrict("u3", "ETH", "0x3")
	surf.Put(e3)
	_, _ = svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e3.UserID, Chain: e3.Chain, Address: e3.Address, IdempotencyKey: "idem-p3",
	})
	if _, err := pump.Handle(context.Background(), destentrygate.LifecycleEvent{Kind: "revoke", Entry: e3, Actor: "pump"}); err != nil {
		t.Fatal(err)
	}
	if _, err := destentrygate.NewUnconsumedPatrolWorker(svc, time.Second).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := destentrygate.NewProofExportWorker(svc, time.Second).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPatrolRevokesDead(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xpat")
	surf.Put(e)
	svc := newSvc(surf, nil)
	_, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-pat",
	})
	if err != nil {
		t.Fatal(err)
	}
	dead := e
	dead.Live = false
	dead.Reason = "missing"
	surf.Put(dead)
	n, err := svc.PatrolUnconsumed(context.Background())
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestPolicyHelpers(t *testing.T) {
	p := destentrygate.DefaultPolicy()
	if p.ClampTTL(0) != destentrygate.DefaultTTLSeconds || p.ClampTTL(99999) != destentrygate.MaxTTLSeconds {
		t.Fatal("clamp")
	}
	if err := p.MustHaveLease(true, ""); !errors.Is(err, destentrygate.ErrWhitelistOnly) {
		t.Fatal(err)
	}
	if err := p.CheckGen(1, 2); !errors.Is(err, destentrygate.ErrEntryGenDrift) {
		t.Fatal(err)
	}
	if err := p.CheckRenew(destentrygate.LiveEntry{Live: true, CooldownFull: false}); !errors.Is(err, destentrygate.ErrRenewDenied) {
		t.Fatal(err)
	}
	if !destentrygate.ProofKindValid(destentrygate.DriftTempExpire) || destentrygate.ProofKindValid("nope") {
		t.Fatal("catalog")
	}
	if len(destentrygate.KnownProofKinds()) < 10 || len(destentrygate.KnownAuditActions()) < 10 {
		t.Fatal("catalog size")
	}
}

func TestDestallowSnapshotAndAdapter(t *testing.T) {
	book := destallow.New()
	if _, err := book.Add("u1", "ETH", "0xabc", "lab"); err != nil {
		t.Fatal(err)
	}
	s := book.SnapshotEntry(context.Background(), "u1", "ETH", "0xabc")
	if !s.InBook || s.EntryID == "" || s.Mode != destallow.EntryModeStrict {
		t.Fatalf("snap %+v", s)
	}
	if s.EntryID != destallow.ComputeEntryID("u1", "ETH", "0xabc") {
		t.Fatal("id mismatch destallow")
	}
	if s.EntryID != destentrygate.DeriveEntryID("u1", "ETH", "0xabc") {
		t.Fatal("id mismatch gate")
	}
	ad := destentrygate.BookAdapter{Book: book, Pref: destentrygate.StaticPref{On: true}}
	if !ad.WhitelistOnly(context.Background(), "u1") {
		t.Fatal("pref")
	}
	live, err := ad.Snapshot(context.Background(), "u1", "ETH", "0xabc")
	if err != nil || !live.Live {
		t.Fatalf("adapter %v %+v", err, live)
	}
	obs := destallow.ObserveBook(book)
	if !destallow.LiveForLease(obs.SnapshotEntry(context.Background(), "u1", "ETH", "0xabc")) {
		t.Fatal("live for lease")
	}
}

func TestApplyTransitionNil(t *testing.T) {
	if err := destentrygate.ApplyLeaseTransition(nil, destentrygate.LeaseBound, "", time.Now()); !errors.Is(err, destentrygate.ErrLeaseNotFound) {
		t.Fatal(err)
	}
	lease := destentrygate.DestEntryLease{State: destentrygate.LeaseIssued, IssuedAt: time.Now(), TTLSeconds: 10}
	if err := destentrygate.ApplyLeaseTransition(&lease, destentrygate.LeaseBound, "", time.Now()); err != nil || lease.BoundAt == nil {
		t.Fatal(err)
	}
	if err := destentrygate.AssertIssuedForBind(&lease, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := destentrygate.AssertBoundForConsume(&destentrygate.DestEntryLease{State: destentrygate.LeaseIssued, IssuedAt: time.Now(), TTLSeconds: 10}, time.Now()); !errors.Is(err, destentrygate.ErrLeaseNotBound) {
		t.Fatal(err)
	}
}

func TestOpsRevokeAndGetters(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xops")
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-ops",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.OpsRevoke(context.Background(), lease.LeaseID, "ops", "manual")
	if err != nil || got.State != destentrygate.LeaseRevoked {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := svc.GetLease(context.Background(), lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	aud, err := svc.ListAudit(context.Background(), lease.LeaseID, 20)
	if err != nil || len(aud) == 0 {
		t.Fatal(err)
	}
}

func TestMemStoreLists(t *testing.T) {
	m := destentrygate.NewMemStore()
	now := time.Now().UTC()
	l := destentrygate.DestEntryLease{
		LeaseID:	"L1", EntryID: "E1", EntryGen: 1, State: destentrygate.LeaseIssued,
		IssuedAt:	now.Add(-time.Hour), TTLSeconds: 1, CreatedAt: now, UpdatedAt: now,
		IdempotencyKey:	"k1",
	}
	if err := m.SaveLease(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetLease(context.Background(), "no"); !errors.Is(err, destentrygate.ErrLeaseNotFound) {
		t.Fatal(err)
	}
	got, ok, err := m.GetLeaseByIdempotencyKey(context.Background(), "k1")
	if err != nil || !ok || got.LeaseID != "L1" {
		t.Fatal(err)
	}
	if _, err := m.ListExpiredIssued(context.Background(), now, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ListUnconsumed(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveGen(context.Background(), destentrygate.EntryGeneration{EntryID: "E1", Gen: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetGen(context.Background(), "E1"); err != nil {
		t.Fatal(err)
	}
	p := destentrygate.DestEntryDriftProof{ProofID: "P1", Kind: destentrygate.DriftNoLease}
	if err := m.SaveProof(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetProof(context.Background(), "P1"); err != nil {
		t.Fatal(err)
	}
	if m.CountProofs() != 1 {
		t.Fatal("count")
	}
}

func TestNormalizeModeAndTuple(t *testing.T) {
	if destentrygate.NormalizeMode("TEMP") != destentrygate.ModeTemp || destentrygate.NormalizeMode("x") != "" {
		t.Fatal("mode")
	}
	l := destentrygate.DestEntryLease{UserID: "u", Chain: "ETH", Address: "a"}
	if !destentrygate.SameTuple(l, "u", "eth", "a") {
		t.Fatal("tuple")
	}
}

func TestAssembleBundle(t *testing.T) {
	b := destentrygate.Assemble(destentrygate.Options{})
	if b.Service == nil || b.Expire == nil || b.TempScan == nil {
		t.Fatal("bundle")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.StartWorkers(ctx)
	b.Publish(destentrygate.LifecycleEvent{Kind: "revoke"})
}

func TestProofJSONAndSummary(t *testing.T) {
	p := destentrygate.DestEntryDriftProof{Kind: destentrygate.DriftEntryGen, LeaseID: "L", EntryID: "E", BindGen: 1, LiveGen: 2, Evidence: "x"}
	if _, err := destentrygate.ProofJSON(p); err != nil {
		t.Fatal(err)
	}
	s := destentrygate.SummarizeProof(p)
	if !s.GenDrift || s.Digest == "" {
		t.Fatalf("%+v", s)
	}
	if destentrygate.FormatDetected(time.Time{}) != "" {
		t.Fatal("zero")
	}
}

func TestOnRestoreRevokes(t *testing.T) {
	surf := destentrygate.NewMemorySurface(true)
	e := liveStrict("u1", "ETH", "0xrst")
	surf.Put(e)
	svc := newSvc(surf, nil)
	lease, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	e.UserID, Chain: e.Chain, Address: e.Address, IdempotencyKey: "idem-rst",
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.OnRestoreCooldownNotFull(context.Background(), e, "restore")
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got, _ := svc.GetLease(context.Background(), lease.LeaseID)
	if got.State != destentrygate.LeaseRevoked {
		t.Fatalf("state %s", got.State)
	}
}

func TestInvalidPrecheck(t *testing.T) {
	svc := newSvc(destentrygate.NewMemorySurface(true), nil)
	if _, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{}); !errors.Is(err, destentrygate.ErrInvalidReq) {
		t.Fatal(err)
	}
	if _, err := svc.PrecheckWithLease(context.Background(), destentrygate.PrecheckWithLeaseReq{
		UserID:	"u", Chain: "ETH", Address: "a",
	}); !errors.Is(err, destentrygate.ErrIdem) {
		t.Fatal(err)
	}
}

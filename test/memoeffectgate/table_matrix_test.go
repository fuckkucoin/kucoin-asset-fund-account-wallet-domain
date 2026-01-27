package memoeffectgate_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
)

func TestFailClosedMatrix(t *testing.T) {
	type tc struct {
		name	string
		pending	bool
		effect	bool
		occ	bool
		auto	bool
		want	error
	}
	cases := []tc{
		{"ok", true, true, false, false, nil},
		{"not pending", false, true, false, false, memoeffectgate.ErrTicketNotPending},
		{"occupied", true, true, true, false, memoeffectgate.ErrOutputOccupied},
		{"auto", true, true, true, true, memoeffectgate.ErrAutoClaimOccupied},
		{"no effect", true, false, false, false, memoeffectgate.ErrEffectRequired},
		{"not pending occupied", false, false, true, true, memoeffectgate.ErrTicketNotPending},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			err := memoeffectgate.FailClosedApprove(c.pending, c.effect, c.occ, c.auto)
			if c.want == nil {
				if err != nil {
					t.Fatalf("got %v", err)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v want %v", err, c.want)
			}
		})
	}
}

func TestPolicyMatrix(t *testing.T) {
	for _, row := range memoeffectgate.DefaultPolicyMatrix() {
		row := row
		t.Run(row.Name, func(t *testing.T) {
			if err := memoeffectgate.EvaluatePolicy(row); err != nil && row.AllowApprove {
				t.Fatalf("%s: %v", row.Name, err)
			}
		})
	}
}

func TestFSMMatrix(t *testing.T) {
	type tc struct {
		from	memoeffectgate.EffectState
		ev	string
		to	memoeffectgate.EffectState
		fail	bool
	}
	cases := []tc{
		{memoeffectgate.EffectBound, memoeffectgate.EffectEvCredit, memoeffectgate.EffectCredited, false},
		{memoeffectgate.EffectBound, memoeffectgate.EffectEvReject, memoeffectgate.EffectRejectedBlocked, false},
		{memoeffectgate.EffectBound, memoeffectgate.EffectEvExpire, memoeffectgate.EffectExpired, false},
		{memoeffectgate.EffectBound, memoeffectgate.EffectEvQuarantine, memoeffectgate.EffectQuarantined, false},
		{memoeffectgate.EffectCredited, memoeffectgate.EffectEvReject, memoeffectgate.EffectRejectedBlocked, false},
		{memoeffectgate.EffectExpired, memoeffectgate.EffectEvCredit, "", true},
		{memoeffectgate.EffectRejectedBlocked, memoeffectgate.EffectEvCredit, "", true},
		{memoeffectgate.EffectCredited, memoeffectgate.EffectEvExpire, "", true},
	}
	for _, c := range cases {
		next, err := memoeffectgate.MustApplyEffect(c.from, c.ev)
		if c.fail {
			if err == nil {
				t.Fatalf("%s %s should fail", c.from, c.ev)
			}
			continue
		}
		if err != nil || next != c.to {
			t.Fatalf("%s %s -> %s %v want %s", c.from, c.ev, next, err, c.to)
		}
	}
}

func TestHalfFSMMatrix(t *testing.T) {
	next, err := memoeffectgate.MustApplyHalf(memoeffectgate.HalfOpen, memoeffectgate.HalfEvReverse)
	if err != nil || next != memoeffectgate.HalfReversed {
		t.Fatalf("%s %v", next, err)
	}
	next, err = memoeffectgate.MustApplyHalf(memoeffectgate.HalfOpen, memoeffectgate.HalfEvAck)
	if err != nil || next != memoeffectgate.HalfAcked {
		t.Fatalf("%s %v", next, err)
	}
	if _, err := memoeffectgate.MustApplyHalf(memoeffectgate.HalfReversed, memoeffectgate.HalfEvAck); err == nil {
		t.Fatal("terminal")
	}
}

func TestOutputKeyMatrix(t *testing.T) {
	type tc struct {
		chain	string
		hash	string
		idx	int
		fail	bool
		key	string
	}
	cases := []tc{
		{"eth", "0x1", 0, false, "ETH|0x1|0"},
		{"BTC", "ab", 3, false, "BTC|ab|3"},
		{"", "h", 0, true, ""},
		{"ETH", "", 0, true, ""},
		{"ETH", "h", -1, true, ""},
		{"xrp", "tx", 1, false, "XRP|tx|1"},
	}
	for _, c := range cases {
		k, err := memoeffectgate.CanonicalOutputKey(c.chain, c.hash, c.idx)
		if c.fail {
			if err == nil {
				t.Fatalf("expected fail %s %s %d", c.chain, c.hash, c.idx)
			}
			continue
		}
		if err != nil || k != c.key {
			t.Fatalf("got %s %v want %s", k, err, c.key)
		}
		ch, h, i, err := memoeffectgate.SplitOutputKey(k)
		if err != nil || ch != memoeffectgate.NormalizeAsset(c.chain) || h != c.hash || i != c.idx {
			t.Fatalf("split %s %s %d %v", ch, h, i, err)
		}
	}
}

func TestCatalogOutputRules(t *testing.T) {
	rules := memoeffectgate.BuildOutputRules()
	if len(rules) < 80 {
		t.Fatalf("rules %d", len(rules))
	}
	for _, r := range rules {
		err := memoeffectgate.CheckOutputRule(r.Chain, r.Index, "memo")
		if r.Allowed && err != nil {
			t.Fatalf("%s: %v", memoeffectgate.FormatOutputRule(r), err)
		}
		if !r.Allowed && err == nil {
			t.Fatalf("should fail %s", memoeffectgate.FormatOutputRule(r))
		}
	}
	if memoeffectgate.CountAllowedRules() == 0 {
		t.Fatal("no allowed")
	}
	if memoeffectgate.CatalogSize() < 10 {
		t.Fatal("catalog thin")
	}
}

func TestValidateApproveMatrix(t *testing.T) {
	cases := []struct {
		req	memoeffectgate.ApproveMemoWithEffectReq
		err	error
	}{
		{memoeffectgate.ApproveMemoWithEffectReq{TicketID: "t", Asset: "ETH"}, nil},
		{memoeffectgate.ApproveMemoWithEffectReq{Asset: "ETH"}, memoeffectgate.ErrTicketRequired},
		{memoeffectgate.ApproveMemoWithEffectReq{TicketID: "t"}, memoeffectgate.ErrAssetRequired},
		{memoeffectgate.ApproveMemoWithEffectReq{TicketID: "t", Asset: "ETH", OutputIndex: -2}, memoeffectgate.ErrOutputKeyInvalid},
		{memoeffectgate.ApproveMemoWithEffectReq{TicketID: "t", Asset: "ETH", TTL: "-1s"}, memoeffectgate.ErrTTLInvalid},
		{memoeffectgate.ApproveMemoWithEffectReq{TicketID: "t", Asset: "ETH", TTL: "nope"}, memoeffectgate.ErrTTLInvalid},
	}
	for i, c := range cases {
		err := memoeffectgate.ValidateApprove(c.req)
		if c.err == nil && err != nil {
			t.Fatalf("%d %v", i, err)
		}
		if c.err != nil && !errors.Is(err, c.err) {
			t.Fatalf("%d got %v want %v", i, err, c.err)
		}
	}
}

func TestValidateDecideMatrix(t *testing.T) {
	cases := []struct {
		req	memoeffectgate.DecideMemoGuardedReq
		err	error
	}{
		{memoeffectgate.DecideMemoGuardedReq{TicketID: "t", Status: "rejected", Actor: "a"}, nil},
		{memoeffectgate.DecideMemoGuardedReq{Status: "rejected", Actor: "a"}, memoeffectgate.ErrTicketRequired},
		{memoeffectgate.DecideMemoGuardedReq{TicketID: "t", Status: "nope", Actor: "a"}, memoeffectgate.ErrStatusInvalid},
		{memoeffectgate.DecideMemoGuardedReq{TicketID: "t", Status: "rejected"}, memoeffectgate.ErrActorRequired},
	}
	for i, c := range cases {
		err := memoeffectgate.ValidateDecide(c.req)
		if c.err == nil && err != nil {
			t.Fatalf("%d %v", i, err)
		}
		if c.err != nil && !errors.Is(err, c.err) {
			t.Fatalf("%d %v", i, err)
		}
	}
}

func TestSilentKeepAndSecondCredit(t *testing.T) {
	if err := memoeffectgate.MustBlockSilentKeep(true, true, false); !errors.Is(err, memoeffectgate.ErrSilentKeepForbidden) {
		t.Fatalf("%v", err)
	}
	if err := memoeffectgate.MustBlockSilentKeep(true, true, true); err != nil {
		t.Fatal(err)
	}
	if err := memoeffectgate.MustBlockSecondCredit(2, false); !errors.Is(err, memoeffectgate.ErrSecondFullCredit) {
		t.Fatal(err)
	}
	if err := memoeffectgate.MustBlockSecondCredit(1, false); err != nil {
		t.Fatal(err)
	}
}

func TestFenceNextMatrix(t *testing.T) {
	type tc struct {
		from	memoeffectgate.FencePhase
		ok	bool
		to	memoeffectgate.FencePhase
	}
	cases := []tc{
		{memoeffectgate.FenceIdle, true, memoeffectgate.FenceObserve},
		{memoeffectgate.FenceIdle, false, memoeffectgate.FenceBlocked},
		{memoeffectgate.FenceObserve, true, memoeffectgate.FenceApply},
		{memoeffectgate.FenceObserve, false, memoeffectgate.FenceHalfCut},
		{memoeffectgate.FenceApply, true, memoeffectgate.FenceTicket},
		{memoeffectgate.FenceApply, false, memoeffectgate.FenceHalfCut},
		{memoeffectgate.FenceTicket, true, memoeffectgate.FenceSealed},
		{memoeffectgate.FenceHalfCut, true, memoeffectgate.FenceApply},
	}
	for _, c := range cases {
		got,
			err := memoeffectgate.NextFence(c.from, c.ok)
		if err != nil || got != c.to {
			t.Fatalf("%s %v -> %s %v", c.from, c.ok, got, err)
		}
	}
}

func TestMemMutexAcquireMatrix(t *testing.T) {
	m := memoeffectgate.NewMemClaimMutex()
	ctx := context.Background()
	k := m.OutputKey("ETH", "tx", 0)
	ok, _, err := m.Acquire(ctx, k, memoeffectgate.HolderMemoApprove, "t1")
	if err != nil || !ok {
		t.Fatal(err)
	}
	ok, occ, err := m.Acquire(ctx, k, memoeffectgate.HolderAutoClaim, "t2")
	if err == nil || ok || !occ.Taken {
		t.Fatalf("should occupy %v %v", ok, err)
	}
	ok, _, err = m.Acquire(ctx, k, memoeffectgate.HolderMemoApprove, "t1")
	if err != nil || !ok {
		t.Fatalf("same holder replay %v", err)
	}
}

func TestHalfCutHelpers(t *testing.T) {
	f := memoeffectgate.TicketCreditFence{HalfCut: true, ObserveOK: true, CreditCount: 1, Phase: memoeffectgate.FenceHalfCut}
	if !memoeffectgate.DiscoverHalfCut(f) || !memoeffectgate.ResumeAllowed(f, memoeffectgate.MemoEffect{State: memoeffectgate.EffectBound}) {
		t.Fatal("resume")
	}
	if memoeffectgate.CreditCountAfterResume(f) != 1 {
		t.Fatal("count")
	}
	if memoeffectgate.ResumeAllowed(f, memoeffectgate.MemoEffect{State: memoeffectgate.EffectCredited}) {
		t.Fatal("credited cannot resume")
	}
}

func TestProofKindsExport(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := pendingTicket(book, "ETH", "hx", "m")
	mustApprove(t, eng, tk.ID, "ETH")
	_, _ = eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
		TicketID:	tk.ID, Status: "rejected", Actor: "ops",
	})
	rows, err := eng.ExportProofView(context.Background(), memoeffectgate.ProofRejectAfterCredit, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected proofs")
	}
	if rows[0].SilentKeep {
		t.Fatal("silent")
	}
}

func TestOpsReverseAck(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := pendingTicket(book, "ETH", "hz", "m")
	mustApprove(t, eng, tk.ID, "ETH")
	d, _ := eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
		TicketID:	tk.ID, Status: "rejected", Actor: "ops",
	})
	if d.Proof == nil {
		t.Fatal("proof")
	}
	p, err := eng.OpsReverseGate(context.Background(), memoeffectgate.ReverseGateReq{ProofID: d.Proof.ID, Actor: "ops", AckOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.State != memoeffectgate.HalfAcked {
		t.Fatalf("%s", p.State)
	}
}

func TestNeedMemoChains(t *testing.T) {
	for _, ch := range []string{"XRP", "XLM", "EOS", "TON"} {
		if !memoeffectgate.NeedMemo(ch) {
			t.Fatalf("%s need memo", ch)
		}
	}
	if memoeffectgate.NeedMemo("ETH") {
		t.Fatal("eth")
	}
}

func TestFingerprintStable(t *testing.T) {
	e := memoeffectgate.MemoEffect{TicketID: "t", UserID: "u", Chain: "ETH", TxHash: "h", Asset: "ETH", State: memoeffectgate.EffectBound}
	if memoeffectgate.EffectFingerprint(e) != memoeffectgate.EffectFingerprint(e) {
		t.Fatal("fp")
	}
	if memoeffectgate.EffectFingerprint(e) == "" {
		t.Fatal("empty")
	}
}

func TestNilEngine(t *testing.T) {
	var e *memoeffectgate.Engine
	if _, err := e.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{}); !errors.Is(err, memoeffectgate.ErrNilEngine) {
		t.Fatalf("%v", err)
	}
}

func TestOtherHolderOccupied(t *testing.T) {
	eng, book, _, mux := testEng(t)
	tk := pendingTicket(book, "ETH", "occ", "m")
	k := mux.OutputKey("ETH", "occ", 0)
	if _, _, err := mux.Acquire(context.Background(), k, "other-leg", "x"); err != nil {
		t.Fatal(err)
	}
	_, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "ETH", Actor: "ops",
	})
	if !errors.Is(err, memoeffectgate.ErrOutputOccupied) {
		t.Fatalf("%v", err)
	}
}

func TestDecideNoCreditRejectOK(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := pendingTicket(book, "ETH", "nc", "m")
	resp, err := eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
		TicketID:	tk.ID, Status: "rejected", Actor: "ops",
	})
	if err != nil || !resp.OK || resp.Ticket.Status != "rejected" {
		t.Fatalf("%v %#v", err, resp)
	}
}

func TestWorkersNoopEmpty(t *testing.T) {
	eng, _, _, _ := testEng(t)
	if n, err := eng.PatrolHalfCut(context.Background(), 10); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if n, err := eng.PatrolRejectAfterCredit(context.Background(), 10); err != nil {
		t.Fatal(err)
	} else if n < 0 {
		t.Fatal(n)
	}
	if n, err := eng.ExportOpenProofs(context.Background(), 10); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestHealth(t *testing.T) {
	eng, _, _, _ := testEng(t)
	h := eng.Health(context.Background())
	if h["ok"] != true {
		t.Fatalf("%v", h)
	}
}

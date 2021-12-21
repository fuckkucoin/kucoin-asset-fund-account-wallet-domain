package usageleasegate_test

import (
	"context"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/usageleasegate"
)

func TestSnapshotTable(t *testing.T) {
	eng := testEng(t)
	cases := []struct {
		name	string
		req	usageleasegate.SnapshotWithLeaseReq
		wantErr	string
	}{
		{"ok_withdraw", snapReq("TA1", "USDT", "x", "1", "100", "30s"), ""},
		{"ok_deposit", usageleasegate.SnapshotWithLeaseReq{Kind: usage.KindDeposit, UserID: "TA2", Asset: "BTC", Dest: "", Amount: "0.1", DailyCap: "10", Reserve: true}, ""},
		{"ok_transfer", usageleasegate.SnapshotWithLeaseReq{Kind: usage.KindTransfer, UserID: "TA3", Asset: "ETH", Dest: "book", Amount: "2", DailyCap: "20", Reserve: true}, ""},
		{"empty_user", usageleasegate.SnapshotWithLeaseReq{Kind: usage.KindWithdraw, Asset: "USDT", Amount: "1", DailyCap: "10"}, "user"},
		{"empty_asset", usageleasegate.SnapshotWithLeaseReq{Kind: usage.KindWithdraw, UserID: "TA4", Amount: "1", DailyCap: "10"}, "asset"},
		{"empty_kind", usageleasegate.SnapshotWithLeaseReq{UserID: "TA5", Asset: "USDT", Amount: "1", DailyCap: "10"}, "kind"},
		{"bad_kind", usageleasegate.SnapshotWithLeaseReq{Kind: "loan", UserID: "TA6", Asset: "USDT", Amount: "1", DailyCap: "10"}, "kind"},
		{"zero_amt", snapReq("TA7", "USDT", "x", "0", "10", "1m"), "amount"},
		{"neg_amt", snapReq("TA8", "USDT", "x", "-1", "10", "1m"), "amount"},
		{"bad_cap", snapReq("TA9", "USDT", "x", "1", "0", "1m"), "cap"},
		{"bad_ttl", snapReq("TA10", "USDT", "x", "1", "10", "-1s"), "ttl"},
		{"sci", snapReq("TA11", "USDT", "x", "1e2", "1000", "1m"), "amount"},
		{"vel_neg", usageleasegate.SnapshotWithLeaseReq{Kind: usage.KindWithdraw, UserID: "TA12", Asset: "USDT", Amount: "1", DailyCap: "10", VelocityCap: -1}, "velocity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eng.SnapshotWithLease(context.Background(), tc.req)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErr) {
				t.Fatalf("want %s got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRecordTable(t *testing.T) {
	eng := testEng(t)
	ok, err := eng.SnapshotWithLease(context.Background(), snapReq("TR1", "USDT", "rd", "3", "50", "2m"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	ok.LeaseID, Kind: usage.KindWithdraw, UserID: "TR1", Asset: "USDT", Dest: "rd", Amount: "3",
	}); err != nil {
		t.Fatal(err)
	}
	rejects := []usageleasegate.RecordAssertReq{
		{Kind: usage.KindWithdraw, UserID: "TR2", Asset: "USDT", Dest: "rd", Amount: "1"},
		{LeaseID: "no-such", Kind: usage.KindWithdraw, UserID: "TR1", Asset: "USDT", Dest: "rd", Amount: "3"},
		{LeaseID: ok.LeaseID, Kind: usage.KindWithdraw, UserID: "TRX", Asset: "USDT", Dest: "rd", Amount: "3"},
		{LeaseID: ok.LeaseID, Kind: usage.KindWithdraw, UserID: "TR1", Asset: "USDT", Dest: "rd", Amount: "3"},
	}
	for i, req := range rejects {
		if _, err := eng.RecordAssert(context.Background(), req); err == nil {
			t.Fatalf("row %d should fail", i)
		}
	}
}

func TestFSMTable(t *testing.T) {
	m := usageleasegate.LeaseStateMachine()
	type edge struct {
		from, ev, to	string
		ok		bool
	}
	edges := []edge{
		{string(usageleasegate.LeaseSnapshotted), usageleasegate.LeaseEvReserve, string(usageleasegate.LeaseReserved), true},
		{string(usageleasegate.LeaseReserved), usageleasegate.LeaseEvCommit, string(usageleasegate.LeaseCommitted), true},
		{string(usageleasegate.LeaseSnapshotted), usageleasegate.LeaseEvExpire, string(usageleasegate.LeaseExpired), true},
		{string(usageleasegate.LeaseSnapshotted), usageleasegate.LeaseEvRevoke, string(usageleasegate.LeaseRevoked), true},
		{string(usageleasegate.LeaseReserved), usageleasegate.LeaseEvExpire, string(usageleasegate.LeaseExpired), true},
		{string(usageleasegate.LeaseReserved), usageleasegate.LeaseEvRevoke, string(usageleasegate.LeaseRevoked), true},
		{string(usageleasegate.LeaseCommitted), usageleasegate.LeaseEvCommit, string(usageleasegate.LeaseCommitted), false},
		{string(usageleasegate.LeaseExpired), usageleasegate.LeaseEvReserve, string(usageleasegate.LeaseReserved), false},
		{string(usageleasegate.LeaseRevoked), usageleasegate.LeaseEvCommit, string(usageleasegate.LeaseCommitted), false},
		{string(usageleasegate.LeaseSnapshotted), usageleasegate.LeaseEvCommit, string(usageleasegate.LeaseCommitted), false},
	}
	for _, e := range edges {
		next, err := m.Apply(e.from, e.ev)
		if e.ok {
			if err != nil || next != e.to {
				t.Fatalf("%s+%s => %s %v", e.from, e.ev, next, err)
			}
		} else if err == nil {
			t.Fatalf("illegal allowed %s+%s", e.from, e.ev)
		}
	}
	if !usageleasegate.CanReserve(usageleasegate.LeaseSnapshotted) || usageleasegate.CanReserve(usageleasegate.LeaseReserved) {
		t.Fatal("can reserve")
	}
	if !usageleasegate.CanCommit(usageleasegate.LeaseReserved) || usageleasegate.CanCommit(usageleasegate.LeaseSnapshotted) {
		t.Fatal("can commit")
	}
	if !usageleasegate.IsTerminal(usageleasegate.LeaseCommitted) || usageleasegate.IsLive(usageleasegate.LeaseExpired) {
		t.Fatal("term")
	}
	if !usageleasegate.CanExpire(usageleasegate.LeaseReserved) || !usageleasegate.CanRevoke(usageleasegate.LeaseSnapshotted) {
		t.Fatal("expire/revoke")
	}
}

func TestWindowKeyAndKinds(t *testing.T) {
	if usageleasegate.WindowKey(usage.KindWithdraw, "u", "usdt", "d") != "withdraw|u|USDT|d" {
		t.Fatal(usageleasegate.WindowKey(usage.KindWithdraw, "u", "usdt", "d"))
	}
	for _, k := range []usage.Kind{usage.KindWithdraw, usage.KindDeposit, usage.KindTransfer} {
		if _, err := usageleasegate.NormalizeKind(k); err != nil {
			t.Fatal(k, err)
		}
	}
}

func TestWouldExceedMatrix(t *testing.T) {
	rows := []struct {
		today, res, att, cap	string
		ex			bool
	}{
		{"0", "0", "10", "100", false},
		{"90", "0", "20", "100", true},
		{"50", "40", "20", "100", true},
		{"50", "40", "10", "100", false},
		{"100", "0", "1", "100", true},
		{"0", "0", "1", "1", false},
		{"0", "1", "1", "1", true},
		{"1", "0", "1", "1", true},
	}
	for i, r := range rows {
		if usageleasegate.WouldExceed(mustZeroable(r.today), mustZeroable(r.res), mustPos(t, r.att), mustPos(t, r.cap)) != r.ex {
			t.Fatalf("row %d %+v", i, r)
		}
	}
}

func mustZeroable(s string) money.Amount {
	if s == "0" || s == "" {
		return money.Zero()
	}
	a, err := money.Parse(s)
	if err != nil {
		panic(err)
	}
	return a
}

func mustPos(t *testing.T, s string) money.Amount {
	t.Helper()
	a, err := money.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDriftKindCoverage(t *testing.T) {
	kinds := []usageleasegate.DriftKind{usageleasegate.DriftWindowChanged, usageleasegate.DriftFingerprint, usageleasegate.DriftExpired, usageleasegate.DriftMissingLease, usageleasegate.DriftWouldExceedCap, usageleasegate.DriftConcurrentOverCap, usageleasegate.DriftReservationExpire, usageleasegate.DriftNotReserved, usageleasegate.DriftRevoked, usageleasegate.DriftOwner, usageleasegate.DriftBinding, usageleasegate.DriftAlreadyCommitted, usageleasegate.DriftVelocity, usageleasegate.DriftAlreadyBreached,
	}
	if len(kinds) < 10 {
		t.Fatal("kinds")
	}
	for _, k := range kinds {
		if k == "" {
			t.Fatal("empty kind")
		}
	}
}

func TestMemStoreRoundtrip(t *testing.T) {
	ctx := context.Background()
	st := usageleasegate.NewMemStore()
	l := usageleasegate.UsageLease{ID: "L1", UserID: "U", Kind: usage.KindWithdraw, Asset: "USDT", State: usageleasegate.LeaseSnapshotted, Idem: "i1"}
	if _, err := st.SaveLease(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetLease(ctx, "L1")
	if err != nil || got.UserID != "U" {
		t.Fatal(got, err)
	}
	if _, err := st.GetLeaseByIdem(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	r := usageleasegate.UsageReservation{ID: "R1", LeaseID: "L1", WindowKey: "k", State: usageleasegate.ResHeld, Amount: "1"}
	if _, err := st.SaveReservation(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetReservation(ctx, "R1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetReservationByLease(ctx, "L1"); err != nil {
		t.Fatal(err)
	}
	p := usageleasegate.UsageDriftProof{ID: "P1", LeaseID: "L1", UserID: "U", Kind: usageleasegate.DriftExpired}
	if _, err := st.SaveProof(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetProof(ctx, "P1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveAudit(ctx, usageleasegate.UsageLeaseAudit{ID: "A1", LeaseID: "L1", Event: "x"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountByState(ctx, usageleasegate.LeaseSnapshotted); n != 1 {
		t.Fatal(n)
	}
	if n, _ := st.CountProofs(ctx); n != 1 {
		t.Fatal(n)
	}
	if n, _ := st.CountAudits(ctx); n != 1 {
		t.Fatal(n)
	}
	if n, _ := st.NextSeq(ctx, "k"); n != 1 {
		t.Fatal(n)
	}
}

func TestVelocityCapRevoke(t *testing.T) {
	eng := testEng(t)
	_ = eng.Usage().Add(usage.KindWithdraw, "TV1", "USDT", "d", "1")
	_ = eng.Usage().Add(usage.KindWithdraw, "TV1", "USDT", "d", "1")
	_, err := eng.SnapshotWithLease(context.Background(), usageleasegate.SnapshotWithLeaseReq{
		Kind:	usage.KindWithdraw, UserID: "TV1", Asset: "USDT", Dest: "d",
		Amount:	"1", DailyCap: "100", VelocityCap: 1, Reserve: true,
	})
	if err == nil {
		t.Fatal("velocity")
	}
}

func TestUsageSurfaceWouldExceed(t *testing.T) {
	s := usage.New()
	ok, err := s.WouldExceedDaily(usage.KindWithdraw, "UW1", "USDT", "60", "100")
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	_ = s.Add(usage.KindWithdraw, "UW1", "USDT", "d", "60")
	ok, err = s.WouldExceedDaily(usage.KindWithdraw, "UW1", "USDT", "50", "100")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if s.WindowFingerprint(usage.KindWithdraw, "UW1", "USDT", "d") == "" {
		t.Fatal("print")
	}
	if usage.CapFingerprint("100", 3) == "" {
		t.Fatal("cap")
	}
}

func TestRecordIfAccepted(t *testing.T) {
	s := usage.New()
	if err := s.RecordIfAccepted(usage.KindWithdraw, "UA1", "USDT", "d", "1", false); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 0 {
		t.Fatal("rejected counted")
	}
	if err := s.RecordIfAccepted(usage.KindWithdraw, "UA1", "USDT", "d", "1", true); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatal(s.Len())
	}
}

func TestRecordWithLeaseNilBinder(t *testing.T) {
	s := usage.New()
	if err := s.RecordWithLease(nil, "", usage.KindWithdraw, "UB1", "USDT", "d", "1"); err != usage.ErrLeaseRequired {
		t.Fatalf("%v", err)
	}
}

func TestBinderAssert(t *testing.T) {
	eng := testEng(t)
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("UB2", "USDT", "d", "1", "10", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	b := usageleasegate.Binder{Eng: eng}
	if err := b.AssertRecord(s.LeaseID, "withdraw", "UB2", "USDT", "d", "1"); err != nil {
		t.Fatal(err)
	}
}

func TestHeldSum(t *testing.T) {
	held := []usageleasegate.UsageReservation{
		{State: usageleasegate.ResHeld, Amount: "10"},
		{State: usageleasegate.ResHeld, Amount: "5"},
		{State: usageleasegate.ResReleased, Amount: "9"},
		{State: usageleasegate.ResHeld, Amount: "bad"},
	}
	if usageleasegate.SumHeld(held).String() != "15" {
		t.Fatal(usageleasegate.SumHeld(held))
	}
	if usageleasegate.HeldExcept(held, "x").String() != "15" {
		t.Fatal(usageleasegate.HeldExcept(held, "x"))
	}
}

func TestFailClosedStates(t *testing.T) {
	e := testEng(t)
	now := e.now()
	if _, err := e.failClosedLease(usageleasegate.UsageLease{}, now); err == nil {
		t.Fatal("empty")
	}
	if _, err := e.failClosedLease(usageleasegate.UsageLease{ID: "x", State: usageleasegate.LeaseExpired}, now); err == nil {
		t.Fatal("expired")
	}
	if _, err := e.failClosedLease(usageleasegate.UsageLease{ID: "x", State: usageleasegate.LeaseRevoked}, now); err == nil {
		t.Fatal("revoked")
	}
	if _, err := e.failClosedLease(usageleasegate.UsageLease{ID: "x", State: usageleasegate.LeaseCommitted}, now); err == nil {
		t.Fatal("committed")
	}
}

func TestSQLStoreNilDB(t *testing.T) {
	s := usageleasegate.NewSQLStore(nil)
	if _, err := s.SaveLease(context.Background(), usageleasegate.UsageLease{ID: "x"}); err != usageleasegate.ErrStoreUnavailable {
		t.Fatal(err)
	}
	if _, err := s.GetLease(context.Background(), "x"); err != usageleasegate.ErrStoreUnavailable {
		t.Fatal(err)
	}
}

func TestWrapDrift(t *testing.T) {
	err := usageleasegate.WrapDrift(usageleasegate.DriftExpired, usageleasegate.ErrLeaseExpired)
	if err == nil || !strings.Contains(err.Error(), string(usageleasegate.DriftExpired)) {
		t.Fatal(err)
	}
}

func TestNormalizeKindOrEmpty(t *testing.T) {
	if usageleasegate.NormalizeKindOrEmpty("withdraw") != usage.KindWithdraw {
		t.Fatal("withdraw")
	}
	if usageleasegate.NormalizeKindOrEmpty("zzz") != usage.Kind("zzz") {
		t.Fatal("passthrough")
	}
}

func TestWorkersJobs(t *testing.T) {
	eng := testEng(t)
	j1 := usageleasegate.NewExpireReleaseJob(eng, usageleasegate.WorkerConfig{}, nopLog())
	j2 := usageleasegate.NewWindowDriftPatrolJob(eng, usageleasegate.WorkerConfig{}, nopLog())
	j3 := usageleasegate.NewOverCapDiscoveryJob(eng, usageleasegate.WorkerConfig{}, nopLog())
	if err := j1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := j2(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := j3(context.Background()); err != nil {
		t.Fatal(err)
	}
}

package ledshard_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/ledshard"
)

func boot(t *testing.T) *ledshard.Engine {
	t.Helper()
	e := ledshard.New()
	if err := e.Bootstrap(8); err != nil {
		t.Fatal(err)
	}
	return e
}

func pair(t *testing.T, e *ledshard.Engine,) (src, dst string) {
	t.Helper()
	src, dst, _, _, err := e.FindPair("USDT")
	if err != nil {
		t.Fatal(err)
	}
	return src, dst
}

func TestCrossShardConserveAndReplay(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	if _, err := e.Seed(src, "funding", "USDT", "100"); err != nil {
		t.Fatal(err)
	}
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"ok-1", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "40",
	})
	if err != nil {
		t.Fatal(err)
	}
	if it.SrcShard == it.DstShard {
		t.Fatal("fixture users landed on one shard")
	}
	done, err := e.Run(it.ID)
	if err != nil {
		t.Fatalf("run %s %v", done.State, err)
	}
	if done.State != ledshard.StateCommitted || done.CreditCount != 1 {
		t.Fatalf("state %+v", done)
	}
	rep, err := e.Conserve(done.ID)
	if err != nil || !rep.OK {
		t.Fatalf("conserve %+v %v", rep, err)
	}
	if rep.SrcOut != "40" || rep.DstIn != "40" || rep.BridgeNet != "0" {
		t.Fatalf("nets %+v", rep)
	}
	v, err := e.Voucher(done.VoucherID)
	if err != nil || v.State != ledshard.VoucherConsumed || v.CreditCount != 1 {
		t.Fatalf("voucher %+v %v", v, err)
	}
	srcBal := e.BalanceOn(done.SrcShard, done.SrcSubject, "USDT")
	dstBal := e.BalanceOn(done.DstShard, done.DstSubject, "USDT")
	br := e.BalanceOn(ledshard.ShardBridge, ledshard.SubBridge, "USDT")
	if srcBal.Available != "60" || srcBal.Frozen != "0" || dstBal.Available != "40" || br.Available != "0" {
		t.Fatalf("bals src=%+v dst=%+v br=%+v", srcBal, dstBal, br)
	}
	again, err := e.Run(done.ID)
	if err != nil || again.CreditCount != 1 {
		t.Fatalf("replay %+v %v", again, err)
	}
	dstBal = e.BalanceOn(done.DstShard, done.DstSubject, "USDT")
	if dstBal.Available != "40" {
		t.Fatalf("double credit %s", dstBal.Available)
	}
	same, err := e.Open(ledshard.OpenReq{
		Idem:	"ok-1", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "40",
	})
	if err != nil || same.ID != done.ID {
		t.Fatalf("idem %v %+v", err, same)
	}
	_, err = e.Open(ledshard.OpenReq{
		Idem:	"ok-1", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "1",
	})
	if !errors.Is(err, ledshard.ErrIdemConflict) {
		t.Fatalf("conflict %v", err)
	}
	if len(e.Hints()) == 0 {
		t.Fatal("missing edge hints")
	}
	steps := e.Steps(done.ID)
	if len(steps) < 5 || steps[0].LedgerTo != string(ledger.StateSpotFrozen) {
		t.Fatalf("steps %+v", steps)
	}
	out, err := money.Parse(rep.SrcOut)
	if err != nil {
		t.Fatal(err)
	}
	in, err := money.Parse(rep.DstIn)
	if err != nil {
		t.Fatal(err)
	}
	if err := money.Conserve(out, in); err != nil {
		t.Fatal(err)
	}
}

func TestKillPreparingNoFreezeNoCredit(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	if _, err := e.Seed(src, "funding", "USDT", "25"); err != nil {
		t.Fatal(err)
	}
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"kill-1", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "trading",
		Asset:	"USDT", Amount: "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	it, err = e.Advance(it.ID)
	if err != nil || it.State != ledshard.StatePreparing || it.FreezePosted {
		t.Fatalf("prepare %+v %v", it, err)
	}
	if _, err := e.Kill(it.ID); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Tick()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Aborted != 1 {
		t.Fatalf("tick %+v", rep)
	}
	got, err := e.Get(it.ID)
	if err != nil || got.State != ledshard.StateAborted || got.CreditCount != 0 || got.VoucherID != "" {
		t.Fatalf("after %+v", got)
	}
	bal := e.BalanceOn(got.SrcShard, got.SrcSubject, "USDT")
	if bal.Available != "25" || bal.Frozen != "0" {
		t.Fatalf("frozen stuck %+v", bal)
	}
	dstBal := e.BalanceOn(got.DstShard, got.DstSubject, "USDT")
	if dstBal.Available != "0" {
		t.Fatalf("credited %s", dstBal.Available)
	}
	if _, err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	dstBal = e.BalanceOn(got.DstShard, got.DstSubject, "USDT")
	if dstBal.Available != "0" {
		t.Fatal("second tick credited")
	}
	crep, err := e.Conserve(got.ID)
	if err != nil || !crep.OK {
		t.Fatalf("conserve %+v %v", crep, err)
	}
}

func TestRouteWindowPinsOldIntent(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	if _, err := e.Seed(src, "funding", "USDT", "10"); err != nil {
		t.Fatal(err)
	}
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"old", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	pinnedSrc, pinnedDst := it.SrcShard, it.DstShard
	if err != nil {
		t.Fatal(err)
	}
	if done.SrcShard != pinnedSrc || done.DstShard != pinnedDst || done.RouteVersion != 1 {
		t.Fatalf("rewritten %+v", done)
	}
	for _, p := range e.Postings(done.ID) {
		if p.ShardID != pinnedSrc && p.ShardID != pinnedDst && p.ShardID != ledshard.ShardBridge {
			t.Fatalf("posting on foreign shard %+v", p)
		}
	}
	if _, err := e.Activate(draft.Version); err != nil {
		t.Fatal(err)
	}
	again, err := e.Get(it.ID)
	if err != nil || again.SrcShard != pinnedSrc {
		t.Fatalf("activate moved old intent %+v", again)
	}
	fresh, err := e.Open(ledshard.OpenReq{
		Idem:	"after", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.RouteVersion != draft.Version {
		t.Fatalf("new route %d", fresh.RouteVersion)
	}
	if fresh.SrcShard == pinnedSrc && fresh.DstShard == pinnedDst {
		t.Fatal("reversed ring did not move placement")
	}
}

func TestSystemSubjectsStayPinned(t *testing.T) {
	e := boot(t)
	checks := []struct{ user, shard string }{
		{ledshard.SubBridge, ledshard.ShardBridge},
		{ledshard.SubFee, ledshard.ShardFee},
		{ledshard.SubCold, ledshard.ShardCold},
	}
	for _, c := range checks {
		a, err := e.ResolveSubject(0, c.user, "", "USDT")
		if err != nil || a.ShardID != c.shard || a.Pin != c.user || a.Hashed {
			t.Fatalf("%s %+v %v", c.user, a, err)
		}
	}
	if _, err := e.Draft("btc", nil, map[string]string{"BTC": "sh-00"}); err != nil {
		t.Fatal(err)
	}
	routes := e.ListRoutes()
	var draft int64
	for _, r := range routes {
		if r.Status == ledshard.RouteDraft {
			draft = r.Version
		}
	}
	if _, err := e.Publish(draft); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activate(draft); err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		a, err := e.ResolveSubject(0, c.user, "funding", "BTC")
		if err != nil || a.ShardID != c.shard {
			t.Fatalf("affinity scattered %s %+v %v", c.user, a, err)
		}
	}
	alice, err := e.ResolveSubject(0, "alice", "funding", "BTC")
	if err != nil || alice.ShardID != "sh-00" || alice.Affinity != "BTC" {
		t.Fatalf("affinity %+v %v", alice, err)
	}
	route := e.ListRoutes()
	var active ledshard.RouteVersion
	for _, r := range route {
		if r.Status == ledshard.RouteActive {
			active = r
		}
	}
	for i := 0; i < 1500; i++ {
		user := "user-" + strconvI(i)
		a, err := ledshard.Resolve(active, user, "funding", "ETH")
		if err != nil {
			t.Fatal(err)
		}
		if a.ShardID == ledshard.ShardBridge || a.ShardID == ledshard.ShardFee || a.ShardID == ledshard.ShardCold {
			t.Fatalf("hash leaked %s -> %s", user, a.ShardID)
		}
	}
	if _, err := e.Draft("bad", []string{ledshard.ShardBridge, "sh-01"}, nil); !errors.Is(err, ledshard.ErrPinLeak) {
		t.Fatalf("pin leak %v", err)
	}
}

func TestDeskUnfreezesWithoutCredit(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	if _, err := e.Seed(src, "funding", "USDT", "9"); err != nil {
		t.Fatal(err)
	}
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"desk-1", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Advance(it.ID); err != nil {
		t.Fatal(err)
	}
	it, err = e.Advance(it.ID)
	if err != nil || it.State != ledshard.StateFrozen {
		t.Fatalf("freeze %+v %v", it, err)
	}
	held := e.BalanceOn(it.SrcShard, it.SrcSubject, "USDT")
	if held.Frozen != "4" || held.Available != "5" {
		t.Fatalf("hold %+v", held)
	}
	parked, err := e.Park(it.ID, "operator hold")
	if err != nil || parked.State != ledshard.StateDesk || parked.DeskID == "" {
		t.Fatalf("park %+v %v", parked, err)
	}
	if _, err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	still, _ := e.Get(it.ID)
	if still.State != ledshard.StateDesk || still.CreditCount != 0 {
		t.Fatalf("tick moved %+v", still)
	}
	if _, err := e.ApproveDesk(parked.DeskID, "ledshard-worker"); !errors.Is(err, ledshard.ErrActor) {
		t.Fatalf("worker approve %v", err)
	}
	if _, err := e.ApproveDesk(parked.DeskID, "ada"); err != nil {
		t.Fatalf("approve %v", err)
	}
	d, err := e.GetDesk(parked.DeskID)
	if err != nil || d.State != ledshard.DeskApproved || d.ApprovedBy != "ada" {
		t.Fatalf("desk %+v %v", d, err)
	}
	if _, err := e.ExecuteDesk(parked.DeskID, "ada"); !errors.Is(err, ledshard.ErrDual) {
		t.Fatalf("one human %v", err)
	}
	out, err := e.ExecuteDesk(parked.DeskID, "bea")
	if err != nil {
		t.Fatal(err)
	}
	if out.State != ledshard.StateCompensated || out.CreditCount != 0 {
		t.Fatalf("exec %+v", out)
	}
	bal := e.BalanceOn(out.SrcShard, out.SrcSubject, "USDT")
	if bal.Available != "9" || bal.Frozen != "0" {
		t.Fatalf("restored %+v", bal)
	}
	dstBal := e.BalanceOn(out.DstShard, out.DstSubject, "USDT")
	if dstBal.Available != "0" {
		t.Fatalf("target %s", dstBal.Available)
	}
}

func TestDriftDoesNotRewriteShard(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	if _, err := e.Seed(src, "funding", "USDT", "8"); err != nil {
		t.Fatal(err)
	}
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"drift", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Advance(it.ID); err != nil {
		t.Fatal(err)
	}
	before := e.Postings(it.ID)
	if _, err := e.DrillMutateRing(it.RouteVersion); err != nil {
		t.Fatal(err)
	}
	got, err := e.Advance(it.ID)
	if !errors.Is(err, ledshard.ErrDrift) || got.State != ledshard.StateDesk {
		t.Fatalf("drift %+v %v", got, err)
	}
	if got.SrcShard != it.SrcShard || got.DstShard != it.DstShard {
		t.Fatalf("shard rewritten %+v", got)
	}
	if len(e.Postings(it.ID)) != len(before) {
		t.Fatal("drift wrote postings")
	}
	if e.Healthy() {
		t.Fatal("drift should fail health")
	}
}

func TestSameUserShortCircuit(t *testing.T) {
	e := boot(t)
	plan, err := e.SettleHop("alice", "funding", "trading", "USDT", "1", "hop-1")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SameShard || plan.IntentID != "" || plan.SrcShard == "" || plan.SrcShard != plan.DstShard {
		t.Fatalf("plan %+v", plan)
	}
	if plan.SrcShard == ledshard.ShardBridge || plan.SrcShard == ledshard.ShardFee || plan.SrcShard == ledshard.ShardCold {
		t.Fatal("user on system shard")
	}
	_, err = e.Open(ledshard.OpenReq{
		Idem:	"same", SrcUser: "alice", SrcBook: "funding", DstUser: "alice", DstBook: "trading",
		Asset:	"USDT", Amount: "1",
	})
	if !errors.Is(err, ledshard.ErrSameShard) {
		t.Fatalf("open same %v", err)
	}
}

func TestLedgerApplyStaysReadOnly(t *testing.T) {
	next, err := ledshard.AdmitLedgerPhase(ledger.StateInitiated, ledger.EventFreezeSpot)
	if err != nil || next != ledger.StateSpotFrozen {
		t.Fatalf("admit %s %v", next, err)
	}
	if _, err := ledshard.AdmitLedgerPhase(ledger.StateCredited, ledger.EventFreezeSpot); err == nil {
		t.Fatal("skip credit must fail")
	}
	if _, err := ledshard.AdmitLedgerPhase(ledger.StateCredited, ledger.EventCredit); err == nil {
		t.Fatal("second credit phase must fail")
	}
}

func TestInsufficientFreezeWritesNothing(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"short", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Advance(it.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.Advance(it.ID)
	if !errors.Is(err, ledshard.ErrInsufficient) {
		t.Fatalf("freeze %v %+v", err, got)
	}
	if len(e.Postings(it.ID)) != 0 {
		t.Fatal("insufficient freeze wrote postings")
	}
	bal := e.BalanceOn(got.SrcShard, got.SrcSubject, "USDT")
	if bal.Available != "0" || bal.Frozen != "0" || got.FreezePosted {
		t.Fatalf("balance moved %+v %+v", bal, got)
	}
}

func TestCompensateVoucherDoesNotCredit(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	if _, err := e.Seed(src, "funding", "USDT", "12"); err != nil {
		t.Fatal(err)
	}
	it, err := e.Open(ledshard.OpenReq{
		Idem:	"void-1", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "5",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		it, err = e.Advance(it.ID)
		if err != nil {
			t.Fatalf("step %d %v %+v", i, err, it)
		}
	}
	if it.State != ledshard.StateVouchered || it.CreditCount != 0 {
		t.Fatalf("vouchered %+v", it)
	}
	out, err := e.Compensate(it.ID, "kill after voucher")
	if err != nil {
		t.Fatal(err)
	}
	if out.State != ledshard.StateCompensated || out.CreditCount != 0 {
		t.Fatalf("comp %+v", out)
	}
	v, err := e.Voucher(out.VoucherID)
	if err != nil || v.State != ledshard.VoucherVoided || v.CreditCount != 0 {
		t.Fatalf("voucher %+v %v", v, err)
	}
	srcBal := e.BalanceOn(out.SrcShard, out.SrcSubject, "USDT")
	dstBal := e.BalanceOn(out.DstShard, out.DstSubject, "USDT")
	br := e.BalanceOn(ledshard.ShardBridge, ledshard.SubBridge, "USDT")
	if srcBal.Available != "12" || srcBal.Frozen != "0" || dstBal.Available != "0" || br.Available != "0" {
		t.Fatalf("src=%+v dst=%+v br=%+v", srcBal, dstBal, br)
	}
	if _, err := e.Advance(out.ID); !errors.Is(err, ledshard.ErrTerminal) {
		t.Fatalf("replay advance %v", err)
	}
	dstBal = e.BalanceOn(out.DstShard, out.DstSubject, "USDT")
	if dstBal.Available != "0" {
		t.Fatal("compensate replay credited")
	}
}

func TestExpectShardMismatch(t *testing.T) {
	e := boot(t)
	src, dst := pair(t, e)
	_, err := e.Open(ledshard.OpenReq{
		Idem:	"mis", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		Asset:	"USDT", Amount: "1", ExpectSrc: ledshard.ShardBridge,
	})
	if !errors.Is(err, ledshard.ErrShardMismatch) {
		t.Fatalf("mismatch %v", err)
	}
	if len(e.ListIntents()) != 0 {
		t.Fatal("mismatch wrote an intent")
	}
}

func strconvI(i int) string {
	return string(rune('0'+i%10)) + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [16]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

package scoreleasegate_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/scoreleasegate"
)

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", "scoreleasegate", name)
}

func allowEval() scoreleasegate.FixedEval {
	return scoreleasegate.FixedEval{Snap: scoreleasegate.EvalSnap{Allow: true, Mode: "enforce", Pack: "kyc0"}}
}

func newTestSvc() *scoreleasegate.Service {
	return scoreleasegate.NewService("test-secret-r18", allowEval(), scoreleasegate.NewMemStore())
}

func issueReq(user, chain, dest, asset, amt, nonce string) scoreleasegate.IssueReq {
	return scoreleasegate.IssueReq{
		UserID:	user, Chain: chain, Dest: dest, Asset: asset, Amount: amt,
		Pack:	"kyc0", Kind: "withdraw", Nonce: nonce,
	}
}

func submitReq(id, user, chain, dest, asset, amt, ticket, actor string, consume bool) scoreleasegate.SubmitReq {
	return scoreleasegate.SubmitReq{
		ScoreLeaseID:	id, UserID: user, Chain: chain, Dest: dest, Asset: asset,
		Amount:	amt, Ticket: ticket, Actor: actor, Consume: consume,
	}
}

func TestIssueThenConsume(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.EvalWithLease(ctx, issueReq("u-ok-01", "ETH", "0xabc0123456789aaa", "USDT", "10.000000000000000000", "nonce-ok-0001"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Status != scoreleasegate.LeaseIssued || l.ID == "" || l.IsolationMark != scoreleasegate.IsolationMark {
		t.Fatalf("lease=%+v", l)
	}
	got, proof, err := svc.AssertLease(ctx, submitReq(l.ID, l.UserID, l.Chain, l.Dest, l.Asset, l.Amount, "wd-t-ok", "wd-actor-ok", true))
	if err != nil {
		t.Fatalf("submit: %v proof=%+v", err, proof)
	}
	if got.Status != scoreleasegate.LeaseConsumed {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestHaltAfterAllow(t *testing.T) {
	b, err := os.ReadFile(testdata("halt_after_allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID		string	`json:"user_id"`
		Chain		string	`json:"chain"`
		Dest		string	`json:"dest"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Nonce		string	`json:"nonce"`
		Ticket		string	`json:"ticket"`
		Actor		string	`json:"actor"`
		Operator	string	`json:"operator"`
		ExpectProof	string	`json:"expect_proof"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq(g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SetHalt(ctx, scoreleasegate.SetHaltReq{On: true, Actor: g.Operator, Reason: "halt"}); err != nil {
		t.Fatal(err)
	}
	_, proof, err := svc.Submit(ctx, submitReq(l.ID, g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Ticket, g.Actor, true))
	if err == nil {
		t.Fatal("expected fail-closed after halt")
	}
	if proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("proof=%+v want %s", proof, g.ExpectProof)
	}
	if !scoreleasegate.IsFailClosed(err) {
		t.Fatalf("not fail-closed: %v", err)
	}
}

func TestPackAfterAllow(t *testing.T) {
	b, err := os.ReadFile(testdata("pack_after_allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID		string	`json:"user_id"`
		Chain		string	`json:"chain"`
		Dest		string	`json:"dest"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		FlipPack	string	`json:"flip_pack"`
		Nonce		string	`json:"nonce"`
		Ticket		string	`json:"ticket"`
		Actor		string	`json:"actor"`
		Operator	string	`json:"operator"`
		ExpectProof	string	`json:"expect_proof"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq(g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SetPack(ctx, scoreleasegate.SetPackReq{Pack: g.FlipPack, Actor: g.Operator, Reason: "pack"}); err != nil {
		t.Fatal(err)
	}
	_,
		proof, err := svc.Submit(ctx, submitReq(l.ID, g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Ticket, g.Actor, true))
	if err == nil {
		t.Fatal("expected pack flip deny")
	}
	if proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("proof=%+v want %s", proof, g.ExpectProof)
	}
}

func TestModeAfterAllow(t *testing.T) {
	b, err := os.ReadFile(testdata("mode_after_allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID		string	`json:"user_id"`
		Chain		string	`json:"chain"`
		Dest		string	`json:"dest"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Nonce		string	`json:"nonce"`
		Ticket		string	`json:"ticket"`
		Actor		string	`json:"actor"`
		Operator	string	`json:"operator"`
		FlipMode	string	`json:"flip_mode"`
		ExpectProof	string	`json:"expect_proof"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	l,
		err := svc.Issue(ctx, issueReq(g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SetMode(ctx, scoreleasegate.SetModeReq{Mode: g.FlipMode, Actor: g.Operator, Reason: "mode"}); err != nil {
		t.Fatal(err)
	}
	_,
		proof, err := svc.Submit(ctx, submitReq(l.ID, g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Ticket, g.Actor, true))
	if err == nil {
		t.Fatal("expected mode flip deny")
	}
	if proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestWindowAfterAllow(t *testing.T) {
	b, err := os.ReadFile(testdata("window_after_allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID		string	`json:"user_id"`
		Chain		string	`json:"chain"`
		Dest		string	`json:"dest"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Nonce		string	`json:"nonce"`
		Ticket		string	`json:"ticket"`
		Actor		string	`json:"actor"`
		Operator	string	`json:"operator"`
		ExpectProof	string	`json:"expect_proof"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq(g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.BumpWindow(ctx, scoreleasegate.BumpWindowReq{Digest: "jumped-window-digest", Actor: g.Operator, Reason: "win"}); err != nil {
		t.Fatal(err)
	}
	_, proof, err := svc.Submit(ctx, submitReq(l.ID, g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Ticket, g.Actor, true))
	if err == nil {
		t.Fatal("expected window deny")
	}
	if proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestMissingLease(t *testing.T) {
	b, err := os.ReadFile(testdata("missing_lease.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID		string	`json:"user_id"`
		Chain		string	`json:"chain"`
		Dest		string	`json:"dest"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Ticket		string	`json:"ticket"`
		Actor		string	`json:"actor"`
		ExpectProof	string	`json:"expect_proof"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	_, proof, err := svc.Submit(context.Background(), submitReq("", g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Ticket, g.Actor, true))
	if err == nil || proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("err=%v proof=%+v", err, proof)
	}
}

func TestExpireLease(t *testing.T) {
	b, err := os.ReadFile(testdata("expire_lease.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID		string	`json:"user_id"`
		Chain		string	`json:"chain"`
		Dest		string	`json:"dest"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Nonce		string	`json:"nonce"`
		Ticket		string	`json:"ticket"`
		Actor		string	`json:"actor"`
		TTLSeconds	int	`json:"ttl_seconds"`
		ExpectProof	string	`json:"expect_proof"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	now := base
	svc := scoreleasegate.NewService("test-secret-r18", allowEval(), scoreleasegate.NewMemStore()).WithClock(func() time.Time { return now })
	req := issueReq(g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Nonce)
	req.TTLSeconds = g.TTLSeconds
	l,
		err := svc.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(3 * time.Second)
	_, proof, err := svc.Submit(context.Background(), submitReq(l.ID, g.UserID, g.Chain, g.Dest, g.Asset, g.Amount, g.Ticket, g.Actor, true))
	if err == nil || proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("err=%v proof=%+v", err, proof)
	}
}

func TestShadowCannotIssue(t *testing.T) {
	svc := scoreleasegate.NewService("test-secret-r18", scoreleasegate.FixedEval{Snap: scoreleasegate.EvalSnap{Allow: true, Mode: "shadow", Pack: "kyc0"}}, scoreleasegate.NewMemStore())
	_, err := svc.Issue(context.Background(), issueReq("u-score-shd-01", "ETH", "0xscoreshadow01aaa", "USDT", "2.000000000000000000", "nonce-score-shd-01"))
	if err != scoreleasegate.ErrShadowLease {
		t.Fatalf("err=%v", err)
	}
}

func TestIsolationNonce(t *testing.T) {
	svc := newTestSvc()
	_, err := svc.Issue(context.Background(), issueReq("u-score-iso-01", "ETH", "0xscoreiso01aaaaaa", "USDT", "1.000000000000000000", "screen_lease_id-forbidden"))
	if err != scoreleasegate.ErrIsolationViolation {
		t.Fatalf("err=%v", err)
	}
}

func TestIdempotentNonce(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	req := issueReq("u-score-idem-01", "ETH", "0xscoreidem01aaaaa", "USDT", "4.000000000000000000", "nonce-score-idem-01")
	a, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("ids %s vs %s", a.ID, b.ID)
	}
}

func TestAmountDestDrift(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq("u-amt-01", "ETH", "0xscoreamt01aaaaaa", "USDT", "9.000000000000000000", "nonce-score-amt-01"))
	if err != nil {
		t.Fatal(err)
	}
	_, p, err := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Chain, l.Dest, l.Asset, "1.000000000000000000", "wd-amt", "wd-actor-amt", true))
	if err != scoreleasegate.ErrLeaseAmount || p == nil || p.Kind != scoreleasegate.ProofAmountDrift {
		t.Fatalf("err=%v p=%+v", err, p)
	}
	_, p2, err := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Chain, "0xotherdest01aaaa", l.Asset, l.Amount, "wd-dst", "wd-actor-dst", true))
	if err != scoreleasegate.ErrLeaseDest || p2 == nil || p2.Kind != scoreleasegate.ProofDestDrift {
		t.Fatalf("err=%v p=%+v", err, p2)
	}
}

func TestNoAllow(t *testing.T) {
	svc := scoreleasegate.NewService("test-secret-r18", scoreleasegate.FixedEval{Snap: scoreleasegate.EvalSnap{Allow: false, Mode: "enforce"}}, scoreleasegate.NewMemStore())
	_, err := svc.Issue(context.Background(), issueReq("u-deny-01", "ETH", "0xscoredeny01aaaaa", "USDT", "1.000000000000000000", "nonce-score-deny-01"))
	if err != scoreleasegate.ErrEvalDenied {
		t.Fatalf("err=%v", err)
	}
}

func TestQuota(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	for i := 0; i < scoreleasegate.MaxOpenLeasesPerUser; i++ {
		nonce := "nonce-q-" + string(rune('a'+i)) + "xxxxxxx"
		if _, err := svc.Issue(ctx, issueReq("u-quota-01", "ETH", "0xscorequota01aaaa", "USDT", "1.000000000000000000", nonce)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := svc.Issue(ctx, issueReq("u-quota-01", "ETH", "0xscorequota01aaaa", "USDT", "1.000000000000000000", "nonce-q-overflowx"))
	if err != scoreleasegate.ErrLeaseQuota {
		t.Fatalf("err=%v", err)
	}
}

func TestConcurrentBind(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq("u-score-race-01", "ETH", "0xscorerace01aaaaa", "USDT", "6.000000000000000000", "nonce-score-race-01"))
	if err != nil {
		t.Fatal(err)
	}
	hold := make(chan struct{})
	svc.hold.TryLock(l.ID)
	var denied atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-hold
		_, _, err := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Chain, l.Dest, l.Asset, l.Amount, "wd-score-race", "wd-actor-race", true))
		if err == scoreleasegate.ErrConcurrent {
			denied.Add(1)
		}
	}()
	close(hold)
	wg.Wait()
	svc.hold.Unlock(l.ID)
	if denied.Load() != 1 {
		t.Fatalf("denied=%d", denied.Load())
	}
}

func TestFourEyeAndExport(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq("u-eye-01", "ETH", "0xscoreeye01aaaaaa", "USDT", "1.000000000000000000", "nonce-score-eye-01"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = svc.SetHalt(ctx, scoreleasegate.SetHaltReq{On: true, Actor: "ops-a1", Reason: "halt"})
	_, proof, err := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Chain, l.Dest, l.Asset, l.Amount, "wd-eye", "wd-actor-eye", true))
	if err == nil || proof == nil {
		t.Fatal("want proof")
	}
	closed, err := svc.FourEye(ctx, scoreleasegate.FourEyeReq{ProofID: proof.ID, ActorA: "ops-left", ActorB: "ops-right", Reason: "clear-halt"})
	if err != nil || closed.Status != scoreleasegate.ProofClosed {
		t.Fatalf("err=%v closed=%+v", err, closed)
	}
	ex, err := svc.Export(ctx, l.ID)
	if err != nil || ex.Mark != scoreleasegate.IsolationMark || ex.Host != "kyt.kucoin.net" {
		t.Fatalf("export=%+v err=%v", ex, err)
	}
	sum := scoreleasegate.SummarizeProof(*proof)
	if sum.Kind != string(scoreleasegate.ProofHaltFlip) {
		t.Fatalf("sum=%+v", sum)
	}
}

func TestCasesCSV(t *testing.T) {
	f, err := os.Open(testdata("cases.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 10 {
		t.Fatalf("rows=%d", len(rows))
	}
	for _, row := range rows[1:] {
		if len(row) < 3 {
			t.Fatalf("row=%v", row)
		}
		if row[2] != "ok" && row[2] != "deny" {
			t.Fatalf("expect=%s", row[2])
		}
	}
}

func TestFSMAndCatalog(t *testing.T) {
	for _, e := range scoreleasegate.FSMEdges() {
		if e.OK != scoreleasegate.TransitionAllowed(e.From, e.To) && e.From != e.To {
			if e.OK && !scoreleasegate.TransitionAllowed(e.From, e.To) {
				t.Fatalf("edge %+v", e)
			}
		}
	}
	if !scoreleasegate.IllegalRollback(scoreleasegate.LeaseConsumed, scoreleasegate.LeaseIssued) {
		t.Fatal("rollback")
	}
	if _, ok := scoreleasegate.CatalogByKind(scoreleasegate.ProofHaltFlip); !ok {
		t.Fatal("catalog")
	}
	k, code := scoreleasegate.ClassifySubmitErr(scoreleasegate.ErrHaltFlip)
	if k != scoreleasegate.ProofHaltFlip || code != "SCORE_HALT_FLIP" {
		t.Fatalf("%s %s", k, code)
	}
}

func TestWorkerExpireAndHalt(t *testing.T) {
	base := time.Date(2026,
		3, 2, 8, 0, 0, 0, time.UTC)
	now := base
	svc := scoreleasegate.NewService("test-secret-r18", allowEval(), scoreleasegate.NewMemStore()).WithClock(func() time.Time { return now })
	req := issueReq("u-wk-01", "ETH", "0xscorewk01aaaaaaaa", "USDT", "1.000000000000000000", "nonce-score-wk-01")
	req.TTLSeconds = 1
	if _, err := svc.Issue(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	now = base.Add(3 * time.Second)
	w := scoreleasegate.NewWorker(svc)
	n, err := w.TickExpire(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	l2, err := svc.Issue(context.Background(), issueReq("u-wk-02", "ETH", "0xscorewk02aaaaaaaa", "USDT", "1.000000000000000000", "nonce-score-wk-02"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SetHalt(context.Background(), scoreleasegate.SetHaltReq{On: true, Actor: "ops-wk", Reason: "halt"}); err != nil {
		t.Fatal(err)
	}
	hn, err := w.TickHaltRevoke(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = hn
	got, err := svc.GetLease(context.Background(), l2.ID)
	if err != nil || got.Status != scoreleasegate.LeaseRevoked {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestMemStoreCAS(t *testing.T) {
	m := scoreleasegate.NewMemStore()
	ctx := context.Background()
	g, err := m.GetGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CASBumpHalt(ctx, g.HaltGen+1, true, time.Now().UTC()); err != scoreleasegate.ErrStaleGen {
		t.Fatalf("err=%v", err)
	}
	if _, err := m.CASBumpHalt(ctx, g.HaltGen, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestRiskscoreAdapterHalt(t *testing.T) {
	// 适配层 Halt 必须失败即关，不得签发。
	ad := scoreleasegate.RiskscoreEval{}
	_, err := ad.Eval(context.Background(), scoreleasegate.IssueReq{})
	if err != scoreleasegate.ErrUnavailable {
		t.Fatalf("err=%v", err)
	}
}

func TestReplayHaltScript(t *testing.T) {
	svc := newTestSvc()
	steps := []scoreleasegate.ReplayStep{
		{Op: "issue", Req: map[string]string{"nonce": "nonce-rp-0001", "user_id": "u-rp-01", "dest": "0xscorerp01aaaaaaaa"}, Expect: "ok"},
		{Op: "halt", Req: map[string]string{"on": "true"}, Expect: "ok"},
		{Op: "submit", Req: map[string]string{"user_id": "u-rp-01", "dest": "0xscorerp01aaaaaaaa"}, Expect: "deny", Kind: "halt_flip"},
	}
	out := scoreleasegate.Replay(context.Background(),
		svc, steps)
	if len(out) != 3 || !out[0].OK || !out[2].OK {
		t.Fatalf("%+v", out)
	}
	if out[2].Proof != "halt_flip" {
		t.Fatalf("proof=%s", out[2].Proof)
	}
}

func TestObserveAndPolicy(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	if _, err := svc.Issue(ctx, issueReq("u-ob-01", "ETH", "0xscoreob01aaaaaaaa", "USDT", "1.000000000000000000", "nonce-score-ob-01")); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Observe(ctx)
	if err != nil || snap.Open < 1 {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
	p := scoreleasegate.DefaultPolicy()
	if !p.ShadowForbidden() || scoreleasegate.ClampTTL(0) != scoreleasegate.DefaultLeaseTTL {
		t.Fatal("policy")
	}
	if scoreleasegate.RejectScreenLeaseID("scr-abc") != scoreleasegate.ErrIsolationViolation {
		t.Fatal("fence")
	}
	if scoreleasegate.FencePayload([]string{"limitgate"}) != scoreleasegate.ErrIsolationViolation {
		t.Fatal("fence keys")
	}
}

func TestSQLStoreNil(t *testing.T) {
	s := scoreleasegate.NewSQLStore(nil)
	if _, err := s.GetGeneration(context.Background()); err != scoreleasegate.ErrUnavailable {
		t.Fatalf("err=%v", err)
	}
}

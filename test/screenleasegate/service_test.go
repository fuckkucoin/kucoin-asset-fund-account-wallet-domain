package screenleasegate_test

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
	"account-assets.kucoin.net/funding/internal/screenleasegate"
)

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", "screenleasegate", name)
}

func newTestSvc() *screenleasegate.Service {
	sc := screenleasegate.StaticScreener{Allow: true, Pack: "kyc0", Mode: "enforce"}
	return screenleasegate.NewService("test-secret-r16", sc, screenleasegate.NewMemStore())
}

func issueReq(user, asset, amt, dest, nonce string) screenleasegate.IssueReq {
	return screenleasegate.IssueReq{
		UserID:	user, Asset: asset, Amount: amt, Dest: dest,
		Pack:	"kyc0", Kind: "withdraw", Nonce: nonce,
	}
}

func submitReq(lease, user, asset, amt, dest, ticket, actor string, consume bool) screenleasegate.SubmitReq {
	return screenleasegate.SubmitReq{
		LeaseID:	lease,
		UserID:		user, Asset: asset, Amount: amt,
		Dest:	dest, Ticket: ticket, Actor: actor, Consume: consume,
	}
}

func TestIssueThenConsume(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq("u-ok-01", "USDT", "10.000000000000000000", "0xabc0123456789aaa", "nonce-ok-0001"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Status != screenleasegate.LeaseOpen || l.ID == "" || l.IsolationMark != screenleasegate.IsolationMark {
		t.Fatalf("lease=%+v", l)
	}
	got, proof, err := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Asset, l.Amount, l.Dest, "wd-t-ok", "wd-actor-ok", true))
	if err != nil {
		t.Fatalf("submit: %v proof=%+v", err, proof)
	}
	if got.Status != screenleasegate.LeaseConsumed {
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
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Dest		string	`json:"dest"`
		Pack		string	`json:"pack"`
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
	l, err := svc.Issue(ctx, issueReq(g.UserID, g.Asset, g.Amount, g.Dest, g.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SetHalt(ctx, screenleasegate.SetHaltReq{On: true, Operator: g.Operator}); err != nil {
		t.Fatal(err)
	}
	_, proof, err := svc.Submit(ctx, submitReq(l.ID, g.UserID, g.Asset, g.Amount, g.Dest, g.Ticket, g.Actor, true))
	if err == nil {
		t.Fatal("expected fail-closed after halt")
	}
	if proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("proof=%+v want %s", proof, g.ExpectProof)
	}
	if !screenleasegate.IsFailClosed(err) {
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
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Dest		string	`json:"dest"`
		Pack		string	`json:"pack"`
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
	l, err := svc.Issue(ctx, issueReq(g.UserID, g.Asset, g.Amount, g.Dest, g.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.FlipPack(ctx, screenleasegate.FlipPackReq{Pack: g.FlipPack, Mode: "enforce", Operator: g.Operator}); err != nil {
		t.Fatal(err)
	}
	_,
		proof, err := svc.Submit(ctx, submitReq(l.ID, g.UserID, g.Asset, g.Amount, g.Dest, g.Ticket, g.Actor, false))
	if err == nil {
		t.Fatal("expected deny after pack flip")
	}
	if proof == nil || string(proof.Kind) != g.ExpectProof {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestMissingLeaseFailClosed(t *testing.T) {
	b, err := os.ReadFile(testdata("missing_lease.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID, Asset, Amount, Dest, Ticket, Actor, ExpectProof string
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	_, proof, err := svc.Submit(context.Background(), submitReq("", g.UserID, g.Asset, g.Amount, g.Dest, g.Ticket, g.Actor, true))
	if err != screenleasegate.ErrLeaseMissing {
		t.Fatalf("err=%v", err)
	}
	if proof == nil || proof.Kind != screenleasegate.ProofMissingLease {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestExpireLease(t *testing.T) {
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	svc := newTestSvc().WithClock(clock)
	ctx := context.Background()
	req := issueReq("u-screen-04", "USDT", "3.250000000000000000", "TXYZscreenlease01", "nonce-exp-0001")
	req.TTLSeconds = 1
	l, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	n, err := svc.ExpireSweep(ctx)
	if err != nil || n != 1 {
		t.Fatalf("expire n=%d err=%v", n, err)
	}
	_, proof, err := svc.Submit(ctx, submitReq(l.ID, req.UserID, req.Asset, req.Amount, req.Dest, "wd-ticket-exp-01", "wd-actor-d", true))
	if err == nil {
		t.Fatal("expected expired")
	}
	if proof == nil || proof.Kind != screenleasegate.ProofExpired {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestIdempotentNonce(t *testing.T) {
	b, err := os.ReadFile(testdata("idempotent_nonce.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID	string	`json:"user_id"`
		Asset	string	`json:"asset"`
		Amount	string	`json:"amount"`
		Dest	string	`json:"dest"`
		Pack	string	`json:"pack"`
		Nonce	string	`json:"nonce"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	req := issueReq(g.UserID, g.Asset, g.Amount, g.Dest, g.Nonce)
	a, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b2.ID {
		t.Fatalf("not idempotent %s %s", a.ID, b2.ID)
	}
}

func TestAmountAndDestDrift(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq("u-drift-01", "USDT", "2.000000000000000000", "0xdest0123456789aa", "nonce-drift-0001"))
	if err != nil {
		t.Fatal(err)
	}
	_, p1, err := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Asset, "9.000000000000000000", l.Dest, "wd-t-amt", "wd-actor-f", false))
	if err != screenleasegate.ErrLeaseAmount || p1 == nil || p1.Kind != screenleasegate.ProofAmountDrift {
		t.Fatalf("amount err=%v p=%+v", err, p1)
	}
	// 未关闭证明挡后续提交。
	_, _, err = svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Asset, l.Amount, l.Dest, "wd-t-ok2", "wd-actor-f", false))
	if err != screenleasegate.ErrProofBlocked {
		t.Fatalf("want blocked got %v", err)
	}
}

func TestScreenDenyNoLease(t *testing.T) {
	svc := screenleasegate.NewService("test-secret-r16", screenleasegate.StaticScreener{Allow: false, Pack: "kyc0", Mode: "enforce"}, screenleasegate.NewMemStore())
	_, err := svc.Issue(context.Background(), issueReq("u-deny-01", "USDT", "1.000000000000000000", "0xdeny0123456789aa", "nonce-deny-0001"))
	if err != screenleasegate.ErrScreenDenied {
		t.Fatalf("err=%v", err)
	}
}

func TestFourEye(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	_, proof, err := svc.Submit(ctx, submitReq("", "u-eye-01", "USDT", "1.000000000000000000", "0xeye0123456789aaa", "wd-t-eye", "wd-actor-g", false))
	if err != screenleasegate.ErrLeaseMissing || proof == nil {
		t.Fatalf("err=%v p=%+v", err, proof)
	}
	closed, err := svc.FourEye(ctx, screenleasegate.FourEyeReq{ProofID: proof.ID, OperatorA: "ops-a1", OperatorB: "ops-b1", Reason: "clear missing after review"})
	if err != nil || closed.Status != screenleasegate.ProofClosed {
		t.Fatalf("close=%+v err=%v", closed, err)
	}
	if _, err := svc.FourEye(ctx, screenleasegate.FourEyeReq{ProofID: proof.ID, OperatorA: "ops-a1", OperatorB: "ops-a1", Reason: "x"}); err != screenleasegate.ErrFourEyeSame {
		t.Fatalf("same ops: %v", err)
	}
}

func TestConcurrentBind(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, issueReq("u-screen-06", "USDT", "5.000000000000000000", "0xmutex0123456789", "nonce-mutex-0001"))
	if err != nil {
		t.Fatal(err)
	}
	var ok, deny atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ticket := "wd-ticket-m1"
			if i%2 == 0 {
				ticket = "wd-ticket-m2"
			}
			_, _, e := svc.Submit(ctx, submitReq(l.ID, l.UserID, l.Asset, l.Amount, l.Dest, ticket, "wd-actor-e", false))
			if e == nil {
				ok.Add(1)
			} else {
				deny.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() == 0 {
		t.Fatal("expected at least one bind success")
	}
}

func TestIsolationFence(t *testing.T) {
	b, err := os.ReadFile(testdata("isolation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		ForbiddenKeys	[]string	`json:"forbidden_keys"`
		IsolationMark	string		`json:"isolation_mark"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if g.IsolationMark != screenleasegate.IsolationMark {
		t.Fatalf("mark %s", g.IsolationMark)
	}
	keys := map[string]string{}
	for _, k := range g.ForbiddenKeys {
		keys[k] = "x"
	}
	if err := screenleasegate.ForbiddenPayload(keys); err != screenleasegate.ErrIsolationViolation {
		t.Fatalf("err=%v", err)
	}
	if err := screenleasegate.AssertIsolation(screenleasegate.IsolationMark); err != nil {
		t.Fatal(err)
	}
}

func TestGoldenCasesCSV(t *testing.T) {
	f, err := os.Open(testdata("cases.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0][0] != "case_id" {
		t.Fatalf("header %v", rows[0])
	}
	g, err := os.Open(testdata("golden_digest.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	gr := csv.NewReader(g)
	grows, err := gr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range grows[1:] {
		if row[3] != screenleasegate.IsolationMark {
			t.Fatalf("isolation %s", row[3])
		}
		if !screenleasegate.KnownProofKind(screenleasegate.ProofKind(row[1])) {
			t.Fatalf("unknown kind %s", row[1])
		}
	}
}

func TestFSMTable(t *testing.T) {
	if to, ok := screenleasegate.CanTransit(screenleasegate.LeaseOpen, "bind"); !ok || to != screenleasegate.LeaseBound {
		t.Fatal("open bind")
	}
	if _, ok := screenleasegate.CanTransit(screenleasegate.LeaseConsumed, "bind"); ok {
		t.Fatal("consumed must not bind")
	}
}

func TestWorkerExpire(t *testing.T) {
	now := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	svc := newTestSvc().WithClock(clock)
	req := issueReq("u-wk-01", "USDT", "1.100000000000000000", "0xwork0123456789aa", "nonce-work-0001")
	req.TTLSeconds = 1
	if _, err := svc.Issue(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	w := screenleasegate.NewWorker(svc, time.Hour)
	n, err := w.Tick(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestReplayHalt(t *testing.T) {
	svc := newTestSvc()
	steps := []screenleasegate.ReplayStep{
		{Op: "issue", Issue: &screenleasegate.IssueReq{UserID: "u-rp-01", Asset: "USDT", Amount: "1.000000000000000000", Dest: "0xrep0123456789aaa", Pack: "kyc0", Kind: "withdraw", Nonce: "nonce-rep-0001"}, Expect: "ok"},
		{Op: "halt", Halt: &screenleasegate.SetHaltReq{On: true, Operator: "ops-rp"}, Expect: "ok"},
		{Op: "submit", Submit: &screenleasegate.SubmitReq{UserID: "u-rp-01", Asset: "USDT", Amount: "1.000000000000000000", Dest: "0xrep0123456789aaa", Ticket: "wd-rp", Actor: "wd-actor-r"}, Expect: "deny", Proof: "halt_flip"},
	}
	out := screenleasegate.Replay(context.Background(), svc, steps)
	for _, r := range out {
		if !r.OK {
			t.Fatalf("step %+v", r)
		}
	}
}

func TestCatalogAndClassify(t *testing.T) {
	if !screenleasegate.KnownLeaseStatus(screenleasegate.LeaseOpen) || !screenleasegate.KnownProofKind(screenleasegate.ProofHaltFlip) {
		t.Fatal("catalog")
	}
	k, act, den := screenleasegate.ClassifySubmitErr(screenleasegate.ErrHaltFlip)
	if k != screenleasegate.ProofHaltFlip || act == "" || !den {
		t.Fatalf("%s %s", k, act)
	}
}

func TestPersistRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	l := screenleasegate.ScreenLease{ID: "sl-x", UserID: "u", Dest: "d", Asset: "USDT", Amount: "1", Status: screenleasegate.LeaseOpen, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}
	if screenleasegate.RowToLease(screenleasegate.LeaseToRow(l)).ID != l.ID {
		t.Fatal("lease row")
	}
	p := screenleasegate.ScreenDriftProof{ID: "sdp-x", Kind: screenleasegate.ProofHaltFlip, Status: screenleasegate.ProofOpen, OpenedAt: now}
	if screenleasegate.RowToProof(screenleasegate.ProofToRow(p)).Kind != screenleasegate.ProofHaltFlip {
		t.Fatal("proof row")
	}
}

func TestNilService(t *testing.T) {
	var s *screenleasegate.Service
	if _, err := s.Issue(context.Background(), screenleasegate.IssueReq{}); err != screenleasegate.ErrUnavailable {
		t.Fatal(err)
	}
}

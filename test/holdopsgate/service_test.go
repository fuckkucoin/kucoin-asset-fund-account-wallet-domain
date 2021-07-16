package holdopsgate_test

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
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", "holdopsgate", name)
}

func newTestSvc() *holdopsgate.Service {
	return holdopsgate.NewService("test-secret-r17", holdopsgate.NewMemStore())
}

func TestPlaceThenRelease(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.Issue(ctx, holdopsgate.IssueReq{
		Kind:	holdopsgate.OpsPlace, UserID: "u-ok-01", Asset: "USDT", Amount: "10",
		TokenID:	"tok-ok-01", GapID: "gap-ok-01", Actor: "ops-a", Nonce: "n-ok-place",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: l.ID, Kind: holdopsgate.OpsPlace, Consume: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != holdopsgate.LeaseConsumed {
		t.Fatalf("status=%s", got.Status)
	}
	tok, err := svc.GetToken(ctx, "tok-ok-01")
	if err != nil || !tok.Open {
		t.Fatalf("token=%+v err=%v", tok, err)
	}
	rel, err := svc.Issue(ctx, holdopsgate.IssueReq{
		Kind:	holdopsgate.OpsRelease, UserID: "u-ok-01", Asset: "USDT", Amount: "10",
		TokenID:	"tok-ok-01", GapID: "gap-ok-01", Actor: "ops-a", Nonce: "n-ok-rel",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err = svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: rel.ID, Kind: holdopsgate.OpsRelease, Consume: true})
	if err != nil {
		t.Fatal(err)
	}
	tok, _ = svc.GetToken(ctx, "tok-ok-01")
	if tok.Open {
		t.Fatal("token should be closed")
	}
}

func TestConcurrentDualRelease(t *testing.T) {
	b, err := os.ReadFile(testdata("dual_release.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID	string	`json:"user_id"`
		Asset	string	`json:"asset"`
		Amount	string	`json:"amount"`
		TokenID	string	`json:"token_id"`
		GapID	string	`json:"gap_id"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	pl, err := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsPlace, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, TokenID: g.TokenID, GapID: g.GapID, Actor: "ops-a", Nonce: "n-p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: pl.ID, Kind: holdopsgate.OpsPlace, Consume: true}); err != nil {
		t.Fatal(err)
	}
	a, err := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsRelease, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, TokenID: g.TokenID, GapID: g.GapID, Actor: "ops-a", Nonce: "n-ra"})
	if err != nil {
		t.Fatal(err)
	}
	bb, err := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsRelease, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, TokenID: g.TokenID, GapID: g.GapID, Actor: "ops-b", Nonce: "n-rb"})
	if err != nil {
		t.Fatal(err)
	}
	var okN, failN atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	run := func(id, ticket string) {
		defer wg.Done()
		<-start
		_, p, err := svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: id, Kind: holdopsgate.OpsRelease, Ticket: ticket, Consume: true})
		if err != nil {
			failN.Add(1)
			if p == nil || (p.Kind != holdopsgate.ProofDualRelease && p.Kind != holdopsgate.ProofConcurrent && p.Kind != holdopsgate.ProofTokenMismatch) {
				if p != nil && p.Kind == holdopsgate.ProofDualRelease {
					return
				}
			}
			return
		}
		okN.Add(1)
	}
	wg.Add(2)
	go run(a.ID, "t-a")
	go run(bb.ID, "t-b")
	close(start)
	wg.Wait()
	if okN.Load() != 1 {
		t.Fatalf("winners=%d losers=%d", okN.Load(), failN.Load())
	}
	if failN.Load() < 1 {
		t.Fatal("expected loser with proof")
	}
}

func TestGapCloseWhileOpenToken(t *testing.T) {
	b, err := os.ReadFile(testdata("gap_close_open_token.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID	string	`json:"user_id"`
		Asset	string	`json:"asset"`
		Amount	string	`json:"amount"`
		TokenID	string	`json:"token_id"`
		GapID	string	`json:"gap_id"`
	}
	if json.Unmarshal(b,
		&g) != nil {
		t.Fatal("json")
	}
	svc := newTestSvc()
	ctx := context.Background()
	pl, _ := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsPlace, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, TokenID: g.TokenID, GapID: g.GapID, Actor: "ops-c", Nonce: "np"})
	if _, _, err := svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: pl.ID, Kind: holdopsgate.OpsPlace, Consume: true}); err != nil {
		t.Fatal(err)
	}
	ackL, _ := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsGapAck, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, TokenID: g.TokenID, GapID: g.GapID, Actor: "ops-c", Nonce: "na"})
	if _, p, err := svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: ackL.ID, Kind: holdopsgate.OpsGapAck}); err != nil {
		t.Fatalf("ack: %v", err)
	} else if p != nil && p.Kind != holdopsgate.ProofAckNotRelease {
		t.Fatalf("ack proof=%s", p.Kind)
	}
	tok,
		_ := svc.GetToken(ctx, g.TokenID)
	if !tok.Open {
		t.Fatal("ack must not release token")
	}
	gap, _ := svc.GetGap(ctx, g.GapID)
	if !gap.Acked || gap.Closed {
		t.Fatalf("gap=%+v", gap)
	}
	cl, _ := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsGapClose, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, TokenID: g.TokenID, GapID: g.GapID, Actor: "ops-c", Nonce: "nc"})
	_,
		proof, err := svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: cl.ID, Kind: holdopsgate.OpsGapClose, Consume: true})
	if err == nil {
		t.Fatal("close must fail")
	}
	if proof == nil || proof.Kind != holdopsgate.ProofGapOpenToken {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestExpiredLeaseFailClosed(t *testing.T) {
	f, err := os.Open(testdata("expired_lease.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatal(err)
	}
	now := time.Date(2026,
		3, 1, 12, 0, 0, 0, time.UTC)
	for i, row := range rows[1:] {
		svc := newTestSvc().WithClock(func() time.Time { return now })
		ctx := context.Background()
		kind := holdopsgate.OpsKind(row[1])
		l, err := svc.Issue(ctx, holdopsgate.IssueReq{
			Kind:	kind, UserID: "u-exp", Asset: "USDT", Amount: "1",
			TokenID:	"tok-exp", GapID: "gap-exp", Actor: "ops-e", Nonce: "n-exp-" + row[0], TTLSec: 2,
		})
		if err != nil {
			t.Fatalf("row %d issue: %v", i, err)
		}
		if row[0] == "revoke_after_bump" {
			if _, err := svc.Bump(ctx, holdopsgate.BumpReq{Expect: 1, Actor: "ops-admin", Note: "fence"}); err != nil {
				t.Fatal(err)
			}
		} else {
			svc.WithClock(func() time.Time { return now.Add(10 * time.Second) })
		}
		_, _, err = svc.Exec(ctx, holdopsgate.ExecReq{LeaseID: l.ID, Kind: kind, Consume: true})
		if err == nil {
			t.Fatalf("row %d expected deny", i)
		}
	}
}

func TestMissingLease(t *testing.T) {
	svc := newTestSvc()
	_, p, err := svc.Exec(context.Background(), holdopsgate.ExecReq{Kind: holdopsgate.OpsRelease, UserID: "u-miss"})
	if err != holdopsgate.ErrLeaseMissing || p == nil || p.Kind != holdopsgate.ProofMissingLease {
		t.Fatalf("err=%v proof=%+v", err, p)
	}
}

func TestIsolationReject(t *testing.T) {
	svc := newTestSvc()
	_, err := svc.Issue(context.Background(), holdopsgate.IssueReq{
		Kind:	holdopsgate.OpsPlace, UserID: "u-iso", Asset: "USDT", Amount: "1",
		Actor:	"ops-i", Nonce: "holdpatrol-block", TokenID: "t",
	})
	if err != holdopsgate.ErrIsolation {
		t.Fatalf("err=%v", err)
	}
}

func TestIdempotentNonce(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	req := holdopsgate.IssueReq{Kind: holdopsgate.OpsPlace, UserID: "u-id", Asset: "USDT", Amount: "1", TokenID: "tok-id", Actor: "ops-id", Nonce: "same-n"}
	a, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatal("idempotent lease id")
	}
}

func TestFourEye(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	p, err := svc.proof.OpenDualRelease(ctx, holdopsgate.HoldOpsLease{ID: "w", OpsGen: 1, TokenID: "t", UserID: "u-fe"}, holdopsgate.HoldOpsLease{ID: "l", OpsID: "o", TokenID: "t", UserID: "u-fe", OpsGen: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FourEye(ctx, holdopsgate.FourEyeReq{ProofID: p.ID, Eye: "ops-1", Reason: "clear"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FourEye(ctx, holdopsgate.FourEyeReq{ProofID: p.ID, Eye: "ops-1", Reason: "clear"}); err != holdopsgate.ErrFourEyeSame {
		t.Fatalf("same eye: %v", err)
	}
	got, err := svc.FourEye(ctx, holdopsgate.FourEyeReq{ProofID: p.ID, Eye: "ops-2", Reason: "clear"})
	if err != nil || got.Status != holdopsgate.ProofClosed {
		t.Fatalf("close: %v %+v", err, got)
	}
}

func TestFSMTable(t *testing.T) {
	to, ok := holdopsgate.CanTransit(holdopsgate.LeaseOpen, "bind")
	if !ok || to != holdopsgate.LeaseBound {
		t.Fatal(to)
	}
	_, ok = holdopsgate.CanTransit(holdopsgate.LeaseConsumed, "bind")
	if ok {
		t.Fatal("consumed bind")
	}
}

func TestClassify(t *testing.T) {
	if holdopsgate.Classify(holdopsgate.ErrDualRelease).Kind != holdopsgate.ProofDualRelease {
		t.Fatal("class")
	}
	if holdopsgate.Classify(holdopsgate.ErrGapOpenToken).Retry {
		t.Fatal("retry")
	}
}

func TestJournalAndObserve(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, _ := svc.Issue(ctx, holdopsgate.IssueReq{Kind: holdopsgate.OpsPlace, UserID: "u-j", Asset: "USDT", Amount: "1", TokenID: "tok-j", Actor: "ops-j", Nonce: "nj"})
	j, err := holdopsgate.BuildJournal(ctx, svc.Store(), l.OpsID)
	if err != nil || len(j) == 0 {
		t.Fatalf("journal %+v %v", j, err)
	}
	view := holdopsgate.Observe(ctx, svc)
	if view.Stats.Issued < 1 {
		t.Fatal("stats")
	}
}

func TestReplayGolden(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	steps := []holdopsgate.ReplayStep{
		{Name: "issue", Action: "issue", Issue: holdopsgate.IssueReq{Kind: holdopsgate.OpsPlace, UserID: "u-rp", Asset: "USDT", Amount: "1", TokenID: "tok-rp", GapID: "gap-rp", Actor: "ops-rp", Nonce: "nrp"}},
		{Name: "exec", Action: "exec", Exec: holdopsgate.ExecReq{Kind: holdopsgate.OpsPlace, Consume: true}, Expect: "ok"},
	}
	out := holdopsgate.Replay(ctx, svc, steps)
	if !out[0].OK || !out[1].OK {
		t.Fatalf("%+v", out)
	}
}

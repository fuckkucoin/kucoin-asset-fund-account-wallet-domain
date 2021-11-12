package signpipegate_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/signpipegate"
)

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", "signpipegate", name)
}

func newTestSvc() *signpipegate.Service {
	return signpipegate.NewService("test-secret-r15", signpipegate.StaticObserver{}, signpipegate.NewMemStore())
}

func armReq(mv, job, chain, to, amount, nonce, actor string) signpipegate.ArmReq {
	return signpipegate.ArmReq{MovementID: mv, JobID: job, Chain: chain, To: to, Amount: amount, Nonce: nonce, Actor: actor}
}

func TestArmThenSeal(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, armReq("mv-se-01", "job-se-01", "ETH", "0xabc8", "8.000000000000000000", "nonce-se-01", "signer-j"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enter(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	sealed, art, err := svc.Seal(ctx, signpipegate.SealReq{LeaseID: l.ID, Actor: "signer-j", SigHex: "aabb"})
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Status != signpipegate.LeaseSealed || art == nil || art.Digest == "" {
		t.Fatalf("sealed=%+v art=%+v", sealed, art)
	}
}

func TestIdempotentNonce(t *testing.T) {
	b, err := os.ReadFile(testdata("idempotent_nonce.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		MovementID	string	`json:"movement_id"`
		JobID		string	`json:"job_id"`
		Chain		string	`json:"chain"`
		To		string	`json:"to"`
		Amount		string	`json:"amount"`
		Nonce		string	`json:"nonce"`
		Actor		string	`json:"actor"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	req := armReq(g.MovementID, g.JobID, g.Chain, g.To, g.Amount, g.Nonce, g.Actor)
	a, err := svc.ArmRetry(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := svc.ArmRetry(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b2.ID || a.Generation != b2.Generation {
		t.Fatalf("not idempotent %s %s", a.ID, b2.ID)
	}
}

func TestDoubleSealForbidden(t *testing.T) {
	b, err := os.ReadFile(testdata("double_seal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		MovementID	string	`json:"movement_id"`
		JobID		string	`json:"job_id"`
		Chain		string	`json:"chain"`
		To		string	`json:"to"`
		Amount		string	`json:"amount"`
		Nonce		string	`json:"nonce"`
		Actor		string	`json:"actor"`
		SigHex		string	`json:"sig_hex"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, armReq(g.MovementID, g.JobID, g.Chain, g.To, g.Amount, g.Nonce, g.Actor))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enter(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Seal(ctx, signpipegate.SealReq{LeaseID: l.ID, Actor: g.Actor, SigHex: g.SigHex}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ArmRetry(ctx, armReq(g.MovementID, "job-ds-02", g.Chain, g.To, g.Amount, "nonce-ds-02", g.Actor))
	if err != signpipegate.ErrDoubleArtifact {
		t.Fatalf("want double artifact got %v", err)
	}
}

func TestFaultMatrixGolden(t *testing.T) {
	b, err := os.ReadFile(testdata("fault_matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Cases []struct {
			Kind		string	`json:"kind"`
			ExpectErr	string	`json:"expect_err"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	for i, cs := range g.Cases {
		req := armReq("mv-fm-"+cs.Kind, "job-fm-"+cs.Kind, "ETH", "0xabcd", "1.000000000000000000", "nonce-fm-"+cs.Kind, "signer-z")
		l, err := svc.ArmRetry(ctx, req)
		if err != nil {
			t.Fatalf("arm %s: %v", cs.Kind, err)
		}
		_, ferr := svc.InjectFault(ctx, signpipegate.InjectReq{LeaseID: l.ID, Kind: signpipegate.FaultKind(cs.Kind), Operator: "ops-1"})
		if ferr == nil {
			t.Fatalf("case %d %s must fail", i, cs.Kind)
		}
		if !strings.Contains(ferr.Error(), cs.ExpectErr) && signpipegate.ClassifyInjectError(ferr) != signpipegate.FaultKind(cs.Kind) {
			t.Fatalf("case %s err %v want substring %s", cs.Kind, ferr, cs.ExpectErr)
		}
	}
}

func TestCSVCasesLoad(t *testing.T) {
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
		t.Fatalf("rows %d", len(rows))
	}
	svc := newTestSvc()
	ctx := context.Background()
	for i, row := range rows[1:] {
		if len(row) < 9 {
			continue
		}
		name,
			mv, job, chain, to, amount, nonce, actor, kind := row[0], row[1], row[2], row[3], row[4], row[5], row[6], row[7], row[8]
		l,
			err := svc.ArmRetry(ctx, armReq(mv, job, chain, to, amount, nonce, actor))
		if name == "double_seal" || name == "concurrent_mutex" {
			if err != nil && err != signpipegate.ErrInflightOccupied && err != signpipegate.ErrDoubleArtifact {
				t.Fatalf("row %d %s: %v", i, name, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("row %d %s arm: %v", i, name, err)
		}
		if kind != "" {
			_, _ = svc.InjectFault(ctx, signpipegate.InjectReq{LeaseID: l.ID, Kind: signpipegate.FaultKind(kind), Operator: "ops-csv"})
		}
	}
}

func TestConcurrentArmMutex(t *testing.T) {
	b, err := os.ReadFile(testdata("mutex_race.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		MovementID	string		`json:"movement_id"`
		JobID		string		`json:"job_id"`
		Chain		string		`json:"chain"`
		To		string		`json:"to"`
		Amount		string		`json:"amount"`
		Actors		[]string	`json:"actors"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc := newTestSvc()
	ctx := context.Background()
	var ok, deny atomic.Int64
	var wg sync.WaitGroup
	for i, actor := range g.Actors {
		wg.Add(1)
		go func(i int, actor string) {
			defer wg.Done()
			_, err := svc.ArmRetry(ctx, armReq(g.MovementID, g.JobID, g.Chain, g.To, g.Amount, "nonce-race-"+actor, actor))
			if err == nil {
				ok.Add(1)
			} else if err == signpipegate.ErrInflightOccupied || err == signpipegate.ErrConcurrentRetry {
				deny.Add(1)
			} else {
				t.Errorf("unexpected %v", err)
			}
		}(i, actor)
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Fatalf("winners %d deny %d", ok.Load(), deny.Load())
	}
}

func TestExpireThenSeal(t *testing.T) {
	now := time.Date(2026,
		9, 21, 12, 0, 0, 0, time.UTC)
	svc := newTestSvc().WithClock(func() time.Time { return now })
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, signpipegate.ArmReq{
		MovementID:	"mv-ex-01", JobID: "job-ex-01", Chain: "ETH", To: "0xabc3",
		Amount:	"3.000000000000000000", Nonce: "nonce-ex-01", Actor: "signer-e",
		TTL:	time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now.Add(time.Second) })
	_, _, err = svc.Seal(ctx, signpipegate.SealReq{LeaseID: l.ID, Actor: "signer-e", SigHex: "ff"})
	if err != signpipegate.ErrLeaseExpired {
		t.Fatalf("want expired got %v", err)
	}
}

func TestIsolationMark(t *testing.T) {
	b, err := os.ReadFile(testdata("isolation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Forbidden	[]string	`json:"forbidden_marks"`
		RequiredMark	string		`json:"required_mark"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if signpipegate.IsolationMark != g.RequiredMark {
		t.Fatalf("mark %s", signpipegate.IsolationMark)
	}
	for _, f := range g.Forbidden {
		if err := signpipegate.IsolationOK(f); err == nil {
			t.Fatalf("forbidden %s passed", f)
		}
	}
	ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0133_aa_signpipegate.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := signpipegate.IsolationSQLGuard(string(ddl)); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerExpireCloseout(t *testing.T) {
	now := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)
	svc := newTestSvc().WithClock(func() time.Time { return now })
	ctx := context.Background()
	_,
		err := svc.ArmRetry(ctx, signpipegate.ArmReq{
		MovementID:	"mv-tt-01", JobID: "job-tt-01", Chain: "ETH", To: "0xabca",
		Amount:	"1.100000000000000000", Nonce: "nonce-tt-01", Actor: "signer-l",
		TTL:	time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now.Add(time.Second) })
	w := signpipegate.NewWorker(svc)
	rep, err := w.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Expired == 0 {
		t.Fatalf("expected expire %+v", rep)
	}
}

func TestAbortAndList(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, armReq("mv-ab-01", "job-ab-01", "ETH", "0xab", "1.000000000000000000", "nonce-ab-01", "signer-n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Abort(ctx, signpipegate.AbortReq{LeaseID: l.ID, Operator: "ops-2", Reason: "desk halt"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != signpipegate.LeaseAborted {
		t.Fatalf("%+v", got)
	}
	if _, err := svc.ListMovement(ctx, "mv-ab-01"); err != nil {
		t.Fatal(err)
	}
}

func TestReplayScript(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	res, err := svc.Replay(ctx, []signpipegate.ReplayStep{
		{Op: "arm", MovementID: "mv-rp-01", JobID: "job-rp-01", Chain: "ETH", To: "0x1", Amount: "1.000000000000000000", Nonce: "n-rp", Actor: "sig-1"},
		{Op: "enter"},
		{Op: "seal", Actor: "sig-1", SigHex: "11"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if !r.OK {
			t.Fatalf("%+v", r)
		}
	}
}

func TestSQLStoreHosts(t *testing.T) {
	st := signpipegate.NewSQLStore()
	p, s := st.Hosts()
	if p != "pg-0.kucoin.net" || s != "pg-1.kucoin.net" {
		t.Fatalf("%s %s", p, s)
	}
	if !strings.Contains(st.RenderBump(), "movement_id") {
		t.Fatal("render")
	}
}

func TestCatalogAndFSM(t *testing.T) {
	if len(signpipegate.FaultCatalog()) != 10 {
		t.Fatalf("catalog %d", len(signpipegate.FaultCatalog()))
	}
	if !signpipegate.LeaseCan(signpipegate.LeaseArmed, signpipegate.LeaseInflight, "enter") {
		t.Fatal("fsm")
	}
	if signpipegate.DefaultPolicy().MaxGenerations != 3 {
		t.Fatal("policy")
	}
}

func TestDualActorArm(t *testing.T) {
	svc := newTestSvc()
	_, err := svc.ArmRetry(context.Background(), signpipegate.ArmReq{
		MovementID:	"mv-duact", JobID: "job-duact", Chain: "ETH", To: "0x2",
		Amount:	"1.000000000000000000", Nonce: "n-du", Actor: "same",
		RequestedBy:	"same", ApprovedBy: "same",
	})
	if err != signpipegate.ErrDualActor {
		t.Fatalf("%v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []signpipegate.ArmReq{
		{},
		{MovementID: "bad space", JobID: "j", Chain: "ETH", To: "a", Amount: "1", Nonce: "n", Actor: "a"},
		{MovementID: "m", JobID: "j", Chain: "", To: "a", Amount: "1", Nonce: "n", Actor: "a"},
		{MovementID: "m", JobID: "j", Chain: "ETH", To: "a", Amount: "x", Nonce: "n", Actor: "a"},
		{MovementID: "m", JobID: "j", Chain: "ETH", To: "a", Amount: "1.0", Nonce: "", Actor: "a"},
		{MovementID: "m", JobID: "j", Chain: "ETH", To: "a", Amount: "1.0", Nonce: "n", Actor: "system"},
	}
	for _, c := range cases {
		if err := signpipegate.ValidateArm(c); err == nil {
			t.Fatalf("want err %+v", c)
		}
	}
}

func TestGoldenDigestCSV(t *testing.T) {
	f, err := os.Open(testdata("golden_digest.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if len(r) == 2 && r[0] == "max_retry_generations" && r[1] == "3" {
			found = true
		}
	}
	if !found {
		t.Fatal("golden")
	}
}

func TestExportBundle(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, armReq("mv-exb", "job-exb", "ETH", "0xee", "1.000000000000000000", "n-exb", "sig-e"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Export(ctx, l.ID)
	if err != nil || b.Lease == nil || b.IsolationMark != signpipegate.IsolationMark {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestReplaySQLFixtureExists(t *testing.T) {
	b, err := os.ReadFile(testdata("replay.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "signpipe_retry_generations") {
		t.Fatal("sql")
	}
}

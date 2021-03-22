package balproofgate_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/balproofgate"
)

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", "balproofgate", name)
}

func newTestSvc(avail, frozen string, gen int64) (*balproofgate.Service, *balproofgate.MutableObserver,) {
	obs := &balproofgate.MutableObserver{Avail: avail, Frozen: frozen, ObserveGen: gen}
	svc := balproofgate.NewService("test-secret-r14", obs, balproofgate.NewMemStore())
	return svc, obs
}

type goldenIssue struct {
	Name	string	`json:"name"`
	UserID	string	`json:"user_id"`
	Book	string	`json:"book"`
	Asset	string	`json:"asset"`
	Nonce	string	`json:"nonce"`
	TTLSec	int64	`json:"ttl_sec"`
	Issued	struct {
		Available	string	`json:"available"`
		Frozen		string	`json:"frozen"`
		ObserveGen	int64	`json:"observe_gen"`
	}	`json:"issued"`
	After	struct {
		Available	string	`json:"available"`
		Frozen		string	`json:"frozen"`
		ObserveGen	int64	`json:"observe_gen"`
	}	`json:"after"`
	Expect	struct {
		RecheckError	string	`json:"recheck_error"`
		DriftKind	string	`json:"drift_kind"`
		DriftStatus	string	`json:"drift_status"`
	}	`json:"expect"`
}

func loadGolden(t *testing.T, name string) goldenIssue {
	t.Helper()
	b, err := os.ReadFile(testdata(name))
	if err != nil {
		t.Fatal(err)
	}
	var g goldenIssue
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestIssueThenCreditDrift(t *testing.T) {
	g := loadGolden(t, "issue_credit_drift.json")
	svc, obs := newTestSvc(g.Issued.Available, g.Issued.Frozen, g.Issued.ObserveGen)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: g.UserID, Book: g.Book, Asset: g.Asset, Nonce: g.Nonce})
	if err != nil {
		t.Fatal(err)
	}
	if lease.ProofLeaseID == "" || lease.Status != balproofgate.LeaseBound {
		t.Fatalf("lease %+v", lease)
	}
	obs.Avail = g.After.Available
	obs.Frozen = g.After.Frozen
	obs.ObserveGen = g.After.ObserveGen
	_, drift, err := svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: lease.Signature})
	if err == nil || err.Error() != g.Expect.RecheckError {
		t.Fatalf("err=%v", err)
	}
	if drift == nil || string(drift.Kind) != g.Expect.DriftKind {
		t.Fatalf("drift %+v", drift)
	}
	if drift.Status != balproofgate.DriftProven {
		t.Fatalf("status %s", drift.Status)
	}
	_, _, err = svc.Consume(ctx, balproofgate.ConsumeReq{LeaseID: lease.ID, Signature: lease.Signature, GateRef: "g1"})
	if err == nil {
		t.Fatal("consume must deny after drift")
	}
}

func TestIssueThenFrozenDrift(t *testing.T) {
	g := loadGolden(t, "frozen_drift.json")
	svc, obs := newTestSvc(g.Issued.Available, g.Issued.Frozen, g.Issued.ObserveGen)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: g.UserID, Book: g.Book, Asset: g.Asset, Nonce: g.Nonce})
	if err != nil {
		t.Fatal(err)
	}
	obs.Avail = g.After.Available
	obs.Frozen = g.After.Frozen
	obs.ObserveGen = g.After.ObserveGen
	_, drift, err := svc.Consume(ctx, balproofgate.ConsumeReq{LeaseID: lease.ID, Signature: lease.Signature})
	if err == nil || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("err=%v", err)
	}
	if drift == nil || drift.Kind != balproofgate.DriftFrozenUp {
		t.Fatalf("drift %+v", drift)
	}
}

func TestExpireThenRecheckDenied(t *testing.T) {
	g := loadGolden(t, "expire_check.json")
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	obs := &balproofgate.MutableObserver{Avail: g.Issued.Available, Frozen: g.Issued.Frozen, ObserveGen: g.Issued.ObserveGen}
	svc := balproofgate.NewService("test-secret-r14", obs, balproofgate.NewMemStore()).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease,
		err := svc.Issue(ctx, balproofgate.IssueReq{UserID: g.UserID, Book: g.Book, Asset: g.Asset, Nonce: g.Nonce, TTLSec: 1})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now.Add(2 * time.Second) })
	w := balproofgate.NewWorker(svc, time.Second)
	if _, err := w.ScanExpire(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: lease.Signature})
	if err != balproofgate.ErrLeaseExpired {
		t.Fatalf("err=%v", err)
	}
	got,
		err := svc.GetLease(ctx, lease.ID)
	if err != nil || got.Status != balproofgate.LeaseExpired {
		t.Fatalf("lease %+v err=%v", got, err)
	}
}

func TestIdempotentNonceNoDoubleSign(t *testing.T) {
	g := loadGolden(t, "idempotent_nonce.json")
	svc, _ := newTestSvc(g.Issued.Available, g.Issued.Frozen, g.Issued.ObserveGen)
	ctx := context.Background()
	req := balproofgate.IssueReq{UserID: g.UserID,
		Book:	g.Book, Asset: g.Asset, Nonce: g.Nonce}
	a, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Issue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || a.Signature != b.Signature || a.ProofLeaseID != b.ProofLeaseID {
		t.Fatalf("double sign a=%+v b=%+v", a, b)
	}
	gen, err := svc.GetGeneration(ctx, g.UserID, g.Book, g.Asset)
	if err != nil || gen.Generation != 1 {
		t.Fatalf("gen %+v err=%v", gen, err)
	}
}

func TestRevokeBlocksOldSignature(t *testing.T) {
	b, err := os.ReadFile(testdata("revoke_mutex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		UserID	string	`json:"user_id"`
		Book	string	`json:"book"`
		Asset	string	`json:"asset"`
		Nonce	string	`json:"nonce"`
		Issued	struct {
			Available	string	`json:"available"`
			Frozen		string	`json:"frozen"`
			ObserveGen	int64	`json:"observe_gen"`
		}	`json:"issued"`
		Revoke	struct {
			Reason		string	`json:"reason"`
			Operator	string	`json:"operator"`
		}	`json:"revoke"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	svc, _ := newTestSvc(g.Issued.Available, g.Issued.Frozen, g.Issued.ObserveGen)
	ctx := context.Background()
	lease,
		err := svc.Issue(ctx, balproofgate.IssueReq{UserID: g.UserID, Book: g.Book, Asset: g.Asset, Nonce: g.Nonce})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Revoke(ctx, balproofgate.RevokeReq{LeaseID: lease.ID, Reason: g.Revoke.Reason, Operator: g.Revoke.Operator}); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: lease.Signature})
	if err != balproofgate.ErrRevoked {
		t.Fatalf("err=%v", err)
	}
}

func TestCSVCasesClassifyAndGate(t *testing.T) {
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
	ctx := context.Background()
	for i, row := range rows {
		if i == 0 {
			continue
		}
		t.Run(row[0], func(t *testing.T) {
			user,
				book, asset, nonce := row[1], row[2], row[3], row[4]
			avail0, frz0, avail1, frz1 := row[5], row[6], row[7], row[8]
			kind, gate := row[9], row[10]
			obs := &balproofgate.MutableObserver{Avail: avail0, Frozen: frz0, ObserveGen: 1}
			svc := balproofgate.NewService("test-secret-r14", obs, balproofgate.NewMemStore())
			lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: user, Book: book, Asset: asset, Nonce: nonce})
			if err != nil {
				t.Fatal(err)
			}
			switch gate {
			case "idempotent":
				again,
					err := svc.Issue(ctx, balproofgate.IssueReq{UserID: user, Book: book, Asset: asset, Nonce: nonce})
				if err != nil || again.ID != lease.ID {
					t.Fatalf("idempotent fail %v", err)
				}
				return
			case "revoked":
				if _, _, err := svc.Revoke(ctx, balproofgate.RevokeReq{LeaseID: lease.ID, Reason: "csv", Operator: "ops"}); err != nil {
					t.Fatal(err)
				}
				_,
					_, err = svc.Consume(ctx, balproofgate.ConsumeReq{LeaseID: lease.ID, Signature: lease.Signature})
				if err != balproofgate.ErrRevoked {
					t.Fatalf("err=%v", err)
				}
				return
			case "expired":
				svc.WithClock(func() time.Time { return time.Now().UTC().Add(20 * time.Minute) })
				_, _, err = svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: lease.Signature})
				if err != balproofgate.ErrLeaseExpired {
					t.Fatalf("err=%v", err)
				}
				return
			}
			obs.Avail, obs.Frozen = avail1, frz1
			if avail1 != avail0 || frz1 != frz0 {
				obs.ObserveGen++
			}
			_, drift, err := svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: lease.Signature})
			if gate == "allow" {
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := svc.Consume(ctx, balproofgate.ConsumeReq{LeaseID: lease.ID, Signature: lease.Signature}); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected deny")
			}
			if kind != "" && (drift == nil || string(drift.Kind) != kind) {
				t.Fatalf("kind want %s got %+v", kind, drift)
			}
		})
	}
}

func TestConcurrentIssueMutex(t *testing.T) {
	obs := &balproofgate.MutableObserver{Avail: "1", Frozen: "0", ObserveGen: 1}
	store := balproofgate.NewMemStore()
	svc := balproofgate.NewService("test-secret-r14", obs, store)
	ctx := context.Background()
	var wg sync.WaitGroup
	errc := make(chan error, 16)
	ids := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 不同 nonce：必须互斥抬世代，不得出现相同 generation 双签。
			l, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-race", Book: "main", Asset: "USDT", Nonce: "n-" + balproofgate.Itoa64(int64(i))})
			if err != nil {
				errc <- err
				return
			}
			ids <- l.ID + ":" + balproofgate.Itoa64(l.Generation)
		}(i)
	}
	wg.Wait()
	close(errc)
	close(ids)
	gens := map[int64]int{}
	uniq := map[string]struct{}{}
	for s := range ids {
		uniq[s] = struct{}{}
		parts := strings.Split(s, ":")
		gens[mustParse(parts[1])]++
	}
	if len(uniq) == 0 {
		t.Fatal("no leases")
	}
	for g, n := range gens {
		if n > 1 {
			t.Fatalf("generation %d used %d times", g, n)
		}
	}
}

func mustParse(s string) int64 {
	n, _ := balproofgate.ParseLooseInt(s)
	return n
}

func TestIsolationRejectsPorNonce(t *testing.T) {
	svc, _ := newTestSvc("1", "0", 1)
	_, err := svc.Issue(context.Background(), balproofgate.IssueReq{UserID: "u1", Book: "main", Asset: "USDT", Nonce: "por_root-xxx"})
	if err != balproofgate.ErrIsolationViolation {
		t.Fatalf("err=%v", err)
	}
	b, err := os.ReadFile(testdata("isolation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), balproofgate.IsolationMark) {
		t.Fatal("golden isolation mark")
	}
	if err := balproofgate.IsolationSQLGuard("create table x (por_root text)"); err != balproofgate.ErrIsolationViolation {
		t.Fatal(err)
	}
}

func TestHappyConsumeAndExport(t *testing.T) {
	svc, _ := newTestSvc("10", "2", 9)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-ok", Book: "main", Asset: "USDT", Nonce: "n-ok"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Consume(ctx, balproofgate.ConsumeReq{LeaseID: lease.ID, Signature: lease.Signature, GateRef: "gate-a"})
	if err != nil {
		t.Fatal(err)
	}
	exp, err := svc.Export(ctx, lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exp.ProofLeaseID != lease.ID || exp.IsolationMark != balproofgate.IsolationMark {
		t.Fatalf("%+v", exp)
	}
	raw, err := balproofgate.MarshalExport(exp)
	if err != nil || !strings.Contains(string(raw), "proof_lease_id") {
		t.Fatal(err)
	}
}

func TestBadInputs(t *testing.T) {
	svc, _ := newTestSvc("1", "0", 1)
	ctx := context.Background()
	cases := []balproofgate.IssueReq{
		{UserID: "", Book: "main", Asset: "USDT", Nonce: "n"},
		{UserID: "system", Book: "main", Asset: "USDT", Nonce: "n"},
		{UserID: "u", Book: "", Asset: "USDT", Nonce: "n"},
		{UserID: "u", Book: "main", Asset: "", Nonce: "n"},
		{UserID: "u", Book: "main", Asset: "USDT", Nonce: ""},
	}
	for _, c := range cases {
		if _, err := svc.Issue(ctx, c); err == nil {
			t.Fatalf("want error %+v", c)
		}
	}
	if _, err := balproofgate.NewService("", &balproofgate.MutableObserver{Avail: "1"}, balproofgate.NewMemStore()).Issue(ctx, balproofgate.IssueReq{UserID: "u", Book: "main", Asset: "USDT", Nonce: "n"}); err != balproofgate.ErrSecret {
		t.Fatalf("secret %v", err)
	}
}

func TestSigMismatch(t *testing.T) {
	svc, _ := newTestSvc("3", "0", 1)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-sig", Book: "main", Asset: "USDT", Nonce: "n-sig"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: "00"})
	if err != balproofgate.ErrSig {
		t.Fatalf("err=%v", err)
	}
}

func TestStaleGenerationAfterSecondIssue(t *testing.T) {
	obs := &balproofgate.MutableObserver{Avail: "5", Frozen: "0", ObserveGen: 1}
	svc := balproofgate.NewService("test-secret-r14", obs, balproofgate.NewMemStore())
	ctx := context.Background()
	a, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-stale", Book: "main", Asset: "USDT", Nonce: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-stale", Book: "main", Asset: "USDT", Nonce: "n2"}); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: a.ID, Signature: a.Signature})
	if err != balproofgate.ErrStaleGen {
		t.Fatalf("err=%v", err)
	}
}

func TestCloseDrift(t *testing.T) {
	svc, obs := newTestSvc("1", "0", 1)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-d", Book: "main", Asset: "USDT", Nonce: "n-d"})
	if err != nil {
		t.Fatal(err)
	}
	obs.CreditAvail("4")
	_, drift, err := svc.Recheck(ctx, balproofgate.RecheckReq{LeaseID: lease.ID, Signature: lease.Signature})
	if err != balproofgate.ErrLiveDrift || drift == nil {
		t.Fatalf("err=%v drift=%v", err, drift)
	}
	if err := svc.Drift().Verify(ctx, drift.ID); err != nil {
		t.Fatal(err)
	}
	closed, err := svc.CloseDrift(ctx, drift.ID, "ops-blaise")
	if err != nil || closed.Status != balproofgate.DriftClosed {
		t.Fatalf("%v %+v", err, closed)
	}
}

func TestDumpRestore(t *testing.T) {
	svc, _ := newTestSvc("9", "1", 2)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-dump", Book: "main", Asset: "USDT", Nonce: "n-dump"})
	if err != nil {
		t.Fatal(err)
	}
	mem := svc.store.(*balproofgate.MemStore)
	raw, err := balproofgate.MarshalDump(mem)
	if err != nil {
		t.Fatal(err)
	}
	d, err := balproofgate.UnmarshalDump(raw)
	if err != nil {
		t.Fatal(err)
	}
	dst := balproofgate.NewMemStore()
	balproofgate.RestoreStore(dst, d)
	got,
		err := dst.GetLease(ctx, lease.ID)
	if err != nil || got == nil || got.Signature != lease.Signature {
		t.Fatalf("%v %+v", err, got)
	}
}

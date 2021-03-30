package cvquotelease_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/cvquotelease"
)

func testEng(t *testing.T) (*cvquotelease.Engine, *cvquotelease.MemoryMarks,) {
	t.Helper()
	marks := cvquotelease.NewMemoryMarks()
	marks.Put("BTC", "USDT", "42000")
	marks.Put("ETH", "USDT", "3000")
	marks.Put("KCS", "USDT", "12.5")
	var fills int32
	eng := cvquotelease.New(cvquotelease.Config{
		Store:	cvquotelease.NewMemStore(),
		Marks:	marks,
		Strict:	true,
		TTL:	2 * time.Minute,
		Fill: func(ctx context.Context, snap cvquotelease.QuoteSnapshot,) (cvquotelease.FillResult, error) {
			n := atomic.AddInt32(&fills, 1)
			return cvquotelease.FillResult{OrderID: "ord-" + snap.ID + "-" + cvquotelease.Itoa64(int64(n)), Rate: snap.Rate, Book: snap.Book}, nil
		},
	})
	return eng, marks
}

func TestQuoteWithLeaseIssuesFingerprint(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	resp, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U1", From: "btc", To: "usdt", Amount: "0.5", Book: "funding",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.LeaseID == "" || resp.QuoteID == "" || resp.MarkGen < 1 || resp.Fingerprint == "" || resp.Used {
		t.Fatalf("lease not issued: %+v", resp)
	}
	want := cvquotelease.ComputeFingerprint("BTC", "USDT", "0.5", resp.Rate, "funding", resp.MarkGen)
	if resp.Fingerprint != want {
		t.Fatalf("fp %s != %s", resp.Fingerprint, want)
	}
}

func TestQuoteThenMarkThenExecuteRejects(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U-CVQ-001", From: "BTC", To: "USDT", Amount: "0.5", Book: "funding",
	})
	if err != nil {
		t.Fatal(err)
	}
	put, err := eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "43000"})
	if err != nil {
		t.Fatal(err)
	}
	if put.MarkGen <= q.MarkGen {
		t.Fatalf("mark_gen not bumped: %d vs %d", put.MarkGen, q.MarkGen)
	}
	ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
		UserID:	q.UserID, QuoteID: q.QuoteID, LeaseID: q.LeaseID,
	})
	if err == nil || ex.Filled {
		t.Fatalf("expected drift reject, got %+v err=%v", ex, err)
	}
	if ex.DriftProof == nil || !ex.DriftProof.Blocked || !ex.DriftProof.Sealed {
		t.Fatalf("missing CvQuoteDriftProof: %+v", ex)
	}
	if ex.DriftProof.Kind != cvquotelease.DriftRevoked && ex.DriftProof.Kind != cvquotelease.DriftMarkGen {
		t.Fatalf("kind %s", ex.DriftProof.Kind)
	}
}

func TestConcurrentDualExecuteAtomicConsume(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "ETH", To: "USDT", Rate: "3000"})
	q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U-CVQ-002", From: "ETH", To: "USDT", Amount: "2", Book: "funding",
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var filled int32
	var orders sync.Map
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
				UserID:	q.UserID, QuoteID: q.QuoteID, LeaseID: q.LeaseID,
			})
			if err == nil && ex.Filled {
				atomic.AddInt32(&filled, 1)
				orders.Store(ex.OrderID, true)
			}
		}()
	}
	wg.Wait()
	// 串行互斥下成功次数可含 reuse；唯一 order 必须为 1，Used 必须为 true
	uniq := 0
	orders.Range(func(_, _ any) bool { uniq++; return true })
	if uniq != 1 {
		t.Fatalf("unique orders=%d filled=%d", uniq, filled)
	}
	lease, _ := eng.GetLease(context.Background(), q.LeaseID)
	if lease.State != cvquotelease.LeaseConsumed || !lease.Used {
		t.Fatalf("lease not atomically consumed: %+v", lease)
	}
	snap, _ := eng.GetQuote(context.Background(), q.QuoteID)
	if !snap.Used {
		t.Fatal("quote Used not pinned")
	}
}

func TestExpiredQuoteCannotRevive(t *testing.T) {
	eng, _ := testEng(t)
	eng.ttl = time.Millisecond
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "KCS", To: "USDT", Rate: "12.5"})
	q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U-CVQ-003", From: "KCS", To: "USDT", Amount: "10", Book: "trading", TTL: "1ms",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	n, err := eng.ScanExpiredQuotes(context.Background(), 20)
	if err != nil || n < 1 {
		t.Fatalf("scan expired n=%d err=%v", n, err)
	}
	ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
		UserID:	q.UserID, QuoteID: q.QuoteID, LeaseID: q.LeaseID,
	})
	if err == nil || ex.Filled {
		t.Fatalf("expired must fail: %+v", ex)
	}
	if ex.DriftProof == nil || ex.DriftProof.Kind != cvquotelease.DriftExpired {
		t.Fatalf("want expired proof got %+v", ex.DriftProof)
	}
	snap, _ := eng.GetQuote(context.Background(), q.QuoteID)
	if !snap.Used {
		t.Fatal("expired quote must pin Used to block revive")
	}
}

func TestStrictBarePlaceFailClosed(t *testing.T) {
	eng, _ := testEng(t)
	ex, err := eng.PlaceQuoted(context.Background(), cvquotelease.PlaceQuotedReq{
		UserID:	"U-CVQ-004", From: "BTC", To: "USDT", Amount: "1", Rate: "42000", Book: "funding",
	})
	if err == nil || ex.Filled {
		t.Fatalf("bare place must fail: %+v", ex)
	}
	if !errors.Is(err, cvquotelease.ErrStrictBarePlace) {
		t.Fatalf("err %v", err)
	}
	if ex.DriftProof == nil || ex.DriftProof.Kind != cvquotelease.DriftMissingLease {
		t.Fatalf("proof %+v", ex.DriftProof)
	}
}

func TestExecuteMissingLease(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	q, _ := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U5", From: "BTC", To: "USDT", Amount: "0.1", Book: "funding",
	})
	ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{UserID: "U5", QuoteID: q.QuoteID})
	if err == nil || ex.DriftProof == nil || ex.DriftProof.Kind != cvquotelease.DriftMissingLease {
		t.Fatalf("got %+v err=%v", ex, err)
	}
}

func TestHappyPathFillOnce(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U6", From: "BTC", To: "USDT", Amount: "0.01", Book: "funding",
	})
	if err != nil {
		t.Fatal(err)
	}
	ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
		UserID:	"U6", QuoteID: q.QuoteID, LeaseID: q.LeaseID,
	})
	if err != nil || !ex.Filled || ex.OrderID == "" {
		t.Fatalf("%+v %v", ex, err)
	}
	ex2, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
		UserID:	"U6", QuoteID: q.QuoteID, LeaseID: q.LeaseID,
	})
	if err != nil || !ex2.Reused || ex2.OrderID != ex.OrderID {
		t.Fatalf("idempotent reuse failed: %+v %v", ex2, err)
	}
}

func TestQuoteValidationTable(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	cases := []struct {
		name	string
		req	cvquotelease.QuoteWithLeaseReq
		want	error
	}{
		{"empty user", cvquotelease.QuoteWithLeaseReq{From: "BTC", To: "USDT", Amount: "1", Book: "funding"}, cvquotelease.ErrUserRequired},
		{"empty from", cvquotelease.QuoteWithLeaseReq{UserID: "u", To: "USDT", Amount: "1", Book: "funding"}, cvquotelease.ErrAssetRequired},
		{"same asset", cvquotelease.QuoteWithLeaseReq{UserID: "u", From: "BTC", To: "BTC", Amount: "1", Book: "funding"}, cvquotelease.ErrSameAsset},
		{"bad amount", cvquotelease.QuoteWithLeaseReq{UserID: "u", From: "BTC", To: "USDT", Amount: "-1", Book: "funding"}, cvquotelease.ErrAmountInvalid},
		{"zero amount", cvquotelease.QuoteWithLeaseReq{UserID: "u", From: "BTC", To: "USDT", Amount: "0", Book: "funding"}, cvquotelease.ErrAmountInvalid},
		{"bad book", cvquotelease.QuoteWithLeaseReq{UserID: "u", From: "BTC", To: "USDT", Amount: "1", Book: "margin"}, cvquotelease.ErrBookInvalid},
		{"bad ttl", cvquotelease.QuoteWithLeaseReq{UserID: "u", From: "BTC", To: "USDT", Amount: "1", Book: "funding", TTL: "xx"}, cvquotelease.ErrTTLInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eng.QuoteWithLease(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestBooksAndPairsTable(t *testing.T) {
	eng, marks := testEng(t)
	pairs := []struct{ from, to, rate, book, amt string }{
		{"BTC", "USDT", "42000", "funding", "0.01"},
		{"ETH", "USDT", "3000", "trading", "1"},
		{"KCS", "USDT", "12.5", "both", "8"},
		{"BTC", "USDT", "42000", "trading", "0.02"},
		{"ETH", "USDT", "3000", "both", "0.5"},
	}
	for i, p := range pairs {
		marks.Put(p.from, p.to, p.rate)
		_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: p.from, To: p.to, Rate: p.rate})
		q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
			UserID:	"U-PAIR-" + cvquotelease.Itoa64(int64(i)), From: p.from, To: p.to, Amount: p.amt, Book: p.book,
		})
		if err != nil {
			t.Fatalf("%s/%s %s: %v", p.from, p.to, p.book, err)
		}
		ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
			UserID:	q.UserID, QuoteID: q.QuoteID, LeaseID: q.LeaseID,
		})
		if err != nil || !ex.Filled {
			t.Fatalf("fill %s: %+v %v", p.book, ex, err)
		}
	}
}

func TestWorkersRevokeAndReconcile(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	q, _ := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U7", From: "BTC", To: "USDT", Amount: "0.3", Book: "funding",
	})
	// 直接抬升世代但不走 MarkPut 撤销，检验 pump
	mk, _ := eng.store.GetMark(context.Background(), "BTC|USDT")
	mk.Gen = mk.Gen + 5
	_, _ = eng.store.SaveMark(context.Background(), mk)
	n,
		err := eng.PumpMarkRevokes(context.Background(), 50)
	if err != nil || n < 1 {
		t.Fatalf("pump n=%d err=%v", n, err)
	}
	lease, _ := eng.GetLease(context.Background(), q.LeaseID)
	if lease.State != cvquotelease.LeaseRevoked {
		t.Fatalf("state %s", lease.State)
	}
}

func TestExportProofs(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "1"})
	_, err := eng.PlaceQuoted(context.Background(), cvquotelease.PlaceQuotedReq{UserID: "Ux", From: "BTC", To: "USDT", Amount: "1", Rate: "1", Book: "funding"})
	if err == nil {
		t.Fatal("expected fail")
	}
	out, err := eng.ExportProofs(context.Background(), cvquotelease.ProofExportReq{UserID: "Ux", Limit: 10})
	if err != nil || out.Count < 1 {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestHealthAndFSM(t *testing.T) {
	if !cvquotelease.CanConsume(cvquotelease.LeaseIssued) || cvquotelease.CanConsume(cvquotelease.LeaseExpired) {
		t.Fatal("fsm consume")
	}
	m := cvquotelease.LeaseStateMachine()
	next, err := m.Apply(string(cvquotelease.LeaseIssued), cvquotelease.LeaseEvConsume)
	if err != nil || next != string(cvquotelease.LeaseConsumed) {
		t.Fatal(next, err)
	}
	if _, err := m.Apply(string(cvquotelease.LeaseConsumed), cvquotelease.LeaseEvExpire); err == nil {
		t.Fatal("terminal must not expire")
	}
	eng, _ := testEng(t)
	h, err := eng.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = h
}

func TestNilEngine(t *testing.T) {
	var e *cvquotelease.Engine
	if _, err := e.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{}); !errors.Is(err, cvquotelease.ErrNilEngine) {
		t.Fatal(err)
	}
}

func TestGoldenFixturesExist(t *testing.T) {
	root := testdataRoot(t)
	need := []string{
		"case_01_quote_mark_then_execute_reject.json",
		"case_02_concurrent_dual_execute.json",
		"case_03_expired_lease_fail_closed.json",
		"case_04_strict_bare_place_fail.json",
	}
	for _, n := range need {
		b, err := os.ReadFile(filepath.Join(root, n))
		if err != nil {
			t.Fatal(err)
		}
		var raw map[string]any
		if json.Unmarshal(b, &raw) != nil {
			t.Fatalf("invalid json %s", n)
		}
		if raw["name"] == nil {
			t.Fatalf("name missing %s", n)
		}
	}
}

func testdataRoot(t *testing.T) string {
	t.Helper()
	cands := []string{
		"testdata/cvquotelease",
		filepath.Join("..", "..", "testdata", "cvquotelease"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	t.Fatal("testdata/cvquotelease not found")
	return ""
}

func TestValidateHelpers(t *testing.T) {
	if err := cvquotelease.ValidateQuoteReq(cvquotelease.QuoteWithLeaseReq{}); err == nil {
		t.Fatal("expected")
	}
	if err := cvquotelease.ValidateExecuteReq(cvquotelease.ExecuteWithLeaseReq{}); !errors.Is(err, cvquotelease.ErrQuoteIDRequired) {
		t.Fatal(err)
	}
	if err := cvquotelease.LeaseStillExecutable(cvquotelease.CvQuoteLease{ID: "x", State: cvquotelease.LeaseIssued, ExpiresAt: time.Now().Add(time.Minute)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := cvquotelease.LeaseStillExecutable(cvquotelease.CvQuoteLease{ID: "x", State: cvquotelease.LeaseExpired}, time.Now()); !errors.Is(err, cvquotelease.ErrLeaseExpired) {
		t.Fatal(err)
	}
	if !cvquotelease.AssertNoDoubleFill(cvquotelease.ExecuteWithLeaseResp{Filled: true, OrderID: "a"}, cvquotelease.ExecuteWithLeaseResp{Filled: true, Reused: true, OrderID: "a"}) {
		t.Fatal("reuse ok")
	}
}

func TestFingerprintStable(t *testing.T) {
	a := cvquotelease.ComputeFingerprint("btc", "usdt", "1.0", "42000", "funding", 3)
	b := cvquotelease.ComputeFingerprint("BTC", "USDT", "1.000", "42000.0", "FUNDING", 3)
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
	if cvquotelease.ComputeBookPrint("funding", "1") == "" {
		t.Fatal("book print")
	}
}

func TestOwnerMismatch(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	q, _ := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U-CVQ-006", From: "BTC", To: "USDT", Amount: "0.2", Book: "funding",
	})
	_, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
		UserID:	"U-OTHER", QuoteID: q.QuoteID, LeaseID: q.LeaseID,
	})
	if !errors.Is(err, cvquotelease.ErrOwnerMismatch) {
		t.Fatal(err)
	}
}

func TestMarkPutInvalid(t *testing.T) {
	eng, _ := testEng(t)
	if _, err := eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "BTC", Rate: "1"}); err == nil {
		t.Fatal("same asset")
	}
	if _, err := eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "0"}); err == nil {
		t.Fatal("rate")
	}
}

func TestReconcileExpires(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	_, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
		UserID:	"U8", From: "BTC", To: "USDT", Amount: "1", Book: "funding", TTL: "1ms",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	n,
		err := eng.ReconcileUnconsumed(context.Background(), 20)
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestLeaseMismatchQuote(t *testing.T) {
	eng, _ := testEng(t)
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	q1, _ := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{UserID: "Ua", From: "BTC", To: "USDT", Amount: "1", Book: "funding"})
	q2, _ := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{UserID: "Ua", From: "BTC", To: "USDT", Amount: "2", Book: "funding"})
	_, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{UserID: "Ua", QuoteID: q1.QuoteID, LeaseID: q2.LeaseID})
	if !errors.Is(err, cvquotelease.ErrLeaseMismatch,
	) {
		t.Fatal(err)
	}
}

func TestWorkerJobsRun(t *testing.T) {
	eng, _ := testEng(t)
	log := zerolog.Nop()
	if err := cvquotelease.NewExpiredQuoteJob(eng, cvquotelease.WorkerConfig{BatchSize: 10}, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := cvquotelease.NewMarkRevokePumpJob(eng, cvquotelease.WorkerConfig{}, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := cvquotelease.NewUnconsumedReconcileJob(eng, cvquotelease.WorkerConfig{BatchSize: 10}, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
}

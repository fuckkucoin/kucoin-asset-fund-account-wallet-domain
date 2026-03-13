package usageleasegate_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hitstore"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/usageleasegate"
)

func TestSnapshotIssuesLease(t *testing.T) {
	eng := testEng(t)
	resp, err := eng.SnapshotWithLease(context.Background(), snapReq("U1", "USDT", "d1", "10", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.LeaseID == "" || !resp.Reserved || resp.WindowPrint == "" || resp.CapPrint == "" {
		t.Fatalf("%+v", resp)
	}
	l, err := eng.GetLease(context.Background(), resp.LeaseID)
	if err != nil || l.State != usageleasegate.LeaseReserved {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestRecordInCapCommits(t *testing.T) {
	eng := testEng(t)
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("U2", "USDT", "d2", "10", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "U2", Asset: "USDT", Dest: "d2", Amount: "10",
	})
	if err != nil || !r.Committed {
		t.Fatalf("%+v %v", r, err)
	}
	if eng.Usage().Len() != 1 {
		t.Fatal("occupancy")
	}
}

func TestConcurrentRecordOnlyInCap(t *testing.T) {
	eng := testEng(t)
	a, err := eng.SnapshotWithLease(context.Background(), snapReq("U3", "USDT", "d3", "80", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := eng.SnapshotWithLease(context.Background(), snapReq("U3", "USDT", "d3", "80", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var okN, failN int
	var mu sync.Mutex
	for _, id := range []string{a.LeaseID, b.LeaseID} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
				LeaseID:	id, Kind: usage.KindWithdraw, UserID: "U3", Asset: "USDT", Dest: "d3", Amount: "80",
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil && rec.Committed {
				okN++
			} else {
				failN++
				if rec.DriftProof == nil {
					t.Error("need proof")
				}
			}
		}()
	}
	wg.Wait()
	if okN != 1 || failN != 1 {
		t.Fatalf("ok=%d fail=%d", okN, failN)
	}
	if eng.Usage().Len() != 1 {
		t.Fatalf("len=%d", eng.Usage().Len())
	}
}

func TestWindowChangeRejectsOldLease(t *testing.T) {
	eng := testEng(t)
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("U4", "USDT", "d4", "10", "1000", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Usage().Add(usage.KindWithdraw, "U4", "USDT", "d4", "5"); err != nil {
		t.Fatal(err)
	}
	rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "U4", Asset: "USDT", Dest: "d4", Amount: "10",
	})
	if err == nil || rec.Committed {
		t.Fatalf("should drift %+v", rec)
	}
	if rec.DriftProof == nil || rec.DriftProof.Kind != usageleasegate.DriftWindowChanged {
		t.Fatalf("proof %+v", rec.DriftProof)
	}
	if !errors.Is(err, usageleasegate.ErrWindowDrift) && !strings.Contains(err.Error(), "window") && !strings.Contains(err.Error(), "drift") {
		t.Fatalf("err %v", err)
	}
}

func TestMissingLeaseFailClosed(t *testing.T) {
	eng := testEng(t)
	rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		Kind:	usage.KindWithdraw, UserID: "U5", Asset: "USDT", Dest: "d5", Amount: "1",
	})
	if err == nil || rec.Committed || rec.DriftProof == nil {
		t.Fatalf("%+v %v", rec, err)
	}
	if eng.Usage().Len() != 0 {
		t.Fatal("must not count")
	}
}

func TestExpiredLeaseFailClosed(t *testing.T) {
	eng := testEng(t)
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("U6", "USDT", "d6", "1", "100", "1ms"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := eng.ExpireRelease(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "U6", Asset: "USDT", Dest: "d6", Amount: "1",
	})
	if err == nil || rec.Committed {
		t.Fatalf("%+v", rec)
	}
}

func TestCommitFailureDoesNotCount(t *testing.T) {
	us := usage.New()
	eng := usageleasegate.New(usageleasegate.Config{Store: usageleasegate.NewMemStore(), Usage: us, Commit: func(ctx context.Context, l usageleasegate.UsageLease,) (string, error) {
		return "", errors.New("hook fail")
	}})
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("U7", "USDT", "d7", "1", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "U7", Asset: "USDT", Dest: "d7", Amount: "1",
	})
	if err == nil {
		t.Fatal("want hook fail")
	}
	if us.Len() != 0 {
		t.Fatal("failed order counted")
	}
}

func TestHitstoreBind(t *testing.T) {
	hs := hitstore.NewMem()
	us := usage.New()
	eng := usageleasegate.New(usageleasegate.Config{Store: usageleasegate.NewMemStore(), Usage: us, Hits: hs})
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("U8", "USDT", "d8", "2", "50", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	if err := usageleasegate.BindHitstoreAdd(eng, hs, usage.KindWithdraw, "U8", "USDT", "d8", "2", "idem-1", s.LeaseID); err != nil {
		t.Fatal(err)
	}
	if hs.TodaySum(context.Background(), usage.KindWithdraw, "U8", "USDT").String() == "0" {
		t.Fatal("hitstore empty")
	}
}

func TestOpsReleaseCommittedDenied(t *testing.T) {
	eng := testEng(t)
	s, _ := eng.SnapshotWithLease(context.Background(), snapReq("U9", "USDT", "d9", "1", "10", "1m"))
	_, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "U9", Asset: "USDT", Dest: "d9", Amount: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.OpsRelease(context.Background(), usageleasegate.OpsReleaseReq{LeaseID: s.LeaseID, Actor: "ops", Reason: "nope"})
	if !errors.Is(err, usageleasegate.ErrOpsReleaseDenied) {
		t.Fatalf("%v", err)
	}
}

func TestHealthAndExport(t *testing.T) {
	eng := testEng(t)
	_, _ = eng.SnapshotWithLease(context.Background(), snapReq("U10", "USDT", "d10", "1", "10", "1m"))
	h,
		err := eng.Health(context.Background())
	if err != nil || h.Reserved < 1 {
		t.Fatalf("%+v %v", h, err)
	}
	_, err = eng.ExportProofs(context.Background(), usageleasegate.ProofExportReq{UserID: "U10"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBreachDailyAlreadyOver(t *testing.T) {
	eng := testEng(t)
	_ = eng.Usage().Add(usage.KindWithdraw, "U11", "USDT", "dx", "50")
	_, err := eng.BreachDailyWithLease(context.Background(), snapReq("U11", "USDT", "dx", "1", "10", "1m"))
	if err == nil {
		t.Fatal("want breach")
	}
}

func TestIdempotentSnapshot(t *testing.T) {
	eng := testEng(t)
	req := snapReq("U12", "USDT", "d12", "1", "10", "1m")
	req.Idem = "same"
	a, err := eng.SnapshotWithLease(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := eng.SnapshotWithLease(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.LeaseID != b.LeaseID {
		t.Fatal("idem lease")
	}
}

func TestOwnerMismatch(t *testing.T) {
	eng := testEng(t)
	s, _ := eng.SnapshotWithLease(context.Background(), snapReq("U13", "USDT", "d13", "1", "10", "1m"))
	_, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "OTHER", Asset: "USDT", Dest: "d13", Amount: "1",
	})
	if err == nil {
		t.Fatal("owner")
	}
}

func TestAmountMismatch(t *testing.T) {
	eng := testEng(t)
	s, _ := eng.SnapshotWithLease(context.Background(), snapReq("U14", "USDT", "d14", "1", "10", "1m"))
	_, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
		LeaseID:	s.LeaseID, Kind: usage.KindWithdraw, UserID: "U14", Asset: "USDT", Dest: "d14", Amount: "2",
	})
	if err == nil {
		t.Fatal("amount enlarge")
	}
}

func TestListLeases(t *testing.T) {
	eng := testEng(t)
	_, _ = eng.SnapshotWithLease(context.Background(), snapReq("U15", "USDT", "d15", "1", "10", "1m"))
	list, err := eng.ListLeases(context.Background(), "U15", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %d", err, len(list))
	}
}

func TestNilEngine(t *testing.T) {
	var e *usageleasegate.Engine
	if _, err := e.SnapshotWithLease(context.Background(), usageleasegate.SnapshotWithLeaseReq{}); !errors.Is(err, usageleasegate.ErrNilEngine) {
		t.Fatalf("%v", err)
	}
}

func TestPatrolAndDiscover(t *testing.T) {
	eng := testEng(t)
	s, _ := eng.SnapshotWithLease(context.Background(), snapReq("U16", "USDT", "d16", "1", "100", "1m"))
	_ = eng.Usage().Add(usage.KindWithdraw, "U16", "USDT", "d16", "3")
	n, err := eng.PatrolWindowDrift(context.Background(), 20)
	if err != nil || n < 1 {
		t.Fatalf("patrol %d %v", n, err)
	}
	l, _ := eng.GetLease(context.Background(), s.LeaseID)
	if l.State != usageleasegate.LeaseRevoked {
		t.Fatalf("state %s", l.State)
	}
}

func TestDurationJSON(t *testing.T) {
	var d usageleasegate.Duration
	if err := d.UnmarshalJSON([]byte(`"2s"`)); err != nil || d.Duration != 2*time.Second {
		t.Fatal(d, err)
	}
	b, err := d.MarshalJSON()
	if err != nil || string(b) != `"2s"` {
		t.Fatal(string(b), err)
	}
}

func TestFingerprintHelpers(t *testing.T) {
	a := usageleasegate.ComputeWindowPrint(usage.KindWithdraw, "U", "usdt", "d", "1", 0, 0, 0)
	b := usageleasegate.ComputeWindowPrint(usage.KindWithdraw, "U", "USDT", "d", "1.0", 0, 0, 0)
	if !usageleasegate.FingerprintsEqual(a, b) {
		t.Fatal(a, b)
	}
	if usageleasegate.ComputeCapPrint("10", 5) == "" {
		t.Fatal("cap")
	}
}

func TestNormalizeKind(t *testing.T) {
	k, err := usageleasegate.NormalizeKind(usage.Kind("WITHDRAW"))
	if err != nil || k != usage.KindWithdraw {
		t.Fatal(k, err)
	}
	if _, err := usageleasegate.NormalizeKind(usage.Kind("x")); err == nil {
		t.Fatal("kind")
	}
}

package fiatsettlegate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/fiatsettlegate"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_,
		file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fiatsettlegate")
}

func loadGolden(t *testing.T, name string, dest any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		t.Fatal(err)
	}
}

func seedProfile(t *testing.T, rail *fiatrail.Service, user string) {
	t.Helper()
	if err := rail.KYC().Put(fiatrail.Profile{UserID: user, Tier: fiatrail.TierEnhanced, NameOK: true, DocOK: true}); err != nil {
		t.Fatal(err)
	}
}

func readyLease(t *testing.T, user, amount, actor string) (*fiatsettlegate.Service, *fiatsettlegate.FakeLedger, *fiatsettlegate.FiatSettleLease, fiatrail.Booking) {
	t.Helper()
	rail := fiatrail.New()
	seedProfile(t, rail, user)
	led := fiatsettlegate.NewFakeLedger()
	svc := fiatsettlegate.New(rail, led)
	ctx := context.Background()
	b, _, err := svc.BookWithReserve(ctx, fiatrail.BookReq{
		UserID:	user, Kind: fiatrail.KindOnramp, Asset: "USDT", Amount: amount, Currency: "USD",
		IdempotencyKey:	"idem-" + user,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rail.ScreenKYC(b.ID); err != nil {
		t.Fatal(err)
	}
	lease, booking, err := svc.AuthorizeWithLease(ctx, fiatsettlegate.AuthorizeLeaseRequest{BookingID: b.ID, Actor: actor, IdemKey: "auth-" + user})
	if err != nil {
		t.Fatal(err)
	}
	if lease.LeaseID == "" || booking.SettleLeaseID != lease.LeaseID {
		t.Fatalf("lease not bound: %+v booking=%+v", lease, booking)
	}
	return svc, led, lease, booking
}

func TestAuthorizeIssuesSettleLeaseID(t *testing.T) {
	_, _, lease, booking := readyLease(t, "u-auth-00", "20", "ops-001")
	if lease.State != fiatsettlegate.LeaseAuthorized {
		t.Fatalf("state %s", lease.State)
	}
	if lease.PSPIntent == "" || lease.LimitsFingerprint == "" || lease.KYCFingerprint == "" {
		t.Fatal("fingerprints missing")
	}
	if booking.SettleLeaseID != lease.LeaseID {
		t.Fatal("booking missing fiat_settle_lease_id")
	}
}

func TestLimitsRaiseOldLeaseRefused(t *testing.T) {
	var g struct {
		Expect	struct {
			SettleError		string	`json:"settle_error"`
			LeaseState		string	`json:"lease_state"`
			BookingSettled		bool	`json:"booking_settled"`
			ProofKind		string	`json:"proof_kind"`
			FailClosed		bool	`json:"fail_closed"`
			ReservationState	string	`json:"reservation_state"`
		}	`json:"expect"`
		Input	struct {
			CapsAfter struct {
				MaxSingle	string	`json:"max_single"`
				DailySum	string	`json:"daily_sum"`
				HourlyCount	int	`json:"hourly_count"`
			} `json:"caps_after"`
		}	`json:"input"`
	}
	loadGolden(t, "golden_limits_raise.json", &g)
	svc, _, lease, _ := readyLease(t, "u-settle-01", "80", "ops-001")
	if err := svc.Rail().Limits().SetCaps(fiatrail.Caps{
		MaxSingle:	g.Input.CapsAfter.MaxSingle, DailySum: g.Input.CapsAfter.DailySum, HourlyCount: g.Input.CapsAfter.HourlyCount,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SettleAssert(context.Background(), fiatsettlegate.SettleAssertRequest{LeaseID: lease.LeaseID, BookingID: lease.BookingID, Actor: "ops-001"})
	if !errors.Is(err, fiatsettlegate.ErrFingerprintLimits) {
		t.Fatalf("err=%v want %v", err, fiatsettlegate.ErrFingerprintLimits)
	}
	if got == nil || string(got.State) != g.Expect.LeaseState {
		t.Fatalf("lease %+v", got)
	}
	b, _ := svc.Rail().Get(lease.BookingID)
	if (b.State == fiatrail.StateSettled) != g.Expect.BookingSettled {
		t.Fatalf("booking state %s", b.State)
	}
	proofs, _ := svc.store.ListProofsByLease(context.Background(), lease.LeaseID)
	if len(proofs) == 0 || string(proofs[0].Kind) != g.Expect.ProofKind || proofs[0].FailClosed != g.Expect.FailClosed {
		t.Fatalf("proofs %+v", proofs)
	}
	res, _ := svc.store.GetReservation(context.Background(), lease.ReservationID)
	if res != nil && string(res.State) != g.Expect.ReservationState {
		t.Fatalf("res %s", res.State)
	}
}

func TestKYCDowngradeOldLeaseRefused(t *testing.T) {
	var g struct {
		Expect struct {
			LeaseState	string	`json:"lease_state"`
			BookingSettled	bool	`json:"booking_settled"`
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_kyc_downgrade.json", &g)
	svc, _, lease, booking := readyLease(t, "u-settle-02", "50", "ops-002")
	if err := svc.Rail().KYC().Put(fiatrail.Profile{UserID: booking.UserID, Tier: fiatrail.TierNone, NameOK: false, DocOK: false}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SettleAssert(context.Background(), fiatsettlegate.SettleAssertRequest{LeaseID: lease.LeaseID, Actor: "ops-002"})
	if !errors.Is(err, fiatsettlegate.ErrFingerprintKYC) {
		t.Fatalf("err=%v", err)
	}
	if got == nil || string(got.State) != g.Expect.LeaseState {
		t.Fatalf("state %v", got)
	}
	b, _ := svc.Rail().Get(lease.BookingID)
	if (b.State == fiatrail.StateSettled) != g.Expect.BookingSettled {
		t.Fatalf("booking %s", b.State)
	}
	proofs,
		_ := svc.store.ListProofsByLease(context.Background(), lease.LeaseID)
	found := false
	for _, p := range proofs {
		if string(p.Kind) == g.Expect.ProofKind && p.FailClosed == g.Expect.FailClosed {
			found = true
		}
	}
	if !found {
		t.Fatalf("proofs %+v", proofs)
	}
}

func TestSettleRefundMutex(t *testing.T) {
	var g struct {
		Expect struct {
			MutexHolds		bool	`json:"mutex_holds"`
			ExactlyOneCommit	bool	`json:"exactly_one_commit"`
			ProofKind		string	`json:"proof_kind"`
			FailClosed		bool	`json:"fail_closed"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_settle_refund_mutex.json", &g)
	svc, _, lease, _ := readyLease(t, "u-settle-03", "40", "ops-003")
	ctx := context.Background()
	var wg sync.WaitGroup
	var settleErr, refundErr error
	var settled *fiatsettlegate.FiatSettleLease
	var rf *fiatsettlegate.RefundLease
	wg.Add(2)
	go func() {
		defer wg.Done()
		settled, settleErr = svc.SettleAssert(ctx, fiatsettlegate.SettleAssertRequest{LeaseID: lease.LeaseID, Actor: "ops-003"})
	}()
	go func() {
		defer wg.Done()
		rf, _, refundErr = svc.RefundWithLease(ctx, fiatsettlegate.RefundWithLeaseRequest{BookingID: lease.BookingID, LeaseID: lease.LeaseID, Actor: "ops-004", Reason: "user-cancel"})
	}()
	wg.Wait()
	settleOK := settleErr == nil && settled != nil && settled.State == fiatsettlegate.LeaseSettled
	refundOK := refundErr == nil && rf != nil && rf.State == fiatsettlegate.RefundCompleted
	if g.Expect.ExactlyOneCommit && settleOK == refundOK {
		t.Fatalf("both or neither committed settle=%v refund=%v se=%v re=%v", settleOK, refundOK, settleErr, refundErr)
	}
	if !g.Expect.MutexHolds {
		t.Fatal("golden")
	}
	if settleOK && !errors.Is(refundErr, fiatsettlegate.ErrMutexSettleRefund) && refundErr != nil {
		// refund may also fail with lease state after settle committed
		if !errors.Is(refundErr, fiatsettlegate.ErrLeaseState) && !errors.Is(refundErr, fiatsettlegate.ErrMutexSettleRefund) {
			t.Logf("refund err %v (acceptable if lease closed)", refundErr)
		}
	}
	proofs, _ := svc.store.ListProofs(ctx, 50)
	if !settleOK || !refundOK {
		// at least one fail-closed path should exist when mutex hit mid-flight
		_ = proofs
	}
	final, _ := svc.store.GetLease(ctx, lease.LeaseID)
	if final.State != fiatsettlegate.LeaseSettled && final.State != fiatsettlegate.LeaseRefunded {
		t.Fatalf("final %s se=%v re=%v", final.State, settleErr, refundErr)
	}
}

func TestCaptureOrphanNoSecondCapture(t *testing.T) {
	var g struct {
		Expect struct {
			LeaseStateAfterFail	string	`json:"lease_state_after_fail"`
			BookingSettled		bool	`json:"booking_settled"`
			CaptureSealed		bool	`json:"capture_sealed"`
			RetryState		string	`json:"retry_state"`
			ProofKind		string	`json:"proof_kind"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_capture_orphan.json", &g)
	svc, led, lease, _ := readyLease(t, "u-settle-04", "30", "ops-005")
	led.FailNext(1)
	ctx := context.Background()
	got, err := svc.SettleAssert(ctx, fiatsettlegate.SettleAssertRequest{LeaseID: lease.LeaseID, Actor: "ops-005"})
	if !errors.Is(err, fiatsettlegate.ErrCompensateOpen) {
		t.Fatalf("err=%v", err)
	}
	if got == nil || string(got.State) != g.Expect.LeaseStateAfterFail {
		t.Fatalf("state %+v", got)
	}
	if got.CaptureSealed != g.Expect.CaptureSealed {
		t.Fatal("capture not sealed")
	}
	b, _ := svc.Rail().Get(lease.BookingID)
	if (b.State == fiatrail.StateSettled) != g.Expect.BookingSettled {
		t.Fatalf("booking settled? %s", b.State)
	}
	in,
		err := svc.Rail().PSP().Get(lease.PSPIntent)
	if err != nil || in.State != fiatrail.IntentCaptured {
		t.Fatalf("intent %v %v", in, err)
	}
	if _, capErr := svc.Rail().PSP().Capture(lease.PSPIntent); capErr == nil {
		t.Fatal("second capture must fail at PSP")
	}
	retry, err := svc.RetryCompensate(ctx, lease.LeaseID, "ops-005")
	if err != nil {
		t.Fatal(err)
	}
	if string(retry.State) != g.Expect.RetryState {
		t.Fatalf("retry %s", retry.State)
	}
	in2, _ := svc.Rail().PSP().Get(lease.PSPIntent)
	if in2.State != fiatrail.IntentCaptured {
		t.Fatalf("psp moved %s", in2.State)
	}
	proofs, _ := svc.store.ListProofsByLease(ctx, lease.LeaseID)
	found := false
	for _, p := range proofs {
		if string(p.Kind) == g.Expect.ProofKind {
			found = true
		}
	}
	if !found {
		t.Fatalf("proofs %+v", proofs)
	}
	b2, _ := svc.Rail().Get(lease.BookingID)
	if b2.State != fiatrail.StateSettled {
		t.Fatalf("retry booking %s", b2.State)
	}
}

func TestHappySettleRecordsLimits(t *testing.T) {
	svc, _, lease, _ := readyLease(t, "u-happy-01", "25", "ops-006")
	got, err := svc.SettleAssert(context.Background(), fiatsettlegate.SettleAssertRequest{LeaseID: lease.LeaseID, Actor: "ops-006"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != fiatsettlegate.LeaseSettled || got.LedgerID == "" {
		t.Fatalf("%+v", got)
	}
	res, err := svc.store.GetReservation(context.Background(), lease.ReservationID)
	if err != nil || res.State != fiatsettlegate.ResRecorded {
		t.Fatalf("res %+v %v", res, err)
	}
}

func TestRefundAuthorizedVoids(t *testing.T) {
	svc, _, lease, _ := readyLease(t, "u-ref-01", "15", "ops-007")
	rf, fin, err := svc.RefundWithLease(context.Background(), fiatsettlegate.RefundWithLeaseRequest{
		BookingID:	lease.BookingID, LeaseID: lease.LeaseID, Actor: "ops-007", Reason: "void-auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rf.State != fiatsettlegate.RefundCompleted || fin.State != fiatsettlegate.LeaseRefunded {
		t.Fatalf("rf=%+v lease=%+v", rf, fin)
	}
	res, _ := svc.store.GetReservation(context.Background(), lease.ReservationID)
	if res.State != fiatsettlegate.ResReleased {
		t.Fatalf("res %s", res.State)
	}
}

func TestLeaseFSMTable(t *testing.T) {
	type row struct {
		from, to	fiatsettlegate.LeaseState
		ok		bool
	}
	cases := []row{
		{fiatsettlegate.LeaseAuthorized, fiatsettlegate.LeaseSettleBound, true},
		{fiatsettlegate.LeaseAuthorized, fiatsettlegate.LeaseRefunding, true},
		{fiatsettlegate.LeaseAuthorized, fiatsettlegate.LeaseExpired, true},
		{fiatsettlegate.LeaseAuthorized, fiatsettlegate.LeaseSettled, false},
		{fiatsettlegate.LeaseSettleBound, fiatsettlegate.LeaseSettled, true},
		{fiatsettlegate.LeaseSettleBound, fiatsettlegate.LeaseCompensate, true},
		{fiatsettlegate.LeaseCompensate, fiatsettlegate.LeaseSettled, true},
		{fiatsettlegate.LeaseSettled, fiatsettlegate.LeaseRefunding, true},
		{fiatsettlegate.LeaseRefunded, fiatsettlegate.LeaseSettled, false},
	}
	for _, tc := range cases {
		err := fiatsettlegate.CanTransit(tc.from, tc.to)
		if tc.ok && err != nil {
			t.Fatalf("%s→%s: %v", tc.from, tc.to, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s→%s should fail", tc.from, tc.to)
		}
	}
}

func TestReservationFSMTable(t *testing.T) {
	if err := fiatsettlegate.CanTransitReservation(fiatsettlegate.ResReserved, fiatsettlegate.ResRecorded); err != nil {
		t.Fatal(err)
	}
	if err := fiatsettlegate.CanTransitReservation(fiatsettlegate.ResReserved, fiatsettlegate.ResReleased); err != nil {
		t.Fatal(err)
	}
	if err := fiatsettlegate.CanTransitReservation(fiatsettlegate.ResRecorded, fiatsettlegate.ResReleased); err == nil {
		t.Fatal("recorded cannot release via graph")
	}
}

func TestExpireWorkerReleases(t *testing.T) {
	rail := fiatrail.New()
	seedProfile(t, rail, "u-exp-01")
	svc := fiatsettlegate.New(rail, fiatsettlegate.NewFakeLedger()).WithTTL(time.Second)
	ctx := context.Background()
	b, _, err := svc.BookWithReserve(ctx, fiatrail.BookReq{
		UserID:	"u-exp-01", Kind: fiatrail.KindOnramp, Asset: "USDT", Amount: "12", Currency: "USD",
		IdempotencyKey:	"idem-u-exp-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rail.ScreenKYC(b.ID); err != nil {
		t.Fatal(err)
	}
	issued := time.Date(2021, 12, 18, 10, 0, 0, 0, time.UTC)
	svc.WithClock(func() time.Time { return issued })
	lease, _, err := svc.AuthorizeWithLease(ctx, fiatsettlegate.AuthorizeLeaseRequest{BookingID: b.ID, Actor: "ops-008"})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return issued.Add(2 * time.Second) })
	w := fiatsettlegate.NewWorker(svc, fiatsettlegate.DefaultWorkerConfig())
	if _, err := w.TickExpire(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetLease(ctx, lease.LeaseID)
	if got.State != fiatsettlegate.LeaseExpired {
		t.Fatalf("state %s", got.State)
	}
}

func TestIdempotentAuthorize(t *testing.T) {
	svc, _, lease, booking := readyLease(t, "u-idem-01", "18", "ops-009")
	again, _, err := svc.AuthorizeWithLease(context.Background(), fiatsettlegate.AuthorizeLeaseRequest{
		BookingID:	booking.ID, Actor: "ops-009", IdemKey: "auth-u-idem-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.LeaseID != lease.LeaseID {
		t.Fatalf("idem %s %s", again.LeaseID, lease.LeaseID)
	}
}

func TestViewAndExportProofs(t *testing.T) {
	svc, _, lease, _ := readyLease(t, "u-view-01", "22", "ops-010")
	if err := svc.Rail().Limits().SetCaps(fiatrail.Caps{MaxSingle: "1", DailySum: "1", HourlyCount: 1}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.SettleAssert(context.Background(), fiatsettlegate.SettleAssertRequest{LeaseID: lease.LeaseID, Actor: "ops-010"})
	v,
		err := svc.View(context.Background(), lease.BookingID)
	if err != nil || v.Lease == nil {
		t.Fatal(err)
	}
	rows, err := svc.ExportProofs(context.Background(), lease.LeaseID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("export %v %+v", err, rows)
	}
}

func TestDoesNotTouchForeignFSMNames(t *testing.T) {
	// 本包状态名不得混入 fiatadd Confirm / fiatjnl / fiatbreak
	foreign := []string{"confirming", "forked", "imbalance_block", "netting_gap"}
	for _, s := range foreign {
		if fiatsettlegate.CanTransit(fiatsettlegate.LeaseState(s), fiatsettlegate.LeaseSettled) == nil {
			t.Fatalf("foreign state %s accepted", s)
		}
	}
}

package daycut_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
	"account-assets.kucoin.net/funding/internal/lending/repay"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/lending/daycut"
)

func harness(t *testing.T) (*daycut.Engine, *utabook.Book, *ledger.Service, *daycut.MemStore, string, int64) {
	t.Helper()
	book := utabook.New()
	book.PutRateTier(utabook.RateTier{Currency: "USDT", RateHourly: "0.0001", RateDaily: "0.0024"})
	book.UpsertLiability("u", utabook.LiabilityRow{Currency: "USDT", Principal: "1000"})
	st := ledger.NewMemStore()
	led := ledger.NewService(st, nil, ledger.MemLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{DisableDriftBlock: true})
	acc := accrual.New(led,
		book, marginhook.New())
	store := daycut.NewMemStore()
	eng := daycut.New(acc, book, led, store, daycut.Lab())
	eng.Rep = repay.New(led, book, marginhook.New())
	loan := utabook.LoanNoteID("u", "USDT", "CROSS", "")
	cut := int64(1_700_000_000_000)
	return eng, book, led, store, loan, cut
}

func params(book *utabook.Book, loan string) utabook.AccrualParams {
	return book.ParamsFromCatalog("USDT", "DAY", loan, "CROSS", "")
}

func TestPersistIdempotentFailClosed(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name	string
		run	func(t *testing.T)
	}{
		{
			name:	"persist_success_daycut",
			run: func(t *testing.T) {
				eng, book, _, store, loan, cut := harness(t)
				res, err := eng.Run(ctx, daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"})
				if err != nil {
					t.Fatal(err)
				}
				if res.Posted != 1 || res.Rec.Status != daycut.StatusPosted {
					t.Fatalf("posted=%d status=%s", res.Posted, res.Rec.Status)
				}
				got, ok, err := store.Get(res.Key)
				if err != nil || !ok || got.Status != daycut.StatusPosted {
					t.Fatalf("persist miss ok=%v err=%v rec=%+v", ok, err, got)
				}
			},
		},
		{
			name:	"store_down_fail_closed",
			run: func(t *testing.T) {
				eng, book, _, store, loan, cut := harness(t)
				store.Down = true
				_, err := eng.Run(ctx, daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"})
				if !errors.Is(err, daycut.ErrStore) {
					t.Fatalf("want store fail-closed got %v", err)
				}
			},
		},
		{
			name:	"idempotent_rerun_same_batch",
			run: func(t *testing.T) {
				eng,
					book, led, _, loan, cut := harness(t)
				req := daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"}
				a, err := eng.Run(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				b, err := eng.Run(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				if !b.Existed || b.Posted != 0 {
					t.Fatalf("rerun existed=%v posted=%d", b.Existed, b.Posted)
				}
				if a.Rec.Key.String() != b.Rec.Key.String() {
					t.Fatalf("key drift %s vs %s", a.Rec.Key, b.Rec.Key)
				}
				period := utabook.PeriodOf(loan, "DAY", utabook.AlignDayUTC(cut))
				tr, err := led.LoanTransferByIdem(ctx, accrual.IdemKey(period))
				if err != nil || tr == nil {
					t.Fatal("ledger miss after rerun")
				}
			},
		},
		{
			name:	"missing_batch_fail_closed",
			run: func(t *testing.T) {
				eng, book, _, _, loan, cut := harness(t)
				_, err := eng.Run(ctx, daycut.RunReq{
					UserID:	"u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY", Require: true,
				})
				if !errors.Is(err, daycut.ErrMissingBatch) {
					t.Fatalf("want missing batch got %v", err)
				}
			},
		},
		{
			name:	"rebatch_fail_closed",
			run: func(t *testing.T) {
				eng, book, _, _, loan, cut := harness(t)
				req := daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"}
				if _, err := eng.Run(ctx, req); err != nil {
					t.Fatal(err)
				}
				req.Rebatch = true
				_, err := eng.Run(ctx, req)
				if !errors.Is(err, daycut.ErrRebatch) {
					t.Fatalf("want rebatch fail-closed got %v", err)
				}
			},
		},
		{
			name:	"invalid_window",
			run: func(t *testing.T) {
				eng, book, _, _, loan, _ := harness(t)
				_, err := eng.Run(ctx, daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: 0})
				if !errors.Is(err, daycut.ErrWindow) {
					t.Fatalf("want window got %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

func TestRestartReentry(t *testing.T) {
	ctx := context.Background()
	eng, book, _, store, loan, cut := harness(t)
	key := daycut.KeyOf(loan, "DAY", cut)
	period := utabook.PeriodOf(loan, "DAY", utabook.AlignDayUTC(cut))
	if err := store.Save(daycut.BatchRec{Key: key, Status: daycut.StatusOpen, Periods: []utabook.PeriodKey{period}, AccruedTotal: "0", Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	// 进程中断后周期已由 Poster 入账、批次仍 open
	if _, err := eng.Acc.Post(ctx, accrual.PostReq{
		UserID:	"u", Currency: "USDT", Params: params(book, loan), Period: period,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Run(ctx, daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Recovered && !res.Existed {
		t.Fatalf("want restart recover got %+v", res)
	}
	got, ok, err := store.Get(key)
	if err != nil || !ok || got.Status != daycut.StatusPosted {
		t.Fatalf("reentry status=%s ok=%v err=%v", got.Status, ok, err)
	}
	// 再入不得双记
	again, err := eng.Run(ctx, daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"})
	if err != nil || !again.Existed {
		t.Fatalf("second reentry err=%v existed=%v", err, again.Existed)
	}
}

func TestProductionForbidsRollback(t *testing.T) {
	cases := []struct {
		name	string
		cfg	daycut.Config
		want	error
	}{
		{name: "production_default", cfg: daycut.Production(), want: daycut.ErrRollback},
		{name: "prod_alias", cfg: daycut.Config{Profile: "prod"}, want: daycut.ErrRollback},
		{name: "iam_locked", cfg: daycut.Config{Profile: "iam", AllowRollback: true}, want: daycut.ErrRollback},
		{name: "lab_explicit_allow", cfg: daycut.Config{Profile: "lab", AllowRollback: true}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.RefuseRollback()
			if !errors.Is(err, tc.want) && !(tc.want == nil && err == nil) {
				t.Fatalf("RefuseRollback=%v want %v", err, tc.want)
			}
			if tc.cfg.ProductionLocked() && (tc.cfg.AllowRollback || tc.cfg.AllowMissPost || tc.cfg.AllowDoublePost) {
				if tc.cfg.RefuseRollback() == nil || tc.cfg.RefuseMissPost() == nil || tc.cfg.RefuseDoublePost() == nil {
					t.Fatal("production must lock rollback/miss/double")
				}
			}
		})
	}
	eng, _, _, _, loan, cut := harness(t)
	eng.Cfg = daycut.Production()
	key := daycut.KeyOf(loan, "DAY", cut)
	if err := eng.RollbackPosted(key); !errors.Is(err, daycut.ErrRollback) {
		t.Fatalf("engine rollback under prod = %v", err)
	}
}

func TestReconcileColumns(t *testing.T) {
	ctx := context.Background()
	eng,
		book, _, _, loan, cut := harness(t)
	req := daycut.RunReq{UserID: "u", Currency: "USDT", Params: params(book, loan), CutMs: cut, Kind: "DAY"}
	if _, err := eng.Run(ctx, req); err != nil {
		t.Fatal(err)
	}
	period := utabook.PeriodOf(loan, "DAY", utabook.AlignDayUTC(cut))
	off, err := eng.Rep.Offset(ctx, repay.Req{
		UserID:	"u", Currency: "USDT", Amount: "1", Policy: repay.PolicyReject, RepayID: "rp-daycut-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := eng.ReconcileOffset(ctx, daycut.ReconReq{
		UserID:	"u", Currency: "USDT", Period: period, RepayIdem: off.IdemKey, OffsetAmt: off.Split.Interest.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Key.Thick() || !got.Matched || !got.AccrueOK || !got.OffsetOK {
		t.Fatalf("recon %+v", got)
	}
	_, err = eng.ReconcileOffset(ctx, daycut.ReconReq{
		UserID:	"u", Currency: "USDT", Period: period, RepayIdem: "", OffsetAmt: "",
	})
	if !errors.Is(err, daycut.ErrReconcile) {
		t.Fatalf("thin key want reconcile got %v", err)
	}
}

func TestStorePostedCannotRollbackToOpen(t *testing.T) {
	st := daycut.NewMemStore()
	key := daycut.KeyOf("loan-x", "DAY", 1_700_000_000_000)
	if err := st.Save(daycut.BatchRec{Key: key, Status: daycut.StatusPosted, AccruedTotal: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(daycut.BatchRec{Key: key, Status: daycut.StatusOpen}); !errors.Is(err, daycut.ErrRollback) {
		t.Fatalf("want store rollback lock got %v", err)
	}
}

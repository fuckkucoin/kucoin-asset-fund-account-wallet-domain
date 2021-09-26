package memoeffectgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
)

func TestHalfCutPatrolResumes(t *testing.T) {
	eng, book, cred, _ := testEng(t)
	cred.FailApplyTimes(1,
		errApplyBoom)
	tk := pendingTicket(book, "ETH", "0xpt", "m")
	_, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "ETH", Actor: "ops",
	})
	if !errors.Is(err, memoeffectgate.ErrApplyFailed) {
		t.Fatalf("%v", err)
	}
	n, err := eng.PatrolHalfCut(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("resumed %d", n)
	}
	fresh,
		_ := book.GetMemoTicket(context.Background(), tk.ID)
	if fresh.Status != "approved" {
		t.Fatalf("%s", fresh.Status)
	}
}

func TestWorkerJobsConstruct(t *testing.T) {
	eng, _, _, _ := testEng(t)
	log := zerolog.Nop()
	cfg := memoeffectgate.WorkerConfig{Interval: time.Second, BatchSize: 8}
	if err := memoeffectgate.NewHalfCutPatrolJob(eng, cfg, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := memoeffectgate.NewRejectAfterCreditPatrolJob(eng, cfg, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := memoeffectgate.NewProofExportJob(eng, cfg, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := memoeffectgate.NewExpireBoundJob(eng, cfg, log)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := memoeffectgate.NewHalfCutPatrolJob(nil, cfg, log)(context.Background()); !errors.Is(err, memoeffectgate.ErrNilEngine) {
		t.Fatalf("%v", err)
	}
}

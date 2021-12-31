package workerbnd_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/workerbnd"
)

func TestAssembleRefuseSkipBindSQLHealthy(t *testing.T) {
	rt := &app.Runtime{}
	_, err := workerbnd.Assemble(rt, workerbnd.Options{
		Bind:		kcfund.BindFundsRecon,
		SQLBound:	false,
		RequireSQL:	true,
		Segments:	workerbnd.ProdSegments(),
	})
	if !errors.Is(err, workerbnd.ErrBindSQL) {
		t.Fatalf("got %v", err)
	}
}

func TestFakeFlowMissingSegment(t *testing.T) {
	f := &workerbnd.FakeFlow{}
	if err := f.Run(workerbnd.SegmentSet{Observe: true, Credit: true, Recon: true}); !errors.Is(err, workerbnd.ErrSegment) {
		t.Fatalf("got %v", err)
	}
}

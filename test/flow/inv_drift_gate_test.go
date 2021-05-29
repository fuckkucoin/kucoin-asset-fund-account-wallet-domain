package flow

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/invcheck"
)

// stubDriftChecker 用于的 DriftChecker stub。
type stubDriftChecker struct {
	has bool
	err error
}

   func (s *stubDriftChecker) HasActiveAdvice(_ context.Context, _ string) (bool, error) {
	  return s.has, s.err
   }

  func TestDriftGate_NoAdvice_PassThrough(t *testing.T) {
	  gate := invcheck.NewDriftGate(&stubDriftChecker{has: false, err: nil})
	  if err := gate.Check(context.Background(), "USDT"); err != nil {
		  t.Fatalf("expected pass, got: %v", err)
	   }
  }

  func TestDriftGate_ActiveAdvice_Blocked(t *testing.T) {
	   gate := invcheck.NewDriftGate(&stubDriftChecker{has: true, err: nil})
	  err := gate.Check(context.Background(), "BTC")
	   if err == nil {
		  t.Fatal("expected error when advice is active")
	  }
	   if !errors.Is(err, invcheck.ErrSealAdviceActive) {
		  t.Fatalf("expected ErrSealAdviceActive, got: %v", err)
	  }
   }

  func TestDriftGate_CheckerError_FailClosed(t *testing.T) {
	   checkerErr := errors.New("db timeout")
	   gate := invcheck.NewDriftGate(&stubDriftChecker{has: false,
		  err: checkerErr})
	   err := gate.Check(context.Background(), "ETH")
	   if err == nil {
		   t.Fatal("expected fail-closed error when checker returns error")
	   }
	  if !errors.Is(err, invcheck.ErrCheckerUnavailable) {
		  t.Fatalf("expected ErrCheckerUnavailable, got: %v", err)
	  }
   }

  func TestDriftGate_NilChecker_PassThrough(t *testing.T) {
	   gate := invcheck.NewDriftGate(nil)
	   if err := gate.Check(context.Background(), "USDT"); err != nil {
		   t.Fatalf("nil checker should pass: %v", err)
	  }
   }

   func TestDriftGate_EmptyAsset_PassThrough(t *testing.T) {
	   gate := invcheck.NewDriftGate(&stubDriftChecker{has: true,
		   err: nil})
	   if err := gate.Check(context.Background(), ""); err != nil {
		   t.Fatalf("empty asset should pass: %v", err)
	  }
   }

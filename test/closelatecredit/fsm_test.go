package closelatecredit_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

  func TestEpochTransitions(t *testing.T) {
	   cases := []struct {
		   from, to closelatecredit.EpochState
		  ok bool
	  }{
		   {closelatecredit.EpochOpen, closelatecredit.EpochClosing, true},
		   {closelatecredit.EpochClosing, closelatecredit.EpochClosed, true},
		  {closelatecredit.EpochClosed, closelatecredit.EpochReopened, true},
		  {closelatecredit.EpochReopened, closelatecredit.EpochOpen, true},
		  {closelatecredit.EpochClosed, closelatecredit.EpochOpen, false},
		   {closelatecredit.EpochOpen,
			   closelatecredit.EpochClosed, false},
	  }
	   for _, tc := range cases {
		  err := closelatecredit.AssertEpochTransit(tc.from, tc.to)
		  if tc.ok && err != nil {
			   t.Fatalf("%s→%s: %v", tc.from, tc.to, err)
		  }
		  if !tc.ok && err == nil {
			   t.Fatalf("%s→%s allowed", tc.from, tc.to)
		  }
	  }
   }

   func TestLateAndHoldTransitions(t *testing.T) {
	   if err := closelatecredit.AssertLateTransit(closelatecredit.LateOpen, closelatecredit.LateDisputed); err != nil {
		  t.Fatal(err)
	   }
	   if err := closelatecredit.AssertLateTransit(closelatecredit.LateClosed, closelatecredit.LateOpen); err == nil {
		   t.Fatal("closed→open")
	   }
	   if err := closelatecredit.AssertHoldTransit(closelatecredit.HoldParked, closelatecredit.HoldResolved); err != nil {
		  t.Fatal(err)
	  }
	   var te closelatecredit.TransitError
	  err := closelatecredit.AssertHoldTransit(closelatecredit.HoldExported, closelatecredit.HoldParked)
	  if !errors.As(err, &te) {
		   t.Fatalf("want TransitError got %v", err)
	  }
  }

  func TestDecisionHelpers(t *testing.T) {
	  if closelatecredit.DecisionForMissing() != closelatecredit.DecisionReject {
		   t.Fatal("missing")
	   }
	   if closelatecredit.DecisionForStale() != closelatecredit.DecisionReject {
		  t.Fatal("stale")
	   }
	   if closelatecredit.DecisionForClosed(closelatecredit.SourceObserve, false) != closelatecredit.DecisionDispute {
		   t.Fatal("closed dispute")
	   }
	  if closelatecredit.AllowedWithoutEpoch(closelatecredit.SourceOutbox) {
		  t.Fatal("policy leak")
	   }
	   if !closelatecredit.CreditRequiresEpoch(ptr(1)) || closelatecredit.CreditRequiresEpoch(nil) {
		  t.Fatal("requires")
	   }
   }

   func TestAmountAsset(t *testing.T) {
	  if !closelatecredit.AmountOK("12.50") || closelatecredit.AmountOK("0") || closelatecredit.AmountOK("x") {
		   t.Fatal("amount")
	  }
	  if !closelatecredit.AssetOK("USDT") || closelatecredit.AssetOK("us-dt") {
		   t.Fatal("asset")
	   }
   }

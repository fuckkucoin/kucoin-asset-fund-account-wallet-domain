package flow

import (
	"context"
	"net/http"
	"testing"
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

   func TestTravelWithdrawRequiresMessageWhenOverThreshold(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	  book := travelvasp.NewThresholdBook("1", nil)
	  book.BindUsdRates(map[string]string{"USDT": "1"})
	  rt.TravelThresh = book
	  gate := travelvasp.GateForRuntime(rt)
	   req, err := gate.Requirements(travelvasp.RequirementsReq{Asset: "USDT", Amount: "100"})
	   if err != nil || !req.Required {
		   t.Fatalf("expected travel required, got %+v err=%v", req, err)
	  }
	  if err := gate.ValidateWithdraw(travelvasp.WithdrawCheck{
		   UserID: uid, Asset: "USDT", Amount: "100", Chain: "ETH",
		  ToAddress: "0xabc", BeneficiaryVASP: "PEER",
	   }); err == nil {
		   t.Fatal("expected travel message required")
	  }
	  msg, err := gate.OpenOutbound(ctx, travelvasp.OpenOutboundReq{
		   UserID: uid, Asset: "USDT", Amount: "100", Chain: "ETH",
		   ToAddress: "0xabc", BeneficiaryVASP: "PEER", BeneficiaryName: "bob",
		  ClientOid: "travel-flow-1",
	   })
	  if err != nil {
		  t.Fatal(err)
	   }
	   msg, err = gate.SubmitOutbound(msg.ID, "test")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if err := gate.ValidateWithdraw(travelvasp.WithdrawCheck{
		  UserID: uid, Asset: "USDT", Amount: "100", Chain: "ETH",
		  ToAddress: "0xabc", BeneficiaryVASP: "PEER", TravelMessageID: msg.ID,
	  }); err != nil {
		   t.Fatalf("validate after submit: %v", err)
	   }
   }

  func TestTravelPublicRequirementsHTTP(t *testing.T) {
	   rt, h := router(t)
	   w := doJSON(t, rt, h, http.MethodGet, "/api/v1/travel/requirements?currency=USDT&amount=0.01", nil)
	   if w.Code != 200 {
		  t.Fatalf("requirements %d %s", w.Code, w.Body.String())
	   }
   }

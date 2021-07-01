package fundmesh_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/fundmesh"
)

  func TestBO31InvalidKeyCorpus(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  for _, k := range fundmesh.InvalidOutboundKeys() {
		  _, err := eng.SubmitWithdraw(context.Background(), outboundReq(k, "USDT", "1", "ETH", "0x1"))
		   if err == nil {
			   t.Fatalf("key %q accepted", k)
		   }
	  }
	   if debitCalls(eng) != 0 {
		  t.Fatal("corpus debit")
	  }
  }

  func TestBO32InvalidAmountCorpus(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   for _, amt := range fundmesh.InvalidOutboundAmounts() {
		  _, err := eng.SubmitWithdraw(context.Background(), outboundReq("client-key-alpha01", "USDT", amt, "ETH", "0x1"))
		   if err == nil {
			   t.Fatalf("amount %q accepted", amt)
		  }
	  }
	  if debitCalls(eng) != 0 {
		   t.Fatal("amount debit")
	   }
   }

  func TestBO33ValidBareKeysCanonical(t *testing.T) {
	  for _, k := range fundmesh.ValidBareClientKeys() {
		   c,
			   err := fundmesh.CanonicalizeClientKey(k)
		  if err != nil {
			   t.Fatalf("%s %v", k, err)
		   }
		   if !strings.HasPrefix(c, fundmesh.IdemSchemeV1+".c.") {
			  t.Fatalf("%s => %s", k, c)
		  }
		   if _, err := fundmesh.ParseOutboundKey(c); err != nil {
			   t.Fatalf("parse %s %v", c, err)
		  }
	  }
  }

  func TestBO34HTTPGetOutboundAndProofs(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   res, err := eng.SubmitWithdraw(context.Background(), outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc"))
	   if err != nil {
		   t.Fatal(err)
	  }
	   r := newHTTP(eng)
	   w := do(r, http.MethodGet, "/v1/funding/withdrawals/"+res.WithdrawID+"/outbound", nil, "u1")
	  if w.Code != 200 {
		  t.Fatalf("get %d %s", w.Code, w.Body.String())
	  }
	  w2 := do(r, http.MethodGet, "/v1/funding/mesh/outbound-proofs", nil, "u1")
	  if w2.Code != 200 {
		  t.Fatalf("proofs %d", w2.Code)
	  }
	   w3 := do(r, http.MethodGet, "/v1/funding/mesh/outbound-inspect", nil, "u1")
	   if w3.Code != 200 {
		   t.Fatalf("inspect %d", w3.Code)
	   }
	  if debitCalls(eng) != 1 {
		  t.Fatal("read path debit")
	   }
   }

  func TestBO35PolicyAndFSM(t *testing.T) {
	  if !fundmesh.DefaultPolicy().OutboundFailClosed() {
		  t.Fatal("policy")
	   }
	  if fundmesh.OutboundDebitAllowed(fundmesh.OutboundAccepted, fundmesh.WithdrawFrozen) {
		  t.Fatal("accepted must not debit")
	   }
	  if !fundmesh.CanReplaySameResult(&fundmesh.OutboundRecord{State: fundmesh.OutboundAccepted}) {
		  t.Fatal("replay accepted")
	   }
	   if fundmesh.ReplayCodeFor(&fundmesh.OutboundRecord{State: fundmesh.OutboundClosed, Phase: fundmesh.WithdrawCancelled}) != fundmesh.CodeOutboundCancelled {
		  t.Fatal("closed code")
	   }
  }

   func TestBO36KeyScope(t *testing.T) {
	  if fundmesh.KeyScope("u1", "k") != "u1|k" {
		   t.Fatal("scope")
	  }
  }

  func TestBO37HTTPConflict(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   r := newHTTP(eng)
	  body := map[string]string{"asset": "USDT", "amount": "3", "chain": "ETH", "dest": "0xabc", "idempotency_key": "client-key-alpha01"}
	   if w := do(r, http.MethodPost, "/v1/funding/withdrawals", body, "u1"); w.Code != 200 {
		   t.Fatalf("%d %s", w.Code, w.Body.String())
	  }
	  body["amount"] = "4"
	   w := do(r, http.MethodPost, "/v1/funding/withdrawals", body, "u1")
	   if w.Code != http.StatusConflict {
		  t.Fatalf("conflict http %d %s", w.Code, w.Body.String())
	   }
   }

  func TestBO38NilEngine(t *testing.T) {
	  var eng *fundmesh.Engine
	  if _, err := eng.SubmitWithdraw(context.Background(), outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0x1")); err == nil {
		  t.Fatal("nil engine")
	   }
   }

package fundmesh_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"account-assets.kucoin.net/funding/internal/fundmesh"
)

   func TestBO18CancelThenReplayNoDebit(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   ctx := context.Background()
	   sub, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-alpha01", "USDT", "10", "ETH", "0xabc"))
	   if err != nil {
		   t.Fatal(err)
	   }
	   if debitCalls(eng) != 1 {
		  t.Fatal("submit debit")
	   }
	   can, err := eng.CancelWithdraw(ctx, cancelReq(sub.WithdrawID, "DELETE"))
	   if err != nil {
		   t.Fatal(err)
	  }
	   if can.Code != fundmesh.CodeOK {
		  t.Fatalf("cancel %s", can.Code)
	  }
	  rec, err := eng.OutboundAfterCancel(ctx, sub.WithdrawID)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if rec.State != fundmesh.OutboundClosed || rec.Phase != fundmesh.WithdrawCancelled {
		   t.Fatalf("align %+v", rec)
	   }
	  again, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-alpha01", "USDT", "10", "ETH", "0xabc"))
	  if err != nil {
		  t.Fatal(err)
	   }
	   if again.Code != fundmesh.CodeOutboundCancelled || !again.Idempotent {
		   t.Fatalf("replay after cancel %+v", again)
	   }
	  if debitCalls(eng) != 1 {
		   t.Fatalf("cancel replay debit %d", debitCalls(eng))
	   }
   }

  func TestBO19CancelEntryStillHomologous(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  a, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xa"))
	   if err != nil {
		   t.Fatal(err)
	   }
	   b, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-beta0002", "USDT", "1", "ETH", "0xb"))
	  if err != nil {
		   t.Fatal(err)
	   }
	   del, err := eng.CancelWithdraw(ctx, cancelReq(a.WithdrawID, "DELETE"))
	   if err != nil {
		  t.Fatal(err)
	   }
	   post, err := eng.CancelWithdraw(ctx, cancelReq(b.WithdrawID, "POST"))
	   if err != nil {
		   t.Fatal(err)
	   }
	  if !fundmesh.SameCancelCode(del, post) {
		  t.Fatalf("cancel fork %s %s", del.Code, post.Code)
	  }
   }

  func TestBO20IdempotentCancelAfterOutbound(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  sub, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc"))
	   if err != nil {
		   t.Fatal(err)
	  }
	   first, err := eng.CancelWithdraw(ctx, cancelReq(sub.WithdrawID, "POST"))
	  if err != nil {
		   t.Fatal(err)
	   }
	   second, err := eng.CancelWithdraw(ctx, cancelReq(sub.WithdrawID, "DELETE"))
	  if err != nil {
		  t.Fatal(err)
	   }
	  if first.Code != fundmesh.CodeOK || second.Code != fundmesh.CodeIdempotent {
		  t.Fatalf("first=%s second=%s", first.Code, second.Code)
	   }
   }

  func TestBO21CancelHTTPAfterSubmit(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   sub, err := eng.SubmitWithdraw(context.Background(), outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc"))
	   if err != nil {
		   t.Fatal(err)
	   }
	   r := newHTTP(eng)
	   w := do(r, http.MethodDelete, "/v1/funding/withdrawals/"+sub.WithdrawID, nil, "u1")
	  if w.Code != 200 {
		   t.Fatalf("cancel http %d %s", w.Code, w.Body.String())
	   }
   }

  func TestBO22MissingPayload(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   _, err := eng.SubmitWithdraw(context.Background(), fundmesh.OutboundRequest{UserID: "u1"})
	  if !errors.Is(err, fundmesh.ErrOutboundPayload) {
		  t.Fatalf("got %v", err)
	   }
  }

   func TestBO23PathsCatalog(t *testing.T) {
	  if len(fundmesh.OutboundWritePaths()) != 2 {
		  t.Fatal("write paths")
	   }
	   if len(fundmesh.CancelWritePaths()) != 3 {
		  t.Fatal("cancel paths")
	   }
	   if len(fundmesh.OutboundWriteMethods()) != 2 {
		  t.Fatal("methods")
	  }
   }

   func TestBO24ParseAndDerive(t *testing.T) {
	  canon, err := fundmesh.CanonicalizeClientKey("client-key-alpha01")
	  if err != nil {
		   t.Fatal(err)
	   }
	   p, err := fundmesh.ParseOutboundKey(canon)
	  if err != nil || !p.Client {
		   t.Fatalf("%+v %v", p, err)
	   }
	  req := outboundReq("", "USDT", "1", "ETH", "0xabc")
	  raw, parts, err := fundmesh.DeriveOutboundKey(req)
	   if err != nil || !parts.Derived || raw == "" {
		   t.Fatal(err)
	   }
	  if !fundmesh.SameIdemKey(raw, raw) {
		   t.Fatal("same key")
	  }
  }

  func TestBO25FingerprintBind(t *testing.T) {
	  req := outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc")
	  if err := fundmesh.ValidateOutboundRequest(&req); err != nil {
		  t.Fatal(err)
	   }
	  fp, err := fundmesh.FingerprintOf(req)
	  if err != nil || !fundmesh.ValidFingerprint(fp) {
		   t.Fatal(err)
	   }
	  key,
		  _, err := fundmesh.NormalizeOutboundKey(req)
	  if err != nil {
		   t.Fatal(err)
	  }
	  if err := fundmesh.BindKeyAndFingerprint(key, fp); err != nil {
		   t.Fatal(err)
	  }
	  if !fundmesh.FingerprintDrift(fp, "00"+fp[2:]) {
		   t.Fatal("drift")
	  }
  }

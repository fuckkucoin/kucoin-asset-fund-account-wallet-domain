package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledshard"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func TestLedShardCrossShardFixture(t *testing.T) {
	  e := ledshard.New()
	  if err := e.Bootstrap(8); err != nil {
		  t.Fatal(err)
	  }
	   src, dst, _, _, err := e.FindPair("USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if _, err := e.Seed(src, "funding", "USDT", "100"); err != nil {
		   t.Fatal(err)
	  }
	  it, err := e.Open(ledshard.OpenReq{
		  Idem: "flow-ok", SrcUser: src, SrcBook: "funding", DstUser: dst, DstBook: "funding",
		  Asset: "USDT", Amount: "15",
	   })
	   if err != nil {
		  t.Fatal(err)
	   }
	   done, err := e.Run(it.ID)
	   if err != nil || done.State != "committed" {
		  t.Fatalf("run %+v %v", done, err)
	   }
	  rep, err := e.Conserve(done.ID)
	   if err != nil || !rep.OK || rep.Voucher == "" {
		   t.Fatalf("conserve %+v %v", rep, err)
	   }
	  if _, err := e.Voucher(done.VoucherID); err != nil {
		  t.Fatal(err)
	   }
   }

   func TestLedShardHTTPDeskAndResolve(t *testing.T) {
	  gin.SetMode(gin.TestMode)
	  rt := openMem(t)
	   h := svcboot.Router("led-shard", "ledger-transfer-freeze-reconciliation.kucoin.net", rt, ledshard.Register)
	   w := postShard(t, rt, h, "/v1/ledshard/resolve", map[string]any{"user_id": "system.bridge", "book": "", "asset": "USDT"})
	   if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("sh-sys-bridge")) {
		   t.Fatalf("resolve %d %s", w.Code, w.Body.String())
	  }
	   w = postShard(t, rt, h, "/v1/ledshard/resolve", map[string]any{"user_id": "system.fee", "asset": "USDT"})
	   if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("sh-sys-fee")) {
		  t.Fatalf("fee %d %s", w.Code, w.Body.String())
	  }
	   w = postShard(t, rt, h, "/v1/ledshard/resolve", map[string]any{"user_id": "system.cold", "asset": "USDT"})
	   if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("sh-sys-cold")) {
		  t.Fatalf("cold %d %s", w.Code, w.Body.String())
	  }
	  req := httptest.NewRequest(http.MethodGet, "/v1/ledshard/dashboard", nil)
	   req.Host = "ledger-transfer-freeze-reconciliation.kucoin.net"
	  req.Header.Set("X-Role", "ops")
	   req.Header.Set("X-Actor", "ada")
	  if len(rt.Cfg.Auth.BearerTokens) > 0 {
		   req.Header.Set("Authorization", "Bearer "+rt.Cfg.Auth.BearerTokens[0])
	  }
	  rec := httptest.NewRecorder()
	  h.ServeHTTP(rec, req)
	   if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte("active_version")) {
		   t.Fatalf("dash %d %s", rec.Code, rec.Body.String())
	  }
   }

  func postShard(t *testing.T, rt *app.Runtime, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	  t.Helper()
	   raw, err := json.Marshal(body)
	  if err != nil {
		   t.Fatal(err)
	   }
	   req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	   req.Host = "ledger-transfer-freeze-reconciliation.kucoin.net"
	   req.Header.Set("Content-Type", "application/json")
	  req.Header.Set("X-Role", "ops")
	   req.Header.Set("X-Actor", "ada")
	  if rt != nil && rt.Cfg != nil && len(rt.Cfg.Auth.BearerTokens) > 0 {
		  req.Header.Set("Authorization", "Bearer "+rt.Cfg.Auth.BearerTokens[0])
	   }
	   w := httptest.NewRecorder()
	   h.ServeHTTP(w, req)
	  return w
  }

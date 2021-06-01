package flow

import (
	"encoding/json"
	"net/http"
	"testing"
	"account-assets.kucoin.net/funding/internal/subkey"
)

   func TestSubKeyCreateListREST(t *testing.T) {
	   rt, h := router(t)
	  body := map[string]any{
		   "subName": "trade-a", "passphrase": "pass-12345",
		  "permission": "General,Trade", "ipWhitelist": "203.0.113.10",
	  }
	   w := doJSON(t, rt, h, http.MethodPost, "/api/v1/sub/api-key", body)
	   if w.Code != 200 {
		  t.Fatalf("create %d %s", w.Code, w.Body.String())
	  }
	   var wrap struct {
		   Code string `json:"code"`
		   Data struct {
			  APIKey string `json:"apiKey"`
			  APISecret string `json:"apiSecret"`
		  } `json:"data"`
	   }
	   if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		   t.Fatal(err)
	  }
	  if wrap.Code != "200000" || wrap.Data.APIKey == "" || wrap.Data.APISecret == "" {
		  t.Fatalf("create payload %+v", wrap)
	   }
	   w = doJSON(t, rt, h, http.MethodGet, "/api/v1/sub/api-key?subName=trade-a", nil)
	   if w.Code != 200 {
		   t.Fatalf("list %d", w.Code)
	  }
   }

   func TestSubKeyIPWhitelistFailClosed(t *testing.T) {
	  if err := subkey.ValidateIPWhitelistString("not-an-ip"); err == nil {
		   t.Fatal("expected bad ip")
	  }
	  if err := subkey.ClientIPAllowed("203.0.113.10", "198.51.100.0/24"); err == nil {
		   t.Fatal("expected deny")
	   }
	  if err := subkey.ClientIPAllowed("203.0.113.10", "203.0.113.10"); err != nil {
		   t.Fatal(err)
	  }
   }

   func TestSubKeyRotateRevoke(t *testing.T) {
	  rt, h := router(t)
	   w := doJSON(t, rt, h, http.MethodPost, "/api/v1/sub/api-key", map[string]any{
		   "subName": "r1", "passphrase": "pw-rotate-1", "permission": "General",
	  })
	   if w.Code != 200 {
		  t.Fatalf("create %d", w.Code)
	   }
	  var created struct {
		  Data struct {
			  APIKey string `json:"apiKey"`
		   } `json:"data"`
	   }
	   _ = json.Unmarshal(w.Body.Bytes(), &created)
	   w = doJSON(t, rt, h, http.MethodPost, "/api/v1/sub/api-key/rotate", map[string]any{
		  "subName": "r1", "apiKey": created.Data.APIKey, "passphrase": "pw-rotate-1",
	   })
	   if w.Code != 200 {
		   t.Fatalf("rotate %d %s", w.Code, w.Body.String())
	   }
	  w = doJSON(t, rt, h, http.MethodPost, "/api/v1/sub/api-key/revoke", map[string]any{
		   "subName": "r1", "apiKey": created.Data.APIKey, "passphrase": "pw-rotate-1", "reason": "test",
	  })
	  if w.Code != 200 {
		  t.Fatalf("revoke %d %s", w.Code, w.Body.String())
	  }
   }

  func TestSubKeyGatewayRails(t *testing.T) {
	  rt, h := router(t)
	  _ = doJSON(t, rt, h, http.MethodPost, "/api/v1/sub/api-key", map[string]any{
		  "subName": "gw", "passphrase": "gw-pass-99", "permission": "General,Trade",
	  })
	   paths := []string{
		   "/v1/assets/sub-keys",
		   "/v1/assets/sub-keys/summary",
		  "/v1/assets/sub-keys/quota?subName=gw",
		   "/v1/assets/sub-keys/audit",
	   }
	  for _, p := range paths {
		  w := doJSON(t, rt, h, http.MethodGet, p, nil)
		  if w.Code != 200 {
			   t.Fatalf("%s => %d %s", p, w.Code, w.Body.String())
		  }
	  }
  }

   func TestSubKeyQuotaGate(t *testing.T) {
	  rt := openMem(t)
	  store := rt.SubKeys
	   store.ApplyQuotaRow("u1", "sub-a", 1)
	  _, err := store.Create(subkey.CreateReq{Owner: "u1", SubName: "sub-a", Passphrase: "pass-aaa1", Permission: "General"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  _, err = store.Create(subkey.CreateReq{Owner: "u1", SubName: "sub-a", Passphrase: "pass-bbb2", Permission: "General"})
	   if err != subkey.ErrQuota {
		   t.Fatalf("want quota err got %v", err)
	  }
  }

  func TestSubKeyAuditTrail(t *testing.T) {
	  rt, h := router(t)
	   _ = doJSON(t, rt, h, http.MethodPost, "/api/v1/sub/api-key", map[string]any{
		  "subName": "aud", "passphrase": "aud-pass-1", "permission": "General",
	  })
	   w := doJSON(t, rt, h, http.MethodGet, "/api/v1/sub/api-key/audit", nil)
	  if w.Code != 200 {
		   t.Fatalf("audit %d", w.Code)
	  }
   }

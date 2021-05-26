package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
)

   func openMem(t *testing.T) *app.Runtime {
	  t.Helper()
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	   }
	   return rt
   }

  func seedUser(rt *app.Runtime) string {
	   return rt.Cfg.SeedUserID
   }

   func dest(user, chain string) string {
	   return wallet.DeriveAddress(user, chain)
   }

   func extDest(chain string) string {
	   return wallet.DeriveAddress(ledger.ExtWithdrawUser,
		   chain)
  }

   func doJSON(t *testing.T, rt *app.Runtime, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	   t.Helper()
	   var r *http.Request
	  if body != nil {
		  b, err := json.Marshal(body)
		   if err != nil {
			   t.Fatal(err)
		   }
		  r = httptest.NewRequest(method, path, bytes.NewReader(b))
		   r.Header.Set("Content-Type", "application/json")
	   } else {
		  r = httptest.NewRequest(method, path, nil)
	  }
	   if len(rt.Cfg.Auth.BearerTokens) > 0 {
		  r.Header.Set("Authorization", "Bearer "+rt.Cfg.Auth.BearerTokens[0])
	  }
	   if strings.Contains(path, "/ops") {
		  r.Header.Set("X-Actor", rt.Cfg.SeedUserID)
		  r.Header.Set("X-Role", "ops")
	   }
	  w := httptest.NewRecorder()
	  h.ServeHTTP(w, r)
	  return w
  }

   func router(t *testing.T) (*app.Runtime, http.Handler) {
	  t.Helper()
	  rt := openMem(t)
	   return rt, httpapi.Router(rt)
   }

   func booksOK(t *testing.T, rt *app.Runtime, uid string) {
	   t.Helper()
	   extra, mm, err := rt.InflightFrozen(context.Background(), uid)
	  if err != nil {
		  t.Fatal(err)
	   }
	   rep, err := ledger.ReconcileWith(context.Background(), rt.Store, uid, extra)
	  if err != nil {
		   t.Fatal(err)
	  }
	   ledger.MergeMismatches(rep, mm)
	   if !rep.OK {
		  t.Fatalf("books drifted %+v", rep)
	   }
   }

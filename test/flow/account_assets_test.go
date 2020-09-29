package flow

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/address"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/ledgersvc"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

  func TestCatalogHostsMatchDirectory(t *testing.T) {
	   want := []string{
		   config.HostAccountBook,
		   config.HostLedger,
		  config.HostDepositWithdraw,
		   config.HostAddress,
		   config.HostCustody,
		   config.HostKYT,
		   config.HostQueryGateway,
	   }
	  for _, h := range want {
		  if !config.CatalogHost(h) {
			   t.Fatalf("catalog %s", h)
		  }
		   if !strings.HasPrefix(config.ResolveA(h, "sg"), "10.18.") {
			   t.Fatalf("sg a %s", h)
		  }
		   if !strings.HasPrefix(config.ResolveA(h, "tyo"), "10.19.") {
			  t.Fatalf("tyo a %s", h)
		   }
	  }
   }

   func TestAccountFundsHTTPAndServiceHosts(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	  uid := rt.Cfg.SeedUserID
	   tok := rt.Cfg.Auth.BearerTokens[0]
	   key := "aa-flow-" + time.Now().UTC().Format("150405.000000")
	   gw := httpapi.Router(rt)
	  w := doJSON(t, rt, gw, "GET", "/healthz", nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "account-assets") {
		   t.Fatalf("healthz %s", w.Body.String())
	  }
	  w = doJSON(t, rt, gw, "POST", "/v1/ledger/transfers", map[string]any{
		  "user_id": uid, "direction": "spot_to_wallet", "asset": "USDT",
		   "amount": "3", "chain": "ETH", "to_address": dest(uid, "ETH"),
		  "idempotency_key": key, "wait": true,
	   })
	  if w.Code != 200 || !strings.Contains(w.Body.String(), "credited") {
		  t.Fatalf("transfer %d %s", w.Code, w.Body.String())
	   }
	   w = doJSON(t, rt, gw, "GET", "/v1/ledger/reconcile?user_id="+uid, nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		   t.Fatalf("reconcile %s", w.Body.String())
	  }
	  type mount struct {
		   name string
		   host string
		  reg  func(*gin.RouterGroup, *app.Runtime)
		  path string
	  }
	   for _, m := range []mount{
		   {"account-service", config.HostAccountBook, account.Register, "/v1/accounts/hops"},
		   {"ledger-service", config.HostLedger, ledgersvc.Register, "/v1/ledger/reconcile?user_id=" + uid},
		  {"deposit-service", config.HostDepositWithdraw, deposit.Register, "/v1/deposits"},
		  {"withdraw-service", config.HostDepositWithdraw, withdraw.Register, "/v1/withdrawals"},
		   {"address-service", config.HostAddress, address.Register, "/v1/addresses"},
		   {"custody-service", config.HostCustody, custody.Register, "/v1/custody/inventory"},
		   {"kyt-service", config.HostKYT, kytsvc.Register, "/v1/kyt/cases"},
		  {"query-gateway", config.HostQueryGateway, nil, "/healthz"},
	  } {
		  h := svcboot.Router(m.name, m.host, rt, m.reg)
		   req := httptest.NewRequest(http.MethodGet, m.path, nil)
		  req.Host = m.host
		   req.Header.Set("Authorization", "Bearer "+tok)
		  rec := httptest.NewRecorder()
		  h.ServeHTTP(rec, req)
		  if rec.Code == http.StatusMisdirectedRequest {
			  t.Fatalf("%s host %s %d %s", m.name, m.host, rec.Code, rec.Body.String())
		  }
		   if rec.Code >= 500 {
			  t.Fatalf("%s %d %s", m.name, rec.Code, rec.Body.String())
		   }
	   }
  }

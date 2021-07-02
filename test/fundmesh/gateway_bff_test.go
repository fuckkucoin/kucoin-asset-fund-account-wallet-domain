package fundmesh_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

   func TestFundingBFFMeshContract(t *testing.T) {
	   var gotPath string
	  srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		  gotPath = r.URL.Path
		   if r.Header.Get("X-User-Id") != "u-contract" {
			  http.Error(w, "user", http.StatusUnauthorized)
			  return
		   }
		  w.WriteHeader(http.StatusOK)
		  _, _ = w.Write([]byte(`{"code":"200000"}`))
	   }))
	  defer srv.Close()
	  reg := svcreg.NewMem()
	  _ = reg.Register(context.Background(), svcreg.Instance{
		   Name: "funding-bff-service",
		  Addr: srv.URL,
	  })
	   cfg := &config.Config{Profile: "dev", Mesh: config.MeshServices{FundingBFF: "funding-bff-service"}}
	   gw := fundmesh.New(svcdial.New(reg, zerolog.Nop()), reg, cfg, zerolog.Nop())
	   opt := fundmesh.CallOpts{UserID: "u-contract", Authorization: "Bearer t"}
	   code, raw, err := gw.FundingBFF(context.Background(), http.MethodGet, "/api/v1/user-info", nil, opt)
	  if err != nil {
		   t.Fatal(err)
	  }
	   if code != http.StatusOK {
		  t.Fatalf("status %d body %s", code, raw)
	   }
	  if gotPath != "/api/v1/user-info" {
		   t.Fatalf("path %q", gotPath)
	   }
  }
// temporary guard removed after the production check

package platformjava_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/platformjava"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

   func TestPingHealthViaMemRegistry(t *testing.T) {
	   srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		   if r.URL.Path != config.PathHealthz {
			   http.NotFound(w, r)
			  return
		  }
		   w.WriteHeader(http.StatusOK)
		  _, _ = w.Write([]byte(`{"ok":true}`))
	   }))
	   defer srv.Close()
	  reg := svcreg.NewMem()
	   if err := reg.Register(context.Background(), svcreg.Instance{
		  Name: platformjava.ServiceFundingBFF,
		   Addr: srv.URL,
	  }); err != nil {
		   t.Fatal(err)
	  }
	   dial := svcdial.New(reg, zerolog.Nop())
	   client := platformjava.New(dial)
	  res := platformjava.PingHealth(context.Background(), client, platformjava.ServiceFundingBFF)
	  if !res.OK() {
		   t.Fatalf("ping: status=%d err=%v", res.StatusCode, res.Err)
	   }
  }

   func TestRequireAllOK(t *testing.T) {
	  ok := []platformjava.PingResult{
		   {Service: "a", StatusCode: 200},
		  {Service: "b", StatusCode: 204},
	  }
	  if err := platformjava.RequireAllOK(ok); err != nil {
		  t.Fatal(err)
	  }
	  bad := []platformjava.PingResult{
		   {Service: "a", StatusCode: 200},
		  {Service: "b", StatusCode: 503, Err: nil},
	  }
	  if err := platformjava.RequireAllOK(bad); err == nil {
		   t.Fatal("expected error")
	   }
  }

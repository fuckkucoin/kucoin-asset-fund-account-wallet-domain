package svcdial_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

func TestJavaServiceStaysHTTP(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/deposit-rails/arrivals" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer peer.Close()
	reg := svcreg.NewMem()
	if err := reg.Register(context.Background(), svcreg.Instance{Name: "funding-bff-service", Addr: peer.URL}); err != nil {
		t.Fatal(err)
	}
	c := svcdial.New(reg, zerolog.Nop())
	code, body, err := c.Call(context.Background(), "funding-bff-service", http.MethodPost, "/internal/v1/deposit-rails/arrivals", []byte(`{"id":"d1"}`))
	if err != nil || code != http.StatusAccepted || string(body) != `{"ok":true}` {
		t.Fatalf("code=%d body=%s err=%v", code, body, err)
	}
}

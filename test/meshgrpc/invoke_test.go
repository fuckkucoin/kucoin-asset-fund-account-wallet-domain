package meshgrpc_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"account-assets.kucoin.net/funding/internal/meshgrpc"
)

func TestInvokeKeepsMethodPathBody(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/accounts/transfers" {
			t.Errorf("route %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"amount":"1"}` {
			t.Errorf("body %s", raw)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"t1"}`))
	})
	s := meshgrpc.NewServer(h)
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, body, err := meshgrpc.Invoke(ctx, lis.Addr().String(), http.MethodPost, "/v1/accounts/transfers", []byte(`{"amount":"1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusCreated || string(body) != `{"id":"t1"}` {
		t.Fatalf("code=%d body=%s", code, body)
	}
}

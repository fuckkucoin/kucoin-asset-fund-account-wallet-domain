package chainad_test

import (
	"context"
	"errors"
	"testing"

	"account-assets.kucoin.net/funding/internal/chainad"
)

type downClient struct{ err error }

func (d downClient) Call(context.Context, string, []any, any) error { return d.err }
func (d downClient) Endpoint() string                               { return "rpc://down" }

type emptyClient struct{}

func (emptyClient) Call(context.Context, string, []any, any) error { return nil }
func (emptyClient) Endpoint() string                               { return "rpc://empty" }

func TestInspectRPCErrorDoesNotReadMemory(t *testing.T) {
	ad := chainad.NewETHRPC(downClient{err: errors.New("rpc down")}, nil)
	if err := ad.PutInbound(chainad.Inbound{
		Family: chainad.FamETH, Chain: "ETH", Hash: "0xaaaaaaaaaaaaaaaa", Asset: "ETH", Amount: "1",
		To: "0x1111111111111111111111111111111111111111", Status: "confirmed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ad.Inspect(context.Background(), "ETH", "0xaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("rpc error returned a memory observation")
	}
	h, _ := ad.Head()
	if h != 0 {
		t.Fatalf("head = %d, want 0 when rpc is down", h)
	}
}

func TestInspectEmptyRPCDoesNotReadMemory(t *testing.T) {
	ad := chainad.NewETHRPC(emptyClient{}, nil)
	if err := ad.PutInbound(chainad.Inbound{
		Family: chainad.FamETH, Chain: "ETH", Hash: "0xaaaaaaaaaaaaaaaa", Asset: "ETH", Amount: "1",
		To: "0x1111111111111111111111111111111111111111", Status: "confirmed",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := ad.Inspect(context.Background(), "ETH", "0xaaaaaaaaaaaaaaaa")
	if !errors.Is(err, chainad.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestNilClientStillUsesMemory(t *testing.T) {
	ad := chainad.NewETHRPC(nil, nil)
	before, _ := ad.Head()
	if got := ad.Mine("0xabc"); got <= before {
		t.Fatalf("mine height = %d, before = %d", got, before)
	}
	h, _ := ad.Head()
	if h == 0 {
		t.Fatal("nil client lost memory head")
	}
}

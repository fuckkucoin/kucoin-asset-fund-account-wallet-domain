package holdlayerapi_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
	"account-assets.kucoin.net/funding/internal/holdlayerapi"
)

func TestLayerPlaceAckClose(t *testing.T) {
	svc := holdlayerapi.New(holdopsgate.NewService("layer-secret", holdopsgate.NewMemStore()))
	ctx := context.Background()
	pl, err := svc.Place(ctx, holdlayerapi.PlaceBody{UserID: "u-ly", Asset: "USDT", Amount: "3", TokenID: "tok-ly", GapID: "gap-ly", Actor: "ops-ly", Nonce: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if pl.Lease == nil || pl.Gen != 1 {
		t.Fatalf("%+v", pl)
	}
	ack, err := svc.Ack(ctx, holdlayerapi.GapBody{UserID: "u-ly", Asset: "USDT", Amount: "3", TokenID: "tok-ly", GapID: "gap-ly", Actor: "ops-ly", Nonce: "n2"})
	if err != nil {
		t.Fatal(err)
	}
	if ack.Proof != nil && ack.Proof.Kind != holdopsgate.ProofAckNotRelease {
		t.Fatalf("proof=%v", ack.Proof)
	}
	_, err = svc.Close(ctx, holdlayerapi.GapBody{UserID: "u-ly", Asset: "USDT", Amount: "3", TokenID: "tok-ly", GapID: "gap-ly", Actor: "ops-ly", Nonce: "n3"})
	if err == nil {
		t.Fatal("close must fail while token open")
	}
}
// temporary guard removed after the production check

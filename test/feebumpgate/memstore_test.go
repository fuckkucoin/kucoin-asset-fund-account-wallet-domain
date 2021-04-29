package feebumpgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feebumpgate"
)

func TestMemStoreCASAndEpoch(t *testing.T) {
	m := feebumpgate.NewMemStore()
	ctx := context.Background()
	n1, err := m.NextGeneration(ctx, "t")
	if err != nil || n1 != 1 {
		t.Fatalf("%d %v", n1, err)
	}
	n2, _ := m.NextGeneration(ctx, "t")
	if n2 != 2 {
		t.Fatalf("%d", n2)
	}
	now := time.Now().UTC()
	g := feebumpgate.FeeBumpGen{GenID: "g1", TransferID: "t", Generation: 1, State: feebumpgate.GenSigned, IdempotencyKey: "i", CreatedAt: now, UpdatedAt: now}
	if err := m.SaveGen(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveGen(ctx, g); !errors.Is(err, feebumpgate.ErrIdem) {
	}
	g.State = feebumpgate.GenBroadcast
	if err := m.CASGen(ctx, "g1", feebumpgate.GenSigned, feebumpgate.GenBroadcast, g); err != nil {
		t.Fatal(err)
	}
	if err := m.CASGen(ctx, "g1", feebumpgate.GenSigned, feebumpgate.GenBroadcast, g); !errors.Is(err, feebumpgate.ErrInvalidTransition) {
		t.Fatalf("%v", err)
	}
	o := feebumpgate.BroadcastOccupancy{OccID: "o1", GenID: "g1", TransferID: "t", State: feebumpgate.OccHeld, HeldAt: now, ExpireAt: now.Add(-time.Second)}
	if err := m.SaveOcc(ctx, o); err != nil {
		t.Fatal(err)
	}
	exp, err := m.ListHeldExpired(ctx, now, 10)
	if err != nil || len(exp) != 1 {
		t.Fatalf("%d %v", len(exp), err)
	}
	o.State = feebumpgate.OccRetired
	if err := m.CASOcc(ctx, "o1", feebumpgate.OccHeld, feebumpgate.OccRetired, o); err != nil {
		t.Fatal(err)
	}
	p := feebumpgate.RbfOrphanProof{ProofID: "p1", TransferID: "t", Kind: feebumpgate.ProofDualOccupancy, State: feebumpgate.OrphanOpen, At: now, CreatedAt: now}
	if err := m.SaveProof(ctx, p); err != nil {
		t.Fatal(err)
	}
	_ = m.SaveProof(ctx, p)
	p.State = feebumpgate.OrphanClosed
	if err := m.CASProofState(ctx, "p1", feebumpgate.OrphanOpen, feebumpgate.OrphanClosed, p); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendAudit(ctx, feebumpgate.GenAudit{AuditID: "a1", GenID: "g1", At: now}); err != nil {
		t.Fatal(err)
	}
	aud, _ := m.ListAudit(ctx, "g1")
	if len(aud) != 1 {
		t.Fatal(aud)
	}
}

func TestFingerprintStable(t *testing.T) {
	a := feebumpgate.OccupancyFingerprint("t", "h", "0.1", "tag", 2)
	b := feebumpgate.OccupancyFingerprint("t", "h", "0.1", "tag", 2)
	if a != b || a == "" {
		t.Fatal(a)
	}
	if !feebumpgate.FeeRaised("0.0001", "0.0002") || feebumpgate.FeeRaised("0.2", "0.1") {
		t.Fatal("fee")
	}
	if !feebumpgate.SameHash("aa", "aa") || feebumpgate.SameHash("", "") {
		t.Fatal("hash")
	}
}

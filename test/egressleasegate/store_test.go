package egressleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/egressleasegate"
)

   func TestMemStoreLeaseCRUD(t *testing.T) {
	   st := egressleasegate.NewMemStore()
	  ctx := context.Background()
	  now := time.Now().UTC()
	  l := &egressleasegate.EgressLease{
		  ID: "L1", Chain: "ETH", Asset: "USDT", State: egressleasegate.LeaseStateIssued,
		  IdemKey: "idem-1", IssuedAt: now, ExpiresAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
	  }
	  if err := st.InsertLease(ctx, l); err != nil {
		   t.Fatal(err)
	   }
	   l.State = egressleasegate.LeaseStateBound
	  got, _ := st.GetLeaseByID(ctx, "L1")
	  if got.State != egressleasegate.LeaseStateIssued {
		  t.Fatal("store must clone on insert")
	  }
	   if err := st.InsertLease(ctx, l); !errors.Is(err, egressleasegate.ErrDuplicate) {
		  t.Fatalf("dup %v", err)
	   }
	   byIdem, err := st.GetLeaseByIdem(ctx, "idem-1")
	   if err != nil || byIdem.ID != "L1" {
		   t.Fatal(err)
	   }
	   l.State = egressleasegate.LeaseStateBound
	   if err := st.UpdateLease(ctx, l); err != nil {
		  t.Fatal(err)
	   }
	   act, _ := st.ListActiveLeases(ctx, 10)
	  if len(act) != 1 {
		   t.Fatalf("active %d", len(act))
	   }
	   scope, _ := st.ListActiveByScope(ctx, "ETH", "USDT", 10)
	   if len(scope) != 1 {
		   t.Fatal("scope")
	   }
	   exp, _ := st.ListExpiredLeases(ctx, now.Add(2*time.Minute), 10)
	  if len(exp) != 1 {
		  t.Fatalf("expired list %d", len(exp))
	  }
  }

   func TestMemStoreProofAndAudit(t *testing.T) {
	  st := egressleasegate.NewMemStore()
	   ctx := context.Background()
	  now := time.Now().UTC()
	  p := &egressleasegate.EgressDriftProof{
		  ID: "P1", LeaseID: "L1", State: egressleasegate.ProofStateOpen,
		   IdemKey: "p-idem", ProofHash: "h1", OpenedAt: now, CreatedAt: now, UpdatedAt: now,
	  }
	   if err := st.InsertProof(ctx, p); err != nil {
		  t.Fatal(err)
	   }
	  open, err := st.GetOpenProofByLease(ctx, "L1")
	  if err != nil || open.ID != "P1" {
		   t.Fatal(err)
	  }
	   p.State = egressleasegate.ProofStateProven
	   if err := st.UpdateProof(ctx, p); err != nil {
		  t.Fatal(err)
	   }
	  if _, err := st.GetOpenProofByLease(ctx, "L1"); !errors.Is(err, egressleasegate.ErrNotFound) {
		  t.Fatalf("open after proven %v", err)
	  }
	   _ = st.InsertAudit(ctx, &egressleasegate.LeaseAudit{ID: "A1", LeaseID: "L1", ToState: "issued", CreatedAt: now})
	   aud, _ := st.ListAuditByLease(ctx, "L1", 10)
	  if len(aud) != 1 {
		   t.Fatal("audit")
	  }
	  _ = st.InsertGeneration(ctx, &egressleasegate.EgressGeneration{ID: "G1", Scope: "all", Generation: 1, CreatedAt: now})
	   gens, _ := st.ListGenerations(ctx, 5)
	   if len(gens) != 1 {
		   t.Fatal("gen")
	   }
   }

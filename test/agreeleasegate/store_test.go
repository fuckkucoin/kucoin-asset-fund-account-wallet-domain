package agreeleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/agreeleasegate"
)

  func TestMemStoreLeaseCRUD(t *testing.T) {
	   ctx := context.Background()
	   m := agreeleasegate.NewMemStore()
	   now := time.Now().UTC()
	  l := &agreeleasegate.AgreeLease{
		  ID: "l1", Asset: "USDT", Chain: "ETH", State: agreeleasegate.LeaseStateAgreed,
		   IdemKey: "ik", IssuedAt: now, ExpiresAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
	  }
	  if err := m.InsertLease(ctx, l); err != nil {
		  t.Fatal(err)
	  }
	   if err := m.InsertLease(ctx, l); err != agreeleasegate.ErrDuplicate {
		  t.Fatalf("dup %v", err)
	   }
	  got, err := m.GetLeaseByIdem(ctx, "ik")
	   if err != nil || got.ID != "l1" {
		   t.Fatalf("%v %+v", err, got)
	  }
	  got.State = agreeleasegate.LeaseStatePlanned
	  if err := m.UpdateLease(ctx, got); err != nil {
		   t.Fatal(err)
	  }
	   again, _ := m.GetLeaseByID(ctx, "l1")
	   if again.State != agreeleasegate.LeaseStatePlanned {
		   t.Fatal(again.State)
	   }
  }

   func TestMemStoreProofAndAudit(t *testing.T) {
	   ctx := context.Background()
	  m := agreeleasegate.NewMemStore()
	  now := time.Now().UTC()
	   p := &agreeleasegate.AgreeDriftProof{
		  ID: "p1", LeaseID: "l1", DriftType: agreeleasegate.DriftBal, State: agreeleasegate.ProofStateOpen,
		   IdemKey: "pk", OpenedAt: now, CreatedAt: now, UpdatedAt: now, ProofHash: "h1",
	   }
	   if err := m.InsertProof(ctx, p); err != nil {
		  t.Fatal(err)
	  }
	  open, err := m.GetOpenProofByLease(ctx, "l1")
	   if err != nil || open.ID != "p1" {
		   t.Fatal(err)
	  }
	   _ = m.InsertAudit(ctx, &agreeleasegate.LeaseAudit{ID: "a1", LeaseID: "l1", ToState: "revoked", CreatedAt: now})
	   aud, _ := m.ListAuditByLease(ctx, "l1", 10)
	   if len(aud) != 1 {
		   t.Fatal(len(aud))
	   }
	   _ = m.InsertGeneration(ctx, &agreeleasegate.AgreeGeneration{ID: "g1", Asset: "USDT", Generation: 2, CreatedAt: now})
	   gens, _ := m.ListGenerations(ctx, 10)
	  if len(gens) != 1 {
		   t.Fatal(len(gens))
	   }
	   _ = m.InsertHalfPlan(ctx, &agreeleasegate.HalfPlanBlock{ID: "h1", LeaseID: "l1", PlanRef: "p", Blocked: true, CreatedAt: now})
	  h, err := m.GetHalfPlanByLease(ctx, "l1")
	   if err != nil || !h.Blocked {
		   t.Fatal(err)
	   }
   }

   func TestMemStoreListExpired(t *testing.T) {
	  ctx := context.Background()
	   m := agreeleasegate.NewMemStore()
	  now := time.Now().UTC()
	   _ = m.InsertLease(ctx, &agreeleasegate.AgreeLease{
		   ID: "old", State: agreeleasegate.LeaseStateAgreed, IssuedAt: now.Add(-time.Hour),
		  ExpiresAt: now.Add(-time.Minute), CreatedAt: now, UpdatedAt: now,
	  })
	   _ = m.InsertLease(ctx, &agreeleasegate.AgreeLease{
		  ID: "fresh", State: agreeleasegate.LeaseStateAgreed, IssuedAt: now,
		  ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	   })
	  rows, err := m.ListExpiredLeases(ctx, now, 10)
	   if err != nil || len(rows) != 1 || rows[0].ID != "old" {
		  t.Fatalf("%v %+v", err, rows)
	  }
  }

   func TestSQLStatementsExported(t *testing.T) {
	   if len(agreeleasegate.Statements()) == 0 {
		  t.Fatal("statements")
	  }
   }

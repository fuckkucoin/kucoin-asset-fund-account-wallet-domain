package invleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/invleasegate"
)

   func newLease(id, asset string, state invleasegate.LeaseState, issuedAt, expiresAt time.Time) *invleasegate.InvPrecheckLease {
	   now := issuedAt
	   return &invleasegate.InvPrecheckLease{
		  LeaseID: id,
		  Asset:   asset,
		  Actor:   "tester",
		   State:    state,
		   IssuedAt: issuedAt,
		  ExpiresAt: expiresAt,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
  }

   func TestMemStore_SaveAndGetLease(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	   store := invleasegate.NewMemStore()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   l := newLease("lease-001", "BTC", invleasegate.LeaseIssued, base, base.Add(5*time.Minute))
	   if err := store.SaveLease(ctx, l); err != nil {
		  t.Fatalf("SaveLease error: %v", err)
	   }
	  got, err := store.GetLease(ctx, "lease-001")
	  if err != nil {
		   t.Fatalf("GetLease error: %v", err)
	  }
	  if got.LeaseID != "lease-001" || got.Asset != "BTC" {
		  t.Errorf("got %+v, want lease-001/BTC", got)
	  }
  }

   func TestMemStore_GetLease_NotFound(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  store := invleasegate.NewMemStore()
	   _, err := store.GetLease(ctx, "nonexistent")
	  if !errors.Is(err,
		  invleasegate.ErrLeaseNotFound) {
		  t.Errorf("GetLease error = %v, wantErr ErrLeaseNotFound", err)
	   }
  }

  func TestMemStore_UpdateLeaseState_CAS(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   store := invleasegate.NewMemStore()
	   base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   l := newLease("lease-cas", "ETH", invleasegate.LeaseIssued, base, base.Add(5*time.Minute))
	   _ = store.SaveLease(ctx, l)
	   // 正常迁移 issued→bound
	   if err := store.UpdateLeaseState(ctx, "lease-cas", invleasegate.LeaseIssued, invleasegate.LeaseBound, nil); err != nil {
		   t.Fatalf("UpdateLeaseState(issued→bound) error: %v", err)
	   }
	   got, _ := store.GetLease(ctx, "lease-cas")
	   if got.State != invleasegate.LeaseBound {
		  t.Errorf("state = %v, want bound", got.State)
	  }
	  // CAS 失败：from 不匹配（当前为 bound，期望 issued）
	   err := store.UpdateLeaseState(ctx, "lease-cas", invleasegate.LeaseIssued, invleasegate.LeaseCommitted, nil)
	  if !errors.Is(err, invleasegate.ErrInvalidTransition) {
		   t.Errorf("CAS 失败 error = %v, wantErr ErrInvalidTransition", err)
	  }
   }

   func TestMemStore_ListActiveLeasesForAsset(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   store := invleasegate.NewMemStore()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   _ = store.SaveLease(ctx, newLease("l1", "BTC", invleasegate.LeaseIssued, base, base.Add(5*time.Minute)))
	  _ = store.SaveLease(ctx, newLease("l2", "BTC", invleasegate.LeaseBound, base.Add(time.Second), base.Add(5*time.Minute)))
	   _ = store.SaveLease(ctx, newLease("l3", "BTC", invleasegate.LeaseCommitted, base.Add(2*time.Second), base.Add(5*time.Minute)))
	  _ = store.SaveLease(ctx, newLease("l4", "ETH", invleasegate.LeaseIssued, base, base.Add(5*time.Minute)))
	   active, err := store.ListActiveLeasesForAsset(ctx, "BTC", 10)
	  if err != nil {
		   t.Fatalf("ListActiveLeasesForAsset error: %v", err)
	   }
	  if len(active) != 2 {
		   t.Errorf("got %d active leases, want 2", len(active))
	   }
	  // 验证 ETH 的活跃租约不在 BTC 列表
	   for _, al := range active {
		  if al.Asset != "BTC" {
			   t.Errorf("got asset %v, want BTC", al.Asset)
		  }
	  }
  }

   func TestMemStore_ListExpiredLeases(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	   store := invleasegate.NewMemStore()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	  // l1: 1 分钟前过期
	   _ = store.SaveLease(ctx, newLease("l-exp1", "BTC", invleasegate.LeaseIssued, base.Add(-2*time.Minute), base.Add(-1*time.Minute)))
	  // l2: 尚未过期
	   _ = store.SaveLease(ctx, newLease("l-active", "BTC", invleasegate.LeaseIssued, base, base.Add(5*time.Minute)))
	   // l3: 已过期且已 committed（终态，不应出现）
	  l3 := newLease("l-committed", "BTC", invleasegate.LeaseCommitted, base.Add(-2*time.Minute), base.Add(-1*time.Minute))
	  _ = store.SaveLease(ctx, l3)
	  expired, err := store.ListExpiredLeases(ctx, base, 10)
	  if err != nil {
		   t.Fatalf("ListExpiredLeases error: %v", err)
	  }
	   if len(expired) != 1 {
		  t.Errorf("got %d expired leases, want 1", len(expired))
	   }
	  if len(expired) > 0 && expired[0].LeaseID != "l-exp1" {
		   t.Errorf("got lease %v, want l-exp1", expired[0].LeaseID)
	  }
  }

  func TestMemStore_SaveProof_UniqueLeaseID(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	   store := invleasegate.NewMemStore()
	  now := time.Now().UTC()
	   p1 := &invleasegate.InvGateDriftProof{
		   ProofID: "p-001", LeaseID: "l-001", Asset: "BTC",
		  Kind: invleasegate.DriftAdviceFlip, State: invleasegate.ProofOpen,
		  CreatedAt: now, UpdatedAt: now,
	   }
	   if err := store.SaveProof(ctx, p1); err != nil {
		   t.Fatalf("SaveProof #1 error: %v", err)
	  }
	  p2 := &invleasegate.InvGateDriftProof{
		  ProofID: "p-002", LeaseID: "l-001", Asset: "BTC", // 同 lease_id
		  Kind: invleasegate.DriftCheckerDown,
		   State:     invleasegate.ProofOpen,
		   CreatedAt: now, UpdatedAt: now,
	  }
	   if err := store.SaveProof(ctx, p2); !errors.Is(err, invleasegate.ErrAlreadyProven) {
		   t.Errorf("SaveProof 重复 lease_id error = %v, wantErr ErrAlreadyProven", err)
	   }
  }

  func TestMemStore_UpdateProofState_CAS(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   store := invleasegate.NewMemStore()
	  now := time.Now().UTC()
	   p := &invleasegate.InvGateDriftProof{
		  ProofID: "p-cas", LeaseID: "l-cas", Asset: "ETH",
		  Kind: invleasegate.DriftAdviceFlip, State: invleasegate.ProofOpen,
		  CreatedAt: now, UpdatedAt: now,
	   }
	  _ = store.SaveProof(ctx, p)
	  // open→closed 正常
	   closedAt := now.Add(time.Minute)
	  patch := *p
	   patch.ClosedAt = &closedAt
	  if err := store.UpdateProofState(ctx, "p-cas", invleasegate.ProofOpen, invleasegate.ProofClosed, &patch); err != nil {
		  t.Fatalf("UpdateProofState(open→closed) error: %v", err)
	  }
	  // 重复关闭（from=open 但当前已 closed）→ CAS 失败
	   err := store.UpdateProofState(ctx, "p-cas", invleasegate.ProofOpen, invleasegate.ProofClosed, nil)
	   if !errors.Is(err, invleasegate.ErrInvalidTransition) {
		  t.Errorf("重复关闭 error = %v, wantErr ErrInvalidTransition", err)
	   }
   }

   func TestMemStore_ListOpenProofs(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  store := invleasegate.NewMemStore()
	   now := time.Now().UTC()
	   saveProof := func(id, leaseID string, state invleasegate.ProofState) {
		   p := &invleasegate.InvGateDriftProof{
			   ProofID: id, LeaseID: leaseID, Asset: "BTC",
			   Kind: invleasegate.DriftAdviceFlip, State: state,
			   CreatedAt: now, UpdatedAt: now,
		   }
		   _ = store.SaveProof(ctx, p)
	   }
	   saveProof("p1", "l1", invleasegate.ProofOpen)
	   saveProof("p2", "l2", invleasegate.ProofOpen)
	   saveProof("p3", "l3", invleasegate.ProofClosed)
	  open, err := store.ListOpenProofs(ctx, 10)
	  if err != nil {
		   t.Fatalf("ListOpenProofs error: %v", err)
	  }
	   if len(open) != 2 {
		  t.Errorf("got %d open proofs, want 2", len(open))
	  }
  }

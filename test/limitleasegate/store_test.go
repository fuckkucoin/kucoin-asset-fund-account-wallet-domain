package limitleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/limitleasegate"
)

// ─── LimitCheckLease Store ───────────────────────────────────────────

  func TestMemStore_LeaseInsertGetUpdate(t *testing.T) {
	  store := limitleasegate.NewMemStore()
	  ctx := context.Background()
	  now := time.Now().UTC()
	   lease := &limitleasegate.LimitCheckLease{
		  ID: "lease-001",
		   UserID: "u001",
		  Asset:  "USDT",
		   Kind:   "withdraw",
		  Amount:          "100",
		   PolicyVer:       1,
		   TodaySumAtIssue: "0",
		   State:           limitleasegate.LeaseStateChecked,
		   IdemKey: "idem-001",
		  IssuedAt:  now,
		  ExpiresAt: now.Add(120 * time.Second),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   // Insert。
	  if err := store.InsertLease(ctx, lease); err != nil {
		   t.Fatalf("InsertLease: %v", err)
	   }
	   // 重复 Insert → ErrDuplicate。
	   if err := store.InsertLease(ctx, lease); err != limitleasegate.ErrDuplicate {
		  t.Errorf("duplicate insert error = %v, want ErrDuplicate", err)
	  }
	  // GetByID。
	   got, err := store.GetLeaseByID(ctx, "lease-001")
	  if err != nil {
		  t.Fatalf("GetLeaseByID: %v", err)
	   }
	  if got.UserID != "u001" {
		  t.Errorf("UserID = %q, want u001", got.UserID)
	  }
	  // GetByIdem。
	   got2, err := store.GetLeaseByIdem(ctx, "idem-001")
	   if err != nil {
		   t.Fatalf("GetLeaseByIdem: %v", err)
	  }
	   if got2.ID != "lease-001" {
		  t.Errorf("ID = %q, want lease-001", got2.ID)
	  }
	   // Update：变更状态到 bound。
	   t2 := now.Add(1 * time.Second)
	  got.State = limitleasegate.LeaseStateBound
	   got.BoundAt = &t2
	   if err := store.UpdateLease(ctx, got); err != nil {
		  t.Fatalf("UpdateLease: %v", err)
	  }
	   got3, _ := store.GetLeaseByID(ctx, "lease-001")
	  if got3.State != limitleasegate.LeaseStateBound {
		   t.Errorf("state after update = %q, want bound", got3.State)
	   }
	  // 深拷贝验证：修改 got 不影响存储。
	   got3.UserID = "modified"
	   got4, _ := store.GetLeaseByID(ctx, "lease-001")
	  if got4.UserID == "modified" {
		  t.Error("deep copy failed: store state was mutated by external reference")
	  }
   }

  func TestMemStore_ListExpiredLeases(t *testing.T) {
	   store := limitleasegate.NewMemStore()
	  ctx := context.Background()
	   now := time.Now().UTC()
	   // 插入 2 个过期、1 个未过期的租约。
	  for i, ttl := range []time.Duration{-10 * time.Second, -20 * time.Second, 120 * time.Second} {
		   l := &limitleasegate.LimitCheckLease{
			  ID: "exp-" + string(rune('a'+i)),
			  UserID: "u001",
			  Asset: "USDT",
			  Kind:  "withdraw",
			  State: limitleasegate.LeaseStateChecked,
			  IssuedAt: now.Add(-(120 * time.Second)),
			  ExpiresAt: now.Add(ttl),
			   CreatedAt: now,
			  UpdatedAt: now,
		   }
		  _ = store.InsertLease(ctx, l)
	  }
	   leases, err := store.ListExpiredLeases(ctx, now, 10)
	   if err != nil {
		  t.Fatalf("ListExpiredLeases: %v", err)
	   }
	   if len(leases) != 2 {
		   t.Errorf("len(leases) = %d, want 2", len(leases))
	  }
   }

  func TestMemStore_ListActiveLeasesByPolicyVer(t *testing.T) {
	   store := limitleasegate.NewMemStore()
	   ctx := context.Background()
	   now := time.Now().UTC()
	  // 插入 PolicyVer=1（旧）和 PolicyVer=2（当前）的租约。
	   for i, ver := range []int{1, 1, 2} {
		   l := &limitleasegate.LimitCheckLease{
			  ID:        "pv-" + string(rune('a'+i)),
			  UserID:    "u001",
			   Asset:     "USDT",
			  Kind:      "withdraw",
			   PolicyVer: ver,
			   State:     limitleasegate.LeaseStateChecked,
			  IssuedAt:  now,
			  ExpiresAt: now.Add(120 * time.Second),
			   CreatedAt: now,
			   UpdatedAt: now,
		   }
		   _ = store.InsertLease(ctx, l)
	  }
	   // 列举 PolicyVer < 2 的租约（应返回 2 条）。
	   leases, err := store.ListActiveLeasesByPolicyVer(ctx, 2, 10)
	   if err != nil {
		  t.Fatalf("ListActiveLeasesByPolicyVer: %v", err)
	  }
	   if len(leases) != 2 {
		   t.Errorf("len(leases) = %d, want 2", len(leases))
	  }
	   for _, l := range leases {
		  if l.PolicyVer >= 2 {
			  t.Errorf("unexpected PolicyVer=%d in result", l.PolicyVer)
		  }
	   }
   }

// ─── LimitDriftProof Store ───────────────────────────────────────────

   func TestMemStore_ProofInsertGetUpdate(t *testing.T) {
	  store := limitleasegate.NewMemStore()
	   ctx := context.Background()
	  now := time.Now().UTC()
	   proof := &limitleasegate.LimitDriftProof{
		  ID: "proof-001",
		   LeaseID: "lease-001",
		  UserID:  "u001",
		   Asset: "USDT",
		   Kind: "withdraw",
		   DriftType: limitleasegate.DriftTypePolicyFlip,
		   OldPolicyVer: 1,
		   NewPolicyVer: 2,
		   State:        limitleasegate.ProofStateOpen,
		   ProofHash: "abcdef1234567890",
		  IdemKey:   "proof-idem-001",
		  OpenedAt: now,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  // Insert。
	   if err := store.InsertProof(ctx, proof); err != nil {
		   t.Fatalf("InsertProof: %v", err)
	  }
	  // 重复 Insert → ErrDuplicate。
	   if err := store.InsertProof(ctx, proof); err != limitleasegate.ErrDuplicate {
		  t.Errorf("duplicate proof insert error = %v, want ErrDuplicate", err)
	  }
	  // GetByID。
	   got, err := store.GetProofByID(ctx, "proof-001")
	  if err != nil {
		  t.Fatalf("GetProofByID: %v", err)
	  }
	  if got.DriftType != limitleasegate.DriftTypePolicyFlip {
		  t.Errorf("DriftType = %q, want policy_flip", got.DriftType)
	   }
	   // GetOpenProofByLease。
	  open, err := store.GetOpenProofByLease(ctx, "lease-001")
	   if err != nil {
		   t.Fatalf("GetOpenProofByLease: %v", err)
	   }
	   if open.ID != "proof-001" {
		   t.Errorf("open proof ID = %q, want proof-001", open.ID)
	   }
	   // Update：proven 状态。
	   t2 := now.Add(1 * time.Second)
	   got.State = limitleasegate.ProofStateProven
	   got.ProvenAt = &t2
	   if err := store.UpdateProof(ctx, got); err != nil {
		   t.Fatalf("UpdateProof: %v", err)
	   }
	  // proofByLease 索引应被清除（因 state 从 open 变出）。
	  _, openErr := store.GetOpenProofByLease(ctx, "lease-001")
	  if openErr == nil {
		  t.Error("expected ErrNotFound after proof leaves open state, got nil")
	   }
  }

  func TestMemStore_ListProofsByState(t *testing.T) {
	  store := limitleasegate.NewMemStore()
	   ctx := context.Background()
	  now := time.Now().UTC()
	  // 插入 2 open，1 proven。
	  for i, state := range []limitleasegate.ProofState{
		  limitleasegate.ProofStateOpen,
		  limitleasegate.ProofStateOpen,
		   limitleasegate.ProofStateProven,
	  } {
		  p := &limitleasegate.LimitDriftProof{
			   ID: "lp-" + string(rune('a'+i)),
			  LeaseID: "lease-" + string(rune('a'+i)),
			   UserID:  "u001",
			   State:   state,
			  ProofHash: "hash-" + string(rune('a'+i)),
			   OpenedAt:  now,
			  CreatedAt: now,
			  UpdatedAt: now,
		   }
		   if err := store.InsertProof(ctx, p); err != nil {
			   t.Fatalf("InsertProof: %v", err)
		   }
	  }
	   opens,
		   err := store.ListProofsByState(ctx, limitleasegate.ProofStateOpen, 10)
	  if err != nil {
		  t.Fatalf("ListProofsByState: %v", err)
	   }
	  if len(opens) != 2 {
		  t.Errorf("open proofs = %d, want 2", len(opens))
	  }
	  provens, _ := store.ListProofsByState(ctx, limitleasegate.ProofStateProven, 10)
	  if len(provens) != 1 {
		   t.Errorf("proven proofs = %d, want 1", len(provens))
	   }
   }

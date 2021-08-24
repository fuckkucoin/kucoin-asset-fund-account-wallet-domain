package limitleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/limitleasegate"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// ─── MockPolicyVersionReader ─────────────────────────────────────────────
// mockPolicyVersionReader 用于 PolicyVersionRevokeWorker。
type mockPolicyVersionReader struct {
	ver int
}

   func (m *mockPolicyVersionReader) CurrentVersion() (int, error) {
	   return m.ver, nil
  }

// ─── LeaseExpiryWorker ──────────────────────────────────────────────

   func TestLeaseExpiryWorker_RunOnce(t *testing.T) {
	   store := limitleasegate.NewMemStore()
	   reg := policy.NewRegistry(policy.DefaultV1())
	   use := usage.New()
	   cfg := limitleasegate.Config{LeaseTTL: 10 * time.Second}
	   eng := limitleasegate.NewEngine(store,
		  reg, use, cfg)
	   ctx := context.Background()
	   now := time.Now().UTC()
	  eng = eng.WithClock(func() time.Time { return now })
	  // 插入一个已超 TTL 的租约。
	  expiredLease := &limitleasegate.LimitCheckLease{
		   ID: "exp-worker-001",
		  UserID:    "u001",
		  Asset:     "USDT",
		  Kind:      "withdraw",
		   PolicyVer: 1,
		  State:     limitleasegate.LeaseStateChecked,
		  IssuedAt: now.Add(-30 * time.Second),
		  ExpiresAt: now.Add(-10 * time.Second),
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  _ = store.InsertLease(ctx, expiredLease)
	  // 插入一个未超 TTL 的租约。
	   activeLease := &limitleasegate.LimitCheckLease{
		   ID: "active-worker-001",
		  UserID: "u002",
		   Asset:     "USDT",
		  Kind:      "withdraw",
		  PolicyVer: 1,
		   State:     limitleasegate.LeaseStateChecked,
		   IssuedAt:  now,
		   ExpiresAt: now.Add(120 * time.Second),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  _ = store.InsertLease(ctx,
		   activeLease)
	  workerCfg := limitleasegate.WorkerConfig{Interval: time.Hour, BatchSize: 200}
	   worker := limitleasegate.NewLeaseExpiryWorker(eng, store, workerCfg)
	  // 手动调用 Engine.ExpireLease（ Worker 一轮）。
	  leases, _ := store.ListExpiredLeases(ctx, now, 10)
	  for _, l := range leases {
		  _ = eng.ExpireLease(ctx, l.ID)
	  }
	  // 验证：过期租约 → expired。
	   expired, _ := store.GetLeaseByID(ctx, "exp-worker-001")
	  if expired.State != limitleasegate.LeaseStateExpired {
		  t.Errorf("expired lease state = %q, want expired", expired.State)
	  }
	  // 验证：活跃租约不受影响。
	  active, _ := store.GetLeaseByID(ctx, "active-worker-001")
	   if active.State != limitleasegate.LeaseStateChecked {
		  t.Errorf("active lease state = %q, want checked", active.State)
	  }
	   worker.Stop()
  }

// ─── PolicyVersionRevokeWorker ──────────────────────────────────────

  func TestPolicyVersionRevokeWorker_NotifyVersion(t *testing.T) {
	   store := limitleasegate.NewMemStore()
	   reg := policy.NewRegistry(policy.DefaultV1())
	   use := usage.New()
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	  eng := limitleasegate.NewEngine(store, reg, use, cfg)
	   ctx := context.Background()
	   now := time.Now().UTC()
	   eng = eng.WithClock(func() time.Time { return now })
	   // 插入 PolicyVer=1 的活跃租约。
	   for i := 0; i < 3; i++ {
		  l := &limitleasegate.LimitCheckLease{
			   ID: "pv-rev-" + string(rune('a'+i)),
			  UserID:    "u001",
			  Asset:     "USDT",
			   Kind:      "withdraw",
			   PolicyVer: 1,
			   State: limitleasegate.LeaseStateChecked,
			   IssuedAt:  now,
			   ExpiresAt: now.Add(120 * time.Second),
			   CreatedAt: now,
			   UpdatedAt: now,
		  }
		   _ = store.InsertLease(ctx, l)
	  }
	   reader := &mockPolicyVersionReader{ver: 2}
	  workerCfg := limitleasegate.WorkerConfig{Interval: time.Hour,
		   BatchSize: 200}
	  worker := limitleasegate.NewPolicyVersionRevokeWorker(eng, reader, workerCfg)
	  // 手动推送新代次通知。
	  worker.NotifyPolicyVersion(ctx, 2)
	   // 验证：所有 PolicyVer=1 的租约应被吊销。
	   for i := 0; i < 3; i++ {
		   id := "pv-rev-" + string(rune('a'+i))
		  l, _ := store.GetLeaseByID(ctx, id)
		   if l.State != limitleasegate.LeaseStateRevoked {
			  t.Errorf("lease %q state = %q, want revoked", id, l.State)
		   }
	   }
	   worker.Stop()
  }

// ─── ProofProveWorker ────────────────────────────────────────────────

  func TestProofProveWorker_RunOnce(t *testing.T) {
	  store := limitleasegate.NewMemStore()
	  reg := policy.NewRegistry(policy.DefaultV1())
	  use := usage.New()
	   cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   eng := limitleasegate.NewEngine(store, reg, use, cfg)
	   ctx := context.Background()
	  now := time.Now().UTC()
	  // 插入一个 open 证明。
	   proof := &limitleasegate.LimitDriftProof{
		   ID:        "pf-prove-001",
		   LeaseID:   "lease-001",
		  UserID:    "u001",
		   DriftType: limitleasegate.DriftTypePolicyFlip,
		  State:     limitleasegate.ProofStateOpen,
		  ProofHash: "hash-prove-001",
		  OpenedAt:  now,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   _ = store.InsertProof(ctx, proof)
	  workerCfg := limitleasegate.WorkerConfig{Interval: time.Hour, BatchSize: 200}
	  worker := limitleasegate.NewProofProveWorker(eng, store, workerCfg)
	   // 手动 Worker 一轮。
	   proofs, _ := store.ListProofsByState(ctx, limitleasegate.ProofStateOpen, 10)
	   for _, p := range proofs {
		   _ = eng.ProveProof(ctx, p.ID)
	   }
	   // 验证：证明已固化为 proven。
	   p2, _ := store.GetProofByID(ctx, "pf-prove-001")
	  if p2.State != limitleasegate.ProofStateProven {
		  t.Errorf("proof state = %q, want proven", p2.State)
	  }
	   worker.Stop()
  }

// ─── WorkerConfig 默认值 ─────────────────────────────────────────────

  func TestWorkerConfigDefaults(t *testing.T) {
	   store := limitleasegate.NewMemStore()
	  reg := policy.NewRegistry(policy.DefaultV1())
	   use := usage.New()
	  cfg := limitleasegate.Config{}
	   eng := limitleasegate.NewEngine(store, reg, use, cfg)
	   // 零值 WorkerConfig 应使用默认值，不 panic。
	   workerCfg := limitleasegate.WorkerConfig{}
	   w := limitleasegate.NewLeaseExpiryWorker(eng,
		   store, workerCfg)
	   ctx, cancel := context.WithCancel(context.Background())
	   w.Start(ctx)
	   cancel()
	   w.Stop()
   }

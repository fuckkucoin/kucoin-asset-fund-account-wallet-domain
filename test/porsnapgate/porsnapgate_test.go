package porsnapgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/por"
	"account-assets.kucoin.net/funding/internal/porsnapgate"
)

// ─────────────────────────────────────────────────────────────────────────────
// test helpers
// ─────────────────────────────────────────────────────────────────────────────

  func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

  func makeAccount(userID, book, asset, avail, frozen string) *ledger.Account {
	  av, _ := decimal.NewFromString(avail)
	   fr, _ := decimal.NewFromString(frozen)
	  return &ledger.Account{
		   UserID:      userID,
		   AccountType: ledger.AccountType(book),
		   Asset:     asset,
		   Available: av,
		   Frozen:    fr,
	  }
  }

  func newServiceWithAccounts(t *testing.T, now time.Time, accs []*ledger.Account) (*porsnapgate.Service, *porsnapgate.MemStore, *ledger.MemStore) {
	  t.Helper()
	  ctx := context.Background()
	   st := ledger.NewMemStore()
	   for _, a := range accs {
		  _ = st.UpsertAccount(ctx, a)
	   }
	  snapStore := porsnapgate.NewMemStore()
	  svc := porsnapgate.NewService(snapStore, st, fixedClock(now))
	   return svc, snapStore, st
   }

// ─────────────────────────────────────────────────────────────────────────────
// 1. BuildWithGen 基础封印
// ─────────────────────────────────────────────────────────────────────────────

  func TestBuildWithGen_SealBasic(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, snapStore, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		  makeAccount("user-001", "trading", "BTC", "1.50000000", "0.10000000"),
		   makeAccount("user-001", "trading", "ETH", "10.00000000", "0.00000000"),
	   })
	   result, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{
		  Users: []string{"user-001"},
		   TTLSeconds: 3600,
	  })
	   if err != nil {
		  t.Fatalf("BuildWithGen failed: %v", err)
	  }
	  if result.SnapGenID == "" {
		  t.Error("SnapGenID must not be empty")
	   }
	   if result.MerkleRoot == "" {
		   t.Error("MerkleRoot must not be empty")
	   }
	  if result.LeavesDigest == "" {
		   t.Error("LeavesDigest must not be empty")
	  }
	  if result.TotalsFingerprint == "" {
		  t.Error("TotalsFingerprint must not be empty")
	   }
	  if result.LeafCount != 2 {
		  t.Errorf("LeafCount want 2 got %d", result.LeafCount)
	   }
	   gen, err := snapStore.GetGen(ctx, result.SnapGenID)
	  if err != nil {
		  t.Fatalf("GetGen failed: %v", err)
	   }
	  if gen.State != porsnapgate.SnapSealed {
		   t.Errorf("gen.State want sealed got %s", gen.State)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 2. Attest 缺 snap_gen_id 拒绝（验收 2）
// ─────────────────────────────────────────────────────────────────────────────

   func TestAttest_NoGenReject(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  now := time.Date(2026,
		   9, 22, 10, 0, 0, 0, time.UTC)
	   svc, _, _ := newServiceWithAccounts(t, now, nil)
	  cases := []struct {
		  name string
		   req  porsnapgate.AttestRequest
		   wantErr error
	  }{
		  {
			  name:    "empty_snap_gen_id",
			  req:     porsnapgate.AttestRequest{SnapGenID: "", Requester: "ops-001"},
			  wantErr: porsnapgate.ErrNoGen,
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  _,
				  err := svc.Attest(ctx, tc.req)
			   if !errors.Is(err, tc.wantErr) {
				   t.Errorf("want %v got %v", tc.wantErr, err)
			   }
		  })
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 3. Attest → PublishAttest FSM
// ─────────────────────────────────────────────────────────────────────────────

   func TestAttest_PublishFSM(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	   svc, snapStore, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	   })
	  result, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	  if err != nil {
		   t.Fatalf("BuildWithGen: %v", err)
	   }
	  a, err := svc.Attest(ctx, porsnapgate.AttestRequest{SnapGenID: result.SnapGenID, Requester: "ops-001"})
	  if err != nil {
		   t.Fatalf("Attest: %v", err)
	  }
	  if a.State != porsnapgate.AttestPending {
		  t.Errorf("attest state want pending got %s", a.State)
	  }
	   published, err := svc.PublishAttest(ctx, a.AttestID)
	   if err != nil {
		   t.Fatalf("PublishAttest: %v", err)
	   }
	   if published.State != porsnapgate.AttestPublished {
		  t.Errorf("attest state want published got %s", published.State)
	  }
	  gen, err := snapStore.GetGen(ctx, result.SnapGenID)
	   if err != nil {
		   t.Fatalf("GetGen: %v", err)
	   }
	  if gen.State != porsnapgate.SnapAttested {
		  t.Errorf("gen state want attested got %s", gen.State)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 4. 封印后改账再 ProveAttested → ErrRootMismatch + DriftProof（验收 1）
// ─────────────────────────────────────────────────────────────────────────────

  func TestProveAttested_RootMismatchAfterMutate(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, _, st := newServiceWithAccounts(t, now, []*ledger.Account{
		  makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	  })
	   buildResult, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	   if err != nil {
		  t.Fatalf("BuildWithGen: %v", err)
	  }
	  sealedRoot := buildResult.MerkleRoot
	  a, _ := svc.Attest(ctx, porsnapgate.AttestRequest{SnapGenID: buildResult.SnapGenID, Requester: "ops-001"})
	  _, _ = svc.PublishAttest(ctx, a.AttestID)
	  // 变更账本余额
	  _ = st.UpsertAccount(ctx, makeAccount("user-001", "trading", "BTC", "2.00000000", "0.00000000"))
	   // ProveAttested 应返回 ErrRootMismatch
	   _, err = svc.ProveAttested(ctx, porsnapgate.ProveAttestedOpts{
		   SnapGenID: buildResult.SnapGenID,
		   UserID: "user-001",
		  Book:   "trading",
		  Asset:  "BTC",
		   Users:  []string{"user-001"},
	   })
	  if !errors.Is(err, porsnapgate.ErrRootMismatch) {
		   t.Errorf("want ErrRootMismatch got %v", err)
	  }
	  // 应产出 DriftProof（kind=replay）
	   proofs, err := svc.GetDriftProofsByGen(ctx, buildResult.SnapGenID)
	   if err != nil {
		   t.Fatalf("GetDriftProofsByGen: %v", err)
	   }
	   if len(proofs) == 0 {
		   t.Fatal("expected at least one drift proof after ErrRootMismatch")
	   }
	   p := proofs[0]
	  if p.Kind != porsnapgate.DriftKindReplay {
		   t.Errorf("drift proof kind want replay got %s", p.Kind)
	  }
	   if p.SealedRoot != sealedRoot {
		  t.Error("drift proof sealed root must match build result")
	  }
	   if p.LiveRoot == sealedRoot {
		  t.Error("live root should differ from sealed root after mutation")
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 5. CompareWithProof 漂移场景（验收 1）
// ─────────────────────────────────────────────────────────────────────────────

   func TestCompareWithProof_DriftDetected(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	   svc, _, st := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	   })
	   buildResult, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	   if err != nil {
		  t.Fatalf("BuildWithGen: %v", err)
	  }
	  _ = st.UpsertAccount(ctx, makeAccount("user-001", "trading", "BTC", "2.50000000", "0.00000000"))
	  result, err := svc.CompareWithProof(ctx, porsnapgate.CompareWithProofOpts{
		   SnapGenID: buildResult.SnapGenID,
		  Users:     []string{"user-001"},
	   })
	   if err != nil {
		   t.Fatalf("CompareWithProof: %v", err)
	  }
	   if !result.Drifted {
		  t.Error("expected drift to be detected")
	   }
	   if result.DriftProof == nil {
		   t.Error("drift proof must not be nil when drifted=true")
	   }
	   if result.DriftProof.Kind != porsnapgate.DriftKindCompare {
		   t.Errorf("drift proof kind want compare got %s", result.DriftProof.Kind)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 6. CompareWithProof 无漂移
// ─────────────────────────────────────────────────────────────────────────────

   func TestCompareWithProof_NoDrift(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	   now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, _, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	  })
	   buildResult, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	  if err != nil {
		  t.Fatalf("BuildWithGen: %v", err)
	   }
	  // 不变更账本，预期无漂移
	   result, err := svc.CompareWithProof(ctx, porsnapgate.CompareWithProofOpts{
		   SnapGenID: buildResult.SnapGenID,
		   Users: []string{"user-001"},
	  })
	   if err != nil {
		   t.Fatalf("CompareWithProof: %v", err)
	   }
	  if result.Drifted {
		   t.Error("expected no drift when ledger unchanged")
	   }
	  if result.DriftProof != nil {
		  t.Error("drift proof must be nil when no drift")
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 7. LiveSnapshot 打标非公证（验收 3）
// ─────────────────────────────────────────────────────────────────────────────

  func TestLiveSnapshot_TagNonAttestation(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	   now := time.Date(2026,
		  9, 22, 10, 0, 0, 0, time.UTC)
	  svc, snapStore, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	   })
	  result, err := svc.LiveSnapshot(ctx, []string{"user-001"})
	   if err != nil {
		  t.Fatalf("LiveSnapshot: %v", err)
	  }
	  if !result.Tag.NonAttestation {
		  t.Error("live snapshot must be tagged non_attestation=true")
	   }
	  if !result.Tag.NonForensic {
		  t.Error("live snapshot must be tagged non_forensic=true")
	   }
	  if result.Tag.Mode != "live" {
		  t.Errorf("tag mode want live got %s", result.Tag.Mode)
	  }
	  // live 模式不得向公证表写入任何行
	  pending, _ := snapStore.ListAttestsByState(ctx, porsnapgate.AttestPending, 10)
	   published, _ := snapStore.ListAttestsByState(ctx, porsnapgate.AttestPublished, 10)
	   if len(pending)+len(published) != 0 {
		   t.Error("live snapshot must not write to attestation table")
	   }
	  // live 模式不得写入 snap_gen 表
	  gens, _ := snapStore.ListGensByState(ctx, porsnapgate.SnapSealed, 10)
	   if len(gens) != 0 {
		  t.Error("live snapshot must not write snap_gen table")
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 8. FSM 非法迁移
// ─────────────────────────────────────────────────────────────────────────────

   func TestFSM_InvalidTransitions(t *testing.T) {
	  t.Parallel()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  genCases := []struct {
		  name string
		  state porsnapgate.SnapGenState
		   to    porsnapgate.SnapGenState
	   }{
		  {"expired_to_sealed", porsnapgate.SnapExpired, porsnapgate.SnapSealed},
		   {"revoked_to_attested", porsnapgate.SnapRevoked, porsnapgate.SnapAttested},
		  {"attested_to_sealed", porsnapgate.SnapAttested, porsnapgate.SnapSealed},
		   {"expired_to_revoked_disallowed", porsnapgate.SnapExpired, porsnapgate.SnapRevoked},
	   }
	   for _, tc := range genCases {
		  tc := tc
		  t.Run("gen_"+tc.name, func(t *testing.T) {
			   t.Parallel()
			  g := porsnapgate.PorSnapshotGen{State: tc.state}
			  err := porsnapgate.ApplyGenTransition(&g,
				  tc.to, now)
			  if !errors.Is(err, porsnapgate.ErrInvalidTransition) {
				  t.Errorf("want ErrInvalidTransition got %v", err)
			  }
		  })
	  }
	  attestCases := []struct {
		   name string
		  state porsnapgate.AttestState
		  to    porsnapgate.AttestState
	   }{
		  {"revoked_to_published", porsnapgate.AttestRevoked, porsnapgate.AttestPublished},
		   {"published_to_pending", porsnapgate.AttestPublished, porsnapgate.AttestPending},
	  }
	  for _, tc := range attestCases {
		   tc := tc
		   t.Run("attest_"+tc.name, func(t *testing.T) {
			   t.Parallel()
			  a := porsnapgate.AttestationBundle{State: tc.state}
			   err := porsnapgate.ApplyAttestTransition(&a, tc.to, "", now)
			   if !errors.Is(err, porsnapgate.ErrInvalidTransition) {
				   t.Errorf("want ErrInvalidTransition got %v", err)
			   }
		  })
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 9. 指纹稳定性
// ─────────────────────────────────────────────────────────────────────────────

  func TestFingerprint_Stability(t *testing.T) {
	   t.Parallel()
	  leaves := []por.Leaf{
		  {UserID: "user-001", Book: "trading", Asset: "BTC", Available: "1.00000000", Frozen: "0.00000000"},
		   {UserID: "user-002", Book: "funding", Asset: "USDT", Available: "500.00000000", Frozen: "0.00000000"},
	   }
	   d1 := porsnapgate.HashLeafSet(leaves)
	   d2 := porsnapgate.HashLeafSet(leaves)
	  if d1 != d2 {
		   t.Error("HashLeafSet must be deterministic")
	  }
	  // 不同顺序应产出相同摘要
	   reversed := []por.Leaf{leaves[1], leaves[0]}
	   d3 := porsnapgate.HashLeafSet(reversed)
	  if d1 != d3 {
		   t.Errorf("HashLeafSet must be order-independent: d1=%s d3=%s", d1, d3)
	  }
	   totals := []porsnapgate.TotalEntry{
		  {Asset: "BTC", Amount: "2.30000000"},
		   {Asset: "ETH", Amount: "10.00000000"},
	  }
	   fp1 := porsnapgate.HashTotals(totals)
	   fp2 := porsnapgate.HashTotals([]porsnapgate.TotalEntry{
		   {Asset: "ETH", Amount: "10.00000000"},
		   {Asset: "BTC", Amount: "2.30000000"},
	  })
	  if fp1 != fp2 {
		  t.Errorf("HashTotals must be order-independent: fp1=%s fp2=%s", fp1, fp2)
	  }
	  // 不同内容应产出不同摘要
	  fp3 := porsnapgate.HashTotals([]porsnapgate.TotalEntry{{Asset: "BTC", Amount: "999.00000000"}})
	  if fp1 == fp3 {
		  t.Error("different totals should produce different fingerprints")
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 10. ExpireArchiveWorker tick 归档过期世代
// ─────────────────────────────────────────────────────────────────────────────

  func TestExpireArchiveWorker_TickExpires(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	   pastTime := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC) // 2 h ago
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  // 用过去时间封印，TTL=1s → 现在已过期
	  svc, snapStore, st := newServiceWithAccounts(t, pastTime, []*ledger.Account{
		  makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	  })
	   result, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{
		   Users:      []string{"user-001"},
		   TTLSeconds: 1,
	   })
	   if err != nil {
		  t.Fatalf("BuildWithGen: %v", err)
	   }
	   // 用现在时间创建新 svc 来运行 worker
	   svc2 := porsnapgate.NewService(snapStore, st, fixedClock(now))
	   worker := porsnapgate.NewExpireArchiveWorker(svc2, time.Minute, 50)
	   if err := worker.TickForTest(ctx, now); err != nil {
		  t.Fatalf("TickForTest: %v", err)
	   }
	   gen,
		   err := snapStore.GetGen(ctx, result.SnapGenID)
	  if err != nil {
		   t.Fatalf("GetGen: %v", err)
	   }
	  if gen.State != porsnapgate.SnapExpired {
		  t.Errorf("gen state want expired got %s", gen.State)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 11. RevokeGen 联动撤销公证
// ─────────────────────────────────────────────────────────────────────────────

  func TestRevokeGen_CascadesAttest(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	   svc, snapStore, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	   })
	  result, _ := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	   a, _ := svc.Attest(ctx, porsnapgate.AttestRequest{SnapGenID: result.SnapGenID, Requester: "ops-001"})
	   _, _ = svc.PublishAttest(ctx, a.AttestID)
	   if err := svc.RevokeGen(ctx, result.SnapGenID, "test revoke"); err != nil {
		   t.Fatalf("RevokeGen: %v", err)
	   }
	   gen, _ := snapStore.GetGen(ctx, result.SnapGenID)
	   if gen.State != porsnapgate.SnapRevoked {
		   t.Errorf("gen state want revoked got %s", gen.State)
	   }
	   attest, _ := snapStore.GetAttest(ctx, a.AttestID)
	   if attest.State != porsnapgate.AttestRevoked {
		   t.Errorf("attest state want revoked got %s", attest.State)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 12. ProveAttested 缺 gen 拒绝
// ─────────────────────────────────────────────────────────────────────────────

  func TestProveAttested_NoGenReject(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, _, _ := newServiceWithAccounts(t, now, nil)
	  _, err := svc.ProveAttested(ctx, porsnapgate.ProveAttestedOpts{
		  SnapGenID: "",
		  UserID: "user-001",
		   Book: "trading",
		   Asset: "BTC",
	  })
	   if !errors.Is(err, porsnapgate.ErrNoGen) {
		  t.Errorf("want ErrNoGen got %v", err)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 13. 重复 BuildWithGen：不同 ID，相同根
// ─────────────────────────────────────────────────────────────────────────────

  func TestBuildWithGen_MultipleSeals(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, _, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	   })
	   r1, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	   if err != nil {
		  t.Fatalf("BuildWithGen first: %v", err)
	   }
	  r2, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	  if err != nil {
		   t.Fatalf("BuildWithGen second: %v", err)
	  }
	   if r1.SnapGenID == r2.SnapGenID {
		   t.Error("each BuildWithGen must produce a unique SnapGenID")
	  }
	  if r1.MerkleRoot != r2.MerkleRoot {
		   t.Error("same ledger state must produce same MerkleRoot")
	  }
	   if r1.TotalsFingerprint != r2.TotalsFingerprint {
		   t.Error("same ledger state must produce same TotalsFingerprint")
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 14. Attest 过期世代拒绝
// ─────────────────────────────────────────────────────────────────────────────

   func TestAttest_ExpiredGenReject(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  pastTime := time.Date(2026,
		  9, 21, 10, 0, 0, 0, time.UTC)
	   now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, snapStore, st := newServiceWithAccounts(t, pastTime, []*ledger.Account{
		  makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	  })
	  result, err := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{
		  Users:      []string{"user-001"},
		  TTLSeconds: 1, // 1 秒后过期
	  })
	   if err != nil {
		  t.Fatalf("BuildWithGen: %v", err)
	   }
	  svcNow := porsnapgate.NewService(snapStore,
		   st, fixedClock(now))
	  _, err = svcNow.Attest(ctx, porsnapgate.AttestRequest{SnapGenID: result.SnapGenID, Requester: "ops-001"})
	  if !errors.Is(err, porsnapgate.ErrGenExpired) {
		   t.Errorf("want ErrGenExpired got %v", err)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 15. CompareWithProof 已撤销世代拒绝
// ─────────────────────────────────────────────────────────────────────────────

  func TestCompareWithProof_RevokedGenReject(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	   now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, _, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		   makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	   })
	  result, _ := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	  _ = svc.RevokeGen(ctx, result.SnapGenID, "test")
	  _, err := svc.CompareWithProof(ctx, porsnapgate.CompareWithProofOpts{
		   SnapGenID: result.SnapGenID,
		  Users: []string{"user-001"},
	  })
	   if !errors.Is(err, porsnapgate.ErrGenRevoked) {
		   t.Errorf("want ErrGenRevoked got %v", err)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 16. AttestReVerifyWorker 一致性校验
// ─────────────────────────────────────────────────────────────────────────────

   func TestAttestReVerify_Consistent(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	  now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	  svc, _, _ := newServiceWithAccounts(t, now, []*ledger.Account{
		  makeAccount("user-001", "trading", "BTC", "1.00000000", "0.00000000"),
	  })
	  result, _ := svc.BuildWithGen(ctx, porsnapgate.BuildWithGenOpts{Users: []string{"user-001"}, TTLSeconds: 86400})
	  a, _ := svc.Attest(ctx, porsnapgate.AttestRequest{SnapGenID: result.SnapGenID, Requester: "ops-001"})
	   pub, _ := svc.PublishAttest(ctx, a.AttestID)
	  worker := porsnapgate.NewAttestReVerifyWorker(svc, time.Minute, 10)
	   vr, err := worker.VerifyAttest(ctx, pub.AttestID)
	  if err != nil {
		  t.Fatalf("VerifyAttest: %v", err)
	  }
	   if !vr.Valid {
		  t.Errorf("attestation should be valid: %s", vr.Reason)
	   }
  }

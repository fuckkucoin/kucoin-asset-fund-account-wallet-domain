package flow

import (
	"context"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/invdrift"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// ---- mem store for testing ----
// memStore 内存实现 invdrift.Store，仅用于单元/流程。
type memStore struct {
	batches map[string]invdrift.DriftBatch
	items   map[string]invdrift.DriftItem
	advices map[string]invdrift.SealAdvice
	proofs  map[string]invdrift.DriftProof
}

  func newMemStore() *memStore {
	   return &memStore{
		   batches: map[string]invdrift.DriftBatch{},
		   items:   map[string]invdrift.DriftItem{},
		  advices: map[string]invdrift.SealAdvice{},
		   proofs: map[string]invdrift.DriftProof{},
	   }
  }

  func (m *memStore) InsertBatch(_ context.Context, b invdrift.DriftBatch) error {
	  m.batches[b.ID] = b
	  return nil
   }

  func (m *memStore) UpdateBatch(_ context.Context, b invdrift.DriftBatch) error {
	   m.batches[b.ID] = b
	  return nil
   }

  func (m *memStore) GetBatch(_ context.Context, id string) (invdrift.DriftBatch, error) {
	  b, ok := m.batches[id]
	  if !ok {
		   return invdrift.DriftBatch{}, invdrift.ErrBatchNotFound
	   }
	   return b, nil
  }

   func (m *memStore) ListBatches(_ context.Context, limit, offset int) ([]invdrift.DriftBatch, error) {
	  out := make([]invdrift.DriftBatch, 0, len(m.batches))
	   for _, b := range m.batches {
		  out = append(out, b)
	   }
	  if offset >= len(out) {
		  return nil, nil
	   }
	  out = out[offset:]
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
   }

   func (m *memStore) ListOpenBatches(_ context.Context) ([]invdrift.DriftBatch, error) {
	  var out []invdrift.DriftBatch
	   for _, b := range m.batches {
		   if b.State != invdrift.BatchStateResolved {
			  out = append(out, b)
		   }
	  }
	  return out, nil
   }

  func (m *memStore) InsertItems(_ context.Context, items []invdrift.DriftItem) error {
	  for _, it := range items {
		   m.items[it.ID] = it
	   }
	   return nil
  }

   func (m *memStore) ListItemsByBatch(_ context.Context, batchID string) ([]invdrift.DriftItem, error) {
	  var out []invdrift.DriftItem
	   for _, it := range m.items {
		   if it.BatchID == batchID {
			   out = append(out, it)
		  }
	  }
	   return out, nil
  }

   func (m *memStore) ListItemsByAsset(_ context.Context, asset string, limit int) ([]invdrift.DriftItem, error) {
	  var out []invdrift.DriftItem
	  for _, it := range m.items {
		  if it.Asset == asset {
			  out = append(out, it)
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
  }

  func (m *memStore) MarkItemResolved(_ context.Context, itemID string, resolvedAt time.Time) error {
	  it, ok := m.items[itemID]
	   if !ok {
		   return invdrift.ErrItemNotFound
	   }
	  it.ResolvedAt = &resolvedAt
	  m.items[itemID] = it
	   return nil
  }

   func (m *memStore) InsertAdvices(_ context.Context, advices []invdrift.SealAdvice) error {
	  for _, a := range advices {
		  m.advices[a.ID] = a
	   }
	   return nil
   }

  func (m *memStore) GetAdvice(_ context.Context, id string) (invdrift.SealAdvice, error) {
	   a, ok := m.advices[id]
	  if !ok {
		  return invdrift.SealAdvice{}, invdrift.ErrAdviceNotFound
	  }
	   return a, nil
   }

   func (m *memStore) ListAdvicesByBatch(_ context.Context, batchID string) ([]invdrift.SealAdvice, error) {
	   var out []invdrift.SealAdvice
	  for _, a := range m.advices {
		   if a.BatchID == batchID {
			  out = append(out, a)
		   }
	   }
	  return out, nil
   }

  func (m *memStore) ListActiveAdvices(_ context.Context) ([]invdrift.SealAdvice, error) {
	   var out []invdrift.SealAdvice
	  for _, a := range m.advices {
		  if a.State == invdrift.AdviceStateProposed {
			  out = append(out, a)
		  }
	  }
	  return out, nil
   }

   func (m *memStore) UpdateAdviceState(_ context.Context,
	  id string, state invdrift.AdviceState,
	   ackedBy string, ackedAt, expiredAt *time.Time, expireReason string) error {
	  a, ok := m.advices[id]
	  if !ok {
		   return invdrift.ErrAdviceNotFound
	  }
	  a.State = state
	  a.AckedBy = ackedBy
	   a.AckedAt = ackedAt
	  a.ExpiredAt = expiredAt
	  a.ExpireReason = expireReason
	  m.advices[id] = a
	  return nil
  }

  func (m *memStore) InsertProof(_ context.Context, p invdrift.DriftProof) error {
	   m.proofs[p.ID] = p
	  return nil
  }

   func (m *memStore) GetProof(_ context.Context, id string) (invdrift.DriftProof, error) {
	  p, ok := m.proofs[id]
	  if !ok {
		   return invdrift.DriftProof{}, invdrift.ErrProofNotFound
	  }
	   return p, nil
  }

   func (m *memStore) ListProofsByBatch(_ context.Context, batchID string) ([]invdrift.DriftProof, error) {
	   var out []invdrift.DriftProof
	  for _, p := range m.proofs {
		   if p.BatchID == batchID {
			  out = append(out, p)
		   }
	   }
	   return out, nil
   }

// ---- mem ledger store ----
// newMemLedger 构造并注入账户余额，返回满足 ledger.Store 的 *ledger.MemStore。
   func newMemLedger(accounts []ledger.Account) *ledger.MemStore {
	  ms := ledger.NewMemStore()
	  for _, a := range accounts {
		   ms.Seed(a.UserID, a.AccountType, a.Asset, a.Available.String())
	   }
	   return ms
   }

// ---- test fixtures ----
// hotShortFixture hotinv 少记场景：账本 hot=600000, custody hot=400000（少 200000 USDT）
// 预期：crit DriftBatch + SealAdvice
   func hotShortFixture() []ledger.Account {
	   return []ledger.Account{
		   {UserID: "system", AccountType: ledger.AccountBridge, Asset: "USDT",
			   Available: decimal.NewFromInt(600000)},
		  {UserID: "system", AccountType: ledger.AccountCold, Asset: "USDT",
			  Available: decimal.NewFromInt(200000)},
	  }
   }

  func hotShortCustody() []custody.InventoryRow {
	  return []custody.InventoryRow{
		   {Asset: "USDT", Available: "400000", Side: "hot"},
		   {Asset: "USDT", Available: "200000", Side: "cold"},
	   }
  }

// coldExcessFixture 冷库存多记场景（账=100, custody cold=108）
// delta_cold = 100 - 108 = -8，百分比 8%，阈 5%→crit

   func coldExcessFixture() []ledger.Account {
	  return []ledger.Account{
		  {UserID: "system", AccountType: ledger.AccountBridge, Asset: "BTC",
			   Available: decimal.NewFromFloat(80)},
		  {UserID: "system", AccountType: ledger.AccountCold, Asset: "BTC",
			   Available: decimal.NewFromFloat(100)},
	  }
  }

   func coldExcessCustody() []custody.InventoryRow {
	  return []custody.InventoryRow{
		   {Asset: "BTC", Available: "80", Side: "hot"},
		  {Asset: "BTC", Available: "108", Side: "cold"},
	   }
  }

// bridgeLagFixture 桥接科目小偏差（热偏 0.2%），期望 info
  func bridgeLagFixture() []ledger.Account {
	  return []ledger.Account{
		  {UserID: "system", AccountType: ledger.AccountBridge, Asset: "ETH",
			   Available: decimal.NewFromFloat(800)},
		  {UserID: "system", AccountType: ledger.AccountCold, Asset: "ETH",
			  Available: decimal.NewFromFloat(200)},
	   }
  }

  func bridgeLagCustody() []custody.InventoryRow {
	   return []custody.InventoryRow{
		  {Asset: "ETH", Available: "798.5", Side: "hot"},
		   {Asset: "ETH", Available: "200", Side: "cold"},
	  }
   }

// ---- helpers ----

  func fixedClock(t time.Time) func() time.Time {
	   return func() time.Time { return t }
  }

// ---- tests ----

   func TestRunScan_HotShort_Crit(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  batchID := uuid.NewString()
	   ledRows := hotShortFixture()
	  custRows := hotShortCustody()
	  out := invdrift.RunScan(batchID, invdrift.ScanInput{
		   LedgerRows: ledRows,
		   CustRows:  custRows,
		  ThreshCfg: invdrift.DefaultThresholds,
	   }, fixedClock(now))
	   if len(out.Items) == 0 {
		  t.Fatal("expected items, got none")
	  }
	   var usdtItem *invdrift.DriftItem
	   for i := range out.Items {
		  if out.Items[i].Asset == "USDT" {
			  usdtItem = &out.Items[i]
		  }
	   }
	   if usdtItem == nil {
		  t.Fatal("expected USDT item")
	   }
	   if usdtItem.Grade != invdrift.GradeCrit {
		   t.Errorf("expected crit, got %s (delta=%s, pct=%s)",
			  usdtItem.Grade, usdtItem.DeltaHot, usdtItem.DeltaPct)
	  }
	   if usdtItem.DeltaHot != "200000" {
		   t.Errorf("expected delta_hot=200000, got %s", usdtItem.DeltaHot)
	  }
   }

   func TestRunScan_ColdExcess_Crit(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  batchID := uuid.NewString()
	   out := invdrift.RunScan(batchID, invdrift.ScanInput{
		   LedgerRows: coldExcessFixture(),
		  CustRows: coldExcessCustody(),
		  ThreshCfg: invdrift.DefaultThresholds,
	  }, fixedClock(now))
	   var btcItem *invdrift.DriftItem
	   for i := range out.Items {
		   if out.Items[i].Asset == "BTC" {
			   btcItem = &out.Items[i]
		  }
	   }
	  if btcItem == nil {
		   t.Fatal("expected BTC item")
	  }
	  // delta_cold = 100 - 108 = -8 BTC, 8% > 5% crit threshold
	  if btcItem.Grade != invdrift.GradeCrit {
		  t.Errorf("expected crit for BTC cold excess, got %s (delta_cold=%s)",
			  btcItem.Grade, btcItem.DeltaCold)
	  }
   }

  func TestRunScan_BridgeLag_Info(t *testing.T) {
	   now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  batchID := uuid.NewString()
	   out := invdrift.RunScan(batchID, invdrift.ScanInput{
		   LedgerRows: bridgeLagFixture(),
		  CustRows: bridgeLagCustody(),
		  ThreshCfg: invdrift.DefaultThresholds,
	   }, fixedClock(now))
	   var ethItem *invdrift.DriftItem
	  for i := range out.Items {
		  if out.Items[i].Asset == "ETH" {
			  ethItem = &out.Items[i]
		   }
	  }
	  if ethItem == nil {
		   t.Fatal("expected ETH item")
	  }
	   // delta_hot = 800 - 798.5 = 1.5 ETH, 0.1875% < 1% warn threshold
	  if ethItem.Grade != invdrift.GradeInfo {
		  t.Errorf("expected info for bridge lag, got %s", ethItem.Grade)
	   }
   }

   func TestBuildSealAdvices_CritItems(t *testing.T) {
	   now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  batchID := uuid.NewString()
	  critItems := []invdrift.DriftItem{
		  {ID: uuid.NewString(), BatchID: batchID, Asset: "USDT", Grade: invdrift.GradeCrit,
			  DeltaHot: "200000", Reason: "critical drift: hot=200000 cold=0"},
	  }
	  advices := invdrift.BuildSealAdvices(batchID, critItems, 1800, fixedClock(now))
	   if len(advices) != 3 {
		  t.Errorf("expected 3 advices (one per kind), got %d", len(advices))
	   }
	  kinds := map[invdrift.SealKind]bool{}
	   for _, a := range advices {
		  kinds[a.Kind] = true
		  if a.State != invdrift.AdviceStateProposed {
			  t.Errorf("expected proposed, got %s", a.State)
		  }
		   if a.Asset != "USDT" {
			  t.Errorf("expected USDT, got %s", a.Asset)
		  }
	  }
	  if !kinds[invdrift.SealKindBlockSign] {
		  t.Error("missing block_sign advice")
	   }
	   if !kinds[invdrift.SealKindBlockSweepOver] {
		  t.Error("missing block_sweep_over advice")
	   }
	  if !kinds[invdrift.SealKindBlockLargeWdPre] {
		   t.Error("missing block_large_wd_pre advice")
	  }
  }

  func TestService_Scan_Crit_ProducesAdvices(t *testing.T) {
	   now := time.Date(2025,
		   11, 3, 12, 0, 0, 0, time.UTC)
	   ms := newMemStore()
	  _ = invdrift.New(ms, newMemLedger(hotShortFixture()), nil, nil,
		   invdrift.WithClock(fixedClock(now)),
		   invdrift.WithThresholds(invdrift.DefaultThresholds),
	  )
	   // 直接用 RunScan+BuildBatchFromItems 验证流程
	  batchID := uuid.NewString()
	  out := invdrift.RunScan(batchID, invdrift.ScanInput{
		  LedgerRows: hotShortFixture(),
		  CustRows:  hotShortCustody(),
		   ThreshCfg: invdrift.DefaultThresholds,
	   }, fixedClock(now))
	   if err := ms.InsertItems(context.Background(), out.Items); err != nil {
		   t.Fatal(err)
	  }
	  critItems := invdrift.FilterCrit(out.Items)
	   advices := invdrift.BuildSealAdvices(batchID, critItems, 1800, fixedClock(now))
	  if err := ms.InsertAdvices(context.Background(), advices); err != nil {
		  t.Fatal(err)
	  }
	   active, err := ms.ListActiveAdvices(context.Background())
	  if err != nil {
		   t.Fatal(err)
	  }
	  if len(active) == 0 {
		   t.Fatal("expected active seal advices after crit scan")
	  }
	   // 验证 USDT 存在活跃建议
	  found := false
	   for _, a := range active {
		   if a.Asset == "USDT" {
			  found = true
			   break
		   }
	  }
	  if !found {
		  t.Error("expected USDT in active advices")
	  }
   }

   func TestService_ForceReview_ResolvesAll(t *testing.T) {
	  now := time.Date(2025,
		  11, 3, 12, 0, 0, 0, time.UTC)
	   ms := newMemStore()
	  batchID := uuid.NewString()
	  batch := invdrift.DriftBatch{
		   ID: batchID, State: invdrift.BatchStateSealed,
		   ScanAt: now, CritCount: 1, CreatedAt: now, UpdatedAt: now,
	  }
	   _ = ms.InsertBatch(context.Background(), batch)
	   item := invdrift.DriftItem{
		   ID: uuid.NewString(), BatchID: batchID, Asset: "USDT",
		   Grade: invdrift.GradeCrit, CreatedAt: now,
	   }
	   _ = ms.InsertItems(context.Background(), []invdrift.DriftItem{item})
	   advice := invdrift.SealAdvice{
		  ID: uuid.NewString(), BatchID: batchID, ItemID: item.ID,
		   Asset: "USDT", Kind: invdrift.SealKindBlockSign,
		   State: invdrift.AdviceStateProposed,
		  Grade: invdrift.GradeCrit, ProposedAt: now,
		  TTLSeconds: 1800, CreatedAt: now,
	   }
	   _ = ms.InsertAdvices(context.Background(), []invdrift.SealAdvice{advice})
	  svc := invdrift.New(ms, newMemLedger([]ledger.Account{}), nil, nil,
		  invdrift.WithClock(fixedClock(now)),
	   )
	   resolved, err := svc.ForceReview(context.Background(), invdrift.ForceReviewRequest{
		   BatchID: batchID, Actor: "ops-alice", Note: "偏离已人工确认",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	  if resolved.State != invdrift.BatchStateResolved {
		   t.Errorf("expected resolved, got %s", resolved.State)
	   }
	   // 建议应已 expired
	  updatedAdvice, _ := ms.GetAdvice(context.Background(), advice.ID)
	  if updatedAdvice.State != invdrift.AdviceStateExpired {
		  t.Errorf("expected expired advice, got %s", updatedAdvice.State)
	  }
	  // 不残留 proposed 建议
	   active, _ := ms.ListActiveAdvices(context.Background())
	   for _, a := range active {
		   if a.BatchID == batchID {
			   t.Errorf("unexpected active advice after resolve: %s", a.ID)
		   }
	   }
  }

   func TestService_HasActiveAdvice_AfterCritScan(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  ms := newMemStore()
	  advice := invdrift.SealAdvice{
		  ID: uuid.NewString(), BatchID: "b1", ItemID: "i1",
		  Asset: "USDT", Kind: invdrift.SealKindBlockSign,
		   State: invdrift.AdviceStateProposed, Grade: invdrift.GradeCrit,
		   ProposedAt: now, TTLSeconds: 1800, CreatedAt: now,
	  }
	  _ = ms.InsertAdvices(context.Background(), []invdrift.SealAdvice{advice})
	   svc := invdrift.New(ms, newMemLedger([]ledger.Account{}), nil, nil,
		  invdrift.WithClock(fixedClock(now)),
	   )
	   has, err := svc.HasActiveAdvice(context.Background(), "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	   if !has {
		  t.Error("expected HasActiveAdvice=true for USDT with proposed advice")
	   }
	   // ETH 无建议，应返回 false
	   has2, _ := svc.HasActiveAdvice(context.Background(), "ETH")
	   if has2 {
		  t.Error("expected HasActiveAdvice=false for ETH")
	  }
  }

  func TestDriftProof_IntegrityOK(t *testing.T) {
	  proof, err := invdrift.BuildProof(
		   "batch-1", "USDT",
		   map[string]string{"hot": "600000", "cold": "200000"},
		   map[string]string{"hot": "400000", "cold": "200000"},
		   map[string]string{"hot": "400000", "cold": "200000"},
		  "200000", "0", invdrift.GradeCrit,
	   )
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !proof.IntegrityOK() {
		  t.Error("expected integrity OK for unmodified proof")
	   }
	  // 篡改 ledger_snap
	   proof.LedgerSnap = `{"hot":"999999","cold":"0"}`
	  if proof.IntegrityOK() {
		  t.Error("expected integrity FAIL after tampered ledger_snap")
	   }
   }

   func TestDriftProof_TamperedDigest(t *testing.T) {
	  proof, err := invdrift.BuildProof(
		  "batch-2", "BTC",
		  map[string]string{"hot": "80", "cold": "100"},
		  map[string]string{"hot": "80", "cold": "108"},
		   map[string]string{"hot": "80", "cold": "100"},
		  "0", "-8", invdrift.GradeCrit,
	   )
	   if err != nil {
		  t.Fatal(err)
	  }
	   // 篡改 custody_digest（保持 snap 不变）
	  proof.CustodyDigest = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	   if proof.IntegrityOK() {
		  t.Error("expected integrity FAIL after tampered custody_digest")
	  }
   }

   func TestDriftProof_TamperedCompositeHash(t *testing.T) {
	   proof, err := invdrift.BuildProof(
		   "batch-3", "ETH",
		  map[string]string{"hot": "800", "cold": "200"},
		  map[string]string{"hot": "798.5", "cold": "200"},
		  map[string]string{"hot": "798.5", "cold": "200"},
		  "1.5", "0", invdrift.GradeInfo,
	   )
	   if err != nil {
		  t.Fatal(err)
	   }
	  // 篡改复合哈希
	   proof.CompositeHash = "0000000000000000000000000000000000000000000000000000000000000000"
	   if proof.IntegrityOK() {
		  t.Error("expected integrity FAIL after tampered composite_hash")
	  }
   }

  func TestExpireStaleAdvices(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  ms := newMemStore()
	   // 已超 TTL 的建议
	   expiredAdvice := invdrift.SealAdvice{
		  ID: uuid.NewString(),
		   BatchID: "b1",
		  ItemID:  "i1",
		  Asset:   "USDT",
		  Kind:  invdrift.SealKindBlockSign,
		  State: invdrift.AdviceStateProposed,
		  Grade:      invdrift.GradeCrit,
		  ProposedAt: now.Add(-2 * time.Hour), // 2h ago
		   TTLSeconds: 1800, // 30min TTL
		   CreatedAt: now.Add(-2 * time.Hour),
	  }
	   _ = ms.InsertAdvices(context.Background(), []invdrift.SealAdvice{expiredAdvice})
	   // 未超 TTL 的建议
	   freshAdvice := invdrift.SealAdvice{
		   ID:      uuid.NewString(),
		   BatchID: "b2",
		   ItemID:  "i2",
		  Asset: "BTC",
		  Kind:  invdrift.SealKindBlockSign,
		   State: invdrift.AdviceStateProposed,
		   Grade: invdrift.GradeCrit,
		  ProposedAt: now.Add(-5 * time.Minute),
		  TTLSeconds: 1800,
		  CreatedAt:  now.Add(-5 * time.Minute),
	  }
	   _ = ms.InsertAdvices(context.Background(), []invdrift.SealAdvice{freshAdvice})
	  svc := invdrift.New(ms, newMemLedger([]ledger.Account{}), nil, nil,
		   invdrift.WithClock(fixedClock(now)),
	   )
	   n, err := svc.ExpireStaleAdvices(context.Background())
	  if err != nil {
		  t.Fatal(err)
	   }
	   if n != 1 {
		  t.Errorf("expected 1 expired, got %d", n)
	  }
	  // 超 TTL 的应为 expired
	  updated, _ := ms.GetAdvice(context.Background(), expiredAdvice.ID)
	  if updated.State != invdrift.AdviceStateExpired {
		  t.Errorf("expected expired, got %s", updated.State)
	  }
	  // 未超 TTL 的仍为 proposed
	  fresh,
		  _ := ms.GetAdvice(context.Background(), freshAdvice.ID)
	   if fresh.State != invdrift.AdviceStateProposed {
		  t.Errorf("expected proposed, got %s", fresh.State)
	  }
   }

   func TestGradeMatrix_TableDriven(t *testing.T) {
	  cases := []struct {
		   name string
		  asset     string
		   ledgerHot string
		  custHot   string
		   wantGrade invdrift.DriftGrade
	  }{
		   {
			   name:  "zero_drift_info",
			   asset: "USDT", ledgerHot: "500000", custHot: "500000",
			  wantGrade: invdrift.GradeInfo,
		   },
		   {
			   name: "small_drift_info",
			  asset: "USDT", ledgerHot: "500000", custHot: "499500",
			   wantGrade: invdrift.GradeInfo, // 0.1% < 1% warn
		   },
		   {
			  name:  "warn_pct",
			  asset: "USDT", ledgerHot: "500000", custHot: "493000",
			  wantGrade: invdrift.GradeWarn, // 1.4% > 1% warn, < 5% crit
		   },
		  {
			   name:  "crit_pct",
			  asset: "USDT", ledgerHot: "500000", custHot: "470000",
			   wantGrade: invdrift.GradeCrit, // 6% > 5% crit
		   },
		   {
			   name: "crit_abs_btc",
			   asset: "BTC", ledgerHot: "10", custHot: "8.5",
			  wantGrade: invdrift.GradeCrit, // delta=1.5 > 1 BTC crit
		   },
		  {
			   name:  "warn_abs_btc",
			   asset: "BTC", ledgerHot: "10", custHot: "9.7",
			   wantGrade: invdrift.GradeWarn, // delta=0.3 > 0.1 BTC warn, < 1 BTC crit
		  },
		   {
			  name:  "exact_zero_hot",
			   asset: "USDT", ledgerHot: "0", custHot: "0",
			  wantGrade: invdrift.GradeInfo,
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   ledBase,
				   _ := decimal.NewFromString(tc.ledgerHot)
			   custV,
				   _ := decimal.NewFromString(tc.custHot)
			  delta := ledBase.Sub(custV)
			   // 调用 RunScan 通过构造 ScanInput 间接分级
			  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
			   batchID := uuid.NewString()
			  out := invdrift.RunScan(batchID, invdrift.ScanInput{
				   LedgerRows: []ledger.Account{
					   {UserID: "system", AccountType: ledger.AccountBridge, Asset: tc.asset,
						  Available: ledBase},
				   },
				   CustRows: []custody.InventoryRow{
					  {Asset: tc.asset, Available: custV.String(), Side: "hot"},
				   },
				   ThreshCfg: invdrift.DefaultThresholds,
			  }, fixedClock(now))
			   _ = delta
			   if len(out.Items) == 0 {
				  t.Fatalf("no items for asset %s", tc.asset)
			   }
			   if out.Items[0].Grade != tc.wantGrade {
				   t.Errorf("asset=%s ledger=%s cust=%s: want grade=%s got %s (delta=%s pct=%s)",
					   tc.asset, tc.ledgerHot, tc.custHot,
					  tc.wantGrade, out.Items[0].Grade,
					  out.Items[0].DeltaHot, out.Items[0].DeltaPct)
			   }
		   })
	  }
  }

   func TestCountByGrade(t *testing.T) {
	  items := []invdrift.DriftItem{
		  {Grade: invdrift.GradeCrit},
		   {Grade: invdrift.GradeCrit},
		   {Grade: invdrift.GradeWarn},
		   {Grade: invdrift.GradeInfo},
		  {Grade: invdrift.GradeInfo},
		  {Grade: invdrift.GradeInfo},
	  }
	  c, w, i := invdrift.CountByGrade(items)
	   if c != 2 {
		   t.Errorf("expected 2 crit, got %d", c)
	  }
	   if w != 1 {
		   t.Errorf("expected 1 warn, got %d", w)
	  }
	  if i != 3 {
		   t.Errorf("expected 3 info, got %d", i)
	  }
  }

  func TestBuildBatchFromItems_SealedState(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  items := []invdrift.DriftItem{
		  {Grade: invdrift.GradeCrit},
		   {Grade: invdrift.GradeWarn},
	   }
	  b := invdrift.BuildBatchFromItems("bid", "dc-sg", "ops-test", items, fixedClock(now))
	   if b.State != invdrift.BatchStateSealed {
		  t.Errorf("expected sealed with crit items, got %s", b.State)
	   }
	  if b.CritCount != 1 {
		  t.Errorf("expected CritCount=1, got %d", b.CritCount)
	   }
	   if b.SealedAt == nil {
		  t.Error("expected SealedAt to be set for crit batch")
	  }
  }

   func TestBuildBatchFromItems_GradedState_NoCrit(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	  items := []invdrift.DriftItem{
		   {Grade: invdrift.GradeWarn},
		  {Grade: invdrift.GradeInfo},
	  }
	   b := invdrift.BuildBatchFromItems("bid2", "dc-sg", "", items, fixedClock(now))
	   if b.State != invdrift.BatchStateGraded {
		  t.Errorf("expected graded without crit items, got %s", b.State)
	   }
	  if b.SealedAt != nil {
		  t.Error("expected SealedAt=nil for non-crit batch")
	  }
   }

   func TestScanOutput_Bundles(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	   out := invdrift.RunScan("b1", invdrift.ScanInput{
		  LedgerRows: hotShortFixture(),
		   CustRows: hotShortCustody(),
		  ThreshCfg: invdrift.DefaultThresholds,
	  }, fixedClock(now))
	  if len(out.Bundles) == 0 {
		   t.Fatal("expected bundles in scan output")
	   }
	   for _, bun := range out.Bundles {
		  if bun.Asset == "" {
			   t.Error("bundle with empty asset")
		   }
		   if bun.Ledger.Asset == "" {
			  t.Error("bundle ledger with empty asset")
		   }
	  }
  }

  func TestRunScan_MultiAsset(t *testing.T) {
	  now := time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC)
	   ledRows := append(hotShortFixture(),
		   coldExcessFixture()...)
	  custRows := append(hotShortCustody(), coldExcessCustody()...)
	   out := invdrift.RunScan("multi-batch", invdrift.ScanInput{
		  LedgerRows: ledRows,
		   CustRows:   custRows,
		   ThreshCfg: invdrift.DefaultThresholds,
	   }, fixedClock(now))
	   assetGrades := map[string]invdrift.DriftGrade{}
	   for _, it := range out.Items {
		   assetGrades[it.Asset] = it.Grade
	   }
	   if assetGrades["USDT"] != invdrift.GradeCrit {
		  t.Errorf("USDT: expected crit, got %s", assetGrades["USDT"])
	   }
	  if assetGrades["BTC"] != invdrift.GradeCrit {
		   t.Errorf("BTC: expected crit, got %s", assetGrades["BTC"])
	   }
  }

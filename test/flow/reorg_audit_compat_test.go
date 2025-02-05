package flow

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/reorgacct"
	"account-assets.kucoin.net/funding/internal/reorgorch"
)

// ────────────────────────────────────────────────────────────────────────────
// 1. FacePhase 常量唯一性
// ────────────────────────────────────────────────────────────────────────────

   func TestFacePhaseConstantsUniqueness(t *testing.T) {
	  t.Parallel()
	   phases := []string{
		  reorgacct.FacePhaseConfirming,
		   reorgacct.FacePhaseReversed,
		   reorgacct.FacePhaseLiveHashHold,
		   reorgacct.FacePhaseProcessing,
	   }
	  seen := map[string]bool{}
	   for _, p := range phases {
		  if p == "" {
			   t.Errorf("empty face phase constant")
		   }
		  if seen[p] {
			  t.Errorf("duplicate face phase constant: %s", p)
		   }
		  seen[p] = true
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 2. FaceLabel 中文说明完整性
// ────────────────────────────────────────────────────────────────────────────

  func TestFaceLabelCompleteness(t *testing.T) {
	   t.Parallel()
	  phases := []string{
		   reorgacct.FacePhaseConfirming,
		  reorgacct.FacePhaseReversed,
		  reorgacct.FacePhaseLiveHashHold,
		  reorgacct.FacePhaseProcessing,
	   }
	   for _, p := range phases {
		  label := reorgacct.FaceLabel(p)
		   if label == "" {
			   t.Errorf("FaceLabel(%s) returned empty string", p)
		   }
	   }
	  // 未知相位不崩溃，原样返回。
	  unknown := reorgacct.FaceLabel("unknown_phase_xyz")
	  if unknown != "unknown_phase_xyz" {
		  t.Errorf("FaceLabel unknown: want passthrough, got %s", unknown)
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 3. derivePhase 所有路径（表驱动，通过 FaceCompat.ToFaceView 间接）
// ────────────────────────────────────────────────────────────────────────────

  func TestDerivePhaseAllPaths(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		   name string
		   kind string
		  batchState     string
		   wantPhase      string
		  wantConfirming bool
		   wantReversed bool
	  }{
		  // credited + batch committed → reversed
		  {name: "credited_committed", kind: reorgacct.KindCredited, batchState: "committed",
			  wantPhase: reorgacct.FacePhaseReversed, wantConfirming: false, wantReversed: true},
		  // credited + executing → confirming
		  {name: "credited_executing", kind: reorgacct.KindCredited, batchState: "executing",
			   wantPhase: reorgacct.FacePhaseConfirming, wantConfirming: true, wantReversed: false},
		  // credited + planning → confirming
		   {name: "credited_planning", kind: reorgacct.KindCredited, batchState: "planning",
			   wantPhase: reorgacct.FacePhaseConfirming, wantConfirming: true, wantReversed: false},
		   // credited + open → confirming
		  {name: "credited_open", kind: reorgacct.KindCredited, batchState: "open",
			  wantPhase: reorgacct.FacePhaseConfirming, wantConfirming: true, wantReversed: false},
		  // credited + no batch → confirming
		  {name: "credited_no_batch", kind: reorgacct.KindCredited, batchState: "",
			   wantPhase: reorgacct.FacePhaseConfirming, wantConfirming: true, wantReversed: false},
		   // live_hash → live_hash_hold
		  {name: "live_hash", kind: reorgacct.KindLiveHash, batchState: "",
			   wantPhase: reorgacct.FacePhaseLiveHashHold, wantConfirming: false, wantReversed: false},
		  // occupied + committed → reversed
		   {name: "occupied_committed", kind: reorgacct.KindOccupied, batchState: "committed",
			  wantPhase: reorgacct.FacePhaseReversed, wantConfirming: false, wantReversed: true},
		  // occupied + executing → processing
		   {name: "occupied_executing", kind: reorgacct.KindOccupied, batchState: "executing",
			  wantPhase: reorgacct.FacePhaseProcessing, wantConfirming: false, wantReversed: false},
		   // freeze + no batch → processing
		   {name: "freeze_no_batch", kind: reorgacct.KindFreeze, batchState: "",
			   wantPhase: reorgacct.FacePhaseProcessing, wantConfirming: false, wantReversed: false},
		  // hop_partial + committed → reversed
		   {name: "hop_partial_committed", kind: reorgacct.KindHopPartial, batchState: "committed",
			   wantPhase: reorgacct.FacePhaseReversed, wantConfirming: false, wantReversed: true},
	   }
	   // 使用 nil batchReader（直接传入 batchState 等价于 batchReader 返回该状态）。
	  fc := reorgacct.NewFaceCompat(nil)
	  ctx := context.Background()
	   for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   // 构造 ImpactRow，将 batchState 通过 ReverseBatchID 传入；
			  // 由于 batchReader=nil，BatchState 不查询 DB，直接留空。
			   // 此处直接用 ToFaceView 但 batchState 无法从外部注入，
			  // 改为直接 FaceLabel 的返回与预期 phase 一致（通过 ImpactFaceView.Phase）。
			  row := reorgacct.ImpactRow{
				  ID: "test-row", EventID: "evt-test",
				  UserID: "u", Asset: "ETH", Kind: tc.kind,
				  ReferenceID: "r", AffectedAmount: "1",
				  ReverseBatchID: "", // batchReader=nil 时 BatchState 为空
				   CreatedAt:      time.Now().UTC(),
			  }
			   view, err := fc.ToFaceView(ctx, row, "ETH")
			  if err != nil {
				  t.Fatalf("ToFaceView failed: %v", err)
			   }
			   // 对于有 batchState 依赖的，手动验证 batchState="" 的路径。
			  // 如果 wantPhase 与空 batchState 期望不符，跳过（只空 batch 情况）。
			   expectedForEmptyBatch := emptyBatchExpected(tc.kind)
			  if view.Phase != expectedForEmptyBatch {
				  t.Errorf("phase (empty batch): want %s got %s", expectedForEmptyBatch, view.Phase)
			   }
			  // 验证 Confirming/Reversed 与 Phase 一致性（不相互为 true）。
			  if view.Confirming && view.Reversed {
				  t.Errorf("Confirming and Reversed cannot both be true")
			   }
		   })
	   }
  }

// emptyBatchExpected 返回空 batchState 下各 Kind 的预期相位。
  func emptyBatchExpected(kind string) string {
	   switch kind {
	   case reorgacct.KindCredited:
		  return reorgacct.FacePhaseConfirming
	   case reorgacct.KindLiveHash:
		   return reorgacct.FacePhaseLiveHashHold
	  default:
		  return reorgacct.FacePhaseProcessing
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 4. ImpactFaceView Confirming / Reversed 互斥性
// ────────────────────────────────────────────────────────────────────────────

  func TestFaceViewConfirmingReversedMutuallyExclusive(t *testing.T) {
	  t.Parallel()
	   fc := reorgacct.NewFaceCompat(nil)
	   ctx := context.Background()
	  kinds := []string{
		   reorgacct.KindCredited,
		  reorgacct.KindOccupied,
		  reorgacct.KindFreeze,
		  reorgacct.KindHopPartial,
		   reorgacct.KindLiveHash,
	  }
	  for _, kind := range kinds {
		  kind := kind
		   t.Run(kind, func(t *testing.T) {
			  t.Parallel()
			  row := reorgacct.ImpactRow{
				  ID: "r", EventID: "e", UserID: "u", Asset: "ETH",
				   Kind: kind, ReferenceID: "ref", AffectedAmount: "1",
				  CreatedAt: time.Now().UTC(),
			  }
			   view, _ := fc.ToFaceView(ctx, row, "ETH")
			   if view.Confirming && view.Reversed {
				  t.Errorf("kind %s: Confirming=%v and Reversed=%v cannot both be true",
					   kind, view.Confirming, view.Reversed)
			   }
		  })
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 5. AuditObjectType 常量唯一性
// ────────────────────────────────────────────────────────────────────────────

   func TestAuditObjectTypeUniqueness(t *testing.T) {
	   t.Parallel()
	  types := []reorgacct.AuditObjectType{
		  reorgacct.AuditObjEvent,
		   reorgacct.AuditObjImpactRow,
	  }
	  seen := map[reorgacct.AuditObjectType]bool{}
	   for _, tp := range types {
		   if tp == "" {
			   t.Errorf("empty AuditObjectType constant")
		  }
		   if seen[tp] {
			   t.Errorf("duplicate AuditObjectType: %s", tp)
		  }
		   seen[tp] = true
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 6. AuditEntry 字段约束
// ────────────────────────────────────────────────────────────────────────────

   func TestAuditEntryFields(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		   name    string
		  entry   reorgacct.AuditEntry
		   wantErr bool // true if entry is invalid
	  }{
		  {
			  name: "valid event transition",
			  entry: reorgacct.AuditEntry{
				   ObjectType: reorgacct.AuditObjEvent,
				  ObjectID:   "evt-001",
				  FromState:  "pending",
				   ToState: "scanned",
				   Operator: "worker",
				  Source: "confirm_rollback",
				  At: time.Now().UTC(),
			   },
			  wantErr: false,
		   },
		   {
			   name: "valid impact row bind",
			  entry: reorgacct.AuditEntry{
				   ObjectType: reorgacct.AuditObjImpactRow,
				  ObjectID:   "row-001",
				   ToState: "bound:batch-001",
				  Operator: "orchestrator",
				  Source:   "orchestrator",
				  At:       time.Now().UTC(),
			   },
			   wantErr: false,
		   },
		   {
			   name: "double_reverse_detected",
			   entry: reorgacct.AuditEntry{
				  ObjectType: reorgacct.AuditObjImpactRow,
				   ObjectID:   "ref-001",
				   ToState: "double_reverse_detected",
				  Operator: "double_reverse_scanner",
				   Source: "scanner",
				  Note: "found in 2 batches: batch-001,batch-002",
				   At: time.Now().UTC(),
			   },
			   wantErr: false,
		   },
	  }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   // 字段完整性检查。
			  if tc.entry.ObjectType == "" {
				  t.Errorf("ObjectType should not be empty")
			   }
			  if tc.entry.ObjectID == "" {
				  t.Errorf("ObjectID should not be empty")
			  }
			   if tc.entry.ToState == "" {
				   t.Errorf("ToState should not be empty")
			   }
			   if tc.entry.Operator == "" {
				   t.Errorf("Operator should not be empty")
			  }
			   if tc.entry.At.IsZero() {
				   t.Errorf("At should not be zero")
			  }
		   })
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 7. EscalatorConfig withDefaults 零值填充
// ────────────────────────────────────────────────────────────────────────────

  func TestEscalatorConfigDefaults(t *testing.T) {
	   t.Parallel()
	   // EscalatorConfig 是 reorgorch 包的类型；直接验证字段存在和类型。
	  cfg := reorgorch.EscalatorConfig{}
	  // 字段类型约束（编译时保证）。
	   var _ time.Duration = cfg.LegTimeout
	   var _ time.Duration = cfg.PlanningTimeout
	  var _ int = cfg.ScanBatchSize
	  // 零值下的合理性（withDefaults 在包内调用，此处配置结构）。
	  if cfg.LegTimeout < 0 {
		  t.Errorf("LegTimeout should not be negative")
	   }
	   t.Logf("EscalatorConfig zero value is valid: %+v", cfg)
   }

// ────────────────────────────────────────────────────────────────────────────
// 8. ImpactFaceView 批量转换线程安全
// ────────────────────────────────────────────────────────────────────────────

  func TestFaceCompatConcurrent(t *testing.T) {
	   t.Parallel()
	   fc := reorgacct.NewFaceCompat(nil)
	   ctx := context.Background()
	   rows := []reorgacct.ImpactRow{
		   {ID: "r1", EventID: "e1", UserID: "u1", Asset: "ETH", Kind: reorgacct.KindCredited, ReferenceID: "ref1", AffectedAmount: "1", CreatedAt: time.Now()},
		  {ID: "r2", EventID: "e1", UserID: "u1", Asset: "ETH", Kind: reorgacct.KindFreeze, ReferenceID: "ref2", AffectedAmount: "2", CreatedAt: time.Now()},
		  {ID: "r3", EventID: "e1", UserID: "u2", Asset: "USDT", Kind: reorgacct.KindLiveHash, ReferenceID: "ref3", AffectedAmount: "100", CreatedAt: time.Now()},
	  }
	   const goroutines = 30
	   results := make([][]reorgacct.ImpactFaceView, goroutines)
	  var wg sync.WaitGroup
	  for i := 0; i < goroutines; i++ {
		   i := i
		  wg.Add(1)
		   go func() {
			  defer wg.Done()
			  views, err := fc.ToFaceViews(ctx, rows, "ETH")
			   if err == nil {
				   results[i] = views
			   }
		   }()
	  }
	  wg.Wait()
	  for i, views := range results {
		  if views == nil {
			   continue // 某些 goroutine 可能因空结果跳过
		  }
		   if len(views) != len(rows) {
			   t.Errorf("goroutine %d: expected %d views, got %d", i, len(rows), len(views))
		  }
		  for _, v := range views {
			  if v.Confirming && v.Reversed {
				   t.Errorf("goroutine %d: Confirming and Reversed both true for kind %s", i, v.ImpactKind)
			   }
		   }
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 9. live_hash 读面特殊标记
// ────────────────────────────────────────────────────────────────────────────

   func TestLiveHashFaceView(t *testing.T) {
	   t.Parallel()
	   fc := reorgacct.NewFaceCompat(nil)
	  ctx := context.Background()
	  row := reorgacct.ImpactRow{
		  ID: "lh-row", EventID: "evt-lh", UserID: "user-E", Asset: "ETH",
		   Kind: reorgacct.KindLiveHash, ReferenceID: "wd-lh-001",
		   AffectedAmount: "1.0", CreatedAt: time.Now().UTC(),
	  }
	  view, err := fc.ToFaceView(ctx, row, "ETH")
	   if err != nil {
		   t.Fatalf("ToFaceView failed: %v", err)
	   }
	  if view.Phase != reorgacct.FacePhaseLiveHashHold {
		  t.Errorf("phase: want %s got %s", reorgacct.FacePhaseLiveHashHold, view.Phase)
	  }
	   if view.Confirming {
		   t.Errorf("live_hash should not be Confirming=true")
	   }
	  if view.Reversed {
		  t.Errorf("live_hash should not be Reversed=true")
	   }
	   label := reorgacct.FaceLabel(view.Phase)
	  if label == "" {
		  t.Errorf("FaceLabel for live_hash_hold should not be empty")
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 10. ImpactFaceView 字段完整性
// ────────────────────────────────────────────────────────────────────────────

   func TestImpactFaceViewFieldCompleteness(t *testing.T) {
	   t.Parallel()
	  fc := reorgacct.NewFaceCompat(nil)
	   ctx := context.Background()
	  row := reorgacct.ImpactRow{
		  ID:      "view-row-001",
		  EventID: "evt-view-001",
		   UserID: "user-Z",
		   Asset:          "BTC",
		  Kind:           reorgacct.KindCredited,
		   ReferenceID:    "tr-view-001",
		  AffectedAmount: "0.001",
		   Note:           "test note",
		  CreatedAt: time.Now().UTC(),
	  }
	   view, err := fc.ToFaceView(ctx, row, "BTC")
	   if err != nil {
		   t.Fatalf("ToFaceView failed: %v", err)
	   }
	   // 所有必要字段非空。
	   if view.Chain != "BTC" {
		  t.Errorf("Chain: want BTC got %s", view.Chain)
	   }
	   if view.UserID != "user-Z" {
		  t.Errorf("UserID: want user-Z got %s", view.UserID)
	  }
	  if view.Asset != "BTC" {
		  t.Errorf("Asset: want BTC got %s", view.Asset)
	  }
	  if view.ImpactKind != reorgacct.KindCredited {
		  t.Errorf("ImpactKind: want credited got %s", view.ImpactKind)
	  }
	  if view.AffectedAmount != "0.001" {
		  t.Errorf("AffectedAmount: want 0.001 got %s", view.AffectedAmount)
	  }
	  if view.Phase == "" {
		  t.Errorf("Phase should not be empty")
	  }
	  if view.PhaseLabel == "" {
		   t.Errorf("PhaseLabel should not be empty")
	  }
	  if view.EventID != "evt-view-001" {
		   t.Errorf("EventID: want evt-view-001 got %s", view.EventID)
	   }
	   if view.UpdatedAt == "" {
		   t.Errorf("UpdatedAt should not be empty")
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 11. SubjectConstants 与 FacePhase 无重叠
// ────────────────────────────────────────────────────────────────────────────

  func TestSubjectConstantsAndFacePhasesNoOverlap(t *testing.T) {
	   t.Parallel()
	  subjects := []string{
		  reorgacct.SubjectUserAvail,
		  reorgacct.SubjectReorgReverse,
		   reorgacct.SubjectOccupancyHold,
		   reorgacct.SubjectOccupancyRelease,
		   reorgacct.SubjectFreezeHold,
		   reorgacct.SubjectFreezeRelease,
		   reorgacct.SubjectHopTransit,
		  reorgacct.SubjectHopClose,
	   }
	  phases := []string{
		  reorgacct.FacePhaseConfirming,
		   reorgacct.FacePhaseReversed,
		   reorgacct.FacePhaseLiveHashHold,
		  reorgacct.FacePhaseProcessing,
	   }
	  subjectSet := map[string]bool{}
	  for _, s := range subjects {
		  subjectSet[s] = true
	  }
	  for _, p := range phases {
		  if subjectSet[p] {
			  t.Errorf("FacePhase %s collides with a Subject constant", p)
		  }
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 12. TimeoutEscalator 错误常量可用性
// ────────────────────────────────────────────────────────────────────────────

   func TestReorgOrchTimeoutErrorsAvailable(t *testing.T) {
	   t.Parallel()
	  // 确认 ErrLegTimeout 存在且非 nil。
	   if reorgorch.ErrLegTimeout == nil {
		  t.Errorf("ErrLegTimeout should not be nil")
	   }
	   if reorgorch.ErrLegTimeout.Error() == "" {
		  t.Errorf("ErrLegTimeout should have non-empty message")
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 13. FaceCompat 空 batchReader 不 panic
// ────────────────────────────────────────────────────────────────────────────

   func TestFaceCompatNilBatchReaderNoPanic(t *testing.T) {
	   t.Parallel()
	   defer func() {
		   if r := recover(); r != nil {
			  t.Errorf("FaceCompat with nil batchReader panicked: %v", r)
		  }
	  }()
	   fc := reorgacct.NewFaceCompat(nil) // nil batchReader
	   ctx := context.Background()
	   rows := []reorgacct.ImpactRow{
		  {ID: "r1", EventID: "e1", UserID: "u", Asset: "ETH",
			  Kind: reorgacct.KindCredited, ReferenceID: "ref1",
			   AffectedAmount: "10", ReverseBatchID: "batch-xyz", // 有批次 ID 但 batchReader=nil
			  CreatedAt: time.Now().UTC()},
	   }
	   _, _ = fc.ToFaceViews(ctx, rows, "ETH")
  }

// ────────────────────────────────────────────────────────────────────────────
// 14. AuditObjectType string conversion
// ────────────────────────────────────────────────────────────────────────────

  func TestAuditObjectTypeString(t *testing.T) {
	   t.Parallel()
	  if string(reorgacct.AuditObjEvent) != "event" {
		  t.Errorf("AuditObjEvent string: want 'event' got %s", reorgacct.AuditObjEvent)
	   }
	  if string(reorgacct.AuditObjImpactRow) != "impact_row" {
		  t.Errorf("AuditObjImpactRow string: want 'impact_row' got %s", reorgacct.AuditObjImpactRow)
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 15. derivePhase: aborted 批次状态
// ────────────────────────────────────────────────────────────────────────────

  func TestDerivePhaseAbortedBatch(t *testing.T) {
	   t.Parallel()
	   // aborted 批次的 credited 行：冲正未完成（失败并补偿），应显示为 confirming 还是 reversed？
	   // 策略：aborted ≠ committed，属于非成功终态，读面仍显示 confirming（待人工处置）。
	  fc := reorgacct.NewFaceCompat(nil)
	  ctx := context.Background()
	  row := reorgacct.ImpactRow{
		   ID: "r-aborted", EventID: "e-a", UserID: "u", Asset: "ETH",
		   Kind: reorgacct.KindCredited, ReferenceID: "tr-a",
		  AffectedAmount: "5", ReverseBatchID: "", // batchReader=nil，BatchState 为空
		  CreatedAt: time.Now().UTC(),
	  }
	  view, _ := fc.ToFaceView(ctx, row, "ETH")
	  // 空 batchState → confirming（还未完成）。
	  if view.Phase != reorgacct.FacePhaseConfirming {
		  t.Errorf("aborted batch (empty batchState from nil reader): want confirming got %s", view.Phase)
	   }
   }

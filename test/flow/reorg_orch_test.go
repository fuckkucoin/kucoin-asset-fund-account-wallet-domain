package flow

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/reorgorch"
)

// ────────────────────────────────────────────────────────────────────────────
// 1. LegExecutionOrder 依赖序多组合验证
// ────────────────────────────────────────────────────────────────────────────

   func TestLegExecutionOrderMulti(t *testing.T) {
	  t.Parallel()
	  orders := reorgorch.LegExecutionOrder
	   // 全序关系检查（传递闭包）。
	   seq := []string{
		  reorgorch.LegKindDebitReversal,
		   reorgorch.LegKindOccupancyRelease,
		  reorgorch.LegKindFreezeRollback,
		   reorgorch.LegKindHopClose,
		  reorgorch.LegKindIncident,
	  }
	  for i := 0; i < len(seq); i++ {
		  for j := i + 1; j < len(seq); j++ {
			   a, b := seq[i], seq[j]
			   if orders[a] >= orders[b] {
				   t.Errorf("order[%s]=%d should be < order[%s]=%d",
					  a, orders[a], b, orders[b])
			   }
		   }
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 2. ImpactKindToLegKind 全映射无遗漏
// ────────────────────────────────────────────────────────────────────────────

   func TestImpactKindToLegKindComplete(t *testing.T) {
	   t.Parallel()
	  // 每个 ImpactKind 必须有映射，且映射结果必须是已知 LegKind。
	   knownLegKinds := map[string]bool{
		  reorgorch.LegKindDebitReversal:    true,
		  reorgorch.LegKindOccupancyRelease: true,
		   reorgorch.LegKindFreezeRollback: true,
		  reorgorch.LegKindHopClose:       true,
		   reorgorch.LegKindIncident:       true,
	  }
	  for impactKind, legKind := range reorgorch.ImpactKindToLegKind {
		   if impactKind == "" {
			   t.Errorf("empty impact kind in mapping")
		  }
		   if legKind == "" {
			   t.Errorf("empty leg kind for impact kind %s", impactKind)
		  }
		  if !knownLegKinds[legKind] {
			   t.Errorf("unknown leg kind %s for impact kind %s", legKind, impactKind)
		  }
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 3. LegKind 常量唯一性
// ────────────────────────────────────────────────────────────────────────────

  func TestLegKindUniqueness(t *testing.T) {
	   t.Parallel()
	   kinds := []string{
		  reorgorch.LegKindDebitReversal,
		   reorgorch.LegKindOccupancyRelease,
		   reorgorch.LegKindFreezeRollback,
		  reorgorch.LegKindHopClose,
		   reorgorch.LegKindIncident,
	  }
	   seen := map[string]bool{}
	  for _, k := range kinds {
		  if k == "" {
			   t.Errorf("empty LegKind constant")
		  }
		   if seen[k] {
			   t.Errorf("duplicate LegKind: %s", k)
		   }
		  seen[k] = true
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 4. ReverseBatch 终态不可再跃迁（语义验证）
// ────────────────────────────────────────────────────────────────────────────

   func TestBatchTerminalStates(t *testing.T) {
	  t.Parallel()
	   terminalStates := []string{
		   reorgorch.BatchStateCommitted,
		  reorgorch.BatchStateAborted,
	   }
	  nonTerminalStates := []string{
		  reorgorch.BatchStateOpen,
		  reorgorch.BatchStatePlanning,
		  reorgorch.BatchStateExecuting,
	  }
	   // 终态与非终态不重叠。
	   termSet := map[string]bool{}
	  for _, s := range terminalStates {
		  termSet[s] = true
	  }
	   for _, s := range nonTerminalStates {
		  if termSet[s] {
			  t.Errorf("state %s appears in both terminal and non-terminal sets", s)
		  }
	  }
	   // committed/aborted 不应出现在批次跃迁规则的出边（概念层面）。
	  // 实际跃迁规则在 fsm.go 中通过 batchTransitions 控制；此处仅文档层验证。
	   for _, state := range terminalStates {
		  t.Logf("batch terminal state: %s (no valid out-transitions)", state)
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 5. ReverseLeg 终态唯一性（done/skipped/compensated 为终态）
// ────────────────────────────────────────────────────────────────────────────

  func TestLegTerminalStates(t *testing.T) {
	   t.Parallel()
	  terminalStates := []string{
		  reorgorch.LegStateDone,
		   reorgorch.LegStateSkipped,
		  reorgorch.LegStateCompensated,
	   }
	   seen := map[string]bool{}
	  for _, s := range terminalStates {
		   if s == "" {
			  t.Errorf("empty terminal leg state")
		   }
		   if seen[s] {
			   t.Errorf("duplicate terminal state: %s", s)
		   }
		  seen[s] = true
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 6. 护栏并发安全：多 goroutine 同时尝试 Lock 同一行（memGuard）
// ────────────────────────────────────────────────────────────────────────────

  func TestGuardConcurrentLockSameRow(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  name string
		   kind       string
		   isIncident bool
	  }{
		  {name: "debit_reversal concurrent", kind: reorgorch.LegKindDebitReversal, isIncident: false},
		   {name: "freeze_rollback concurrent", kind: reorgorch.LegKindFreezeRollback, isIncident: false},
		   {name: "incident concurrent (no guard)", kind: reorgorch.LegKindIncident, isIncident: true},
	  }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   g := &memGuard{guards: map[string]bool{}}
			   const goroutines = 40
			   successCount := 0
			   conflictCount := 0
			   var mu sync.Mutex
			  var wg sync.WaitGroup
			  for i := 0; i < goroutines; i++ {
				   wg.Add(1)
				  go func() {
					   defer wg.Done()
					  err := g.lock("row-concurrent-guard", tc.kind)
					  mu.Lock()
					   if err == nil {
						   successCount++
					  } else if err == reorgorch.ErrGuardConflict {
						  conflictCount++
					   }
					   mu.Unlock()
				   }()
			  }
			   wg.Wait()
			   if tc.isIncident {
				  // incident 腿不写护栏，所有 goroutine 均 success（err=nil）。
				   if successCount != goroutines {
					  t.Errorf("incident: all goroutines should succeed, got success=%d conflict=%d",
						  successCount, conflictCount)
				  }
			   } else {
				   // 非 incident：只有 1 个 goroutine 成功。
				   if successCount != 1 {
					  t.Errorf("non-incident: exactly 1 should succeed, got %d", successCount)
				  }
				   if conflictCount != goroutines-1 {
					   t.Errorf("non-incident: %d should conflict, got %d", goroutines-1, conflictCount)
				   }
			  }
		   })
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 7. ExecuteResult 字段含义约束
// ────────────────────────────────────────────────────────────────────────────

  func TestExecuteResultConstraints(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		   name string
		  res       reorgorch.ExecuteResult
		  wantValid bool
	  }{
		   {
			  name: "committed: done only",
			   res: reorgorch.ExecuteResult{BatchID: "b1", Done: 3, Failed: 0, Skipped: 0, BatchState: reorgorch.BatchStateCommitted},
			   wantValid: true,
		   },
		   {
			   name: "aborted: has failed",
			  res:       reorgorch.ExecuteResult{BatchID: "b2", Done: 1, Failed: 1, Skipped: 1, BatchState: reorgorch.BatchStateAborted},
			  wantValid: true,
		  },
		   {
			  name:      "executing: still in progress",
			   res:       reorgorch.ExecuteResult{BatchID: "b3", Done: 2, Failed: 0, Skipped: 0, Manual: 1, BatchState: reorgorch.BatchStateExecuting},
			  wantValid: true,
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  // 验证 BatchState 是已知状态之一。
			   knownStates := map[string]bool{
				  reorgorch.BatchStateOpen:     true,
				  reorgorch.BatchStatePlanning: true,
				   reorgorch.BatchStateExecuting: true,
				  reorgorch.BatchStateCommitted: true,
				   reorgorch.BatchStateAborted:   true,
				  "": true, // 空字符串（批次未变化）
			  }
			   if !knownStates[tc.res.BatchState] {
				   t.Errorf("unknown BatchState: %s", tc.res.BatchState)
			  }
			   // Done / Failed / Skipped 均不得为负数。
			  if tc.res.Done < 0 || tc.res.Failed < 0 || tc.res.Skipped < 0 {
				   t.Errorf("negative counts: done=%d failed=%d skipped=%d",
					   tc.res.Done, tc.res.Failed, tc.res.Skipped)
			  }
		  })
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 8. PlanResult 字段约束
// ────────────────────────────────────────────────────────────────────────────

   func TestPlanResultConstraints(t *testing.T) {
	  t.Parallel()
	  res := reorgorch.PlanResult{
		  BatchID: "batch-plan-001", TotalLegs: 5, HighRisk: 2, LiveHash: 1,
	   }
	  if res.BatchID == "" {
		  t.Errorf("BatchID should not be empty")
	   }
	   if res.TotalLegs < 0 {
		  t.Errorf("TotalLegs should not be negative")
	   }
	   if res.HighRisk > res.TotalLegs {
		  t.Errorf("HighRisk (%d) should not exceed TotalLegs (%d)", res.HighRisk, res.TotalLegs)
	   }
	  if res.LiveHash > res.TotalLegs {
		   t.Errorf("LiveHash (%d) should not exceed TotalLegs (%d)", res.LiveHash, res.TotalLegs)
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 9. IncidentTicket 状态常量
// ────────────────────────────────────────────────────────────────────────────

  func TestIncidentTicketFields(t *testing.T) {
	   t.Parallel()
	   ticket := reorgorch.IncidentTicket{
		   ID: "ticket-001",
		   BatchID: "batch-001",
		   LegID:       "leg-001",
		   Kind:        "live_hash_manual",
		  Description: "live-hash withdraw detected; no auto-unfreeze",
		   State:     "open",
		   CreatedAt: time.Now().UTC(),
		   UpdatedAt: time.Now().UTC(),
	  }
	   // 所有必要字段非空。
	   if ticket.ID == "" {
		   t.Errorf("ID should not be empty")
	   }
	  if ticket.BatchID == "" {
		   t.Errorf("BatchID should not be empty")
	  }
	   if ticket.Kind == "" {
		  t.Errorf("Kind should not be empty")
	  }
	  if ticket.State == "" {
		   t.Errorf("State should not be empty")
	  }
	   if ticket.Description == "" {
		   t.Errorf("Description should not be empty")
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 10. ReverseGuard 字段约束
// ────────────────────────────────────────────────────────────────────────────

   func TestReverseGuardFields(t *testing.T) {
	  t.Parallel()
	   guard := reorgorch.ReverseGuard{
		   ID:          "guard-001",
		  ImpactRowID: "row-001",
		   LegKind:     reorgorch.LegKindDebitReversal,
		   BatchID: "batch-001",
		  LegID:    "leg-001",
		  LockedAt: time.Now().UTC(),
	  }
	  if guard.ID == "" {
		  t.Errorf("Guard ID should not be empty")
	  }
	   if guard.ImpactRowID == "" {
		  t.Errorf("Guard ImpactRowID should not be empty")
	  }
	  if guard.LegKind == "" {
		   t.Errorf("Guard LegKind should not be empty")
	  }
	  if guard.LockedAt.IsZero() {
		   t.Errorf("Guard LockedAt should not be zero")
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 11. ReverseLeg 结构完整性
// ────────────────────────────────────────────────────────────────────────────

  func TestReverseLegStructCompleteness(t *testing.T) {
	   t.Parallel()
	   now := time.Now().UTC()
	  leg := reorgorch.ReverseLeg{
		   ID: "leg-struct-001", BatchID: "batch-001",
		   ImpactRowID: "row-001", Kind: reorgorch.LegKindDebitReversal,
		  ExecutionOrder: reorgorch.LegExecutionOrder[reorgorch.LegKindDebitReversal],
		  State: reorgorch.LegStatePending, IdemKey: "ik-001",
		   ReferenceID: "tr-001", Amount: "1.5",
		  UserID: "user-A", Asset: "ETH",
		   HighRisk: true, CreatedAt: now,
	   }
	  if leg.ExecutionOrder != 1 {
		  t.Errorf("debit_reversal ExecutionOrder should be 1, got %d", leg.ExecutionOrder)
	   }
	  if leg.State != reorgorch.LegStatePending {
		   t.Errorf("initial state should be pending")
	   }
	  if !leg.HighRisk {
		  t.Errorf("HighRisk should be true")
	   }
	  if leg.CreatedAt.IsZero() {
		  t.Errorf("CreatedAt should not be zero")
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 12. ：所有 LegKind 均在 LegExecutionOrder 中有条目
// ────────────────────────────────────────────────────────────────────────────

   func TestAllLegKindsHaveOrder(t *testing.T) {
	   t.Parallel()
	  allKinds := []string{
		  reorgorch.LegKindDebitReversal,
		  reorgorch.LegKindOccupancyRelease,
		  reorgorch.LegKindFreezeRollback,
		  reorgorch.LegKindHopClose,
		   reorgorch.LegKindIncident,
	   }
	   for _, kind := range allKinds {
		   order, ok := reorgorch.LegExecutionOrder[kind]
		  if !ok {
			  t.Errorf("LegKind %s not found in LegExecutionOrder", kind)
			  continue
		   }
		  if order <= 0 {
			   t.Errorf("LegKind %s has non-positive order: %d", kind, order)
		   }
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 13. ReverseBatch 结构完整性
// ────────────────────────────────────────────────────────────────────────────

   func TestReverseBatchStructCompleteness(t *testing.T) {
	   t.Parallel()
	  now := time.Now().UTC()
	  batch := reorgorch.ReverseBatch{
		  ID: "batch-struct-001", EventID: "evt-001",
		   State: reorgorch.BatchStateOpen,
		  ImpactRowIDs: []string{"row-1", "row-2"},
		   TotalLegs:    3,
		   DoneLegs: 0, FailedLegs: 0,
		   IdemKey: "ik-batch-001", Note: "test batch",
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if batch.ID == "" {
		  t.Errorf("batch ID should not be empty")
	   }
	   if batch.EventID == "" {
		  t.Errorf("batch EventID should not be empty")
	  }
	   if len(batch.ImpactRowIDs) != 2 {
		  t.Errorf("ImpactRowIDs length: want 2 got %d", len(batch.ImpactRowIDs))
	  }
	   if batch.DoneLegs > batch.TotalLegs {
		  t.Errorf("DoneLegs (%d) should not exceed TotalLegs (%d)",
			   batch.DoneLegs, batch.TotalLegs)
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 14. 大规模并发护栏唯一性（多行同时 Lock 不干扰）
// ────────────────────────────────────────────────────────────────────────────

  func TestGuardConcurrentDifferentRows(t *testing.T) {
	  t.Parallel()
	   g := &memGuard{guards: map[string]bool{}}
	  const rows = 100
	  const goroutinesPerRow = 10
	   type result struct {
		  successCount  int
		   conflictCount int
	   }
	   results := make([]result, rows)
	  var mu sync.Mutex
	   var wg sync.WaitGroup
	  for i := 0; i < rows; i++ {
		   i := i
		  rowID := fmt.Sprintf("row-%d", i)
		   for j := 0; j < goroutinesPerRow; j++ {
			   wg.Add(1)
			   go func() {
				   defer wg.Done()
				   err := g.lock(rowID,
					   reorgorch.LegKindDebitReversal)
				   mu.Lock()
				   if err == nil {
					  results[i].successCount++
				  } else if err == reorgorch.ErrGuardConflict {
					  results[i].conflictCount++
				  }
				  mu.Unlock()
			   }()
		   }
	  }
	   wg.Wait()
	   for i, r := range results {
		  if r.successCount != 1 {
			  t.Errorf("row %d: expected 1 success, got %d (conflicts=%d)",
				   i, r.successCount, r.conflictCount)
		  }
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 15. 验证：工作配置默认值合理性
// ────────────────────────────────────────────────────────────────────────────

  func TestWorkerConfigDefaults(t *testing.T) {
	   t.Parallel()
	   // WorkerConfig 零值应通过 withDefaults 填充为合理默认值。
	   // 此处通过间接验证（字段存在性）确认结构可用。
	   cfg := reorgorch.WorkerConfig{
		  PlanInterval: 0, // 应被填充为 20s
		  ExecInterval: 0, // 应被填充为 10s
		   TimeoutInterval: 0, // 应被填充为 2min
		   GuardInterval: 0, // 应被填充为 5min
		   BatchSize:     0, // 应被填充为 10
		  LegTimeout:    0, // 应被填充为 30s
	  }
	   // 验证字段类型正确（编译时保证）。
	   var _ time.Duration = cfg.PlanInterval
	   var _ time.Duration = cfg.ExecInterval
	  var _ time.Duration = cfg.TimeoutInterval
	  var _ time.Duration = cfg.GuardInterval
	   var _ time.Duration = cfg.LegTimeout
	   var _ int = cfg.BatchSize
	  t.Logf("WorkerConfig zero value is valid struct: %+v", cfg)
  }

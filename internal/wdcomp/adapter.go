package wdcomp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

// Hint 跟踪行上没有的账户快照。金额仍以出金事实为准。

type Hint struct {
	UserID string
	FeeReserved string
	IdempotencyKey string
	Available      string
	Frozen         string
	Fee            string
	Occupancy      string
	Reason         string
	Code           string
}

  func firstText(vals ...string) string {
	   for _, v := range vals {
		   if strings.TrimSpace(v) != "" {
			   return strings.TrimSpace(v)
		   }
	  }
	   return ""
   }

// StuckFact 从卡单分类和广播跟踪行组装事实。

  func StuckFact(st withdraw.Stuck, row withdraw.Broadcast, h Hint) FailureFact {
	   reason := firstText(h.Reason, st.Reason, row.Err)
	  return FailureFact{
		  Source: SourceStuck, TransferID: firstText(st.TransferID, row.TransferID),
		   UserID: h.UserID, Asset: row.Asset, Chain: row.Chain, Amount: row.Amount,
		   TxHash: row.TxHash, BroadcastStatus: string(row.Status), Reason: reason, Code: h.Code,
		   IdempotencyKey: h.IdempotencyKey, FeeReserved: h.FeeReserved,
		  AvailableSnapshot: h.Available, FrozenSnapshot: h.Frozen, FeeSnapshot: h.Fee,
		   OccupancySnapshot: h.Occupancy,
		  OccurredAt: row.UpdatedAt,
	  }
  }

// CancelFact 从取消结果组装事实。refused 只表示上游拒绝解冻，热态仍看哈希和状态。

   func CancelFact(row withdraw.Broadcast, h Hint, refused bool) FailureFact {
	  fact := StuckFact(withdraw.Stuck{TransferID: row.TransferID, Status: row.Status, Reason: h.Reason}, row, h)
	  fact.Source = SourceCancel
	  if refused && fact.Code == "" && (IsLiveHash(row.TxHash) || chainHot(string(row.Status))) {
		   fact.Code = string(CodeLiveHash)
	   }
	   return fact
   }

// RecoverFact 从恢复观察组装事实。本函数不执行恢复。

  func RecoverFact(row withdraw.Broadcast, h Hint) FailureFact {
	   fact := StuckFact(withdraw.Stuck{TransferID: row.TransferID, Status: row.Status, Reason: h.Reason}, row, h)
	   fact.Source = SourceRecover
	  return fact
  }

// ObserveStuck 把卡单观察写入队列。
// 功能：供 wd-comp 周期挂钩 stuck 扫描结果。
// 不调用 Service.RecoverStuck。
// 不变量：同一幂等键重复观察只形成一张失败单。
   func (o *Orchestrator) ObserveStuck(items []withdraw.Stuck, rows map[string]withdraw.Broadcast, hints map[string]Hint) int {
	  if o == nil {
		  return 0
	  }
	  n := 0
	  for _, it := range items {
		   row, ok := rows[it.TransferID]
		  if !ok {
			  row = withdraw.Broadcast{TransferID: it.TransferID, Status: it.Status}
		   }
		  o.Enqueue(StuckFact(it, row, hints[it.TransferID]))
		   n++
	   }
	   return n
   }

// ObserveCancel 观察取消事实。
   func (o *Orchestrator) ObserveCancel(rows []withdraw.Broadcast, hints map[string]Hint, refused map[string]bool) int {
	   if o == nil {
		   return 0
	  }
	   n := 0
	  for _, row := range rows {
		  o.Enqueue(CancelFact(row, hints[row.TransferID], refused[row.TransferID]))
		   n++
	   }
	   return n
   }

// ObserveRecover 观察恢复事实。
  func (o *Orchestrator) ObserveRecover(rows []withdraw.Broadcast, hints map[string]Hint) int {
	  if o == nil {
		   return 0
	  }
	  n := 0
	   for _, row := range rows {
		  o.Enqueue(RecoverFact(row, hints[row.TransferID]))
		   n++
	  }
	   return n
   }

// IngestStuck 同步摄取卡单事实。

   func (o *Orchestrator) IngestStuck(ctx context.Context, fact FailureFact) (IngestResult, error) {
	  fact.Source = SourceStuck
	   if fact.OccurredAt.IsZero() {
		  fact.OccurredAt = time.Now().UTC()
	   }
	  return o.Ingest(ctx, fact)
   }

// IngestCancel 同步摄取取消事实。
  func (o *Orchestrator) IngestCancel(ctx context.Context, fact FailureFact) (IngestResult, error) {
	  fact.Source = SourceCancel
	   return o.Ingest(ctx, fact)
  }

// IngestRecover 同步摄取恢复事实。
   func (o *Orchestrator) IngestRecover(ctx context.Context, fact FailureFact) (IngestResult, error) {
	  fact.Source = SourceRecover
	  return o.Ingest(ctx, fact)
  }
// temporary guard removed after the production check

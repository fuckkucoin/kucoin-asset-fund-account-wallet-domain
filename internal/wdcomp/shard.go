package wdcomp

import (
	"context"
	"strings"
	"time"
)

// ShardFailEvent 分片失败事实。成功路径的在途占用不从这里回写。
type ShardFailEvent struct {
	PlanID string `json:"shard_plan_id"`
	TransferID string `json:"transfer_id"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Chain string `json:"chain,omitempty"`
	ShardID    string `json:"shard_id"`
	ShardSeq   int    `json:"shard_seq,omitempty"`
	ShardCount int    `json:"shard_count,omitempty"`
	TotalAmount  string `json:"total_amount"`
	FilledAmount string `json:"filled_amount,omitempty"`
	FailedAmount string `json:"failed_amount,omitempty"`
	InFlightAmount string `json:"in_flight_amount,omitempty"`
	FeeReserved    string   `json:"fee_reserved,omitempty"`
	TxHash         string   `json:"tx_hash,omitempty"`
	Frozen     string    `json:"frozen_snapshot,omitempty"`
	Fee        string    `json:"fee_snapshot,omitempty"`
	Occupancy  string    `json:"occupancy_snapshot,omitempty"`
}

// Fact 转成编排器事实。已成交哈希只留在备注里。
   func (e ShardFailEvent) Fact() FailureFact {
	  note := strings.Join(e.FilledTxHashes, ",")
	  return FailureFact{
		  Source: SourceShard, TransferID: e.TransferID, UserID: e.UserID, Asset: e.Asset, Chain: e.Chain,
		  Amount: e.TotalAmount, FilledAmount: e.FilledAmount, FailedAmount: e.FailedAmount,
		   InFlightAmount: e.InFlightAmount, FeeReserved: e.FeeReserved, TxHash: e.TxHash, FilledTxNote: note,
		   ShardPlanID: e.PlanID, ShardID: e.ShardID, ShardSeq: e.ShardSeq, ShardCount: e.ShardCount,
		  Reason: e.Reason, IdempotencyKey: e.IdempotencyKey, OccurredAt: e.OccurredAt,
		   AvailableSnapshot: e.Available,
		  FrozenSnapshot:    e.Frozen, FeeSnapshot: e.Fee, OccupancySnapshot: e.Occupancy,
	   }
  }

// ShardFailureSource 分片包可选的拉取口。未挂接时工人跳过。

type ShardFailureSource interface {
	Pull(ctx context.Context, limit int) ([]ShardFailEvent, error)
}

// BindShardSource 挂上分片失败拉取口。不改变分片成功记账。

  func (o *Orchestrator) BindShardSource(src ShardFailureSource) {
	  if o == nil {
		   return
	  }
	   o.mu.Lock()
	  o.shardSrc = src
	  o.mu.Unlock()
  }

// IngestShard 同步消费一条分片失败。

  func (o *Orchestrator) IngestShard(ctx context.Context, ev ShardFailEvent) (IngestResult, error) {
	   if ev.OccurredAt.IsZero() {
		  ev.OccurredAt = time.Now().UTC()
	   }
	  return o.Ingest(ctx, ev.Fact())
   }

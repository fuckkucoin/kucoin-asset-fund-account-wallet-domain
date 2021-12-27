package wdbatch

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// 批次残留扩展状态常量（只用于 wdpartial 层显示，不写入 wdbatch.Batch.Status）。
const (
	StatusResidueOpen = "residue_open" // 检测到残留，wdpartial 层等待处置
	StatusResuming    = "resuming"     // 已持有 Resume 世代锁，正在续提
	StatusClosed    = "closed"    // 全部残留处置完成（wdpartial 层收口）
	StatusEscalated = "escalated" // 部分残留升级挂账（wdpartial 层收口）
)

// BatchResidueAdapter 实现 wdpartial.BatchReader。
// 通过 wdbatch.Service.Get 获取批次，将 Item 转换为 wdpartial.BatchItemView，
// 供 wdpartial.Detector 只读消费。
type BatchResidueAdapter struct {
	svc *Service
}

// NewBatchResidueAdapter 构造只读适配器。

   func NewBatchResidueAdapter(svc *Service) *BatchResidueAdapter {
	   return &BatchResidueAdapter{svc: svc}
   }

// BatchItems 实现 wdpartial.BatchReader 接口。

  func (a *BatchResidueAdapter) BatchItems(batchID string) (
	  items []wdpartial.BatchItemView,
	  requested, submitted, skipped, failed string,
	   err error,
  ) {
	  if a == nil || a.svc == nil {
		  return nil, "", "", "", "", wdpartial.ErrUnavailable
	   }
	   batch, berr := a.svc.Get(batchID)
	  if berr != nil {
		  return nil, "", "", "", "", fmt.Errorf("%w: %v", wdpartial.ErrBatchNotFound, berr)
	   }
	   views := make([]wdpartial.BatchItemView,
		  len(batch.Items))
	  for i, it := range batch.Items {
		  views[i] = wdpartial.BatchItemView{
			   Seq: it.Seq,
			  UserID: it.UserID,
			   Asset: it.Asset,
			  Amount: it.Amount,
			  Chain:          it.Chain,
			   ToAddress:      it.ToAddress,
			  IdempotencyKey: it.IdempotencyKey,
			  Status: string(it.Status),
			  Reason: it.Reason,
			   LedgerID: it.LedgerID,
		   }
	  }
	   return views, batch.Requested, batch.Submitted, batch.Skipped, batch.Failed, nil
   }

// SubmittedBatchIDs 实现 wdpartial.BatchLister 接口（返回 submitted 状态批次 ID）。
  func (a *BatchResidueAdapter) SubmittedBatchIDs(limit int) ([]string, error) {
	   if a == nil || a.svc == nil {
		   return nil, wdpartial.ErrUnavailable
	   }
	  batches := a.svc.List(limit)
	  out := make([]string, 0, len(batches))
	  for _, b := range batches {
		  if b.Status == StatusSubmitted {
			  out = append(out, b.ID)
		   }
	  }
	  return out, nil
  }

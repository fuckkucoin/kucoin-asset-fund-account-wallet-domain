package recon

import (
	"context"
	"strings"
)

// CloseSync 结案并同步用户漂移挡冻标记。
// 关单后自动清/留挡冻，避免内存与 KV 漂移。四眼结案仍走 CloseSafe；写失败返回已结工单与错误。store 为空时只结案不持久化挡冻，跨进程读面可能滞后。
  func (w *Workflow) CloseSync(ctx context.Context, id, actor string, epoch int64, token string, store *DriftFreezeStore) (*Ticket, DriftFreeze, error) {
	  t, err := w.CloseSafe(id, actor, epoch, token)
	   if err != nil {
		  return t, DriftFreeze{}, err
	   }
	   uid := ""
	   if t != nil {
		  uid = t.UserID
	  }
	  df,
		   serr := w.SyncDriftFreeze(ctx, store, uid)
	   if serr != nil {
		   return t, df, serr
	  }
	   return t, df, nil
   }

// RejectSync 拒单并同步漂移挡冻。

   func (w *Workflow) RejectSync(ctx context.Context, id, actor string, epoch int64, token string, store *DriftFreezeStore) (*Ticket, DriftFreeze, error) {
	  if err := actorOK(actor); err != nil {
		  return nil, DriftFreeze{}, err
	   }
	   t,
		  err := w.book.Reject(id, actor, epoch, token)
	   if err != nil {
		   return t, DriftFreeze{}, err
	  }
	  uid := ""
	  if t != nil {
		   uid = t.UserID
	   }
	  df, serr := w.SyncDriftFreeze(ctx, store, uid)
	   if serr != nil {
		  return t, df, serr
	  }
	   return t, df, nil
  }

// SyncOpenUsers 扫描未关工单用户并刷新挡冻标记；返回已同步用户数。
// 批量一致内存未关数与 KV 挡冻。同一用户只同步一次；写失败中断并返回已处理数。大簿全量扫描成本高，宜在选主窗口低频调用。
  func (w *Workflow) SyncOpenUsers(ctx context.Context, store *DriftFreezeStore) (int, error) {
	  if w == nil || w.book == nil {
		  return 0, nil
	  }
	  rows := w.book.List("", 500)
	  seen := map[string]struct{}{}
	   n := 0
	   for _, t := range rows {
		   if t.State != StateOpen && t.State != StateAcked {
			   continue
		  }
		  uid := strings.TrimSpace(t.UserID)
		  if uid == "" {
			   continue
		   }
		  if _, ok := seen[uid]; ok {
			  continue
		   }
		  seen[uid] = struct{}{}
		   if _, err := w.SyncDriftFreeze(ctx, store, uid); err != nil {
			  return n, err
		   }
		  n++
	   }
	   return n, nil
   }

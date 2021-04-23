package egressleasegate

import (
	"context"
	"strings"
	"github.com/google/uuid"
)

// BatchRequest 一批出金广播共用一次放行判定。任一条失败则整批不进入成功路径。

type BatchRequest struct {
	BatchID string
	Actor   string
	Items []AssertRequest
}

// BatchResult 逐条结果。Accepted 仅当每一条都已消费。
type BatchResult struct {
	BatchID  string
	Accepted bool
	Items []AssertResult
}

// BroadcastBatch 先复验每一条，全部通过后才消费。失败条目不消费通过条目（fail-closed）。
   func (e *Engine) BroadcastBatch(ctx context.Context, req BatchRequest) (*BatchResult, error) {
	  if err := ctx.Err(); err != nil {
		   return nil, err
	  }
	   if e == nil || e.store == nil || e.desk == nil {
		   return nil, ErrUnavailable
	  }
	   if len(req.Items) == 0 {
		  return nil, ErrBatchRejected
	   }
	  if len(req.Items) > e.cfg.MaxBatchItems {
		  return nil, ErrBatchLimit
	  }
	  batchID := strings.TrimSpace(req.BatchID)
	   if batchID == "" {
		   batchID = uuid.NewString()
	  }
	  type pre struct {
		  req BroadcastRequest
		  ok     bool
		   reason string
	   }
	   rows := make([]pre, len(req.Items))
	   seenRef := map[string]struct{}{}
	   allOK := true
	  for i, item := range req.Items {
		  item.Actor = req.Actor
		   item.BatchID = batchID
		   item.BroadcastRef = strings.TrimSpace(item.BroadcastRef)
		  rows[i].req = item
		  if item.BroadcastRef == "" {
			  rows[i].reason = "scope_mismatch"
			   allOK = false
			  continue
		  }
		   if _, dup := seenRef[item.BroadcastRef]; dup {
			   rows[i].reason = "scope_mismatch"
			   allOK = false
			   continue
		   }
		  seenRef[item.BroadcastRef] = struct{}{}
		   if strings.TrimSpace(item.LeaseID) == "" {
			  rows[i].reason = string(DriftMissing)
			  allOK = false
			  continue
		  }
		  lease, err := e.store.GetLeaseByID(ctx, strings.TrimSpace(item.LeaseID))
		  if err != nil {
			   rows[i].reason = string(DriftMissing)
			   allOK = false
			   continue
		  }
		  if item.Chain != "" && lease.Chain != strings.ToUpper(strings.TrimSpace(item.Chain)) {
			   rows[i].reason = "scope_mismatch"
			  allOK = false
			   continue
		  }
		   if item.Asset != "" && lease.Asset != strings.ToUpper(strings.TrimSpace(item.Asset)) {
			   rows[i].reason = "scope_mismatch"
			   allOK = false
			   continue
		  }
		   if item.Amount != "" && lease.Amount != "" && strings.TrimSpace(item.Amount) != lease.Amount {
			  rows[i].reason = "scope_mismatch"
			  allOK = false
			   continue
		  }
		   if lease.State == LeaseStateConsumed && lease.BroadcastRef == item.BroadcastRef {
			   rows[i].ok = true
			  continue
		  }
		   if !lease.State.Open() || lease.Expired(e.now()) {
			   if lease.Expired(e.now()) || lease.State == LeaseStateExpired {
				   rows[i].reason = string(DriftExpired)
			   } else {
				   rows[i].reason = string(lease.State)
			   }
			  allOK = false
			  continue
		  }
		  live, lerr := e.liveView(lease, item.FromTank || lease.Source == "tank", item.HotAvail)
		   if lerr != nil {
			  return nil, lerr
		   }
		  if drift, dt, _ := e.detectDrift(lease, live); drift {
			   rows[i].reason = string(dt)
			  allOK = false
			  continue
		  }
		  rows[i].ok = true
	  }
	   out := &BatchResult{BatchID: batchID, Accepted: allOK, Items: make([]AssertResult, len(rows))}
	  if !allOK {
		   for i := range rows {
			   if rows[i].ok {
				  out.Items[i] = BroadcastResult{Accepted: false, Allowed: false, RejectReason: "batch_held"}
				  continue
			   }
			  res, err := e.BroadcastAssert(ctx, rows[i].req)
			   if res == nil {
				   res = &BroadcastResult{Accepted: false, Allowed: false, RejectReason: rows[i].reason}
			   }
			  res.Accepted = false
			  res.Allowed = false
			  if res.RejectReason == "" {
				   res.RejectReason = rows[i].reason
			  }
			  out.Items[i] = *res
			  _ = err
		   }
		   e.metrics.Rejected.Add(1)
		   return out, ErrBatchRejected
	   }
	  for i := range rows {
		   res, err := e.BroadcastAssert(ctx, rows[i].req)
		  if err != nil {
			   out.Accepted = false
			   if res == nil {
				  res = &BroadcastResult{RejectReason: err.Error()}
			  }
			  out.Items[i] = *res
			   return out, err
		  }
		  out.Items[i] = *res
	   }
	   return out, nil
   }

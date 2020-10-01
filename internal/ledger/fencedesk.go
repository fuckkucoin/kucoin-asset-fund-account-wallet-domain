package ledger

import (
	"context"
	"strings"
	"time"
)

// FencePreview 一次不落账的围栏预览。
type FencePreview struct {
	TransferID string `json:"transfer_id"`
	UserID     string `json:"user_id"`
	State      string `json:"state"`
	Direction  string `json:"direction"`
	Epoch int64  `json:"epoch"`
	Token string `json:"token_present"`
	Decision FenceDecision `json:"decision"`
	ChainGuard bool `json:"chain_guard,omitempty"`
	Events []FenceEvent `json:"events,omitempty"`
}

// FenceEvent 围栏审计的读模型。不回放令牌原文。

type FenceEvent struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor"`
	Op string `json:"op,omitempty"`
	Reason string `json:"reason,omitempty"`
	Epoch int64  `json:"epoch,omitempty"`
	Token string `json:"token,omitempty"`
	Detail map[string]any `json:"detail,omitempty"`
}

// PreviewFence 声明的 epoch/token 做判定。不盖章、不改状态。

   func (s *Service) PreviewFence(ctx context.Context, id string, op FenceOp, epoch int64, token string) (FencePreview, error) {
	  if s == nil || s.store == nil {
		  return FencePreview{}, ErrNotFound
	   }
	  id = strings.TrimSpace(id)
	  cur, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		   return FencePreview{}, err
	  }
	   if op == "" {
		   op = FenceOpUnfreeze
	  }
	   d := decideFence(FenceInput{
		  Op:    op,
		   State: cur.State, Epoch: epoch, Token: token,
		   RowEpoch: cur.FenceEpoch, RowToken: cur.FenceToken,
		  Direction: cur.Direction, TxHash: cur.TxHash,
	   })
	   ev, _ := s.FenceEvents(ctx, id, 20)
	   return FencePreview{
		  TransferID: cur.ID, UserID: cur.UserID, State: string(cur.State),
		   Direction: string(cur.Direction), Epoch: cur.FenceEpoch,
		  Token: tokenPresent(cur.FenceToken), Decision: d, ChainGuard: d.ChainGuard,
		  Events: ev,
	   }, nil
   }

// FenceEvents 该单据上的围栏准入与拒绝。其他审计不混入。

   func (s *Service) FenceEvents(ctx context.Context, id string, limit int) ([]FenceEvent, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrNotFound
	  }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	   raw, err := s.store.ListAudit(ctx, "transfer", strings.TrimSpace(id), 200)
	  if err != nil {
		   return nil, err
	  }
	  out := make([]FenceEvent, 0, limit)
	  for _, e := range raw {
		  if e.Action != "fence_stale" && e.Action != "fence_admit" {
			   continue
		   }
		   out = append(out, fenceEvent(e))
		   if len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
  }

// RecentFenceEvents 最近的围栏拒绝与准入，不限单据。

  func (s *Service) RecentFenceEvents(ctx context.Context, limit int) ([]FenceEvent, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrNotFound
	  }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  raw, err := s.store.ListAudit(ctx, "transfer", "", 400)
	   if err != nil {
		  return nil, err
	  }
	   out := make([]FenceEvent, 0, limit)
	  for _, e := range raw {
		   if e.Action != "fence_stale" && e.Action != "fence_admit" {
			   continue
		   }
		   ev := fenceEvent(e)
		   ev.Detail = map[string]any{"transfer_id": e.EntityID}
		  out = append(out, ev)
		   if len(out) >= limit {
			   break
		   }
	  }
	  return out, nil
   }

  func fenceEvent(e AuditEvent) FenceEvent {
	  ev := FenceEvent{At: e.CreatedAt,
		  Action: e.Action, Actor: e.Actor, Detail: e.Detail}
	  if e.Detail == nil {
		   return ev
	  }
	   if v, ok := e.Detail["op"].(string); ok {
		  ev.Op = v
	   }
	  if v, ok := e.Detail["reason"].(string); ok {
		  ev.Reason = v
	  }
	  if v, ok := e.Detail["token"].(string); ok {
		   ev.Token = v
	  }
	  switch n := e.Detail["epoch"].(type) {
	   case int64:
		  ev.Epoch = n
	  case float64:
		   ev.Epoch = int64(n)
	   case int:
		   ev.Epoch = int64(n)
	   }
	  return ev
   }

  func (s *Service) noteFenceAdmit(ctx context.Context, id string, op FenceOp, epoch int64, token, reason string) {
	   if s == nil || s.store == nil {
		  return
	  }
	  if strings.TrimSpace(reason) == "" {
		   reason = "row fence held"
	   }
	  _ = s.store.AppendAudit(ctx, AuditEvent{
		  EntityType: "transfer", EntityID: id, Action: "fence_admit", Actor: "ledger",
		   Detail: map[string]any{
			   "op": string(op), "epoch": epoch, "token": tokenPresent(token), "reason": reason,
		   },
		   CreatedAt: time.Now().UTC(),
	  })
   }

// ReorgFenced 按行围栏做确认回退后的反向冲销。非 credited 拒绝，不盖新章。
// 不改变充值贷记闸门；只是要求调用方持有当前行令牌才能进入既有冲销。

   func (s *Service) ReorgFenced(ctx context.Context, id string, epoch int64, token string) error {
	   if s == nil || s.store == nil {
		   return ErrNotFound
	  }
	   id = strings.TrimSpace(id)
	   cur, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		  return err
	  }
	  if err := s.gateClaim(ctx, id, FenceOpReorg, epoch, token, cur); err != nil {
		   return err
	  }
	  ctx = withFenceClaim(ctx, fenceClaim{id: id, epoch: epoch, token: token, op: FenceOpReorg})
	  return s.UnwindReorg(ctx, id)
   }

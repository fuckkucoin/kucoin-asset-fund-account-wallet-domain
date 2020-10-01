package ledger

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// FenceOp 围栏校验所保护的写动作。
type FenceOp string

const (
	FenceOpAdvance  FenceOp = "advance"
	FenceOpUnfreeze FenceOp = "unfreeze"
)

type fenceClaim struct {
	id    string
	epoch int64
	token string
	op    FenceOp
}

type fenceClaimKey struct{}

  func withFenceClaim(ctx context.Context, c fenceClaim) context.Context {
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  return context.WithValue(ctx, fenceClaimKey{}, c)
   }

  func fenceClaimFrom(ctx context.Context) (fenceClaim, bool) {
	  if ctx == nil {
		   return fenceClaim{}, false
	   }
	   c, ok := ctx.Value(fenceClaimKey{}).(fenceClaim)
	  return c, ok && c.id != ""
  }

  func submittedTransfer(st State) bool {
	   return st == StateChainSubmitted || st == StateConfirmed
  }

// admitFenceClaim 用调用方声明的 epoch/token 进入写会话，不递增围栏。
// 令牌不一致或租约已丢时拒绝，避免后到的 worker 覆盖先提交的单。
   func (s *Service) admitFenceClaim(ctx context.Context, id string, lease Guard, claim fenceClaim) (context.Context, error) {
	   if s == nil || s.store == nil {
		   return ctx, ErrNotFound
	   }
	  if lease != nil {
		  if err := lease.Touch(ctx); err != nil {
			   s.rejectFence(ctx, id, claim.op, claim.epoch, claim.token, "lease lost")
			   return ctx, ErrStaleFence
		  }
	   }
	   cur, err := s.store.GetTransfer(ctx, id)
	  if err != nil {
		  return ctx, err
	   }
	  if claim.epoch <= 0 || strings.TrimSpace(claim.token) == "" ||
		   cur.FenceEpoch != claim.epoch || cur.FenceToken != claim.token {
		   s.rejectFence(ctx, id, claim.op, claim.epoch, claim.token, "token mismatch")
		   return ctx, ErrStaleFence
	   }
	   reason := "row fence held"
	   if lease != nil {
		  if leaseTok := strings.TrimSpace(lease.Token()); leaseTok != "" && leaseTok != claim.token {
			   reason = "lease rotated, row fence held"
		  }
	   }
	   s.noteFenceAdmit(ctx, id, claim.op, claim.epoch, claim.token, reason)
	   return withFence(ctx, fenceSession{id: id, token: claim.token, epoch: claim.epoch}), nil
   }

  func (s *Service) rejectFence(ctx context.Context, id string, op FenceOp, epoch int64, token, reason string) {
	  if s == nil || s.store == nil {
		   return
	  }
	  metrics.FenceRejected.Inc()
	   _ = s.store.AppendAudit(ctx, AuditEvent{
		   EntityType: "transfer",
		   EntityID:   id,
		  Action:     "fence_stale",
		  Actor: "ledger",
		  Detail: map[string]any{
			   "op":     string(op),
			  "epoch":  epoch,
			   "reason": reason,
			  "token": tokenPresent(token),
		   },
		  CreatedAt: time.Now().UTC(),
	  })
  }

   func tokenPresent(token string) string {
	  if strings.TrimSpace(token) == "" {
		   return "absent"
	   }
	  return "present"
   }

// UnfreezeFenced 按行围栏解冻。
// 令牌必须与行上当前值一致；陈旧令牌失败即关并写 fence_stale，不回滚冻结。
// 链上 hash 仍存活时的禁止解冻仍由原路径执行，本函数不放宽该条件。
   func (s *Service) UnfreezeFenced(ctx context.Context, id, reason string, epoch int64, token string) error {
	  if s == nil || s.store == nil {
		   return ErrNotFound
	   }
	  id = strings.TrimSpace(id)
	  if id == "" || epoch <= 0 || strings.TrimSpace(token) == "" {
		  s.rejectFence(ctx, id, FenceOpUnfreeze, epoch, token, "claim required")
		  return ErrStaleFence
	  }
	  cur,
		  err := s.store.GetTransfer(ctx, id)
	  if err != nil {
		  return err
	  }
	  if err := s.gateClaim(ctx, id, FenceOpUnfreeze, epoch, token, cur); err != nil {
		   return err
	   }
	   if strings.TrimSpace(reason) == "" {
		   reason = "unfreeze"
	   }
	   ctx = withFenceClaim(ctx, fenceClaim{id: id, epoch: epoch, token: token, op: FenceOpUnfreeze})
	   return s.FailAndRollback(ctx, id, reason)
   }

// AdvanceSubmitted 推进已提交单（chain_submitted / confirmed）。
// 必须携带当前围栏。未提交的单不走本入口，避免把冻单误推进到入账。

   func (s *Service) AdvanceSubmitted(ctx context.Context, id string, epoch int64, token string) (*Transfer, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrNotFound
	  }
	   id = strings.TrimSpace(id)
	  if id == "" || epoch <= 0 || strings.TrimSpace(token) == "" {
		  s.rejectFence(ctx, id, FenceOpAdvance, epoch, token, "claim required")
		  return nil, ErrStaleFence
	   }
	  cur,
		  err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		   return nil, err
	   }
	   if err := s.gateClaim(ctx, id, FenceOpAdvance, epoch, token, cur); err != nil {
		   if err == ErrIllegalTransition {
			  return cur, err
		   }
		  return cur, err
	   }
	   ctx = withFenceClaim(ctx, fenceClaim{id: id, epoch: epoch, token: token, op: FenceOpAdvance})
	  if err := s.ConfirmAndCredit(ctx, id); err != nil {
		   got, _ := s.store.GetTransfer(ctx, id)
		   if got == nil {
			  got = cur
		  }
		   return got, err
	  }
	  return s.store.GetTransfer(ctx, id)
  }

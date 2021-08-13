package keyrotgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	defaultBindTTL = 120 * time.Second
	maxBindTTL = 600 * time.Second
)

// Binder 在途签名绑定管理器。
type Binder struct {
	store Store
	clock func() time.Time
}

// NewBinder 构造 Binder。

   func NewBinder(store Store) *Binder {
	   if store == nil {
		   store = NewMemStore()
	   }
	   return &Binder{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟（用）。
  func (b *Binder) WithClock(fn func() time.Time) *Binder {
	  if fn != nil {
		  b.clock = fn
	  }
	  return b
   }

   // BindSign 发起签名/解锁时绑定源槽指纹。
  // slotRetired：调用方需传入当前槽状态是否已 Retired；若已 Retired 则拒绝并返回 ErrSlotRetiredReject。
  // 同 operation_id 重复调用返回已有绑定（幂等）。

  func (b *Binder) BindSign(ctx context.Context, req BindSignReq, slotRetired bool) (*SignBind, error) {
	   if b == nil || b.store == nil {
		  return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.SrcSlotID) == "" {
		  return nil, ErrSlotIDRequired
	   }
	  if strings.TrimSpace(req.Fingerprint) == "" {
		   return nil, ErrFingerprintRequired
	  }
	  if strings.TrimSpace(req.OperationID) == "" {
		  return nil, ErrOperationIDRequired
	  }
	   kind := strings.TrimSpace(req.OperationKind)
	  if kind != "sign" && kind != "unlock" {
		   return nil, ErrInvalidKind
	  }
	   if strings.TrimSpace(req.BoundBy) == "" {
		   return nil, ErrActorRequired
	   }
	  // 槽已 Retired：直接拒绝，触发漂移检测
	  if slotRetired {
		   return nil, ErrSlotRetiredReject
	  }
	  // 幂等：同 operation_id 已有绑定则返回
	   existing, err := b.store.GetSignBindByOperation(ctx, req.OperationID)
	  if err == nil {
		   return existing, nil
	   }
	   if err != ErrSignBindNotFound {
		  return nil, err
	   }
	   ttl := defaultBindTTL
	  if req.TTLSeconds > 0 {
		  d := time.Duration(req.TTLSeconds) * time.Second
		   if d > maxBindTTL {
			  d = maxBindTTL
		  }
		  ttl = d
	   }
	   now := b.clock()
	   bind := &SignBind{
		  SignBindID: uuid.NewString(),
		   SrcSlotID:  strings.TrimSpace(req.SrcSlotID),
		  Fingerprint:   strings.TrimSpace(req.Fingerprint),
		   OperationID:   strings.TrimSpace(req.OperationID),
		  OperationKind: kind,
		   BoundBy: strings.TrimSpace(req.BoundBy),
		  State: SignBindActive,
		   BoundAt:   now,
		   ExpiresAt: now.Add(ttl),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := b.store.SaveSignBind(ctx, bind); err != nil {
		   return nil, err
	  }
	   MetricBindActive.Inc()
	   return bind, nil
  }

// ReleaseSign 签名/解锁完成后主动释放绑定。幂等：已释放则直接返回。
   func (b *Binder) ReleaseSign(ctx context.Context, req ReleaseSignReq) (*SignBind, error) {
	   if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.SignBindID) == "" {
		   return nil, ErrSignBindNotFound
	   }
	  bind, err := b.store.GetSignBind(ctx, req.SignBindID)
	  if err != nil {
		   return nil, err
	  }
	   if SignBindIsTerminal(bind.State) {
		   return bind, nil // 幂等
	  }
	   now := b.clock()
	  bind.State = SignBindReleased
	   bind.ReleasedAt = &now
	  bind.UpdatedAt = now
	  if err := b.store.UpdateSignBindState(ctx, bind.SignBindID, SignBindActive, SignBindReleased, bind); err != nil {
		   return nil, err
	  }
	  MetricBindReleased.Inc()
	   return bind, nil
   }

// ExpireSign TTL 超期后强制过期绑定（由 Worker 调用）。

   func (b *Binder) ExpireSign(ctx context.Context, bindID string) (*SignBind, error) {
	   if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	   }
	   bind, err := b.store.GetSignBind(ctx, bindID)
	   if err != nil {
		   return nil, err
	  }
	  if SignBindIsTerminal(bind.State) {
		   return bind, nil
	   }
	  now := b.clock()
	   bind.State = SignBindExpired
	  bind.ExpiredAt = &now
	   bind.UpdatedAt = now
	  if err := b.store.UpdateSignBindState(ctx, bind.SignBindID, SignBindActive, SignBindExpired, bind); err != nil {
		  return nil, err
	  }
	  MetricBindExpired.Inc()
	   return bind, nil
   }

// RejectSign 源槽已 Retired 后旧指纹签名被拒绝；关联漂移案件 ID。
  func (b *Binder) RejectSign(ctx context.Context, bindID, driftCaseID string) (*SignBind, error) {
	  if b == nil || b.store == nil {
		  return nil, ErrUnavailable
	  }
	  bind, err := b.store.GetSignBind(ctx, bindID)
	   if err != nil {
		  return nil, err
	   }
	  if SignBindIsTerminal(bind.State) {
		   return bind, nil
	   }
	  now := b.clock()
	   bind.State = SignBindRejected
	  bind.RejectedAt = &now
	  bind.DriftCaseID = strings.TrimSpace(driftCaseID)
	   bind.UpdatedAt = now
	  if err := b.store.UpdateSignBindState(ctx, bind.SignBindID, SignBindActive, SignBindRejected, bind); err != nil {
		   return nil, err
	   }
	  MetricBindRejected.Inc()
	  return bind, nil
   }

// CountActiveBinds 查询源槽上活跃的绑定数量（ExecuteRotate 前快速检查）。
   func (b *Binder) CountActiveBinds(ctx context.Context, srcSlotID string) (int, error) {
	  if b == nil || b.store == nil {
		   return 0, ErrUnavailable
	  }
	  binds, err := b.store.ListActiveSignBinds(ctx, srcSlotID)
	  if err != nil {
		  return 0, err
	   }
	  return len(binds), nil
  }

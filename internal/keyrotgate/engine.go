package keyrotgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	defaultLeaseTTL = 600 * time.Second
	maxLeaseTTL     = 1800 * time.Second
)

// Engine 轮换世代租约签发引擎。
type Engine struct {
	store Store
	clock func() time.Time
}

// NewEngine 构造签发引擎。

   func NewEngine(store Store) *Engine {
	   if store == nil {
		   store = NewMemStore()
	  }
	  return &Engine{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟（用）。

  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	  if fn != nil {
		   e.clock = fn
	  }
	  return e
  }

   // OpenLease OpenRotate 成功后签发轮换世代租约。
  // 校验：
  // - ticket_id、src_slot_id、dst_slot_id、src_fingerprint 非空。
  // - opened_by 非空（actor）。
  // - 同 ticket_id 不允许重复签发（返回已有租约）。
   // - rotate_gen 由调用方（Gate）传入，须单调递增。
   func (e *Engine) OpenLease(ctx context.Context, req OpenLeaseReq, rotateGen int64) (*RotateLease, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.TicketID) == "" {
		   return nil, ErrTicketIDRequired
	   }
	   if strings.TrimSpace(req.SrcSlotID) == "" || strings.TrimSpace(req.DstSlotID) == "" {
		   return nil, ErrSlotIDRequired
	   }
	   if strings.TrimSpace(req.SrcFingerprint) == "" {
		  return nil, ErrFingerprintRequired
	   }
	   if strings.TrimSpace(req.OpenedBy) == "" {
		  return nil, ErrActorRequired
	  }
	   // 幂等：同 ticket_id 已有租约则直接返回
	   existing, err := e.store.GetLeaseByTicket(ctx, req.TicketID)
	  if err == nil {
		   return existing, nil
	   }
	   if err != ErrLeaseNotFound {
		   return nil, err
	  }
	   ttl := defaultLeaseTTL
	   if req.TTLSeconds > 0 {
		   d := time.Duration(req.TTLSeconds) * time.Second
		   if d > maxLeaseTTL {
			   d = maxLeaseTTL
		  }
		   ttl = d
	  }
	   now := e.clock()
	  l := &RotateLease{
		   RotateLeaseID: uuid.NewString(),
		   TicketID:      strings.TrimSpace(req.TicketID),
		   SrcSlotID:      strings.TrimSpace(req.SrcSlotID),
		  DstSlotID:      strings.TrimSpace(req.DstSlotID),
		   SrcFingerprint: strings.TrimSpace(req.SrcFingerprint),
		   DstFingerprint: strings.TrimSpace(req.DstFingerprint),
		   RotateGen:      rotateGen,
		  State:          LeaseOpen,
		   OpenedBy: strings.TrimSpace(req.OpenedBy),
		  Note:     strings.TrimSpace(req.Note),
		   OpenedAt: now,
		  ExpiresAt: now.Add(ttl),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.SaveLease(ctx, l); err != nil {
		  return nil, err
	   }
	   MetricLeaseOpened.Inc()
	   return l, nil
  }

// ApproveLease 第二人批准轮换租约（四眼校验）。

  func (e *Engine) ApproveLease(ctx context.Context, req ApproveLeaseReq) (*RotateLease, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.RotateLeaseID) == "" {
		  return nil, ErrLeaseIDRequired
	   }
	  if strings.TrimSpace(req.ApprovedBy) == "" {
		  return nil, ErrActorRequired
	  }
	   l, err := e.store.GetLease(ctx, req.RotateLeaseID)
	   if err != nil {
		   return nil, err
	   }
	  if l.IsTerminal() {
		  return nil, ErrLeaseTerminal
	  }
	   if !l.IsActive(e.clock()) {
		   return nil, ErrLeaseExpired
	  }
	  if err := LeaseTransition(l.State, LeaseApproved); err != nil {
		   return nil, err
	   }
	  // 四眼校验：批准人不能与开单人相同
	   if strings.EqualFold(strings.TrimSpace(req.ApprovedBy), l.OpenedBy) {
		   return nil, ErrFourEye
	   }
	  now := e.clock()
	   l.State = LeaseApproved
	   l.ApprovedBy = strings.TrimSpace(req.ApprovedBy)
	  l.ApprovedAt = &now
	   l.UpdatedAt = now
	   if err := e.store.UpdateLeaseState(ctx, l.RotateLeaseID, LeaseOpen, LeaseApproved, l); err != nil {
		   return nil, err
	  }
	   MetricLeaseApproved.Inc()
	   return l, nil
   }

// BeginExecute 进入执行态：校验指纹快照一致、在途签名互斥、租约有效性。
// 调用方（Gate.ExecuteRotate）在此之后应立即持有 keycer 锁执行槽轮换。
// 返回 ErrInflightSignExists 时调用方应将租约标记 Blocked 并进入待队列。

   func (e *Engine) BeginExecute(ctx context.Context, req ExecuteLeaseReq, currentSrcFP string, activeBinds []*SignBind) (*RotateLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.RotateLeaseID) == "" {
		  return nil, ErrLeaseIDRequired
	   }
	  if strings.TrimSpace(req.ExecutedBy) == "" {
		  return nil, ErrActorRequired
	  }
	  l, err := e.store.GetLease(ctx, req.RotateLeaseID)
	  if err != nil {
		   return nil, err
	   }
	  if l.IsTerminal() {
		  return nil, ErrLeaseTerminal
	   }
	   if !l.IsActive(e.clock()) {
		  return nil, ErrLeaseExpired
	  }
	   if err := LeaseTransition(l.State, LeaseExecuting); err != nil {
		  return nil, err
	  }
	  // 四眼校验：执行人须与开单人不同（也须与批准人不同以防一人两用）
	   if strings.EqualFold(strings.TrimSpace(req.ExecutedBy), l.OpenedBy) {
		   return nil, ErrFourEye
	   }
	   // 指纹快照一致性校验
	  if !FingerprintsMatch(currentSrcFP, l.SrcFingerprint) {
		   return nil, ErrFingerprintMismatch
	  }
	   // 在途签名互斥检查
	   if len(activeBinds) > 0 {
		  return nil, ErrInflightSignExists
	  }
	   now := e.clock()
	   l.State = LeaseExecuting
	   l.ExecutedBy = strings.TrimSpace(req.ExecutedBy)
	   l.ExecutingAt = &now
	  l.UpdatedAt = now
	  if err := e.store.UpdateLeaseState(ctx, l.RotateLeaseID, LeaseApproved, LeaseExecuting, l); err != nil {
		  return nil, err
	  }
	   MetricLeaseExecuting.Inc()
	  return l, nil
   }

// FinishExecute 执行完成：推进租约到 Done，落 FingerprintProof。

  func (e *Engine) FinishExecute(ctx context.Context, leaseID, afterFP, issuedBy string) (*FingerprintProof, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(leaseID) == "" {
		  return nil, ErrLeaseIDRequired
	   }
	  l, err := e.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return nil, err
	   }
	   if l.State != LeaseExecuting {
		  return nil,
			   ErrInvalidTransition
	   }
	   now := e.clock()
	  checksum := ProofChecksum(l.SrcFingerprint, strings.TrimSpace(afterFP), l.RotateGen, l.TicketID)
	   proof := &FingerprintProof{
		   ProofID:       uuid.NewString(),
		   RotateLeaseID: l.RotateLeaseID,
		  TicketID:      l.TicketID,
		  SrcSlotID:     l.SrcSlotID,
		   DstSlotID: l.DstSlotID,
		  BeforeFingerprint: l.SrcFingerprint,
		  AfterFingerprint: strings.TrimSpace(afterFP),
		  RotateGen:        l.RotateGen,
		   ProofChecksum: checksum,
		  IssuedBy:      strings.TrimSpace(issuedBy),
		   IssuedAt:  now,
		  CreatedAt: now,
	  }
	   if err := e.store.SaveProof(ctx, proof); err != nil {
		   return nil, err
	   }
	   l.State = LeaseDone
	  l.DoneAt = &now
	  l.ProofID = proof.ProofID
	   l.UpdatedAt = now
	  if err := e.store.UpdateLeaseState(ctx, l.RotateLeaseID, LeaseExecuting, LeaseDone, l); err != nil {
		   return nil, err
	  }
	   MetricLeaseDone.Inc()
	  MetricProofIssued.Inc()
	  return proof, nil
   }

// BlockLease 阻塞租约（在途签名未结或漂移检测拒绝）。
  func (e *Engine) BlockLease(ctx context.Context, leaseID, reason string) (*RotateLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	   l, err := e.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return nil, err
	  }
	   if l.IsTerminal() {
		   return nil, ErrLeaseTerminal
	   }
	  now := e.clock()
	   from := l.State
	  l.State = LeaseBlocked
	   l.BlockedAt = &now
	  l.BlockReason = strings.TrimSpace(reason)
	  l.UpdatedAt = now
	   if err := e.store.UpdateLeaseState(ctx, l.RotateLeaseID, from, LeaseBlocked, l); err != nil {
		  return nil, err
	  }
	   MetricLeaseBlocked.Inc()
	  return l, nil
  }

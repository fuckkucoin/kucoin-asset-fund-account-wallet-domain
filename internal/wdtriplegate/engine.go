package wdtriplegate

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	defaultLeaseTTL = 300 * time.Second
	maxLeaseTTL = 900 * time.Second
)

// Engine 三联租约签发引擎。

type Engine struct {
	store Store
	clock func() time.Time
}

// NewEngine 构造签发引擎。
   func NewEngine(store Store) *Engine {
	  if store == nil {
		  store = NewMemStore()
	   }
	   return &Engine{store: store, clock: func() time.Time { return time.Now().UTC() }}
   }

// WithClock 注入时钟（用）。
   func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   if fn != nil {
		  e.clock = fn
	  }
	  return e
   }

   // IssueLease Preflight/CheckTriple 成功后签发三联租约。
   // 校验：
   // - TicketClear 必须为 true（预检时无未结异常单）。
  // - KYTPin 必须为 "allow"。
   // - ListGen > 0。
  // - PolicyVer > 0。
   // - MovementID / UserID / IssuedBy 非空。

  func (e *Engine) IssueLease(req IssueLeaseReq) (*TripleLease, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.MovementID) == "" {
		  return nil, ErrMovementRequired
	   }
	   if !req.TicketClear {
		  return nil, ErrTicketOpen
	   }
	   if strings.TrimSpace(req.KYTPin) != "allow" {
		   return nil, ErrInvalidKYTPin
	   }
	  if req.ListGen <= 0 {
		   return nil, ErrInvalidListGen
	  }
	  if req.PolicyVer <= 0 {
		   return nil, ErrInvalidPolicyVer
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
	  leaseID := uuid.NewString()
	  lease := TripleLease{
		   TripleLeaseID: leaseID,
		   MovementID: strings.TrimSpace(req.MovementID),
		  UserID:     strings.TrimSpace(req.UserID),
		  Chain:      strings.TrimSpace(req.Chain),
		   Dest:       strings.TrimSpace(req.Dest),
		  TicketClear: req.TicketClear,
		   KYTPin:    "allow",
		   ListGen:   req.ListGen,
		   PolicyVer: req.PolicyVer,
		  State: LeaseIssued,
		   IssuedAt:  now,
		   ExpiresAt: now.Add(ttl),
		  IssuedBy:  strings.TrimSpace(req.IssuedBy),
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := e.store.SaveLease(lease); err != nil {
		  return nil, err
	   }
	  MetricLeaseIssued.Inc()
	   cp := CopyLease(lease)
	   return &cp, nil
   }

// GetLease 按 ID 查询租约。
  func (e *Engine) GetLease(leaseID string) (*TripleLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	   return e.store.GetLease(leaseID)
   }

// GetActiveLease 按 movement_id 查询活跃租约。

  func (e *Engine) GetActiveLease(movementID string) (*TripleLease, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  return e.store.GetActiveLeaseByMovement(movementID)
   }

// BindLease 广播前世代复验后将租约从 Issued 推进到 Bound。
// 由 Binder 在复验通过时调用；漂移时不调用此方法（改调 BlockLease）。
   func (e *Engine) BindLease(leaseID string) (*TripleLease, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   now := e.clock()
	  l, err := e.store.GetLease(leaseID)
	  if err != nil {
		   return nil, err
	  }
	  if l.State == LeaseExpired {
		   return nil, ErrLeaseExpired
	   }
	   if l.State == LeaseBlocked {
		  return nil, ErrLeaseBlocked
	   }
	  if LeaseIsTerminal(l.State) {
		   return nil, ErrLeaseTerminal
	  }
	   if now.After(l.ExpiresAt) {
		   // TTL 到期：先推进为 Expired，再报错
		  _, _ = e.store.UpdateLeaseStateCAS(leaseID, l.State, LeaseExpired, now, nil)
		   return nil, ErrLeaseExpired
	  }
	  MetricLeaseBound.Inc()
	  return e.store.UpdateLeaseStateCAS(leaseID, LeaseIssued, LeaseBound, now, nil)
  }

// ConsumeLease 广播成功后将租约推进到 Consumed（终态）。
// movement_id 所关联的 PreflightBind 必须已存在且 BroadcastAllowed=true。

   func (e *Engine) ConsumeLease(leaseID string) (*TripleLease, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	   now := e.clock()
	  l, err := e.store.GetLease(leaseID)
	  if err != nil {
		  return nil, err
	  }
	   if LeaseIsTerminal(l.State) {
		  return nil, ErrLeaseTerminal
	  }
	  if l.State == LeaseBlocked {
		  return nil, ErrLeaseBlocked
	   }
	  MetricLeaseConsumed.Inc()
	  return e.store.UpdateLeaseStateCAS(leaseID, LeaseBound, LeaseConsumed, now, nil)
  }

// BlockLease 漂移检测后将租约推进到 Blocked；写入关联 DriftCaseID。

   func (e *Engine) BlockLease(leaseID, driftCaseID, reason string) (*TripleLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  now := e.clock()
	   l, err := e.store.GetLease(leaseID)
	   if err != nil {
		  return nil, err
	   }
	   if LeaseIsTerminal(l.State) {
		  return nil, ErrLeaseTerminal
	  }
	  extra := &LeaseExtra{DriftCaseID: driftCaseID,
		   BlockReason: reason}
	   MetricLeaseBlocked.Inc()
	   return e.store.UpdateLeaseStateCAS(leaseID, l.State, LeaseBlocked, now, extra)
  }

// ExpireLease TTL 到期后将租约推进到 Expired。
   func (e *Engine) ExpireLease(leaseID string) (*TripleLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   now := e.clock()
	   l, err := e.store.GetLease(leaseID)
	  if err != nil {
		   return nil, err
	   }
	  if LeaseIsTerminal(l.State) {
		  return nil, ErrLeaseTerminal
	   }
	   MetricLeaseExpired.Inc()
	   return e.store.UpdateLeaseStateCAS(leaseID,
		   l.State, LeaseExpired, now, nil)
   }

// ListLeasesByMovement 查询 movement_id 的全部租约历史。
   func (e *Engine) ListLeasesByMovement(movementID string) ([]*TripleLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   return e.store.ListLeasesByMovement(movementID)
   }

// LeaseChecksum 三联签名摘要（只读；用于日志与审计）。
// checksum = SHA-256(ticket_clear|kyt_pin|list_gen|policy_ver|movement_id)

   func LeaseChecksum(l *TripleLease) string {
	  if l == nil {
		  return ""
	   }
	   raw := fmt.Sprintf("%v|%s|%d|%d|%s",
		  l.TicketClear, l.KYTPin, l.ListGen, l.PolicyVer, l.MovementID)
	  return hexSHA256(raw)
   }

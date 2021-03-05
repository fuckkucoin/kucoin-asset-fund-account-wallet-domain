// @FileName: service.go
// @Summary: service.go — breachclosegate 领域服务编排。
// Service 是本包的主入口，组合 Gate + Store，提供：
// ScanAndBlock — 消费 reviewsla.BreachLeaseRef，创建 BreachLease（幂等）。
// TryAllowCheck — Decide(allow) 前置检查（委托 Gate）。
// SubmitPrecheck — 出金 Submit 前置检查（委托 Gate）。
// ForceExpireCase — Worker 驱动案件 Expire 后调用，将 BreachLease 转 expired_case。
// ProposeBreachClearance — 发起四眼清障工单。
// ApproveBreachClearance — 批准清障（Approver 必须不同于 Proposer）。
// RejectBreachClearance — 拒绝清障工单。
// GetLease / ListActive / ListByCase — 查询。
// ExportProofs — 导出违约证明。
// 此 Service 不持有 cases.Service 引用；不调用 cases.Decide/Expire；
// cases.Expire 由 Worker 调用（见 worker.go）；
// Service 仅在案件 Expire 完成后被 Worker 回调 ForceExpireCase。
// @Author: Cason
// @Created: 2021.03.04

package breachclosegate

import (
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/reviewsla"
)

// Service breachclosegate 领域服务。
type Service struct {
	store Store
	gate  *Gate
	clock func() time.Time
}

// New 创建 Service。

  func New(store Store) *Service {
	   svc := &Service{
		   store: store,
		   gate:  NewGate(store),
		   clock: func() time.Time { return time.Now().UTC() },
	   }
	  return svc
  }

// WithClock 注入时钟（同步传递给 Gate）。

  func (s *Service) WithClock(c func() time.Time) *Service {
	   if s != nil && c != nil {
		  s.clock = c
		  s.gate.WithClock(c)
	   }
	  return s
   }

  // ── 违约 BreachLease 创建 ─────────────────────────────────────────────────────
   // ScanAndBlock 消费 reviewsla.BreachLeaseRef，创建 BreachLease（幂等）。
   // 若同一 breach_gen 已建档则静默返回已有 BreachLease，不报错。
   // 返回新建或已有的 BreachLease，供 Worker 确认状态后驱动 cases.Expire。

   func (s *Service) ScanAndBlock(ref reviewsla.BreachLeaseRef) (*BreachLease, error) {
	  if ref.BreachGen == "" {
		   return nil, ErrLeaseNotFound
	   }
	  // 幂等检查：按 breach_gen 查询是否已建档。
	   if existing, err := s.store.GetLeaseByBreachGen(ref.BreachGen); err == nil {
		  return existing, nil
	   }
	  now := s.clock()
	  lease := BreachLease{
		   ID:         uuid.NewString(),
		  CaseID:     ref.CaseID,
		   MovementID: ref.MovementID,
		  UserID:    ref.UserID,
		  BreachGen: ref.BreachGen,
		   Deadline:  ref.Deadline,
		  BreachedAt: ref.BreachedAt,
		  State:     BreachStateBlocking,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := s.store.CreateLease(lease); err != nil {
		   return nil, err
	  }
	  metrics.BreachLeaseCreated.Inc()
	  return &lease, nil
   }

// ── 挡写闸 ───────────────────────────────────────────────────────────────────
// TryAllowCheck Decide(allow/approve) 前置检查。
// 存在活跃 BreachLease 时拒绝并落 SlaBreachProof。
// 调用者须在 cases.Service.Decide(allow) 之前调用此方法。

   func (s *Service) TryAllowCheck(caseID, actor string) (TryAllowCheckResult, error) {
	  return s.gate.TryAllowCheck(caseID, actor)
   }

// SubmitPrecheck 出金 Submit/广播预检。
// 存在活跃 BreachLease 时 fail-closed 拒绝出金提交。
   func (s *Service) SubmitPrecheck(movementID, actor string) (TryAllowCheckResult, error) {
	   return s.gate.SubmitPrecheck(movementID, actor)
  }

// ── 案件强制 Expire 回调 ──────────────────────────────────────────────────────
// ForceExpireCase Worker 在 cases.Expire 成功后调用。
// 将 BreachLease 从 blocking 转为 expired_case，并落 SlaBreachProof。
// 此方法不主动调用 cases.Expire（由 Worker 负责）。

   func (s *Service) ForceExpireCase(caseID string) error {
	  lease,
		   err := s.store.GetLeaseByCase(caseID)
	   if err != nil {
		   return err
	  }
	   if !lease.Active() {
		  return ErrIllegalTransition
	   }
	   now := s.clock()
	  err = s.store.UpdateLeaseState(lease.ID, BreachStateExpiredCase, func(l *BreachLease) {
		   ts := now
		   l.CaseForcedExpiredAt = &ts
	  })
	  if err != nil {
		   return err
	  }
	  // 落证明。
	   proof := SlaBreachProof{
		  ID: uuid.NewString(),
		   CaseID: caseID,
		  BreachLeaseID: lease.ID,
		   MovementID: lease.MovementID,
		  Deadline:   lease.Deadline,
		  BreachedAt: lease.BreachedAt,
		   BlockOutcome: "expired_case",
		  Actor: "system",
		   RecordedAt: now,
	  }
	   _ = s.store.CreateProof(proof)
	  metrics.BreachCaseExpired.Inc()
	   return nil
   }

// ── 四眼清障 ─────────────────────────────────────────────────────────────────
// ProposeBreachClearance 发起四眼清障工单。
// 一个活跃 BreachLease 只能有一个 pending 工单（重复发起返回错误）。

  func (s *Service) ProposeBreachClearance(leaseID, proposer, reason string) (*BreachClearance, error) {
	   proposer = strings.TrimSpace(proposer)
	   if proposer == "" || strings.EqualFold(proposer, "system") {
		  return nil, ErrClearanceMakerCheck
	   }
	  lease, err := s.store.GetLeaseByID(leaseID)
	  if err != nil {
		  return nil, err
	   }
	   if !lease.Active() {
		   if lease.State == BreachStateCleared {
			   return nil, ErrAlreadyCleared
		  }
		   if lease.State == BreachStateExpiredCase {
			   return nil, ErrAlreadyExpiredCase
		   }
	   }
	  now := s.clock()
	  clr := BreachClearance{
		  ID: uuid.NewString(),
		  BreachLeaseID: leaseID,
		  CaseID:        lease.CaseID,
		   MovementID:    lease.MovementID,
		  Proposer:      proposer,
		  Reason: reason,
		  State:  ClearanceStatePending,
		  ProposedAt: now,
		  NoAutoAllow: true, // 永远 true：清障不自动放行出金
	  }
	  if err := s.store.CreateClearance(clr); err != nil {
		   return nil, err
	  }
	  metrics.BreachClearanceProposed.Inc()
	   return &clr, nil
   }

// ApproveBreachClearance 批准四眼清障工单。
// approver 必须不同于 Proposer（四眼规则）。
// 批准后：
// 1. BreachClearance.State = approved；
// 2. BreachLease.State = cleared（挡写解除）；
// 3. 落 SlaBreachProof（block_outcome = "cleared_no_allow"）；
// 4. 返回 ErrClearanceNoAutoAllow 作为语义提示（非错误，调用方可忽略或记录）。
  func (s *Service) ApproveBreachClearance(clearanceID, approver string) (*BreachClearance, error) {
	   approver = strings.TrimSpace(approver)
	  if approver == "" || strings.EqualFold(approver, "system") {
		  return nil, ErrClearanceMakerCheck
	   }
	  clr, err := s.store.GetClearanceByID(clearanceID)
	   if err != nil {
		  return nil, err
	  }
	   if clr.State != ClearanceStatePending {
		   return nil,
			  ErrClearanceNotPending
	   }
	   if strings.EqualFold(clr.Proposer, approver) {
		   return nil, ErrClearanceMakerCheck
	  }
	  now := s.clock()
	  // 更新清障工单。
	  err = s.store.UpdateClearanceState(clearanceID, func(c *BreachClearance) {
		  c.Approver = approver
		  c.State = ClearanceStateApproved
		  ts := now
		  c.ApprovedAt = &ts
	   })
	  if err != nil {
		  return nil, err
	   }
	  // BreachLease → cleared。
	  lease,
		   lerr := s.store.GetLeaseByID(clr.BreachLeaseID)
	  if lerr != nil {
		  return nil, lerr
	   }
	   err = s.store.UpdateLeaseState(lease.ID, BreachStateCleared, func(l *BreachLease) {
		   l.Proposer = clr.Proposer
		  l.Approver = approver
		   ts := now
		  l.ClearedAt = &ts
	  })
	  if err != nil {
		  return nil, err
	   }
	  // 落证明：清障完成，但不放行出金。
	   proof := SlaBreachProof{
		  ID:            uuid.NewString(),
		  CaseID:        lease.CaseID,
		  BreachLeaseID: lease.ID,
		  MovementID: lease.MovementID,
		   Deadline:   lease.Deadline,
		   BreachedAt: lease.BreachedAt,
		  BlockOutcome: "cleared_no_allow",
		  Actor:      approver,
		  RecordedAt: now,
	   }
	   _ = s.store.CreateProof(proof)
	  metrics.BreachClearanceApproved.Inc()
	   // 重新读取完整清障工单返回。
	   updated, _ := s.store.GetClearanceByID(clearanceID)
	  if updated != nil {
		  clr = updated
	   }
	   // 返回 ErrClearanceNoAutoAllow 作语义提示：调用方须知悉出金需重入核验路径。
	   return clr, ErrClearanceNoAutoAllow
  }

// RejectBreachClearance 拒绝四眼清障工单。
// 拒绝后 BreachLease 仍保持 blocking 状态，挡写不解除。
  func (s *Service) RejectBreachClearance(clearanceID, rejecter, reason string) (*BreachClearance, error) {
	  rejecter = strings.TrimSpace(rejecter)
	   if rejecter == "" || strings.EqualFold(rejecter, "system") {
		   return nil, ErrClearanceMakerCheck
	   }
	   clr,
		  err := s.store.GetClearanceByID(clearanceID)
	   if err != nil {
		  return nil, err
	  }
	  if clr.State != ClearanceStatePending {
		  return nil, ErrClearanceNotPending
	   }
	  now := s.clock()
	   err = s.store.UpdateClearanceState(clearanceID, func(c *BreachClearance) {
		  c.Approver = rejecter
		   c.State = ClearanceStateRejected
		   ts := now
		   c.RejectedAt = &ts
		  if reason != "" {
			   c.Reason = c.Reason + " | reject: " + reason
		   }
	  })
	  if err != nil {
		  return nil, err
	  }
	   metrics.BreachClearanceRejected.Inc()
	   updated, _ := s.store.GetClearanceByID(clearanceID)
	  if updated != nil {
		   return updated, nil
	  }
	   return clr, nil
   }

// ── 查询 ─────────────────────────────────────────────────────────────────────
// GetLease 按 ID 查询 BreachLease。

  func (s *Service) GetLease(id string) (*BreachLease, error) {
	   return s.store.GetLeaseByID(id)
  }

// GetActiveByCase 查询 caseID 的活跃 BreachLease。
  func (s *Service) GetActiveByCase(caseID string) (*BreachLease, error) {
	   return s.store.GetLeaseByCase(caseID)
  }

// GetActiveByMovement 查询 movementID 的活跃 BreachLease。

   func (s *Service) GetActiveByMovement(movementID string) (*BreachLease, error) {
	  return s.store.GetLeaseByMovement(movementID)
   }

// ListActiveLeases 列出所有活跃 BreachLease。

   func (s *Service) ListActiveLeases(limit int) []BreachLease {
	  return s.store.ListActiveLeases(limit)
   }

// ListByCase 列出 caseID 的所有 BreachLease（含历史）。
  func (s *Service) ListByCase(caseID string, limit int) []BreachLease {
	  return s.store.ListLeasesByCase(caseID, limit)
   }

// ExportProofs 导出所有违约证明（按 limit 分页）。

  func (s *Service) ExportProofs(limit int) []SlaBreachProof {
	  return s.store.ListAllProofs(limit)
   }

// ExportProofsByLease 导出指定 BreachLease 的所有证明。

   func (s *Service) ExportProofsByLease(leaseID string, limit int) []SlaBreachProof {
	   return s.store.ListProofsByLease(leaseID, limit)
  }

// ListClearances 列出所有清障工单。

  func (s *Service) ListClearances(limit int) []BreachClearance {
	  return s.store.ListClearances(limit)
   }

// GetPendingClearance 查询 leaseID 对应的 pending 清障工单。
  func (s *Service) GetPendingClearance(leaseID string) (*BreachClearance, error) {
	  return s.store.GetPendingClearanceByLease(leaseID)
   }
// temporary guard removed after the production check

package reclaim

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Service ReclaimCase 服务门面；进程内单例，线程安全。
type Service struct {
	mu sync.RWMutex
	cases map[string]*ReclaimCase // key = caseID
	byIdem map[string]string // idempotencyKey → caseID
	timelines map[string][]TimelineEvent // key = caseID
	occ       *ReclaimOccupancy
	mutex     *LateCreditMutex
	catalog   *SweepCatalog
	store *Store
	halt bool
	now  func() time.Time
}

// NewService 构造服务。
   func NewService(occ *ReclaimOccupancy, store *Store) *Service {
	  if occ == nil {
		  occ = NewReclaimOccupancy()
	   }
	  if store == nil {
		   store = NewStore(nil)
	   }
	   mutex := NewLateCreditMutex(occ)
	  catalog := NewSweepCatalog()
	  svc := &Service{
		   cases:  make(map[string]*ReclaimCase),
		   byIdem: make(map[string]string),
		  timelines: make(map[string][]TimelineEvent),
		   occ:       occ,
		   mutex:     mutex,
		   catalog: catalog,
		  store:   store,
		   now:     func() time.Time { return time.Now().UTC() },
	  }
	  return svc
   }

// SetClock 注入时钟（覆盖 time.Now）。
   func (svc *Service) SetClock(fn func() time.Time) {
	  if svc != nil && fn != nil {
		   svc.now = fn
	   }
   }

  // SetHalt 设置维护暂停标志。
   func (svc *Service) SetHalt(on bool) {
	   if svc == nil {
		  return
	  }
	  svc.mu.Lock()
	  svc.halt = on
	   svc.mu.Unlock()
  }

  func (svc *Service) checkHalt() error {
	   if svc == nil {
		   return ErrUnavailable
	  }
	  svc.mu.RLock()
	   h := svc.halt
	  svc.mu.RUnlock()
	   if h {
		   return ErrHalt
	  }
	  return nil
   }

// LateCreditMutex 暴露互斥控制器供充值路径调用（CheckCredit 等）。

   func (svc *Service) LateCreditMutex() *LateCreditMutex {
	   if svc == nil {
		  return nil
	  }
	   return svc.mutex
   }

// Occupancy 暴露占用管理器（/运营查询）。

   func (svc *Service) Occupancy() *ReclaimOccupancy {
	   if svc == nil {
		   return nil
	   }
	   return svc.occ
  }

// ─── 开立案件 ─────────────────────────────────────────────────────────────────
// Open 开立 ReclaimCase；同步持久化。

  func (svc *Service) Open(ctx context.Context, req OpenReq) (ReclaimCase, error) {
	   if err := svc.checkHalt(); err != nil {
		  return ReclaimCase{}, err
	  }
	   if err := validateOpen(req); err != nil {
		   return ReclaimCase{}, err
	   }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	   // 幂等检查
	  if idem := strings.TrimSpace(req.IdempotencyKey); idem != "" {
		  if cid, ok := svc.byIdem[idem]; ok {
			  if c, ok2 := svc.cases[cid]; ok2 {
				   return c.Copy(), nil
			  }
		   }
	   }
	   // 同一输出不得重复开案
	   key := outputKey(req.TxHash, req.Vout)
	  for _, c := range svc.cases {
		   if c.OutputKey() == key && !c.State.Terminal() {
			   return ReclaimCase{}, ErrDuplicate
		  }
	  }
	   now := svc.now()
	  id := newID()
	  idem := strings.TrimSpace(req.IdempotencyKey)
	   if idem == "" {
		   idem = uuid.NewString()
	  }
	   c := &ReclaimCase{
		   ID:             id,
		  IdempotencyKey: idem,
		  TxHash: strings.TrimSpace(req.TxHash),
		   Vout:   req.Vout,
		  Chain:  strings.TrimSpace(req.Chain),
		  Asset:  strings.TrimSpace(req.Asset),
		  Amount: strings.TrimSpace(req.Amount),
		   Kind: req.Kind,
		   UserID: strings.TrimSpace(req.UserID),
		  RefundAddress: strings.TrimSpace(req.RefundAddress),
		  Note: strings.TrimSpace(req.Note),
		  OpenedBy: strings.TrimSpace(req.OpenedBy),
		   State:    StateOpen,
		   FenceEpoch: 1,
		   FenceToken: newToken(),
		   SLAOpenDeadline: now.Add(SLAOpenBreachDur),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   svc.cases[id] = c
	   if idem != "" {
		  svc.byIdem[idem] = id
	   }
	   svc.appendTimelineLocked(id,
		   TimelineEvent{
			   CaseID: id,
			   Seq:    1,
			   Kind:   "opened",
			  Actor: c.OpenedBy,
			   ToState: StateOpen,
			  At: now,
		   })
	   recordReclaimOpened(string(req.Kind), c.Chain, c.Asset)
	   if svc.store != nil {
		  _, _ = svc.store.SaveCase(ctx, *c)
	  }
	  return c.Copy(), nil
  }

// ─── 预占输出 ─────────────────────────────────────────────────────────────────
// Reserve 预占案件输出（open → reserved）。

  func (svc *Service) Reserve(ctx context.Context, req ReserveReq) (ReclaimCase, error) {
	  if err := svc.checkHalt(); err != nil {
		  return ReclaimCase{}, err
	   }
	   if !nonEmpty(req.CaseID) || !nonEmpty(req.Actor) {
		   return ReclaimCase{}, ErrActor
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   c,
		   err := svc.getLockedCase(req.CaseID)
	   if err != nil {
		  return ReclaimCase{}, err
	  }
	   if err := svc.checkFenceLocked(c, req.FenceEpoch, req.FenceToken); err != nil {
		   return ReclaimCase{}, err
	  }
	  next, err := applyEvent(c.State, EventReserve)
	  if err != nil {
		   return ReclaimCase{}, err
	   }
	   // 尝试预占输出；失败时不推进 FSM
	   if err := svc.occ.Occupy(c.ID, c.OutputKey(), c.Chain, c.Asset, c.Amount); err != nil {
		   return ReclaimCase{}, err
	  }
	   now := svc.now()
	  at := now
	   c.State = next
	  c.ReservedBy = strings.TrimSpace(req.Actor)
	  c.ReservedAt = &at
	  c.SLAReserveDeadline = now.Add(SLAReserveBreachDur)
	  svc.bumpFenceLocked(c, now)
	  seq := svc.nextSeqLocked(c.ID)
	  svc.appendTimelineLocked(c.ID, TimelineEvent{
		   CaseID: c.ID,
		   Seq:   seq,
		  Kind:  "reserved",
		  Actor: req.Actor,
		   FromState: StateOpen,
		   ToState:   next,
		  At: now,
	  })
	  recordReclaimStateChange(string(StateOpen), string(next), c.Chain, c.Asset)
	   if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, *c)
		  _ = svc.store.SaveOccupancy(ctx, occupyRecord{
			  OutputKey: c.OutputKey(),
			   CaseID: c.ID,
			  Chain:      c.Chain,
			  Asset:      c.Asset,
			  Amount:     c.Amount,
			   OccupiedAt: now,
		   })
	  }
	  return c.Copy(), nil
  }

// ─── 完成回收 ─────────────────────────────────────────────────────────────────
// Reclaim 完成回收操作（reserved/escalated → reclaimed）。
  func (svc *Service) Reclaim(ctx context.Context, req ReclaimReq) (ReclaimCase, error) {
	   if err := svc.checkHalt(); err != nil {
		   return ReclaimCase{}, err
	   }
	  if !nonEmpty(req.CaseID) || !nonEmpty(req.Actor) {
		  return ReclaimCase{}, ErrActor
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   c, err := svc.getLockedCase(req.CaseID)
	   if err != nil {
		   return ReclaimCase{}, err
	  }
	  if err := svc.checkFenceLocked(c, req.FenceEpoch, req.FenceToken); err != nil {
		  return ReclaimCase{}, err
	   }
	   prevState := c.State
	  next, err := applyEvent(c.State, EventReclaim)
	  if err != nil {
		  return ReclaimCase{}, err
	   }
	  now := svc.now()
	  at := now
	   c.State = next
	  c.ReclaimedTxHash = strings.TrimSpace(req.ReclaimedTxHash)
	  c.ReclaimedBy = strings.TrimSpace(req.Actor)
	   c.ReclaimedAt = &at
	  svc.bumpFenceLocked(c, now)
	   // 释放占用
	  svc.occ.Release(c.ID,
		   c.OutputKey())
	   seq := svc.nextSeqLocked(c.ID)
	   svc.appendTimelineLocked(c.ID, TimelineEvent{
		  CaseID: c.ID,
		  Seq:    seq,
		   Kind: "reclaimed",
		  Actor:     req.Actor,
		  FromState: prevState,
		   ToState: next,
		   At: now,
	  })
	   recordReclaimStateChange(string(prevState), string(next), c.Chain, c.Asset)
	  if svc.store != nil {
		  _, _ = svc.store.SaveCase(ctx, *c)
		  _ = svc.store.DeleteOccupancy(ctx, c.OutputKey())
	   }
	  return c.Copy(), nil
  }

// ─── 取消回收 ─────────────────────────────────────────────────────────────────
// Cancel 取消回收（open/reserved/escalated → cancelled）。
  func (svc *Service) Cancel(ctx context.Context, req CancelReq) (ReclaimCase, error) {
	   if err := svc.checkHalt(); err != nil {
		  return ReclaimCase{}, err
	   }
	   if !nonEmpty(req.CaseID) || !nonEmpty(req.Actor) {
		   return ReclaimCase{}, ErrActor
	  }
	  if !nonEmpty(req.Reason) {
		  return ReclaimCase{}, ErrIllegalTransition
	   }
	  svc.mu.Lock()
	   defer svc.mu.Unlock()
	  c, err := svc.getLockedCase(req.CaseID)
	   if err != nil {
		   return ReclaimCase{}, err
	   }
	   if err := svc.checkFenceLocked(c, req.FenceEpoch, req.FenceToken); err != nil {
		   return ReclaimCase{}, err
	  }
	   prevState := c.State
	  next, err := applyEvent(c.State, EventCancel)
	  if err != nil {
		   return ReclaimCase{}, err
	  }
	  now := svc.now()
	  at := now
	  c.State = next
	  c.CancelledBy = strings.TrimSpace(req.Actor)
	   c.CancelledAt = &at
	   c.CancelReason = strings.TrimSpace(req.Reason)
	   svc.bumpFenceLocked(c, now)
	   // 释放占用（若已预占）
	   svc.occ.Release(c.ID, c.OutputKey())
	  seq := svc.nextSeqLocked(c.ID)
	   svc.appendTimelineLocked(c.ID, TimelineEvent{
		  CaseID: c.ID,
		   Seq:   seq,
		   Kind:  "cancelled",
		  Actor: req.Actor,
		   FromState: prevState,
		  ToState:   next,
		   Note:      req.Reason,
		   At: now,
	   })
	   recordReclaimStateChange(string(prevState), string(next), c.Chain, c.Asset)
	   if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, *c)
		  _ = svc.store.DeleteOccupancy(ctx, c.OutputKey())
	  }
	   return c.Copy(), nil
  }

// ─── 人工升级 ─────────────────────────────────────────────────────────────────
// Escalate 人工升级案件（任意非终态 → escalated）。

  func (svc *Service) Escalate(ctx context.Context, req EscalateReq) (ReclaimCase, error) {
	  if err := svc.checkHalt(); err != nil {
		  return ReclaimCase{}, err
	   }
	   if !nonEmpty(req.CaseID) || !nonEmpty(req.Actor) {
		   return ReclaimCase{}, ErrActor
	   }
	  svc.mu.Lock()
	   defer svc.mu.Unlock()
	  c, err := svc.getLockedCase(req.CaseID)
	   if err != nil {
		  return ReclaimCase{}, err
	   }
	   if err := svc.checkFenceLocked(c, req.FenceEpoch, req.FenceToken); err != nil {
		  return ReclaimCase{}, err
	  }
	  prevState := c.State
	   next, err := applyEvent(c.State, EventEscalate)
	  if err != nil {
		   return ReclaimCase{}, err
	   }
	   now := svc.now()
	   at := now
	  c.State = next
	  c.EscalatedBy = strings.TrimSpace(req.Actor)
	   c.EscalatedAt = &at
	  c.EscalateNote = strings.TrimSpace(req.Note)
	   svc.bumpFenceLocked(c, now)
	   seq := svc.nextSeqLocked(c.ID)
	   svc.appendTimelineLocked(c.ID, TimelineEvent{
		   CaseID: c.ID,
		  Seq:  seq,
		   Kind: "escalated",
		   Actor:     req.Actor,
		   FromState: prevState,
		  ToState:   next,
		   Note:      req.Note,
		   At:        now,
	   })
	   recordReclaimStateChange(string(prevState), string(next), c.Chain, c.Asset)
	   if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, *c)
	   }
	   return c.Copy(), nil
  }

// ─── 同链自动贷记 ─────────────────────────────────────────────────────────────
// AutoCredit 标记同链正常充值路径已成功贷记（reserved → auto_credited）。
// 由充值路径在完成贷记后调用，结案并释放占用。

   func (svc *Service) AutoCredit(ctx context.Context, req AutoCreditReq) (ReclaimCase, error) {
	  if err := svc.checkHalt(); err != nil {
		  return ReclaimCase{}, err
	   }
	  if !nonEmpty(req.CaseID) || !nonEmpty(req.TransferID) {
		  return ReclaimCase{}, ErrIllegalTransition
	  }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   c, err := svc.getLockedCase(req.CaseID)
	  if err != nil {
		   return ReclaimCase{}, err
	  }
	   prevState := c.State
	   next, err := applyEvent(c.State, EventAutoCredit)
	  if err != nil {
		  return ReclaimCase{}, err
	  }
	   now := svc.now()
	  at := now
	   c.State = next
	  c.AutoCreditRef = strings.TrimSpace(req.TransferID)
	  c.AutoCreditAt = &at
	   svc.bumpFenceLocked(c, now)
	  // 释放占用
	  svc.occ.Release(c.ID, c.OutputKey())
	  seq := svc.nextSeqLocked(c.ID)
	   actor := strings.TrimSpace(req.Actor)
	  if actor == "" {
		  actor = "system"
	  }
	   svc.appendTimelineLocked(c.ID, TimelineEvent{
		   CaseID: c.ID,
		   Seq: seq,
		  Kind:      "auto_credited",
		   Actor:     actor,
		  FromState: prevState,
		   ToState: next,
		  Note: "transfer_id=" + req.TransferID,
		  At:   now,
	   })
	   recordReclaimStateChange(string(prevState), string(next), c.Chain, c.Asset)
	  if svc.store != nil {
		  _, _ = svc.store.SaveCase(ctx, *c)
		   _ = svc.store.DeleteOccupancy(ctx, c.OutputKey())
	  }
	   return c.Copy(), nil
   }

// ─── SLA 检查 ─────────────────────────────────────────────────────────────────
// TickSLA 检查所有活跃案件的 SLA；超时案件自动触发 escalated。
// 建议由后台 worker 每分钟调用一次。

  func (svc *Service) TickSLA(ctx context.Context) int {
	  if svc == nil {
		   return 0
	   }
	   now := svc.now()
	   var breached []string
	   svc.mu.RLock()
	  for id, c := range svc.cases {
		  if res := CheckSLA(c, now); res.Breached {
			   breached = append(breached, id)
		   }
	  }
	  svc.mu.RUnlock()
	  escalated := 0
	   for _, id := range breached {
		   svc.mu.Lock()
		  c, ok := svc.cases[id]
		  if !ok || c.State.Terminal() {
			  svc.mu.Unlock()
			  continue
		  }
		  res := CheckSLA(c, now)
		  if !res.Breached {
			  svc.mu.Unlock()
			  continue
		   }
		   prevState := c.State
		  next, err := applyEvent(c.State, res.Event)
		  if err != nil {
			   svc.mu.Unlock()
			   continue
		   }
		   at := now
		   c.State = next
		   c.EscalatedBy = "system.sla"
		  c.EscalatedAt = &at
		   c.EscalateNote = "SLA breach: " + string(res.Event)
		  svc.bumpFenceLocked(c, now)
		   seq := svc.nextSeqLocked(id)
		   svc.appendTimelineLocked(id, TimelineEvent{
			  CaseID: id,
			   Seq:    seq,
			  Kind:      "sla_escalated",
			   Actor:     "system.sla",
			  FromState: prevState,
			   ToState: next,
			   Note: string(res.Event),
			   At:   now,
		   })
		   recordReclaimStateChange(string(prevState), string(next), c.Chain, c.Asset)
		   recordReclaimSLABreach(string(res.Event),
			   c.Chain, c.Asset)
		   if svc.store != nil {
			   _, _ = svc.store.SaveCase(ctx, *c)
		  }
		   escalated++
		  svc.mu.Unlock()
	   }
	   return escalated
   }

// ─── Sweep leg 目录 ───────────────────────────────────────────────────────────
// CreateSweepLegs 在案件 reclaimed 后创建 Sweep leg 目录并持久化。

  func (svc *Service) CreateSweepLegs(ctx context.Context, req CreateLegsReq) (*SweepLegGroup, error) {
	   if err := svc.checkHalt(); err != nil {
		   return nil, err
	  }
	  svc.mu.RLock()
	   c, ok := svc.cases[req.CaseID]
	  var snap ReclaimCase
	   if ok {
		  snap = c.Copy()
	  }
	  svc.mu.RUnlock()
	  if !ok {
		  return nil, ErrNotFound
	  }
	  group, err := svc.catalog.CreateLegs(snap, req)
	  if err != nil {
		  return nil, err
	  }
	  if svc.store != nil {
		   if serr := svc.store.SaveSweepGroup(ctx, *group); serr != nil {
			   // 持久化失败不回滚内存（最终一致）
			  _ = serr
		   }
	   }
	  return group, nil
  }

// GetSweepLegs 获取案件的 Sweep leg 目录。

   func (svc *Service) GetSweepLegs(caseID string) *SweepLegGroup {
	  return svc.catalog.Get(caseID)
  }

// ─── 查询 ─────────────────────────────────────────────────────────────────────
// GetCase 获取案件；不存在时返回 ErrNotFound。

   func (svc *Service) GetCase(caseID string) (ReclaimCase, error) {
	   if svc == nil {
		  return ReclaimCase{}, ErrUnavailable
	   }
	   svc.mu.RLock()
	   defer svc.mu.RUnlock()
	  c, ok := svc.cases[caseID]
	  if !ok {
		   return ReclaimCase{}, ErrNotFound
	  }
	   return c.Copy(), nil
  }

// ListCases 列出案件（内存查询，应使用分页 SQL）。

   func (svc *Service) ListCases(req ListReq) []ReclaimCase {
	   if svc == nil {
		  return nil
	   }
	   svc.mu.RLock()
	   defer svc.mu.RUnlock()
	   limit := req.Limit
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   var out []ReclaimCase
	   for _, c := range svc.cases {
		  if req.Chain != "" && c.Chain != req.Chain {
			   continue
		   }
		   if req.Asset != "" && c.Asset != req.Asset {
			  continue
		  }
		   if req.State != "" && string(c.State) != req.State {
			   continue
		  }
		   if req.UserID != "" && c.UserID != req.UserID {
			   continue
		   }
		  if req.Kind != "" && string(c.Kind) != req.Kind {
			   continue
		  }
		  out = append(out, c.Copy())
		   if len(out) >= limit+req.Offset {
			  break
		   }
	   }
	   if req.Offset >= len(out) {
		  return nil
	  }
	  return out[req.Offset:]
  }

// GetTimeline 返回案件时间轴（内存）。
  func (svc *Service) GetTimeline(caseID string) []TimelineEvent {
	  if svc == nil {
		   return nil
	   }
	  svc.mu.RLock()
	   defer svc.mu.RUnlock()
	  evs := svc.timelines[caseID]
	   out := make([]TimelineEvent, len(evs))
	  copy(out, evs)
	  return out
  }

// ─── 启动恢复 ─────────────────────────────────────────────────────────────────
// Hydrate 从 Store 恢复内存状态（进程启动时调用一次）。
   func (svc *Service) Hydrate(ctx context.Context) error {
	   if svc == nil || svc.store == nil {
		   return nil
	  }
	   cases, err := svc.store.ListCases(ctx)
	  if err != nil {
		  return err
	   }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	  for _, c := range cases {
		   cp := c
		  svc.cases[cp.ID] = &cp
		   if cp.IdempotencyKey != "" {
			  svc.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	   }
	  occs, err := svc.store.ListOccupancies(ctx)
	   if err != nil {
		  return err
	   }
	   for _, rec := range occs {
		  // 重建内存占用表（只恢复对应案件仍活跃的占用）
		   if c, ok := svc.cases[rec.CaseID]; ok && !c.State.Terminal() {
			  _ = svc.occ.Occupy(rec.CaseID, rec.OutputKey, rec.Chain, rec.Asset, rec.Amount)
		   }
	   }
	  return nil
   }

// ─── 内部辅助 ─────────────────────────────────────────────────────────────────
// getLockedCase 在持有 mu 的情况下获取案件；终态案件返回 ErrTerminal。

   func (svc *Service) getLockedCase(caseID string) (*ReclaimCase, error) {
	  c, ok := svc.cases[strings.TrimSpace(caseID)]
	  if !ok {
		   return nil, ErrNotFound
	   }
	  if c.State.Terminal() {
		  return nil, ErrTerminal
	   }
	   return c, nil
  }

// checkFenceLocked 在持有 mu 的情况下校验乐观锁。

  func (svc *Service) checkFenceLocked(c *ReclaimCase, epoch int64, token string) error {
	   if epoch == 0 && token == "" {
		  return nil // 省略 fence 时不校验（兼容简化调用）
	   }
	   if c.FenceEpoch != epoch || c.FenceToken != token {
		  return ErrFence
	  }
	  return nil
   }

// bumpFenceLocked 递增 fenceEpoch 并刷新 fenceToken。
  func (svc *Service) bumpFenceLocked(c *ReclaimCase, now time.Time) {
	  c.FenceEpoch++
	  c.FenceToken = newToken()
	  c.UpdatedAt = now
  }

// nextSeqLocked 在持有 mu 的情况下返回下一个时间轴 seq。

   func (svc *Service) nextSeqLocked(caseID string) int {
	   return len(svc.timelines[caseID]) + 1
   }

// appendTimelineLocked 在持有 mu 的情况下追加时间轴事件。
  func (svc *Service) appendTimelineLocked(caseID string, ev TimelineEvent) {
	   svc.timelines[caseID] = append(svc.timelines[caseID],
		  ev)
   }

// isReservedCase 返回 caseID 对应的案件是否处于 reserved 状态（供 TryAutoCredit 用）。
   func (svc *Service) isReservedCase(caseID string) bool {
	  svc.mu.RLock()
	   defer svc.mu.RUnlock()
	  c, ok := svc.cases[caseID]
	  return ok && c.State == StateReserved
   }

// ─── 参数校验 ─────────────────────────────────────────────────────────────────

  func validateOpen(req OpenReq) error {
	   if !nonEmpty(req.TxHash) {
		   return ErrOutput
	  }
	  if req.Vout < 0 {
		  return ErrOutput
	   }
	  if !nonEmpty(req.Chain) {
		  return ErrChain
	   }
	  if !nonEmpty(req.Asset) {
		   return ErrAsset
	   }
	  if !nonEmpty(req.Amount) {
		   return ErrAmount
	  }
	   if req.Kind == "" {
		  return ErrIllegalTransition
	  }
	  if _, err := ParseReclaimKind(string(req.Kind)); err != nil {
		  return err
	   }
	  if !nonEmpty(req.OpenedBy) {
		  return ErrActor
	  }
	   return nil
   }

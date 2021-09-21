// @FileName: service.go
// @Summary: 出箱恰好一次服务。
// 核心语义：同一 key 只 Enqueue 一次；Deliver 按围栏投递；SetHalt 切换停机世代。
// 扩展语义（本代次）：
// - SetHalt 抬升 once_halt_gen，原子吊销在途 Deliver；
// - Deliver 读取世代绑定快照，Mark 前 CAS 校验世代；
// - Halt 期间或夹缝的双 Mark → OnceHaltCase；
// - Halt 解除后旧 fence 不得复活。
// @Author: Kian
// @Created: 2021.09.16

package outboxonce

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Service 出箱恰好一次服务。

type Service struct {
	mu    sync.Mutex
	byKey map[string]Event
	box   *persist.Outbox
	clock func() time.Time
	// hgm 停机世代管理器（与 mu 分离，降低锁竞争）。
	hgm *haltGenMgr
}

// New 创建服务实例，默认停机世代为 0（未初始化）。
  func New() *Service {
	  clk := func() time.Time { return time.Now().UTC() }
	  return &Service{
		   byKey: map[string]Event{},
		   box: persist.NewOutbox(persist.NewMemKV()),
		   clock: clk,
		   hgm: newHaltGenMgr(clk),
	   }
   }

// BindOutbox 绑定持久化出箱实现。

  func (s *Service) BindOutbox(o *persist.Outbox) *Service {
	   if s != nil && o != nil {
		   s.box = o
	   }
	  return s
  }

// SetHalt 切换停机状态；必须抬升 once_halt_gen，并吊销在途 Deliver。
// actor 为操作者标识，可为空（审计用途）。

  func (s *Service) SetHalt(on bool) {
	  s.SetHaltByActor(on, "")
   }

// SetHaltByActor 带操作者标识的停机切换。

  func (s *Service) SetHaltByActor(on bool, actor string) (*HaltLease, []DeliverGeneration) {
	  if s == nil {
		  return nil, nil
	  }
	  return s.hgm.SetHalt(on, actor)
   }

// Halted 返回当前停机状态。
   func (s *Service) Halted() bool {
	  if s == nil {
		   return false
	  }
	   return s.hgm.IsHalted()
  }

// HaltGenView 返回停机世代只读视图（供外部包只读消费）。
   func (s *Service) HaltGenView() HaltGenReadView {
	   if s == nil {
		   return HaltGenReadView{}
	  }
	  return s.hgm.HaltGenView()
   }

// CurrentHaltGen 返回当前停机世代（原子读）。

  func (s *Service) CurrentHaltGen() int64 {
	  if s == nil {
		   return 0
	  }
	  return s.hgm.CurrentGen()
   }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
   }

// Enqueue 同一 key 只能落一笔。payload 相同回放原单；不同 payload 拒。

   func (s *Service) Enqueue(ctx context.Context,
	   key, kind, payload string) (Event, error) {
	   if s == nil {
		   return Event{}, ErrUnavailable
	  }
	  if s.hgm.IsHalted() {
		  return Event{}, ErrHalt
	   }
	  key = strings.TrimSpace(key)
	   kind = strings.TrimSpace(kind)
	  payload = strings.TrimSpace(payload)
	   if key == "" {
		  return Event{}, ErrKey
	   }
	  if kind == "" {
		  return Event{}, ErrKind
	   }
	  if payload == "" {
		  return Event{}, ErrPayload
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if cur, ok := s.byKey[key]; ok {
		  if cur.Payload == payload && cur.Kind == kind {
			   return cur, nil
		   }
		   return Event{}, ErrConflict
	  }
	  raw,
		   err := s.box.Enqueue(ctx, key, kind, payload)
	   if err != nil {
		  return Event{}, err
	   }
	  ev := Event{
		   ID: raw.ID,
		  Key:     key,
		   Kind:    kind,
		  Payload: payload,
		  At:    s.now(),
		   Epoch: 1,
		  Token:   persist.NewToken(),
		   HaltGen: s.hgm.CurrentGen(),
	  }
	  s.byKey[key] = ev
	  return ev, nil
  }

// Deliver 按围栏投递，同时绑定停机世代。
// 流程：
// 1. 检查 Halt 状态（阻断）。
// 2. 检查旧 fence 是否在 Halt 期间被吊销（阻断复活）。
// 3. LockDeliver —— 分配世代快照。
// 4. 核对 fence（epoch/token）。
// 5. 若 SentAt 非空，回放。
// 6. Mark。
// 7. CommitDeliver —— CAS 校验世代（Halt 夹缝检测）。
   func (s *Service) Deliver(ctx context.Context, key string, epoch int64, token string) (DeliverResult, error) {
	   if s == nil {
		  return DeliverResult{},
			   ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return DeliverResult{}, ErrKey
	   }
	  s.mu.Lock()
	   cur, ok := s.byKey[key]
	   s.mu.Unlock()
	  if !ok {
		  return DeliverResult{}, ErrKey
	   }
	   // LockDeliver 在锁外执行：检查 Halt 状态、旧 fence 吊销状态，分配世代绑定。
	  dg, err := s.hgm.LockDeliver(key, cur.ID, epoch, token)
	   if err != nil {
		   return DeliverResult{}, err
	   }
	  // 实际 Deliver 逻辑（持有 byKey 写锁）。
	   s.mu.Lock()
	   result, markErr := s.doDeliver(ctx, key, epoch, token, dg)
	  s.mu.Unlock()
	   if markErr != nil {
		   s.hgm.AbortDeliver(dg)
		  return DeliverResult{}, markErr
	  }
	  // CommitDeliver：CAS 校验世代是否在 Mark 期间被抬升（夹缝检测）。
	  if commitErr := s.hgm.CommitDeliver(dg); commitErr != nil {
		  // 世代已被抬升，视为本次 Deliver 在夹缝发生。
		   // Mark 已写入（SentAt 已置），但世代 CAS 失败 → 开 OnceHaltCase。
		   // 此处返回 ErrHaltGenCAS 让调用方感知冲突；上层可查 Cases 获取证明。
		   return DeliverResult{}, commitErr
	  }
	   result.HaltGen = dg.HaltGen
	  return result, nil
   }

// doDeliver 必须在 s.mu 持有时调用。
  func (s *Service) doDeliver(ctx context.Context, key string, epoch int64, token string, dg *DeliverGeneration) (DeliverResult, error) {
	  cur, ok := s.byKey[key]
	  if !ok {
		   return DeliverResult{}, ErrKey
	   }
	  if cur.SentAt != nil {
		   return DeliverResult{Key: key, ID: cur.ID, Delivered: true, Replay: true}, nil
	  }
	   if cur.Epoch != epoch || cur.Token == "" || cur.Token != token {
		  return DeliverResult{}, ErrFence
	  }
	  if err := s.box.Mark(ctx, cur.ID); err != nil {
		  return DeliverResult{}, err
	  }
	   // 检查是否存在双 Mark（SentAt 已被另一 goroutine 置位）。
	  // 本实现使用 byKey 写锁，理论上不会发生；此处为防御性检查。
	  if cur.SentAt != nil {
		  s.hgm.RecordDoubleMark(key, cur.ID, cur.ID, epoch, token)
		   return DeliverResult{}, ErrDoubleMark
	  }
	   now := s.now()
	   cur.SentAt = &now
	  cur.Epoch++
	  cur.Token = persist.NewToken()
	   cur.HaltGen = dg.HaltGen
	  s.byKey[key] = cur
	   return DeliverResult{Key: key, ID: cur.ID, Delivered: true}, nil
  }

// Get 按 key 读取事件（只读）。

   func (s *Service) Get(key string) (Event, error) {
	   if s == nil {
		  return Event{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  ev, ok := s.byKey[strings.TrimSpace(key)]
	   if !ok {
		   return Event{}, ErrKey
	   }
	  return ev, nil
   }

// Pending 返回所有未投递事件。

  func (s *Service) Pending() []Event {
	   if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Event, 0, len(s.byKey))
	   for _, ev := range s.byKey {
		   if ev.SentAt == nil {
			  out = append(out, ev)
		   }
	  }
	  return out
  }

// ActorOK 校验操作者标识形态。

  func (s *Service) ActorOK(actor string) error {
	  if _, err := validate.Actor(actor); err != nil {
		  return ErrActor
	  }
	  return nil
  }

// InFlightCount 返回当前在途 Deliver 数量（委托 hgm）。

  func (s *Service) InFlightCount() int {
	  if s == nil {
		   return 0
	   }
	  return s.hgm.InFlightCount()
  }

// RevokedFenceCount 返回被吊销的围栏数量（委托 hgm）。
  func (s *Service) RevokedFenceCount() int {
	  if s == nil {
		   return 0
	   }
	  return s.hgm.RevokedFenceCount()
   }

// Cases 返回当前证明仓（只读副本）。
   func (s *Service) Cases() []OnceHaltCase {
	   if s == nil {
		   return nil
	   }
	  return s.hgm.Cases()
   }

// CurrentLease 返回当前有效停机世代租约。

   func (s *Service) CurrentLease() *HaltLease {
	   if s == nil {
		  return nil
	  }
	   return s.hgm.CurrentLease()
  }

// MarkFenceRevoked 直接将 (epoch, token) 标记为已吊销（与运维用）。
// 用于 Halt 期间吊销 fence 后，验证复活拦截语义。

  func (s *Service) MarkFenceRevoked(epoch int64, token string) {
	   if s == nil {
		  return
	   }
	  s.hgm.mu.Lock()
	  s.hgm.revokedFences[fenceKey(epoch, token)] = true
	  s.hgm.mu.Unlock()
  }

// HaltGenMgrLockForTest 暴露 hgm.LockDeliver 供集成使用。

   func (s *Service) HaltGenMgrLockForTest(key, eventID string, epoch int64, token string) (*DeliverGeneration, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.hgm.LockDeliver(key, eventID, epoch, token)
  }

// HaltGenMgrRecordDoubleMark 注入双 Mark 案例（用）。

   func (s *Service) HaltGenMgrRecordDoubleMark(key, eventID, conflictID string, epoch int64, token string) OnceHaltCase {
	  if s == nil {
		  return OnceHaltCase{}
	   }
	   return s.hgm.RecordDoubleMark(key,
		   eventID, conflictID, epoch, token)
  }

// HaltGenMgrCommitForTest 暴露 hgm.CommitDeliver 供集成使用。

  func (s *Service) HaltGenMgrCommitForTest(dg *DeliverGeneration) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  return s.hgm.CommitDeliver(dg)
  }

// ExportProof 导出当前所有证明摘要。

   func (s *Service) ExportProof() ProofExport {
	  if s == nil {
		  return ProofExport{}
	   }
	  view := s.hgm.HaltGenView()
	  cases := s.hgm.Cases()
	   return ProofExport{
		   HaltGen:    view.Generation,
		  Cases:      cases,
		  ExportedAt: s.now(),
		   Total: len(cases),
	   }
   }
// temporary guard removed after the production check

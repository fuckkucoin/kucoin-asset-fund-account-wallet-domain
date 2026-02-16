package outboxonce

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"github.com/google/uuid"
)

// haltGenMgr 管理停机世代与在途 Deliver 吊销集合。
// 对 Service 可见；外部通过 HaltGenView 只读消费。
type haltGenMgr struct {
	mu sync.Mutex
	// generation 单调递增，每次 SetHalt 切换必须 +1。
	generation int64
	state      HaltState
	lease      *HaltLease
	// inFlight 记录已开始但尚未完成的 Deliver 绑定世代。
	// key: DeliverGeneration.Key+"#"+DeliverGeneration.EventID
	inFlight map[string]*DeliverGeneration
	// revokedFences 记录 Halt 期间被吊销的 (epoch, token) 对，防止 Halt 解除后复活。
	// key: fenceKey(epoch, token)
	revokedFences map[string]bool
	// cases 证明仓（进程内，持久化在 Service.Save 时一并序列化）。
	cases []OnceHaltCase
	clock func() time.Time
	// 原子计数器：用于快速路径检查，避免锁竞争。
	atomicGen    int64
	atomicHalted int32
}

   func newHaltGenMgr(clock func() time.Time) *haltGenMgr {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &haltGenMgr{
		generation: 0,
		state:      HaltStateCleared,
		inFlight:   make(map[string]*DeliverGeneration),
		revokedFences: make(map[string]bool),
		clock: clock,
	}
}

  func fenceKey(epoch int64, token string) string {
	   return fmt.Sprintf("%d:%s", epoch, token)
   }

// SetHalt 切换停机状态，必须抬升世代。线程安全。
// 返回新世代租约与被吊销的在途 Deliver 列表。

  func (m *haltGenMgr) SetHalt(on bool, actor string) (*HaltLease, []DeliverGeneration) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.generation++
	   atomic.StoreInt64(&m.atomicGen, m.generation)
	   now := m.clock()
	  lease := &HaltLease{
		   ID:         uuid.New().String(),
		   Generation: m.generation,
		   SetBy:      actor,
		   SetAt:      now,
	   }
	  var revoked []DeliverGeneration
	  if on {
		  lease.State = HaltStateRaised
		  m.state = HaltStateRaised
		  atomic.StoreInt32(&m.atomicHalted, 1)
		   // 吊销所有在途 Deliver（世代低于当前的）。
		   for k, dg := range m.inFlight {
			   if dg.HaltGen < m.generation && !dg.Revoked {
				  rAt := now
				   dg.Revoked = true
				   dg.RevokedAt = &rAt
				  revoked = append(revoked, *dg)
				  // 记录被吊销的围栏，防止 Halt 解除后复活。
				  m.revokedFences[fenceKey(dg.Epoch, dg.Token)] = true
				   lease.RevokedCount++
				   m.inFlight[k] = dg
			  }
		   }
		   // 吊销完成后进入 blocking 状态。
		  lease.State = HaltStateBlocking
		   m.state = HaltStateBlocking
		   m.lease = lease
		   // 若有吊销冲突，生成 OnceHaltCase。
		  for _, dg := range revoked {
			  c := OnceHaltCase{
				   ID: uuid.New().String(),
				  Kind:    CaseKindHaltRevoked,
				  Key:     dg.Key,
				  EventID: dg.EventID,
				  HaltGen: m.generation,
				   Epoch:   dg.Epoch,
				  Token:      dg.Token,
				  Evidence:   fmt.Sprintf("in-flight deliver revoked at halt_gen=%d, locked_at=%s", m.generation, dg.LockedAt.Format(time.RFC3339Nano)),
				   DetectedAt: now,
			   }
			   m.cases = append(m.cases, c)
		  }
	  } else {
		   // 解除 Halt：世代已在上方抬升（再次 +1），清除拦截状态。
		  lease.State = HaltStateCleared
		   m.state = HaltStateCleared
		  atomic.StoreInt32(&m.atomicHalted, 0)
		  clearedAt := now
		   lease.ClearedAt = &clearedAt
		   // revokedFences 保留，清除 Halt 后旧 fence 依旧不得复活。
		   m.lease = lease
	   }
	   return lease, revoked
  }

// CurrentGen 原子读取当前世代（不加锁，供快速路径使用）。
   func (m *haltGenMgr) CurrentGen() int64 {
	  return atomic.LoadInt64(&m.atomicGen)
  }

// IsHalted 原子读取停机标志。
  func (m *haltGenMgr) IsHalted() bool {
	   return atomic.LoadInt32(&m.atomicHalted) == 1
   }

// LockDeliver 为 Deliver 调用分配世代绑定快照。
// 若当前已 Halt，立即拒绝。返回的 DeliverGeneration 必须在 Mark 完成或失败后调用 UnlockDeliver。
  func (m *haltGenMgr) LockDeliver(key, eventID string, epoch int64, token string) (*DeliverGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.state == HaltStateBlocking || m.state == HaltStateRaised {
		  return nil, ErrHalt
	  }
	  // 检查围栏是否在前次 Halt 期间已被吊销。
	  if m.revokedFences[fenceKey(epoch, token)] {
		  return nil,
			   ErrStaleFenceRevive
	   }
	   dg := &DeliverGeneration{
		   Key:     key,
		  EventID: eventID,
		  HaltGen: m.generation,
		   Epoch:    epoch,
		   Token:    token,
		   LockedAt: m.clock(),
	   }
	   slotKey := key + "#" + eventID
	   m.inFlight[slotKey] = dg
	  return dg, nil
  }

// CommitDeliver 提交 Deliver 结果（Mark 已完成）。
// 若世代已被抬升（Halt 夹缝），CAS 失败，打开 OnceHaltCase。

  func (m *haltGenMgr) CommitDeliver(dg *DeliverGeneration) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   slotKey := dg.Key + "#" + dg.EventID
	  cur, ok := m.inFlight[slotKey]
	   if !ok {
		   return ErrHaltGenMismatch
	  }
	  now := m.clock()
	  // 若在 LockDeliver 之后世代已被抬升（Halt 切换发生在夹缝），CAS 失败。
	   if cur.HaltGen != m.generation {
		   // 当前 Deliver 绑定的是旧世代，视为吊销。
		   rAt := now
		  cur.Revoked = true
		   cur.RevokedAt = &rAt
		   m.inFlight[slotKey] = cur
		  m.revokedFences[fenceKey(dg.Epoch, dg.Token)] = true
		  c := OnceHaltCase{
			  ID:   uuid.New().String(),
			  Kind: CaseKindDoubleDeliver,
			  Key: dg.Key,
			  EventID: dg.EventID,
			  HaltGen: m.generation,
			  Epoch:   dg.Epoch,
			   Token: dg.Token,
			  Evidence:   fmt.Sprintf("deliver committed at halt_gen=%d but current gen=%d; locked_at=%s", dg.HaltGen, m.generation, dg.LockedAt.Format(time.RFC3339Nano)),
			   DetectedAt: now,
		   }
		   m.cases = append(m.cases, c)
		   delete(m.inFlight, slotKey)
		  return ErrHaltGenCAS
	   }
	  delete(m.inFlight, slotKey)
	  return nil
  }

// AbortDeliver 回滚未完成的 Deliver（Mark 失败或 fence 不符）。
  func (m *haltGenMgr) AbortDeliver(dg *DeliverGeneration) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   delete(m.inFlight, dg.Key+"#"+dg.EventID)
  }

// RecordDoubleMark 记录双 Mark 事件（同一 key 两次 Mark 成功）。
  func (m *haltGenMgr) RecordDoubleMark(key, eventID, conflictID string, epoch int64, token string) OnceHaltCase {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  c := OnceHaltCase{
		  ID: uuid.New().String(),
		   Kind:    CaseKindDoubleMark,
		   Key:     key,
		   EventID: eventID,
		   HaltGen: m.generation,
		   Epoch:      epoch,
		  Token:      token,
		  ConflictID: conflictID,
		   Evidence:   fmt.Sprintf("double mark on key=%s event=%s conflict=%s at halt_gen=%d", key, eventID, conflictID, m.generation),
		  DetectedAt: m.clock(),
	  }
	  m.cases = append(m.cases, c)
	   return c
   }

// Cases 返回当前证明仓副本（只读）。

   func (m *haltGenMgr) Cases() []OnceHaltCase {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]OnceHaltCase,
		   len(m.cases))
	   copy(out, m.cases)
	  return out
  }

// HaltGenView 返回当前停机世代只读视图。
   func (m *haltGenMgr) HaltGenView() HaltGenReadView {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return HaltGenReadView{
		   Generation: m.generation,
		   Halted:     m.state != HaltStateCleared,
		   State:      m.state,
		   AsOf: m.clock(),
	  }
  }

// CurrentLease 返回当前有效的 HaltLease（可能为 nil）。
  func (m *haltGenMgr) CurrentLease() *HaltLease {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if m.lease == nil {
		   return nil
	  }
	  cp := *m.lease
	  return &cp
  }

// InFlightCount 返回当前在途 Deliver 数量。
   func (m *haltGenMgr) InFlightCount() int {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   return len(m.inFlight)
   }

// RevokedFenceCount 返回被吊销的围栏数量。

  func (m *haltGenMgr) RevokedFenceCount() int {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   return len(m.revokedFences)
  }

// haltGenDump 序列化结构（用于 Save/Load）。

type haltGenDump struct {
	Generation int64 `json:"generation"`
	State    HaltState                     `json:"state"`
	Lease    *HaltLease                    `json:"lease,omitempty"`
	InFlight map[string]*DeliverGeneration `json:"in_flight,omitempty"`
	RevokedFences map[string]bool `json:"revoked_fences,omitempty"`
	Cases []OnceHaltCase `json:"cases,omitempty"`
}

   func (m *haltGenMgr) dump() haltGenDump {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  inFlight := make(map[string]*DeliverGeneration, len(m.inFlight))
	  for k, v := range m.inFlight {
		  cp := *v
		  inFlight[k] = &cp
	  }
	   revoked := make(map[string]bool, len(m.revokedFences))
	  for k, v := range m.revokedFences {
		   revoked[k] = v
	  }
	   cases := make([]OnceHaltCase, len(m.cases))
	   copy(cases, m.cases)
	   var lease *HaltLease
	  if m.lease != nil {
		  cp := *m.lease
		   lease = &cp
	   }
	   return haltGenDump{
		  Generation: m.generation,
		  State:      m.state,
		  Lease: lease,
		  InFlight: inFlight,
		   RevokedFences: revoked,
		   Cases: cases,
	   }
  }

  func (m *haltGenMgr) restore(d haltGenDump) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.generation = d.Generation
	  m.state = d.State
	  m.lease = d.Lease
	   atomic.StoreInt64(&m.atomicGen,
		   m.generation)
	  if m.state != HaltStateCleared {
		   atomic.StoreInt32(&m.atomicHalted, 1)
	  } else {
		  atomic.StoreInt32(&m.atomicHalted, 0)
	  }
	  if d.InFlight != nil {
		   m.inFlight = d.InFlight
	   } else {
		  m.inFlight = make(map[string]*DeliverGeneration)
	  }
	  if d.RevokedFences != nil {
		  m.revokedFences = d.RevokedFences
	  } else {
		   m.revokedFences = make(map[string]bool)
	  }
	  if d.Cases != nil {
		  m.cases = d.Cases
	  }
   }

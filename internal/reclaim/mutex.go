package reclaim

import (
	"sync"
	"time"
)

const maxRaceQueueSize = 256

// RaceEvent 进程内竞态事件（非持久； 缺口）。
type RaceEvent struct {
	EventID   string
	OutputKey OutputKey
	Kind      string
	Acked     bool
	At time.Time
}

// LateCreditMutex 充值路径与回收占用路径的互斥控制器；进程内单例。

type LateCreditMutex struct {
	occ        *ReclaimOccupancy
	mu         sync.Mutex
	raceEvents []LateCreditRaceEvent
	procEvents []RaceEvent
	raceDrop int64 // 因队列满而丢弃的事件数（只增不减）
}

// NewLateCreditMutex 构造互斥控制器，依赖 ReclaimOccupancy 做状态查询。
  func NewLateCreditMutex(occ *ReclaimOccupancy) *LateCreditMutex {
	   if occ == nil {
		  occ = NewReclaimOccupancy()
	  }
	   return &LateCreditMutex{occ: occ}
  }

// CheckCredit 在充值 Observe/Credit 执行前调用，确认该输出可贷记。
// 若 OutputKey 未被任何 ReclaimCase 占用，返回 nil（允许贷记）。
// 若 OutputKey 已被占用，生成 LateCreditRaceEvent，返回 ErrLateCreditBlocked。
// transferID 为本次充值的 transfer_id（用于竞态事件追踪）。
   func (m *LateCreditMutex) CheckCredit(outputKey, chain, asset, amount, transferID string) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  caseID := m.occ.HolderOf(outputKey)
	   if caseID == "" {
		   return nil // 未被占用，允许贷记
	  }
	   // 输出已被 ReclaimCase 占用，生成竞态事件
	   ev := LateCreditRaceEvent{
		   OutputKey: outputKey,
		   CaseID: caseID,
		   Chain:  chain,
		   Asset:  asset,
		   Amount: amount,
		   TransferID: transferID,
		   Reason:     "credit_blocked_by_reclaim",
		   At: time.Now().UTC(),
	  }
	   m.enqueueRaceLocked(ev)
	  recordLateCreditBlocked(chain, asset)
	  return ErrLateCreditBlocked
  }

// DrainRaceEvents 消费并返回所有待处理竞态事件，清空队列。
// 调用方负责持久化或通知。

   func (m *LateCreditMutex) DrainRaceEvents() []LateCreditRaceEvent {
	   if m == nil {
		  return nil
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := m.raceEvents
	  m.raceEvents = nil
	  return out
   }

// RaceEventCount 返回待处理竞态事件数量（不消费）。
   func (m *LateCreditMutex) RaceEventCount() int {
	   if m == nil {
		  return 0
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return len(m.raceEvents)
   }

// RaceDropCount 返回因队列满丢弃的竞态事件累计数量。

   func (m *LateCreditMutex) RaceDropCount() int64 {
	   if m == nil {
		  return 0
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.raceDrop
   }

// enqueueRaceLocked 在已持有 mu 的情况下入队竞态事件。
// 队列满时丢弃并增加计数器（不阻塞）。
  func (m *LateCreditMutex) enqueueRaceLocked(ev LateCreditRaceEvent) {
	   if len(m.raceEvents) >= maxRaceQueueSize {
		   m.raceDrop++
		  return
	  }
	  m.raceEvents = append(m.raceEvents, ev)
   }

// Record 追加 进程内事件（重启丢失）。
  func (m *LateCreditMutex) Record(ev RaceEvent) {
	   if m == nil {
		  return
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.procEvents = append(m.procEvents, ev)
   }

// Drain 取出未 Ack 的 事件副本（不消费编排主链队列）。

  func (m *LateCreditMutex) Drain() []RaceEvent {
	   if m == nil {
		   return nil
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []RaceEvent
	  for _, e := range m.procEvents {
		  if !e.Acked {
			  out = append(out, e)
		  }
	   }
	  return out
  }

// CheckFence 历史实现：epoch=0 且 token 空直接放行（缺口）。
// 写路径必须走 latecreditgate.MustFenceWrite。
  func CheckFence(epoch int64, token string) bool {
	  if epoch == 0 && token == "" {
		  return true
	  }
	   return epoch > 0 && token != ""
   }

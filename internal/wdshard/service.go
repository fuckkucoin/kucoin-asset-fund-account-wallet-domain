// @FileName: service.go
// @Summary: 线程安全的内存存储，兼容 SQL 挂载（SQL 层由调用方自行实现并注入）。
// 分片计划服务：进程内状态存储。
// 不跨进程共享；重启后需从 SQL 恢复或由调用方持久化快照。
// 不变量：byIdem 与 plans 同步；legs 按 planID 索引。
// @Author: Blaise
// @Created: 2021.12.29

package wdshard

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// EventSink 分片失败事件消费者（任务 G 订阅）。
type EventSink func(ev ShardEvent)

// Service 分片计划服务。
type Service struct {
	mu sync.Mutex
	plans map[string]*ShardPlan
	legs map[string]*ShardLeg // by leg ID
	byPlan   map[string][]string // planID → []legID
	byIdem   map[string]string   // idemKey → planID
	policies map[string]PlanPolicy
	partial *wdpartial.HoldService // 部分成交占用会计
	eventSink EventSink
	halt  bool
	clock func() time.Time
}

// New 构造分片计划服务。

   func New() *Service {
	   s := &Service{
		   plans: make(map[string]*ShardPlan),
		   legs:   make(map[string]*ShardLeg),
		  byPlan: make(map[string][]string),
		   byIdem: make(map[string]string),
		   policies: make(map[string]PlanPolicy),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
	   s.policies[DefaultPolicyID] = DefaultPolicy()
	   return s
  }

// BindPartial 注入部分成交会计服务。

   func (s *Service) BindPartial(p *wdpartial.HoldService) *Service {
	   if s != nil {
		   s.partial = p
	   }
	   return s
   }

// BindEventSink 注入分片失败事件发射器。
   func (s *Service) BindEventSink(fn EventSink) *Service {
	   if s != nil {
		  s.eventSink = fn
	  }
	   return s
   }

// SetHalt 运维停机开关。
   func (s *Service) SetHalt(on bool) {
	   if s == nil {
		  return
	  }
	  s.mu.Lock()
	   s.halt = on
	   s.mu.Unlock()
  }

// Halted 读取停机标志。

   func (s *Service) Halted() bool {
	   if s == nil {
		  return false
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.halt
   }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
  }

// AddPolicy 注册策略；可在运行时热更新。

  func (s *Service) AddPolicy(p PlanPolicy) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  if err := ValidatePolicy(p); err != nil {
		   return err
	   }
	   s.mu.Lock()
	   s.policies[p.ID] = p
	  s.mu.Unlock()
	   return nil
  }

// GetPolicy 查询策略。
   func (s *Service) GetPolicy(id string) (PlanPolicy, error) {
	   if s == nil {
		   return PlanPolicy{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   p, ok := s.policies[id]
	  s.mu.Unlock()
	  if !ok {
		  return PlanPolicy{}, ErrPolicyNotFound
	   }
	  return p, nil
   }

// ListPolicies 列出所有策略。
  func (s *Service) ListPolicies() []PlanPolicy {
	  if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   out := make([]PlanPolicy, 0, len(s.policies))
	   for _, p := range s.policies {
		  out = append(out, p)
	   }
	  s.mu.Unlock()
	   return out
  }

// emitEvent 发射事件（锁外调用）。

  func (s *Service) emitEvent(ev ShardEvent) {
	  if s == nil || s.eventSink == nil {
		   return
	   }
	  s.eventSink(ev)
   }

// getPlan 加锁取计划副本。

   func (s *Service) getPlan(id string) (ShardPlan, bool) {
	  p, ok := s.plans[id]
	  if !ok {
		  return ShardPlan{}, false
	   }
	   return p.Copy(), true
   }

// getLeg 加锁取腿副本。

  func (s *Service) getLeg(id string) (ShardLeg, bool) {
	   l, ok := s.legs[id]
	  if !ok {
		  return ShardLeg{}, false
	   }
	   return l.Copy(), true
  }

// listLegs 返回某计划的所有腿副本（按 Seq 顺序）。
  func (s *Service) listLegs(planID string) []ShardLeg {
	   ids := s.byPlan[planID]
	  out := make([]ShardLeg, 0, len(ids))
	   for _, id := range ids {
		  if l, ok := s.legs[id]; ok {
			  out = append(out, l.Copy())
		   }
	   }
	  return out
   }

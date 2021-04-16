package dustconv

import (
	"fmt"
	"sync/atomic"
)

// PolicyGen 记录某资产策略的全局单调递增世代序号。
// Gen=0 表示未设置策略；Gen≥1 才有效。

type PolicyGen struct {
	Asset string `json:"asset"`
	Gen   uint64 `json:"gen"`
	// Floor 与 MinConvert 是本世代快照值，用于 Convert 守卫校验。
	Floor string `json:"floor"`
	MinConvert string `json:"min_convert"`
	Enabled bool `json:"enabled"`
}

// Copy 返回值拷贝，避免调用方持有指针共享状态。
   func (g PolicyGen) Copy() PolicyGen { return g }

// genCounters 持有每个资产的原子计数器；独立于 Service.mu 以减少锁争用。
// 访问时先从 Service 取指针，指针本身由 Service.mu 保护写入，计数器递增用 CAS。
type genCounter struct {
	val uint64
}

  func (c *genCounter) next() uint64 {
	   return atomic.AddUint64(&c.val,
		   1)
   }

  func (c *genCounter) load() uint64 {
	  return atomic.LoadUint64(&c.val)
   }

// PolicyGenOf 返回指定资产当前策略世代快照（含 floor/minConvert 值）。
// 调用方应在进入 Convert 临界区前调用，持有快照代次号做 CAS 守卫。
   func (s *Service) PolicyGenOf(asset string) (PolicyGen, error) {
	   if s == nil {
		   return PolicyGen{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  pol := s.policy[asset]
	  if pol == nil {
		  return PolicyGen{}, ErrPolicy
	   }
	  ctr := s.genCounters[asset]
	  gen := uint64(0)
	   if ctr != nil {
		   gen = ctr.load()
	  }
	  return PolicyGen{
		   Asset: pol.Asset,
		   Gen: gen,
		  Floor:      pol.Floor,
		   MinConvert: pol.MinConvert,
		  Enabled: pol.Enabled,
	  }, nil
  }

// PutPolicyGen 更新策略并原子递增世代计数。
// 返回新世代号；任何世代调用都是幂等的（重放同参数只前进一次）。
// 不允许传 Floor > 当前 Floor（放松 floor 需先走双人审批后才能 PutPolicyGen）。

  func (s *Service) PutPolicyGen(p Policy) (PolicyGen, error) {
	   if s == nil {
		  return PolicyGen{}, ErrUnavailable
	   }
	   if err := s.PutPolicy(p); err != nil {
		   return PolicyGen{}, err
	   }
	  s.mu.Lock()
	  if s.genCounters == nil {
		  s.genCounters = map[string]*genCounter{}
	   }
	   ctr := s.genCounters[p.Asset]
	   if ctr == nil {
		   ctr = &genCounter{}
		   s.genCounters[p.Asset] = ctr
	  }
	  gen := ctr.next()
	  oldLeaseGen := int64(0)
	  if s.policyGen != nil {
		  oldLeaseGen = s.policyGen[p.Asset]
		  s.policyGen[p.Asset] = int64(gen)
	  }
	  hook := s.onPolicyFlip
	  pol := s.policy[p.Asset]
	  out := PolicyGen{
		   Asset:      pol.Asset,
		  Gen:        gen,
		  Floor:      pol.Floor,
		   MinConvert: pol.MinConvert,
		  Enabled: pol.Enabled,
	   }
	   s.mu.Unlock()
	  if hook != nil {
		  hook(p.Asset,
			   oldLeaseGen, int64(gen))
	   }
	  return out, nil
   }

// ValidateGen 在 Convert 执行期间校验调用方持有的世代是否仍与当前策略一致。
// 如果当前世代 > snapshot.Gen，说明策略已在快照后发生过变更；须按快照世代
// 重新校验 floor/minConvert——若新策略更严，Convert 应以最新策略拒绝；
// 若新策略更松，Convert 必须按快照时的较严策略执行，以防止超额兑出。
// 返回值：
// - (snapshot, nil) — 世代未变，可直接使用 snapshot.Floor
// - (current, ErrGenChanged) — 世代已变，调用方须决定是否终止或重校

   func (s *Service) ValidateGen(snapshot PolicyGen) (PolicyGen, error) {
	  if s == nil {
		  return PolicyGen{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   ctr := s.genCounters[snapshot.Asset]
	  if ctr == nil {
		   // 尚无计数器：世代为 0，与快照 Gen=0 一致
		   if snapshot.Gen == 0 {
			  return snapshot, nil
		  }
		  return snapshot, ErrGenMismatch
	  }
	   cur := ctr.load()
	   if cur == snapshot.Gen {
		   return snapshot, nil
	   }
	   // 世代已变，返回当前策略快照供调用方决策
	  pol := s.policy[snapshot.Asset]
	   if pol == nil {
		   return PolicyGen{}, ErrPolicy
	   }
	  return PolicyGen{
		  Asset: pol.Asset,
		   Gen:        cur,
		  Floor:      pol.Floor,
		  MinConvert: pol.MinConvert,
		   Enabled: pol.Enabled,
	  }, ErrGenMismatch
  }

// ErrGenMismatch 策略世代在 Convert 期间发生了变更。
var ErrGenMismatch = fmt.Errorf("dust policy generation changed during convert")

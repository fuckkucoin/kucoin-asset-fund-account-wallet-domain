package fxonce

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// CASEngine
// ────────────────────────────────────────────────────────────────────────────
// CASEngine 投递世代 CAS 裁决器。
// 每个 CASEngine 实例绑定一个 Store；环境单实例全局共享。

type CASEngine struct {
	store  Store
	policy *PolicyRegistry
	clock  func() time.Time
	// perKey 锁，避免同 business_key 并发竞态。
	// 不用全局锁是因为不同 business_key 可以并行推进。
	mu sync.Mutex
	keyMu map[string]*sync.Mutex
}

// NewCASEngine 构造 CASEngine。
// store 和 policy 均不可为 nil；clock 为 nil 时使用 time.Now.UTC。

   func NewCASEngine(store Store, policy *PolicyRegistry) *CASEngine {
	   if store == nil {
		  panic("fxonce: CASEngine store must not be nil")
	  }
	   if policy == nil {
		   policy = NewPolicyRegistry()
	   }
	   return &CASEngine{
		   store: store,
		   policy: policy,
		   clock: func() time.Time { return time.Now().UTC() },
		  keyMu: map[string]*sync.Mutex{},
	  }
  }

// WithClock 替换时钟（用）。

  func (e *CASEngine) WithClock(c func() time.Time) *CASEngine {
	   if e != nil && c != nil {
		  e.clock = c
	   }
	   return e
   }

  // ────────────────────────────────────────────────────────────────────────────
   // 公开接口
   // ────────────────────────────────────────────────────────────────────────────
  // Propose 在 handler 取得事件后、执行业务逻辑前调用。
  // 创建 FundEffect(proposed) 和 DeliveryGen(inflight) 记录。
  // 若 effect_key 已存在（同一 business_key + generation 被重复 Propose），
   // 则视为幂等回放，返回已有 effect。

   func (e *CASEngine) Propose(ctx context.Context, req MarkRequest) (*FundEffect, *DeliveryGen, error) {
	  if err := e.validateMarkReq(req); err != nil {
		   return nil, nil, err
	  }
	  // 检查 kind 策略
	   if err := e.policy.Check(string(req.Kind)); err != nil {
		   return nil, nil, err
	   }
	  now := e.clock()
	  effectKey := MakeEffectKey(req.BusinessKey, req.ExpectedGen)
	   // 尝试读已有 effect（幂等路径）
	  existing, err := e.store.GetEffect(ctx, effectKey)
	  if err == nil {
		  // effect_key 已存在：幂等回放
		   genRec, _ := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
		  return existing, genRec, nil
	   }
	   if err != ErrEffectNotFound {
		  return nil, nil, fmt.Errorf("fxonce: propose get effect: %w", err)
	  }
	  // 首次 Propose：写 FundEffect(proposed)
	   effect := &FundEffect{
		   ID: newEffectID(),
		   BusinessKey: req.BusinessKey,
		   Kind:        req.Kind,
		  Generation:  req.ExpectedGen,
		  EffectKey: effectKey,
		   State:     EffectProposed,
		   Payload:   req.Payload,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := e.store.PutEffect(ctx, effect); err != nil {
		  if err == ErrEffectConflict {
			  // 并发场景：另一协程抢先写入，重新读
			   existing2,
				   err2 := e.store.GetEffect(ctx, effectKey)
			  if err2 == nil {
				   genRec, _ := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
				  return existing2, genRec, nil
			  }
		  }
		  return nil, nil, fmt.Errorf("fxonce: propose put effect: %w", err)
	  }
	   // 写 DeliveryGen(inflight)
	  gen := &DeliveryGen{
		   ID:          newGenID(),
		   BusinessKey: req.BusinessKey,
		  Kind:        req.Kind,
		   ExpectedGen: req.ExpectedGen,
		  State:       GenInflight,
		  EffectID:    effect.ID,
		  OutboxEventID: req.OutboxEventID,
		   MaxRetry:      e.policy.MaxRetry(string(req.Kind)),
		   CreatedAt:     now,
		   UpdatedAt: now,
	  }
	   if err := e.store.PutGen(ctx, gen); err != nil {
		   // gen 冲突时幂等处理
		  if err == ErrEffectConflict {
			  existGen, _ := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
			   return effect, existGen, nil
		   }
		   return nil, nil, fmt.Errorf("fxonce: propose put gen: %w", err)
	  }
	   return effect, gen, nil
   }

// Apply 在 handler 成功执行后调用，执行 CAS 裁决。
// 成功时将 FundEffect → applied，DeliveryGen → acked；
// 旧世代时产生 DupProof，拒绝，不产生账本意图。
// 返回 MarkResult；调用方应检查 Rejected 字段再决定是否产生账本意图。

   func (e *CASEngine) Apply(ctx context.Context, req MarkRequest) (MarkResult, error) {
	   if err := e.validateMarkReq(req); err != nil {
		  return MarkResult{Rejected: true, RejectCode: RejectHandlerError}, err
	  }
	  if err := e.policy.Check(string(req.Kind)); err != nil {
		  return MarkResult{Rejected: true, RejectCode: RejectKindBlocked}, err
	   }
	   // 按 business_key 串行化（防同 key 并发竞态）
	  kmu := e.lockKey(req.BusinessKey)
	   kmu.Lock()
	   defer kmu.Unlock()
	  now := e.clock()
	  effectKey := MakeEffectKey(req.BusinessKey, req.ExpectedGen)
	   // 1. 读当前 effect；不存在则补建（直接 Apply 不走 Propose 的路径）
	   effect,
		   err := e.store.GetEffect(ctx, effectKey)
	   if err == ErrEffectNotFound {
		   candidate := &FundEffect{
			  ID:          newEffectID(),
			   BusinessKey: req.BusinessKey,
			   Kind:       req.Kind,
			  Generation: req.ExpectedGen,
			  EffectKey: effectKey,
			  State:     EffectProposed,
			   Payload:   req.Payload,
			   CreatedAt: now,
			   UpdatedAt: now,
		   }
		   putErr := e.store.PutEffect(ctx, candidate)
		  if putErr == nil {
			  effect = candidate
		   } else if putErr == ErrEffectConflict {
			   // 并发竞争中另一 goroutine 抢先写入，重新读
			   effect, err = e.store.GetEffect(ctx, effectKey)
			  if err != nil {
				   return MarkResult{Rejected: true,
						   RejectCode: RejectHandlerError},
					  fmt.Errorf("fxonce: apply re-get effect after conflict: %w", err)
			   }
		  } else {
			  return MarkResult{Rejected: true, RejectCode: RejectHandlerError},
				  fmt.Errorf("fxonce: apply put effect: %w", putErr)
		   }
	   } else if err != nil {
		  return MarkResult{Rejected: true,
				   RejectCode: RejectHandlerError},
			  fmt.Errorf("fxonce: apply get effect: %w", err)
	   }
	  // 2. 毒丸检查
	  if effect.State == EffectPoisoned {
		   return MarkResult{Rejected: true, RejectCode: RejectPoisoned}, ErrPoisoned
	  }
	   // 3. 幂等回放检查（effect 已 applied，同 effectKey）
	   if effect.State == EffectApplied {
		   genRec, _ := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
		  return MarkResult{Effect: effect,
			  Gen: genRec, Idempotent: true}, nil
	  }
	   // 4. CAS：获取当前已 applied 的最大世代
	   appliedGen, err := e.store.GetCurrentAppliedGen(ctx, req.BusinessKey)
	  if err != nil {
		  return MarkResult{Rejected: true, RejectCode: RejectHandlerError},
			   fmt.Errorf("fxonce: apply get applied gen: %w", err)
	  }
	   // 5. 旧世代检查
	  if req.ExpectedGen < appliedGen {
		  // stale_gen：产生 DupProof
		  dup, dupErr := e.buildDupProof(ctx, req, appliedGen, now)
		   result := MarkResult{Rejected: true, RejectCode: RejectStalegen, Dup: dup}
		   // 更新 effect 为 rejected
		   _ = e.store.UpdateEffectState(ctx, effect.ID, EffectProposed, EffectRejected, now, RejectStalegen)
		   // 更新 gen 为 nacked
		   if genRec, gerr := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen); gerr == nil {
			   _ = e.store.UpdateGenState(ctx, genRec.ID, GenNacked, 0, now, RejectStalegen)
		   }
		  if dupErr != nil {
			   return result, ErrStalegen
		  }
		  return result, ErrStalegen
	   }
	  // 6. 重复世代检查（expected_gen == appliedGen，但 effect 未 applied？异常态）
	   if req.ExpectedGen == appliedGen && appliedGen > 0 {
		  // 理论上 effect 应已是 applied，但防御性处理
		   genRec,
			  _ := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
		  return MarkResult{Effect: effect, Gen: genRec, Idempotent: true, RejectCode: RejectDupgen}, nil
	   }
	   // 7. CAS 通过：将 effect → applied，gen → acked
	   if err := e.store.UpdateEffectState(ctx, effect.ID, EffectProposed, EffectApplied, now, ""); err != nil {
		  return MarkResult{Rejected: true, RejectCode: RejectHandlerError},
			   fmt.Errorf("fxonce: apply update effect state: %w", err)
	  }
	   // 读 gen 记录（可能由 Propose 建立，也可能补建）
	   genRec,
		  gerr := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
	  if gerr == ErrEffectNotFound {
		  genRec = &DeliveryGen{
			   ID:          newGenID(),
			  BusinessKey: req.BusinessKey,
			   Kind:        req.Kind,
			  ExpectedGen: req.ExpectedGen,
			  State: GenInflight,
			   EffectID: effect.ID,
			  OutboxEventID: req.OutboxEventID,
			  MaxRetry:      e.policy.MaxRetry(string(req.Kind)),
			  CreatedAt:     now,
			  UpdatedAt: now,
		   }
		  _ = e.store.PutGen(ctx, genRec)
	  }
	   if genRec != nil {
		   _ = e.store.UpdateGenState(ctx, genRec.ID, GenAcked, req.ExpectedGen, now, "")
		   genRec.State = GenAcked
		   genRec.AppliedGen = req.ExpectedGen
	  }
	   // 重新读最新 effect
	  effect, _ = e.store.GetEffectByID(ctx, effect.ID)
	   return MarkResult{Effect: effect, Gen: genRec}, nil
   }

// Nack 在 handler 失败后调用（非 rate limit 错误）。
// DeliveryGen → nacked；累计重试次数；超限则触发 Worker 进毒丸隔离。
// Nack 不产生 DupProof（handler 失败 ≠ 双花）。

   func (e *CASEngine) Nack(ctx context.Context, req MarkRequest, cause RejectCode) error {
	   if cause == RejectHTTPRateLimit {
		  // HTTP 限速不计入资金 exactly-once 重试计数，直接返回
		   return nil
	   }
	  now := e.clock()
	   genRec, err := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
	  if err != nil {
		   return fmt.Errorf("fxonce: nack get gen: %w", err)
	  }
	  newRetry := genRec.RetryCount + 1
	  newState := GenNacked
	   if newRetry >= genRec.MaxRetry {
		  newState = GenPoisoned
		   // 同步毒丸 effect
		   effectKey := MakeEffectKey(req.BusinessKey, req.ExpectedGen)
		  if eff, eErr := e.store.GetEffect(ctx, effectKey); eErr == nil {
			  _ = e.store.MarkPoisoned(ctx, eff.ID, newRetry, now)
		   }
	   }
	   _ = e.store.UpdateGenState(ctx, genRec.ID, newState, 0, now, cause)
	  return nil
  }

// ForceReplay 双人授权解除毒丸，将 effect 重置为 proposed。
// actors 形态：actor1+actor2（两人必须不同）。

   func (e *CASEngine) ForceReplay(ctx context.Context, effectID string, actors string) error {
	   if !validDualActors(actors) {
		  return ErrForceReplayDenied
	   }
	  now := e.clock()
	   return e.store.MarkForceReplay(ctx, effectID, actors, now)
  }

// ────────────────────────────────────────────────────────────────────────────
// 内部工具
// ────────────────────────────────────────────────────────────────────────────

   func (e *CASEngine) lockKey(key string) *sync.Mutex {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   m, ok := e.keyMu[key]
	  if !ok {
		  m = &sync.Mutex{}
		  e.keyMu[key] = m
	  }
	   return m
   }

// buildDupProof 构造并持久化双花证明。

   func (e *CASEngine) buildDupProof(ctx context.Context, req MarkRequest, winnerGen int64, now time.Time) (*DupProof, error) {
	   // 找胜者 effect
	  winnerEffectKey := MakeEffectKey(req.BusinessKey, winnerGen)
	   winnerEffect,
		  _ := e.store.GetEffect(ctx, winnerEffectKey)
	  loserEffectKey := MakeEffectKey(req.BusinessKey, req.ExpectedGen)
	   loserEffect,
		   _ := e.store.GetEffect(ctx, loserEffectKey)
	   winnerDelivery, _ := e.store.GetGen(ctx, req.BusinessKey, winnerGen)
	   loserDelivery, _ := e.store.GetGen(ctx, req.BusinessKey, req.ExpectedGen)
	  var winnerEffectID, loserEffectID string
	  var winnerDeliveryID, loserDeliveryID string
	  var winnerAt time.Time
	  if winnerEffect != nil {
		   winnerEffectID = winnerEffect.ID
		   if winnerEffect.AppliedAt != nil {
			   winnerAt = *winnerEffect.AppliedAt
		   }
	  }
	   if loserEffect != nil {
		  loserEffectID = loserEffect.ID
	  }
	  if winnerDelivery != nil {
		   winnerDeliveryID = winnerDelivery.ID
	  }
	  if loserDelivery != nil {
		   loserDeliveryID = loserDelivery.ID
	   }
	  windowMs := now.Sub(winnerAt).Milliseconds()
	   dup := &DupProof{
		  ID: newDupID(),
		  BusinessKey: req.BusinessKey,
		   Kind:        req.Kind,
		   WinnerGen:   winnerGen,
		  LoserGen:       req.ExpectedGen,
		  WinnerEffectID: winnerEffectID,
		   LoserEffectID:    loserEffectID,
		  WinnerDeliveryID: winnerDeliveryID,
		   LoserDeliveryID: loserDeliveryID,
		   WinnerAt: winnerAt,
		   LoserAt:    now,
		   WindowMs:   windowMs,
		   RejectCode: RejectStalegen,
		  CreatedAt: now,
	  }
	   if err := e.store.PutDupProof(ctx, dup); err != nil {
		  return dup, fmt.Errorf("fxonce: put dup proof: %w", err)
	  }
	   return dup, nil
  }

   func (e *CASEngine) validateMarkReq(req MarkRequest) error {
	  if err := ValidateBusinessKey(req.BusinessKey); err != nil {
		  return err
	   }
	  if req.Kind == "" {
		   return ErrKindNotFund
	  }
	   if req.ExpectedGen <= 0 || req.ExpectedGen > MaxGeneration {
		  return ErrGenOverflow
	   }
	   return nil
   }

// validDualActors 检验双人授权形态（actor1+actor2，两人不同）。
   func validDualActors(actors string) bool {
	  if actors == "" {
		   return false
	   }
	   for i, c := range actors {
		  if c == '+' && i > 0 && i < len(actors)-1 {
			  a1 := actors[:i]
			   a2 := actors[i+1:]
			   return a1 != "" && a2 != "" && a1 != a2
		  }
	   }
	   return false
   }

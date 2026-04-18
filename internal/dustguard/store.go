package dustguard

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// Store 防护数据读写接口。

type Store interface {
	// 信号
	SaveSignal(ctx context.Context, sig DustSignal) error
	ListSignals(ctx context.Context,
		chain, address string, since time.Time) ([]DustSignal, error)
	// 分值
	UpsertScore(ctx context.Context, score AttackScore) error
	GetScore(ctx context.Context, chain, address string) (AttackScore, bool)
	ListScores(ctx context.Context, chain string, minLevel AttackLevel) ([]AttackScore, error)
	// 隔离案
	CreateCase(ctx context.Context, c QuarantineCase) error
	UpdateCase(ctx context.Context, c QuarantineCase) error
	GetCase(ctx context.Context, id string) (QuarantineCase, error)
	GetActiveCase(ctx context.Context, chain, address string) (QuarantineCase, bool)
	ListCases(ctx context.Context, state QuarantineState, chain string) ([]QuarantineCase, error)
	// 轮换计划
	CreatePlan(ctx context.Context, p RotatePlan) error
	UpdatePlan(ctx context.Context, p RotatePlan) error
	GetPlan(ctx context.Context, id string) (RotatePlan, error)
	ListPlans(ctx context.Context, state RotateState, chain string) ([]RotatePlan, error)
	// 策略
	CreatePolicy(ctx context.Context,
		pol PoolGuardPolicy) error
	UpdatePolicy(ctx context.Context, pol PoolGuardPolicy) error
	GetPolicy(ctx context.Context,
		id string) (PoolGuardPolicy, error)
	ActivePolicy(ctx context.Context, chain string) (PoolGuardPolicy, bool)
	ListPolicies(ctx context.Context, chain string) ([]PoolGuardPolicy, error)
	// 防护单
	CreateTicket(ctx context.Context,
		t GuardTicket) error
	UpdateTicket(ctx context.Context, t GuardTicket) error
	GetTicket(ctx context.Context, id string) (GuardTicket, error)
	ListTickets(ctx context.Context,
		chain, address string, state TicketState) ([]GuardTicket, error)
	ActiveTicket(ctx context.Context, chain, address, txHash string) (GuardTicket, bool)
	// 审计
	WriteAudit(ctx context.Context, a GuardAudit) error
	ListAudit(ctx context.Context,
		kind AuditKind, refID string) ([]GuardAudit, error)
}

// ─── 内存实现 ────────────────────────────────────────────────────────────
// MemStore 内存实现；所有方法线程安全。非使用。

type MemStore struct {
	mu      sync.Mutex
	signals []DustSignal
	scores  map[string]*AttackScore    // "chain|addr"
	cases   map[string]*QuarantineCase // id
	plans    map[string]*RotatePlan      // id
	policies map[string]*PoolGuardPolicy // id
	tickets  map[string]*GuardTicket     // id
	audits []GuardAudit
	clock  func() time.Time
}

// NewMemStore 构造内存实现。
   func NewMemStore() *MemStore {
	   return &MemStore{
		  scores: make(map[string]*AttackScore),
		  cases:  make(map[string]*QuarantineCase),
		   plans:  make(map[string]*RotatePlan),
		   policies: make(map[string]*PoolGuardPolicy),
		   tickets: make(map[string]*GuardTicket),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func scoreKey(chain, address string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(address))
  }

  func (m *MemStore) SaveSignal(_ context.Context, sig DustSignal) error {
	  if sig.ID == "" {
		   sig.ID = uuid.NewString()
	  }
	  m.mu.Lock()
	   m.signals = append(m.signals, sig)
	  m.mu.Unlock()
	   return nil
  }

   func (m *MemStore) ListSignals(_ context.Context, chain, address string, since time.Time) ([]DustSignal, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	   addr := strings.ToLower(strings.TrimSpace(address))
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []DustSignal
	   for _, s := range m.signals {
		   if strings.ToUpper(s.Chain) == ch && strings.ToLower(s.Address) == addr && !s.SeenAt.Before(since) {
			  out = append(out, s)
		   }
	   }
	  return out, nil
  }

   func (m *MemStore) UpsertScore(_ context.Context, score AttackScore) error {
	  k := scoreKey(score.Chain,
		  score.Address)
	  m.mu.Lock()
	   cp := score
	   m.scores[k] = &cp
	   m.mu.Unlock()
	   return nil
   }

  func (m *MemStore) GetScore(_ context.Context, chain, address string) (AttackScore, bool) {
	  k := scoreKey(chain, address)
	   m.mu.Lock()
	  s, ok := m.scores[k]
	  m.mu.Unlock()
	   if !ok || s == nil {
		  return AttackScore{}, false
	  }
	   return *s, true
  }

   func (m *MemStore) ListScores(_ context.Context, chain string, minLevel AttackLevel) ([]AttackScore, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	   minScore := levelMinScore(minLevel)
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []AttackScore
	   for _, s := range m.scores {
		  if ch != "" && strings.ToUpper(s.Chain) != ch {
			  continue
		   }
		  if s.Score < minScore {
			   continue
		   }
		   out = append(out, *s)
	  }
	  return out, nil
  }

   func (m *MemStore) CreateCase(_ context.Context, c QuarantineCase) error {
	   m.mu.Lock()
	   cp := c
	   m.cases[c.ID] = &cp
	  m.mu.Unlock()
	   return nil
   }

   func (m *MemStore) UpdateCase(_ context.Context, c QuarantineCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.cases[c.ID]; !ok {
		  return ErrCaseNotFound
	  }
	  cp := c
	   m.cases[c.ID] = &cp
	   return nil
   }

  func (m *MemStore) GetCase(_ context.Context, id string) (QuarantineCase, error) {
	  m.mu.Lock()
	   c, ok := m.cases[id]
	  m.mu.Unlock()
	  if !ok || c == nil {
		  return QuarantineCase{}, ErrCaseNotFound
	  }
	   return *c, nil
   }

  func (m *MemStore) GetActiveCase(_ context.Context, chain, address string) (QuarantineCase, bool) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  addr := strings.ToLower(strings.TrimSpace(address))
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, c := range m.cases {
		   if strings.ToUpper(c.Chain) == ch && strings.ToLower(c.Address) == addr && c.IsActive() {
			   return *c, true
		   }
	   }
	   return QuarantineCase{}, false
  }

  func (m *MemStore) ListCases(_ context.Context, state QuarantineState, chain string) ([]QuarantineCase, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []QuarantineCase
	  for _, c := range m.cases {
		  if state != "" && c.State != state {
			  continue
		   }
		   if ch != "" && strings.ToUpper(c.Chain) != ch {
			   continue
		   }
		  out = append(out, *c)
	   }
	  return out, nil
  }

  func (m *MemStore) CreatePlan(_ context.Context, p RotatePlan) error {
	   m.mu.Lock()
	   cp := p
	  m.plans[p.ID] = &cp
	  m.mu.Unlock()
	   return nil
   }

  func (m *MemStore) UpdatePlan(_ context.Context, p RotatePlan) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.plans[p.ID]; !ok {
		  return ErrPlanNotFound
	   }
	  cp := p
	  m.plans[p.ID] = &cp
	  return nil
  }

  func (m *MemStore) GetPlan(_ context.Context, id string) (RotatePlan, error) {
	  m.mu.Lock()
	  p, ok := m.plans[id]
	   m.mu.Unlock()
	  if !ok || p == nil {
		  return RotatePlan{}, ErrPlanNotFound
	  }
	  return *p, nil
   }

  func (m *MemStore) ListPlans(_ context.Context, state RotateState, chain string) ([]RotatePlan, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []RotatePlan
	  for _, p := range m.plans {
		  if state != "" && p.State != state {
			  continue
		   }
		   if ch != "" && strings.ToUpper(p.Chain) != ch {
			  continue
		  }
		  out = append(out, *p)
	  }
	  return out, nil
  }

  func (m *MemStore) CreatePolicy(_ context.Context, pol PoolGuardPolicy) error {
	   m.mu.Lock()
	  cp := pol
	   m.policies[pol.ID] = &cp
	   m.mu.Unlock()
	  return nil
  }

  func (m *MemStore) UpdatePolicy(_ context.Context, pol PoolGuardPolicy) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.policies[pol.ID]; !ok {
		  return ErrPolicyNotFound
	   }
	  cp := pol
	  m.policies[pol.ID] = &cp
	  return nil
  }

  func (m *MemStore) GetPolicy(_ context.Context, id string) (PoolGuardPolicy, error) {
	   m.mu.Lock()
	   p, ok := m.policies[id]
	  m.mu.Unlock()
	  if !ok || p == nil {
		   return PoolGuardPolicy{}, ErrPolicyNotFound
	  }
	  return *p, nil
  }

// ActivePolicy 返回给定链最新一个 active 策略；没有时回退全局。

   func (m *MemStore) ActivePolicy(_ context.Context, chain string) (PoolGuardPolicy, bool) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var bestChain, bestGlobal *PoolGuardPolicy
	  for _, p := range m.policies {
		   if !p.Active {
			  continue
		   }
		  if strings.ToUpper(p.Chain) == ch {
			  if bestChain == nil || p.Version > bestChain.Version {
				  bestChain = p
			  }
		  } else if p.Chain == "" {
			  if bestGlobal == nil || p.Version > bestGlobal.Version {
				  bestGlobal = p
			  }
		   }
	   }
	  if bestChain != nil {
		  return *bestChain, true
	  }
	   if bestGlobal != nil {
		   return *bestGlobal, true
	   }
	  return PoolGuardPolicy{}, false
  }

  func (m *MemStore) ListPolicies(_ context.Context, chain string) ([]PoolGuardPolicy, error) {
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []PoolGuardPolicy
	   for _, p := range m.policies {
		  if ch != "" && strings.ToUpper(p.Chain) != ch && p.Chain != "" {
			  continue
		  }
		  out = append(out, *p)
	  }
	   return out, nil
  }

   func (m *MemStore) CreateTicket(_ context.Context, t GuardTicket) error {
	  m.mu.Lock()
	  cp := t
	  m.tickets[t.ID] = &cp
	  m.mu.Unlock()
	   return nil
  }

   func (m *MemStore) UpdateTicket(_ context.Context, t GuardTicket) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.tickets[t.ID]; !ok {
		   return ErrTicketNotFound
	   }
	   cp := t
	   m.tickets[t.ID] = &cp
	   return nil
  }

   func (m *MemStore) GetTicket(_ context.Context, id string) (GuardTicket, error) {
	  m.mu.Lock()
	  t, ok := m.tickets[id]
	   m.mu.Unlock()
	   if !ok || t == nil {
		  return GuardTicket{}, ErrTicketNotFound
	  }
	  return *t, nil
  }

   func (m *MemStore) ListTickets(_ context.Context, chain, address string, state TicketState) ([]GuardTicket, error) {
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	   addr := strings.ToLower(strings.TrimSpace(address))
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []GuardTicket
	  for _, t := range m.tickets {
		   if ch != "" && strings.ToUpper(t.Chain) != ch {
			  continue
		  }
		   if addr != "" && strings.ToLower(t.Address) != addr {
			   continue
		  }
		   if state != "" && t.State != state {
			  continue
		  }
		   out = append(out, *t)
	  }
	  return out, nil
  }

  func (m *MemStore) ActiveTicket(_ context.Context, chain, address, txHash string) (GuardTicket, bool) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	   addr := strings.ToLower(strings.TrimSpace(address))
	   hash := strings.TrimSpace(txHash)
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   for _, t := range m.tickets {
		   if strings.ToUpper(t.Chain) != ch || strings.ToLower(t.Address) != addr {
			  continue
		   }
		   if hash != "" && t.TxHash != hash {
			   continue
		  }
		  if t.State == TicketOpen || t.State == TicketBlocking {
			  return *t, true
		  }
	  }
	   return GuardTicket{}, false
  }

  func (m *MemStore) WriteAudit(_ context.Context, a GuardAudit) error {
	   if a.ID == "" {
		   a.ID = uuid.NewString()
	   }
	  m.mu.Lock()
	   m.audits = append(m.audits, a)
	  m.mu.Unlock()
	   return nil
   }

   func (m *MemStore) ListAudit(_ context.Context, kind AuditKind, refID string) ([]GuardAudit, error) {
	  ref := strings.TrimSpace(refID)
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []GuardAudit
	  for _, a := range m.audits {
		  if kind != "" && a.Kind != kind {
			   continue
		  }
		  if ref != "" && a.RefID != ref {
			  continue
		   }
		   out = append(out, a)
	  }
	   return out, nil
  }

// ─── 内部辅助 ─────────────────────────────────────────────────────────────

  func levelMinScore(l AttackLevel) int {
	  switch l {
	   case LevelCritical:
		   return 80
	  case LevelAttacking:
		  return 60
	   case LevelSuspicious:
		   return 40
	   default:
		  return 0
	   }
   }

// newAudit 创建一条审计记录（写入前 ID 为空，WriteAudit 补填）。
  func newAudit(kind AuditKind, refID, actor, event, fromState, toState, note string) GuardAudit {
	  return GuardAudit{
		  ID: uuid.NewString(),
		   Kind:  kind,
		   RefID: refID,
		   Actor: actor,
		   Event: event,
		  FromState: fromState,
		   ToState:   toState,
		  Note:      note,
		  FactID:    persist.NewToken(), // 供下游因果图消费者关联事件边
		   At: time.Now().UTC(),
	  }
  }

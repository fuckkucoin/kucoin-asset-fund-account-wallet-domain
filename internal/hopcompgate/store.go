package hopcompgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Store 补偿闸持久化接口，内存实现与 SQL 实现共用该接口。
type Store interface {
	// OpenCase 创建或幂等读取已有补偿仓。
	OpenCase(ctx context.Context, req OpenCaseReq) (*HopCompCase, error)
	// GetCase 按 ID 读取补偿仓。
	GetCase(ctx context.Context, id string) (*HopCompCase, error)
	// GetCaseByIdem 按 hop_idem 读取补偿仓（可能不存在）。
	GetCaseByIdem(ctx context.Context, hopIdem string) (*HopCompCase, error)
	// UpdateCaseState 更新补偿仓状态（带代次检查）。
	UpdateCaseState(ctx context.Context, id string, from, to CaseState, gen int64) (*HopCompCase, error)
	// BumpGeneration 自增世代，仅在 open/blocked→open 过渡时使用。
	BumpGeneration(ctx context.Context, id string) (*HopCompCase, error)
	// ListCases 按过滤条件列出补偿仓。
	ListCases(ctx context.Context, q CaseQueryReq) ([]*HopCompCase, error)
	// SetForceCompensate 记录双人强制补偿操作员信息。
	SetForceCompensate(ctx context.Context,
		id, actor, approver, reason string) (*HopCompCase, error)
	// MarkRetry 记录重试次数（+1）。
	MarkRetry(ctx context.Context,
		id string) (*HopCompCase, error)
	// AcquireLease 申请补偿世代租约。
	AcquireLease(ctx context.Context, req AcquireLeaseReq, now time.Time) (*CompensateLease, error)
	// GetLease 按 ID 读取租约。
	GetLease(ctx context.Context, id string) (*CompensateLease, error)
	// BindLease 将 open 租约迁移到 bound。
	BindLease(ctx context.Context, id, token string) (*CompensateLease, error)
	// CloseLease 将 bound 租约迁移到 done。
	CloseLease(ctx context.Context,
		id string) (*CompensateLease, error)
	// ExpireLease 将 open/bound 租约迁移到 expired（定时器调用）。
	ExpireLease(ctx context.Context, id string, now time.Time) (*CompensateLease, error)
	// ListExpiredLeases 列出到期未完成的租约。
	ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]*CompensateLease, error)
	// WriteProof 写入双移证明（只追加）。
	WriteProof(ctx context.Context, p *DoubleMoveProof) error
	// GetProof 按 ID 读取证明。
	GetProof(ctx context.Context, id string) (*DoubleMoveProof, error)
	// ListProofs 按过滤条件列出证明。
	ListProofs(ctx context.Context, q ProofExportReq) ([]*DoubleMoveProof, error)
	// MarkProofExported 更新 ExportedAt 时间戳。
	MarkProofExported(ctx context.Context, id string, at time.Time) error
}

// ──────────────────────────────────────────────────────────────────────────────
// memStore — 内存实现
// ──────────────────────────────────────────────────────────────────────────────

type memStore struct {
	mu sync.Mutex
	cases map[string]*HopCompCase // id → case
	byIdem   map[string]string           // hop_idem → id
	leases   map[string]*CompensateLease // id → lease
	leByCase map[string][]string         // case_id → []lease_id
	proofs   map[string]*DoubleMoveProof // id → proof
}

// NewMemStore 创建内存仓储（/开发）。
  func NewMemStore() Store {
	   return &memStore{
		   cases:    make(map[string]*HopCompCase),
		  byIdem:   make(map[string]string),
		  leases:   make(map[string]*CompensateLease),
		   leByCase: make(map[string][]string),
		  proofs:   make(map[string]*DoubleMoveProof),
	  }
   }

  func (m *memStore) OpenCase(_ context.Context, req OpenCaseReq) (*HopCompCase, error) {
	  idem := strings.TrimSpace(req.HopIdem)
	  if idem == "" || req.UserID == "" || req.Asset == "" || req.MoveID == "" {
		   return nil, ErrMoveIDMissing
	   }
	  if !req.SameShard {
		  return nil, ErrCrossShardIntent
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if id, ok := m.byIdem[idem]; ok {
		   c := m.cases[id]
		   if c.UserID != req.UserID || c.Asset != req.Asset {
			   return nil, ErrIdemConflict
		  }
		   cp := *c
		  return &cp, nil
	   }
	  now := time.Now().UTC()
	  c := &HopCompCase{
		   ID:      uuid.NewString(),
		  HopIdem: idem,
		  UserID:  req.UserID,
		   Asset:  strings.ToUpper(strings.TrimSpace(req.Asset)),
		   Amount: req.Amount,
		   FromBook: strings.ToLower(strings.TrimSpace(req.FromBook)),
		   ToBook:   strings.ToLower(strings.TrimSpace(req.ToBook)),
		  MoveID: strings.TrimSpace(req.MoveID),
		   BindErr: truncate(req.BindErr,
			  512),
		   State:      CaseOpen,
		   Generation: 1,
		  SameShard:  true,
		  Actor:  strings.TrimSpace(req.Actor),
		  Region: strings.TrimSpace(req.Region),
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   m.cases[c.ID] = c
	  m.byIdem[idem] = c.ID
	  cp := *c
	  return &cp, nil
   }

  func (m *memStore) GetCase(_ context.Context, id string) (*HopCompCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  c,
		  ok := m.cases[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrCaseNotFound
	  }
	  cp := *c
	  return &cp, nil
   }

  func (m *memStore) GetCaseByIdem(_ context.Context, hopIdem string) (*HopCompCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.byIdem[strings.TrimSpace(hopIdem)]
	  if !ok {
		   return nil, ErrCaseNotFound
	  }
	  c := m.cases[id]
	  cp := *c
	  return &cp, nil
  }

   func (m *memStore) UpdateCaseState(_ context.Context, id string, from, to CaseState, gen int64) (*HopCompCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  c, ok := m.cases[strings.TrimSpace(id)]
	  if !ok {
		   return nil, ErrCaseNotFound
	  }
	   if c.State != from {
		  return nil, ErrInvalidTransition
	  }
	  if !from.CanTransition(to) {
		  return nil, ErrInvalidTransition
	  }
	   if c.Generation != gen {
		  return nil, ErrGenerationMismatch
	  }
	   c.State = to
	   c.UpdatedAt = time.Now().UTC()
	  if to == CaseClosed {
		   t := c.UpdatedAt
		   c.ClosedAt = &t
	  }
	  cp := *c
	  return &cp, nil
  }

  func (m *memStore) BumpGeneration(_ context.Context, id string) (*HopCompCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  c,
		   ok := m.cases[strings.TrimSpace(id)]
	   if !ok {
		   return nil, ErrCaseNotFound
	   }
	   c.Generation++
	   c.UpdatedAt = time.Now().UTC()
	   cp := *c
	   return &cp, nil
   }

   func (m *memStore) ListCases(_ context.Context, q CaseQueryReq) ([]*HopCompCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   limit := q.Limit
	   if limit <= 0 {
		   limit = 50
	   }
	  out := make([]*HopCompCase, 0, limit)
	   for _, c := range m.cases {
		  if q.UserID != "" && c.UserID != q.UserID {
			   continue
		   }
		  if q.State != "" && c.State != q.State {
			   continue
		   }
		  if q.HopIdem != "" && c.HopIdem != q.HopIdem {
			   continue
		   }
		   if !q.After.IsZero() && !c.CreatedAt.After(q.After) {
			   continue
		   }
		   if !q.Before.IsZero() && !c.CreatedAt.Before(q.Before) {
			   continue
		  }
		   cp := *c
		   out = append(out, &cp)
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

  func (m *memStore) SetForceCompensate(_ context.Context, id, actor, approver, reason string) (*HopCompCase, error) {
	   if actor == approver {
		  return nil, ErrForceApproverSameAsActor
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.cases[strings.TrimSpace(id)]
	   if !ok {
		   return nil, ErrCaseNotFound
	   }
	  c.ForcedBy = actor + "/" + approver
	   c.ForceReason = truncate(reason, 256)
	   c.UpdatedAt = time.Now().UTC()
	  cp := *c
	  return &cp, nil
  }

   func (m *memStore) MarkRetry(_ context.Context, id string) (*HopCompCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.cases[strings.TrimSpace(id)]
	  if !ok {
		   return nil, ErrCaseNotFound
	  }
	  c.RetryCount++
	   c.UpdatedAt = time.Now().UTC()
	  cp := *c
	  return &cp, nil
  }

  func (m *memStore) AcquireLease(_ context.Context, req AcquireLeaseReq, now time.Time) (*CompensateLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.cases[strings.TrimSpace(req.CaseID)]
	   if !ok {
		  return nil, ErrCaseNotFound
	   }
	   if c.State == CaseClosed {
		   return nil, ErrCaseAlreadyClosed
	   }
	   if c.State == CaseBlocked {
		   return nil, ErrCaseBlocked
	   }
	   ttl := req.TTL
	  if ttl <= 0 {
		  ttl = DefaultLeaseTTL
	   }
	   if ttl > MaxLeaseTTL {
		  ttl = MaxLeaseTTL
	   }
	  l := &CompensateLease{
		  ID:     uuid.NewString(),
		   CaseID: c.ID,
		  Generation: c.Generation,
		  Token:     uuid.NewString(),
		  State:     LeaseOpen,
		   ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
		  CreatedAt: now,
	   }
	  m.leases[l.ID] = l
	  m.leByCase[c.ID] = append(m.leByCase[c.ID],
		   l.ID)
	  cp := *l
	   return &cp, nil
  }

  func (m *memStore) GetLease(_ context.Context, id string) (*CompensateLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	   return &cp, nil
   }

   func (m *memStore) BindLease(_ context.Context, id, token string) (*CompensateLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  if l.State != LeaseOpen {
		  return nil, ErrLeaseExpired
	  }
	   if l.Token != token {
		   return nil, ErrTokenMismatch
	  }
	  now := time.Now().UTC()
	  l.State = LeaseBound
	   l.BoundAt = &now
	   cp := *l
	  return &cp, nil
  }

  func (m *memStore) CloseLease(_ context.Context, id string) (*CompensateLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[strings.TrimSpace(id)]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  if l.State != LeaseBound {
		  return nil, ErrLeaseExpired
	   }
	   now := time.Now().UTC()
	  l.State = LeaseDone
	  l.DoneAt = &now
	  cp := *l
	   return &cp, nil
  }

   func (m *memStore) ExpireLease(_ context.Context, id string, now time.Time) (*CompensateLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	   if l.State == LeaseDone || l.State == LeaseExpired {
		   cp := *l
		   return &cp, nil
	   }
	  l.State = LeaseExpired
	   l.ExpiredAt = &now
	   cp := *l
	   return &cp, nil
  }

   func (m *memStore) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]*CompensateLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 100
	  }
	   out := make([]*CompensateLease, 0, limit)
	  for _, l := range m.leases {
		   if l.State == LeaseDone || l.State == LeaseExpired {
			   continue
		  }
		   if now.After(l.ExpiresAt) {
			  cp := *l
			   out = append(out, &cp)
		  }
		   if len(out) >= limit {
			  break
		   }
	  }
	   return out, nil
  }

   func (m *memStore) WriteProof(_ context.Context, p *DoubleMoveProof) error {
	   if p == nil || p.ID == "" {
		  p = &DoubleMoveProof{}
		  p.ID = uuid.NewString()
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; ok {
		  return nil // 幂等
	  }
	  cp := *p
	  m.proofs[p.ID] = &cp
	   return nil
   }

  func (m *memStore) GetProof(_ context.Context, id string) (*DoubleMoveProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[strings.TrimSpace(id)]
	   if !ok {
		   return nil, ErrCaseNotFound
	   }
	  cp := *p
	   return &cp, nil
  }

   func (m *memStore) ListProofs(_ context.Context, q ProofExportReq) ([]*DoubleMoveProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   limit := q.Limit
	   if limit <= 0 {
		   limit = 100
	   }
	  out := make([]*DoubleMoveProof,
		  0, limit)
	  for _, p := range m.proofs {
		   if q.CaseID != "" && p.CaseID != q.CaseID {
			   continue
		   }
		   if q.Kind != "" && p.Kind != q.Kind {
			   continue
		  }
		   if !q.After.IsZero() && !p.DetectedAt.After(q.After) {
			   continue
		  }
		   cp := *p
		   out = append(out, &cp)
		   if len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
  }

   func (m *memStore) MarkProofExported(_ context.Context, id string, at time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[strings.TrimSpace(id)]
	   if !ok {
		   return ErrCaseNotFound
	  }
	  p.ExportedAt = &at
	   return nil
   }

// truncate 限制字符串长度。
  func truncate(s string, max int) string {
	  if len(s) <= max {
		  return s
	  }
	  return s[:max]
  }

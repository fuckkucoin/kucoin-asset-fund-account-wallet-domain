package wdpartial

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MustNewUUID 生成一个 UUID（辅助函数）。
   func MustNewUUID() string {
	  return uuid.NewString()
  }

// Service wdpartial 综合领域服务，聚合批次残留管理与竞态收口两套能力。

type Service struct {
	// ─── 批次残留管理字段（StellaK）───────────────────────────────────────
	mu       sync.RWMutex
	detector *Detector
	mutex    *MutexManager
	proofMgr *ProofManager
	guard    *ConserveGuard
	book     ResidueBook
	reader   BatchReader
	observer OccupyObserver
	// ─── 竞态收口字段（TianpengLan）──────────────────────────────────────
	holds HoldStore
	fills     FillStore
	leases    LeaseStore
	conserve  ConserveCaseStore
	guardGens GuardGenStore
	// tokensMu 专用于内存模式 FillMutexToken 并发控制；SQL 模式由 DB 行锁替代。
	tokensMu sync.Mutex
	tokens   map[string]*FillMutexToken
	// ─── ShardHold leftover 进程内会计（wdpartial-remgate）─────────────
	shardHolds map[string]*ShardHold
	shardFills map[string]*PartialFill
	byLeg  map[string]string
	byPlan map[string][]string
	fillsByPlan map[string][]string
	remLeases    map[string]*RemReleaseLease
	remConserves map[string]*ConserveCase
	remGuards    map[string]*BroadcastGuardGen
	// ─── 公共 ─────────────────────────────────────────────────────────
	clock func() time.Time
}

// NewService 构造综合服务，两套子系统均使用内存存储（用）。
  func NewService() *Service {
	   book := NewMemResidueBook()
	  proofBook := NewMemProofBook()
	   leaseBook := NewMemLeaseBook()
	   closeoutBook := NewMemCloseoutBook()
	  obs := OccupyObserver(NewMockObserver())
	   reader := BatchReader(&nilBatchReader{})
	  mgr := NewMutexManager(leaseBook)
	  detector := NewDetector(reader, obs, book)
	   proofMgr := NewProofManager(proofBook, book, mgr)
	  guardObj := NewConserveGuard(book, proofBook, closeoutBook, mgr)
	   return &Service{
		   detector: detector,
		  mutex:    mgr,
		  proofMgr: proofMgr,
		  guard:    guardObj,
		  book:     book,
		   reader:   reader,
		  observer: obs,
		   holds: NewMemHoldStore(),
		   fills: NewMemFillStore(),
		  leases:    NewMemLeaseStore(),
		  conserve:  NewMemConserveCaseStore(),
		  guardGens: NewMemGuardGenStore(),
		   tokens: map[string]*FillMutexToken{},
		   shardHolds: map[string]*ShardHold{},
		   shardFills: map[string]*PartialFill{},
		  byLeg:      map[string]string{},
		   byPlan:     map[string][]string{},
		   fillsByPlan: map[string][]string{},
		   remLeases:    map[string]*RemReleaseLease{},
		  remConserves: map[string]*ConserveCase{},
		   remGuards:    map[string]*BroadcastGuardGen{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// NewServiceWith 注入外部 Store 实现（竞态收口模式）。
// 批次残留子系统字段须通过 WithBatchReader / WithObserver 单独配置。
   func NewServiceWith(
	  holds HoldStore,
	  fills FillStore,
	  leases LeaseStore,
	   conserve ConserveCaseStore,
	  guardGens GuardGenStore,
  ) *Service {
	  book := NewMemResidueBook()
	   proofBook := NewMemProofBook()
	   leaseBook := NewMemLeaseBook()
	  closeoutBook := NewMemCloseoutBook()
	  obs := OccupyObserver(NewMockObserver())
	   reader := BatchReader(&nilBatchReader{})
	   mgr := NewMutexManager(leaseBook)
	  detector := NewDetector(reader, obs, book)
	  proofMgr := NewProofManager(proofBook, book, mgr)
	  guardObj := NewConserveGuard(book, proofBook, closeoutBook, mgr)
	  return &Service{
		  detector: detector,
		  mutex: mgr,
		  proofMgr: proofMgr,
		  guard:    guardObj,
		   book:   book,
		  reader: reader,
		   observer: obs,
		   holds:  holds,
		  fills:  fills,
		   leases: leases,
		   conserve:   conserve,
		   guardGens:  guardGens,
		  tokens:     map[string]*FillMutexToken{},
		   shardHolds: map[string]*ShardHold{},
		  shardFills: map[string]*PartialFill{},
		  byLeg:      map[string]string{},
		   byPlan: map[string][]string{},
		   fillsByPlan:  map[string][]string{},
		  remLeases:    map[string]*RemReleaseLease{},
		   remConserves: map[string]*ConserveCase{},
		  remGuards: map[string]*BroadcastGuardGen{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// New 构造内存 Service（含批次残留、store remgate 与 ShardHold leftover）。

   func New() *Service {
	  return NewService()
   }

// WithBatchReader 绑定 BatchReader（环境注入 wdbatch 只读视图）。
  func (s *Service) WithBatchReader(r BatchReader) *Service {
	  if s != nil && r != nil {
		  s.reader = r
		   s.detector = NewDetector(r, s.observer, s.book)
		   s.detector.clock = s.clock
	   }
	   return s
  }

// WithObserver 绑定 OccupyObserver（环境注入冻结/账本观察器）。
   func (s *Service) WithObserver(o OccupyObserver) *Service {
	   if s != nil && o != nil {
		   s.observer = o
		  s.detector = NewDetector(s.reader, o, s.book)
		  s.detector.clock = s.clock
	  }
	   return s
   }

// WithClock 注入时钟（传播到所有子组件）。

  func (s *Service) WithClock(c func() time.Time) *Service {
	   if s == nil || c == nil {
		   return s
	   }
	   s.clock = c
	   s.detector.WithClock(c)
	  s.mutex.WithClock(c)
	  s.proofMgr.WithClock(c)
	   s.guard.WithClock(c)
	  return s
  }

   func (s *Service) now() time.Time {
	  if s.clock != nil {
		   return s.clock()
	   }
	  return time.Now().UTC()
  }

// ─── 残留检测（批次残留管理子系统） ────────────────────────────────────────────
// DetectResidues 对指定批次执行残留检测。
   func (s *Service) DetectResidues(batchID string) (*DetectResult, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  return s.detector.Detect(strings.TrimSpace(batchID))
   }

// ListResidues 查询残留项列表（按过滤器）。
   func (s *Service) ListResidues(f ResidueFilter) ([]*ResidueItem, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  return s.book.ListByBatch(strings.TrimSpace(f.WDBatchID))
  }

// GetResidue 按 ID 获取单个残留项。

   func (s *Service) GetResidue(id string) (*ResidueItem, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.book.Get(strings.TrimSpace(id))
  }

// ─── Resume 互斥世代锁（批次残留管理子系统） ────────────────────────────────────
// AcquireResumeLease 为指定批次获取续提世代锁。
   func (s *Service) AcquireResumeLease(batchID, holder string) (*ResumeLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  return s.mutex.Acquire(strings.TrimSpace(batchID), strings.TrimSpace(holder))
   }

// VerifyLease 校验调用者持有的世代号是否仍有效。

  func (s *Service) VerifyLease(batchID, holder string, gen int64) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  return s.mutex.Verify(strings.TrimSpace(batchID), strings.TrimSpace(holder), gen)
   }

// ReleaseLease 主动释放续提世代锁。
   func (s *Service) ReleaseLease(batchID, holder string, gen int64) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   return s.mutex.Release(strings.TrimSpace(batchID), strings.TrimSpace(holder), gen)
  }

// CurrentLease 查询指定批次当前租约。

   func (s *Service) CurrentLease(batchID string) (*ResumeLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.mutex.CurrentLease(strings.TrimSpace(batchID))
  }

// ─── 释放证明（批次残留管理子系统） ─────────────────────────────────────────────
// RecordReleaseProof 为指定残留项记录释放/升级证明。

  func (s *Service) RecordReleaseProof(req RecordProofReq) (*ReleaseProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.proofMgr.RecordProof(req)
  }

// MarkRetried 将残留项标记为 retried。
   func (s *Service) MarkRetried(residueID, retryBatchID, leaseHolder string, leaseGen int64) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   return s.proofMgr.MarkRetried(residueID, retryBatchID, leaseHolder, leaseGen)
   }

// ─── Conserve 收口（批次残留管理子系统） ─────────────────────────────────────────
// CheckConserve 预检 Conserve 等式（无副作用）。
  func (s *Service) CheckConserve(summary BatchSummary) (*ConserveResult, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   return s.guard.CheckConserve(summary)
  }

// CloseOut 执行批次收口，写入 BatchCloseout。
  func (s *Service) CloseOut(req CloseOutReq) (*BatchCloseout, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.guard.CloseOut(req)
  }

// GetCloseout 获取批次收口单。

   func (s *Service) GetCloseout(batchID string) (*BatchCloseout, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   return s.guard.closeout.GetByBatch(strings.TrimSpace(batchID))
   }

// ─── 内存 ResidueBook 实现 ───────────────────────────────────────────────────
// MemResidueBook 内存 ResidueBook，仅供使用。
type MemResidueBook struct {
	mu sync.RWMutex
	byID map[string]*ResidueItem
	byBatch map[string][]*ResidueItem
	seqKey  map[string]bool
}

// NewMemResidueBook 构造内存 ResidueBook。
  func NewMemResidueBook() *MemResidueBook {
	  return &MemResidueBook{
		  byID: make(map[string]*ResidueItem),
		  byBatch: make(map[string][]*ResidueItem),
		   seqKey:  make(map[string]bool),
	  }
  }

  func (b *MemResidueBook) ExistsByBatchSeq(wdbatchID string, seq int, idemKey string) bool {
	   if b == nil {
		   return false
	  }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	   key := residueSeqKey(wdbatchID, seq, idemKey)
	   return b.seqKey[key]
   }

  func (b *MemResidueBook) Save(items []*ResidueItem) error {
	   if b == nil {
		   return ErrUnavailable
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  for _, it := range items {
		  if _, dup := b.byID[it.ID]; dup {
			  continue
		  }
		  cp := *it
		  b.byID[it.ID] = &cp
		  b.byBatch[it.WDBatchID] = append(b.byBatch[it.WDBatchID], &cp)
		  b.seqKey[residueSeqKey(it.WDBatchID, it.Seq, it.IdemKey)] = true
	  }
	  return nil
   }

  func (b *MemResidueBook) ListByBatch(wdbatchID string) ([]*ResidueItem, error) {
	   if b == nil {
		  return nil, ErrUnavailable
	   }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	   src := b.byBatch[wdbatchID]
	   out := make([]*ResidueItem, len(src))
	   for i, it := range src {
		   cp := *it
		  out[i] = &cp
	  }
	  return out, nil
   }

   func (b *MemResidueBook) Get(id string) (*ResidueItem, error) {
	  if b == nil {
		  return nil, ErrUnavailable
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   it, ok := b.byID[id]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   cp := *it
	   return &cp, nil
  }

   func (b *MemResidueBook) Update(item *ResidueItem) error {
	  if b == nil {
		  return ErrUnavailable
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   cur, ok := b.byID[item.ID]
	  if !ok {
		   return ErrNotFound
	   }
	  cp := *item
	   *cur = cp
	   list := b.byBatch[item.WDBatchID]
	   for i, it := range list {
		  if it.ID == item.ID {
			  list[i] = cur
			  break
		  }
	   }
	   return nil
  }

  func residueSeqKey(batchID string, _ int, idemKey string) string {
	  return batchID + ":" + strings.TrimSpace(idemKey)
   }

// nilBatchReader 空 BatchReader，GetItems 总是返回 ErrBatchNotFound。

type nilBatchReader struct{}

  func (nilBatchReader) BatchItems(_ string) ([]BatchItemView, string, string, string, string, error) {
	   return nil, "", "", "", "", ErrBatchNotFound
   }

// ─── Hold / Fill 登记（竞态收口子系统） ─────────────────────────────────────────
// RegisterHold 登记 Hold 冻结记录。
// SkipLeg 为 true 时须提供 liveHash 并创建 BroadcastGuardGen 绑定。

  func (svc *Service) RegisterHold(ctx context.Context, in HoldInput, liveHash string) (*HoldRecord, error) {
	   now := svc.now()
	   h := &HoldRecord{
		  ID:     uuid.New().String(),
		  PlanID: in.PlanID,
		  UserID: in.UserID,
		  Asset:  in.Asset,
		   Amount: in.Amount,
		   Seq:    in.Seq,
		  Status: HoldStatusOpen,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if in.SkipLeg {
		   if liveHash == "" {
			  return nil, fmt.Errorf("wdpartial: RegisterHold skip leg requires live_hash")
		  }
		  gen, err := svc.createGuardGen(ctx, in.PlanID, liveHash, now)
		   if err != nil {
			   return nil, fmt.Errorf("wdpartial: RegisterHold create guard gen: %w", err)
		   }
		  h.GuardGenID = gen.ID
	  }
	  if err := svc.holds.Create(ctx, h); err != nil {
		   return nil, fmt.Errorf("wdpartial: RegisterHold: %w", err)
	  }
	  return h, nil
  }

  func (svc *Service) createGuardGen(ctx context.Context, planID, liveHash string, now time.Time) (*BroadcastGuardGen, error) {
	   g := &BroadcastGuardGen{
		   ID: uuid.New().String(),
		  PlanID:     planID,
		   LiveHash:   liveHash,
		  Generation: 1,
		   Status:    GuardGenActive,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err := svc.guardGens.Create(ctx, g); err != nil {
		  return nil, err
	  }
	  return g, nil
  }

// RegisterFill 登记部分成交记录（pending 状态，等待 ConfirmFill）。

   func (svc *Service) RegisterFill(ctx context.Context, in FillInput) (*FillRecord, error) {
	   now := svc.now()
	  f := &FillRecord{
		   ID:     uuid.New().String(),
		  PlanID: in.PlanID,
		  HoldID: in.HoldID,
		   UserID: in.UserID,
		  Asset:  in.Asset,
		   Amount: in.Amount,
		   Seq:    in.Seq,
		   Status: FillStatusPending,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := svc.fills.Create(ctx, f); err != nil {
		  return nil, fmt.Errorf("wdpartial: RegisterFill: %w", err)
	  }
	   return f, nil
  }

// ─── 剩余释放租约管理（竞态收口子系统） ────────────────────────────────────────
// IssueLease 颁发 RemReleaseLease。
// 同 planID 若已存在 open 租约则返回 ErrLeaseDuplicate（防止并发双颁）。

  func (svc *Service) IssueStoreLease(ctx context.Context, req IssueLeaseReq) (*RemReleaseLease, error) {
	   now := svc.now()
	  existing, err := svc.leases.GetOpenByPlan(ctx, req.PlanID)
	  if err == nil && existing != nil {
		  return nil, ErrLeaseDuplicate
	  }
	  ttl := req.TTL
	  if ttl <= 0 {
		  ttl = 5 * time.Minute
	  }
	   l := &RemReleaseLease{
		  ID: uuid.New().String(),
		   PlanID:     req.PlanID,
		   UserID:     req.UserID,
		   Generation: 1,
		   Status:     LeaseStatusOpen,
		   ExpiresAt: now.Add(ttl),
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := svc.leases.Create(ctx, l); err != nil {
		   return nil, fmt.Errorf("wdpartial: IssueLease: %w", err)
	   }
	  return l, nil
  }

// ReleasePlanRemainder 释放分片计划的剩余 open Hold。
// 须持有未过期的 open RemReleaseLease；与 FillMutexToken 互斥（持有 Fill 锁时返回 ErrLeaseConflict）。
// 幂等：已全部释放时返回空 releasedHolds 列表，不报错。
// skip 腿 Hold 须核验 BroadcastGuardGen 活跃，失效时返回 ErrGuardGenInvalid。

  func (svc *Service) ReleasePlanRemainderStore(ctx context.Context, req ReleasePlanRemainderReq) (*ReleasePlanRemainderResp, error) {
	  now := svc.now()
	  lease,
		  err := svc.leases.Get(ctx, req.LeaseID)
	   if err != nil {
		  return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder get lease: %w", err)
	   }
	   if lease.PlanID != req.PlanID {
		  return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder lease plan mismatch")
	  }
	   if err := CheckLeaseActive(lease, now); err != nil {
		  return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder check lease: %w", err)
	   }
	  if lease.Generation != req.Generation {
		  return nil, ErrLeaseGenMismatch
	   }
	  if svc.hasActiveFillMutex(req.PlanID, now) {
		  if blockErr := TransitLeaseBlocked(lease, "", now); blockErr == nil {
			   _ = svc.leases.Update(ctx,
				   lease)
		  }
		   return nil, ErrLeaseConflict
	   }
	   if err := TransitLeaseReleasing(lease, now); err != nil {
		   return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder transit releasing: %w", err)
	   }
	  if err := svc.leases.Update(ctx, lease); err != nil {
		   return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder update lease: %w", err)
	   }
	  openHolds, err := svc.holds.ListOpenByPlan(ctx, req.PlanID)
	  if err != nil {
		  return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder list open holds: %w", err)
	   }
	  var releasedIDs []string
	  var totalRemainder decimal.Decimal
	   for _, h := range openHolds {
		   if h.GuardGenID != "" {
			   guardErr := svc.checkGuardGenActive(ctx, h.GuardGenID)
			   if guardErr != nil {
				  _ = TransitLeaseUnblock(lease,
					  now)
				   _ = svc.leases.Update(ctx, lease)
				   return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder guard gen check for hold %s: %w", h.ID, guardErr)
			   }
		   }
		   if err := TransitHoldRelease(h, now); err != nil {
			   if err == ErrHoldAlreadyReleased {
				   continue
			   }
			   return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder transit hold %s: %w", h.ID, err)
		  }
		   if err := svc.holds.Update(ctx, h); err != nil {
			   return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder update hold %s: %w", h.ID, err)
		   }
		  if h.GuardGenID != "" {
			   if err := svc.consumeGuardGen(ctx, h.GuardGenID, now); err != nil {
				   return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder consume guard gen %s: %w", h.GuardGenID, err)
			   }
		  }
		  releasedIDs = append(releasedIDs, h.ID)
		  amt, _ := decimal.NewFromString(h.Amount)
		   totalRemainder = totalRemainder.Add(amt)
	  }
	  if err := TransitLeaseDone(lease, now); err != nil {
		  return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder transit done: %w", err)
	  }
	   if err := svc.leases.Update(ctx, lease); err != nil {
		  return nil, fmt.Errorf("wdpartial: ReleasePlanRemainder update lease done: %w", err)
	   }
	  return &ReleasePlanRemainderResp{
		  PlanID: req.PlanID,
		  LeaseID: req.LeaseID,
		  ReleasedHolds: releasedIDs,
		   Remainder: totalRemainder.String(),
	   }, nil
   }

  func (svc *Service) checkGuardGenActive(ctx context.Context, guardGenID string) error {
	  g, err := svc.guardGens.Get(ctx, guardGenID)
	  if err != nil {
		  return err
	  }
	  return CheckGuardActive(g)
  }

   func (svc *Service) consumeGuardGen(ctx context.Context, guardGenID string, now time.Time) error {
	   g, err := svc.guardGens.Get(ctx, guardGenID)
	  if err != nil {
		   return err
	   }
	   if err := TransitGuardConsume(g, now); err != nil {
		  return err
	   }
	   return svc.guardGens.Update(ctx, g)
  }

// ─── Fill 互斥令牌管理（竞态收口子系统） ────────────────────────────────────────
// IssueFillMutex 颁发 FillMutexToken（ConfirmFill 路径调用）。

  func (svc *Service) IssueFillMutexStore(_ context.Context, req IssueFillMutexReq) (*FillMutexToken, string, error) {
	  now := svc.now()
	  ttl := req.TTL
	   if ttl <= 0 {
		   ttl = 2 * time.Minute
	   }
	  token := uuid.New().String()
	   t := &FillMutexToken{
		   ID:     uuid.New().String(),
		   PlanID: req.PlanID,
		   FillID: req.FillID,
		   Generation: 1,
		   Token:     token,
		   IssuedAt:  now,
		  ExpiresAt: now.Add(ttl),
	   }
	   svc.tokensMu.Lock()
	  svc.tokens[t.ID] = t
	  svc.tokensMu.Unlock()
	   return t, token, nil
  }

   func (svc *Service) hasActiveFillMutex(planID string, now time.Time) bool {
	   svc.tokensMu.Lock()
	  defer svc.tokensMu.Unlock()
	  for _, t := range svc.tokens {
		  if t.PlanID == planID && t.IsValid(now) {
			  return true
		   }
	   }
	  return false
   }

  func (svc *Service) validateFillMutex(tokenID, _ string, planID, fillID string, now time.Time) (*FillMutexToken, error) {
	  svc.tokensMu.Lock()
	   defer svc.tokensMu.Unlock()
	  t, ok := svc.tokens[tokenID]
	  if !ok {
		   return nil, ErrFillMutexNotFound
	   }
	  if t.PlanID != planID || t.FillID != fillID {
		  return nil, ErrFillMutexMismatch
	  }
	   if t.ConsumedAt != nil {
		   return nil, ErrFillMutexConsumed
	  }
	   if !t.IsValid(now) {
		  return nil, ErrFillMutexExpired
	  }
	  return t, nil
   }

   func (svc *Service) consumeFillMutex(tokenID string, now time.Time) error {
	  svc.tokensMu.Lock()
	  defer svc.tokensMu.Unlock()
	  t, ok := svc.tokens[tokenID]
	   if !ok {
		  return ErrFillMutexNotFound
	  }
	   if t.ConsumedAt != nil {
		  return ErrFillMutexConsumed
	  }
	   t.ConsumedAt = &now
	   return nil
  }

// ─── 成交确认与 Hold 扣减（竞态收口子系统） ────────────────────────────────────
// ConfirmFill 成交确认（持有 FillMutexToken 推进）。
   func (svc *Service) ConfirmFillRecord(ctx context.Context, req RemConfirmFillReq) (*FillRecord, error) {
	  now := svc.now()
	   tok,
		   err := svc.validateFillMutex(req.MutexToken, req.MutexToken, req.PlanID, req.FillID, now)
	   if err != nil {
		   return nil, fmt.Errorf("wdpartial: ConfirmFill mutex token: %w", err)
	   }
	   fill, err := svc.fills.Get(ctx, req.FillID)
	   if err != nil {
		  return nil, fmt.Errorf("wdpartial: ConfirmFill get fill: %w", err)
	   }
	  if fill.PlanID != req.PlanID {
		   return nil, fmt.Errorf("wdpartial: ConfirmFill plan mismatch")
	  }
	  if fill.Status == FillStatusConfirmed {
		  return fill, nil
	   }
	  if err := TransitFillConfirm(fill,
		  tok.ID, now); err != nil {
		   return nil, fmt.Errorf("wdpartial: ConfirmFill transit: %w", err)
	  }
	   if err := svc.fills.Update(ctx, fill); err != nil {
		  return nil, fmt.Errorf("wdpartial: ConfirmFill update fill: %w", err)
	  }
	  return fill, nil
  }

// ConsumeHold 扣减 Hold（持有 FillMutexToken，前置无 open ConserveCase）。
  func (svc *Service) ConsumeHoldRecord(ctx context.Context, req RemConsumeHoldReq) (*HoldRecord, error) {
	   now := svc.now()
	   openCase,
		   caseErr := svc.conserve.GetOpenByPlan(ctx, req.PlanID)
	   if caseErr == nil && openCase != nil {
		   return nil, fmt.Errorf("wdpartial: ConsumeHold plan %s: %w (case=%s)", req.PlanID, ErrConserveCaseOpen, openCase.ID)
	  }
	  tok, err := svc.validateFillMutex(req.MutexToken, req.MutexToken, req.PlanID, req.FillID, now)
	  if err != nil {
		   return nil, fmt.Errorf("wdpartial: ConsumeHold mutex token: %w", err)
	   }
	  hold, err := svc.holds.Get(ctx, req.HoldID)
	  if err != nil {
		   return nil, fmt.Errorf("wdpartial: ConsumeHold get hold: %w", err)
	   }
	   if hold.PlanID != req.PlanID {
		  return nil, fmt.Errorf("wdpartial: ConsumeHold hold plan mismatch")
	   }
	   holdAmt,
		  _ := decimal.NewFromString(hold.Amount)
	  consumeAmt, _ := decimal.NewFromString(req.Amount)
	   if consumeAmt.GreaterThan(holdAmt) {
		   return nil, fmt.Errorf("%w: consume %s > hold %s", ErrAmountMismatch, req.Amount, hold.Amount)
	   }
	  if err := TransitHoldConsume(hold, now); err != nil {
		  return nil, fmt.Errorf("wdpartial: ConsumeHold transit: %w", err)
	  }
	   if err := svc.holds.Update(ctx, hold); err != nil {
		   return nil, fmt.Errorf("wdpartial: ConsumeHold update hold: %w", err)
	   }
	   if err := svc.consumeFillMutex(tok.ID, now); err != nil {
		  return nil, fmt.Errorf("wdpartial: ConsumeHold consume mutex: %w", err)
	  }
	  if blockedLease, lErr := svc.getBlockedLease(ctx, req.PlanID); lErr == nil && blockedLease != nil {
		  _ = TransitLeaseUnblock(blockedLease, now)
		   _ = svc.leases.Update(ctx, blockedLease)
	   }
	  return hold, nil
   }

   func (svc *Service) getBlockedLease(ctx context.Context, planID string) (*RemReleaseLease, error) {
	   mem, ok := svc.leases.(*MemLeaseStore)
	   if !ok {
		   return nil, nil
	  }
	  mem.mu.Lock()
	  defer mem.mu.Unlock()
	  for _, r := range mem.rows {
		  if r.PlanID == planID && r.Status == LeaseStatusBlocked {
			   cp := *r
			   return &cp, nil
		   }
	   }
	   return nil, nil
  }

// ─── 守恒校验（竞态收口子系统） ────────────────────────────────────────────────
// PlanConservation 守恒校验。
// 校验：TotalFilled + 待释放 Remainder == TotalHeld。
// 失败时必须创建 ConserveCase（禁止仅返回错误后放行）。

   func (svc *Service) PlanConservationStore(ctx context.Context, req PlanConservationReq) (*PlanConservationResp, error) {
	  now := svc.now()
	  summary, err := svc.computeSummary(ctx, req.PlanID, now)
	  if err != nil {
		   return nil, fmt.Errorf("wdpartial: PlanConservation compute summary: %w", err)
	  }
	   held, _ := decimal.NewFromString(summary.TotalHeld)
	   filled, _ := decimal.NewFromString(summary.TotalFilled)
	   remainder,
		  _ := decimal.NewFromString(summary.Remainder)
	   conserved := !filled.GreaterThan(held)
	   resp := &PlanConservationResp{
		   PlanID:    req.PlanID,
		  Conserved: conserved,
		  Summary:   *summary,
	   }
	   if conserved {
		   return resp, nil
	  }
	   snapBytes, _ := json.Marshal(summary)
	   failReason := fmt.Sprintf("conservation fail: held=%s filled=%s remainder=%s sum=%s",
		   held,
		  filled, remainder, filled.Add(remainder))
	   c := &ConserveCase{
		   ID:     uuid.New().String(),
		   PlanID: req.PlanID,
		  UserID: "",
		   Status:           ConserveCaseOpen,
		   ConserveSnapshot: string(snapBytes),
		   FailReason: failReason,
		   DetectedAt: now,
		   UpdatedAt:  now,
	   }
	   if err := svc.conserve.Create(ctx, c); err != nil {
		  return nil, fmt.Errorf("wdpartial: PlanConservation create conserve case: %w", err)
	  }
	   resp.CaseID = c.ID
	  summary.HasOpenConserveCase = true
	   return resp, ErrConservationFail
  }

   func (svc *Service) computeSummary(ctx context.Context, planID string, now time.Time) (*RemPlanFillSummary, error) {
	  holds,
		   err := svc.holds.GetByPlan(ctx, planID)
	  if err != nil {
		  return nil, err
	   }
	  fills, err := svc.fills.ListConfirmedByPlan(ctx, planID)
	  if err != nil {
		   return nil, err
	   }
	   var totalHeld, totalFilled decimal.Decimal
	   for _, h := range holds {
		  amt, _ := decimal.NewFromString(h.Amount)
		   totalHeld = totalHeld.Add(amt)
	  }
	  for _, f := range fills {
		  amt, _ := decimal.NewFromString(f.Amount)
		  totalFilled = totalFilled.Add(amt)
	  }
	   remainder := totalHeld.Sub(totalFilled)
	  openCase, _ := svc.conserve.GetOpenByPlan(ctx, planID)
	   hasOpenCase := openCase != nil
	   return &RemPlanFillSummary{
		   PlanID:    planID,
		   TotalHeld: totalHeld.String(),
		  TotalFilled: totalFilled.String(),
		  Remainder:   remainder.String(),
		   HoldCount:   len(holds),
		  FillCount:   len(fills),
		  HasOpenConserveCase: hasOpenCase,
		  ComputedAt: now,
	   }, nil
  }

// GetSummary 只读查询分片计划汇总（不触发守恒校验）。

  func (svc *Service) GetSummary(ctx context.Context, planID string) (*RemPlanFillSummary, error) {
	  return svc.computeSummary(ctx, planID, svc.now())
   }

// ─── ConserveCase 管理（竞态收口子系统） ────────────────────────────────────────
// ProveConserveCase 守恒案件证明固化（open→proven）。
   func (svc *Service) ProveConserveCaseStore(ctx context.Context, req ProveConserveCaseReq) (*ConserveCase, error) {
	  now := svc.now()
	   c, err := svc.conserve.Get(ctx, req.CaseID)
	   if err != nil {
		   return nil, err
	  }
	  if err := TransitConserveProven(c, req.ProofPayload, req.Approver, req.Approver2, now); err != nil {
		  return nil, fmt.Errorf("wdpartial: ProveConserveCase: %w", err)
	   }
	  if err := svc.conserve.Update(ctx, c); err != nil {
		  return nil, fmt.Errorf("wdpartial: ProveConserveCase update: %w", err)
	   }
	   return c, nil
   }

// CloseConserveCase 守恒案件收口（proven→closed）。
  func (svc *Service) CloseConserveCaseStore(ctx context.Context, req CloseConserveCaseReq) (*ConserveCase, error) {
	   now := svc.now()
	  c, err := svc.conserve.Get(ctx, req.CaseID)
	   if err != nil {
		  return nil, err
	  }
	   if err := TransitConserveClose(c, req.Approver, now); err != nil {
		  return nil, fmt.Errorf("wdpartial: CloseConserveCase: %w", err)
	   }
	  if err := svc.conserve.Update(ctx, c); err != nil {
		  return nil, fmt.Errorf("wdpartial: CloseConserveCase update: %w", err)
	  }
	   return c, nil
  }

// ForceCloseCase 双人强制收口守恒案件（ops 接口）。

  func (svc *Service) ForceCloseCase(ctx context.Context, req ForceReleaseReq) (*ConserveCase, error) {
	   now := svc.now()
	   c,
		   err := svc.conserve.Get(ctx, req.CaseID)
	   if err != nil {
		   return nil, err
	  }
	  if err := ForceCloseConserveCase(c, req.Reason, req.Approver, req.Approver2, now); err != nil {
		   return nil, fmt.Errorf("wdpartial: ForceCloseCase: %w", err)
	   }
	   if err := svc.conserve.Update(ctx, c); err != nil {
		  return nil, fmt.Errorf("wdpartial: ForceCloseCase update: %w", err)
	  }
	   return c, nil
   }

// ListOpenConserveCases 查询 open 守恒案件列表（Worker 巡检用）。

  func (svc *Service) ListOpenConserveCasesStore(ctx context.Context, limit int) ([]*ConserveCase, error) {
	   return svc.conserve.ListOpen(ctx,
		  limit)
   }

// ─── BroadcastGuardGen 管理（竞态收口子系统） ────────────────────────────────────
// InvalidateGuardGen 使守卫世代失效（live-hash 变更或 FSM 迁移时调用）。

   func (svc *Service) InvalidateGuardGenStore(ctx context.Context, req InvalidateGuardGenReq) (*BroadcastGuardGen, error) {
	   now := svc.now()
	  g, err := svc.guardGens.Get(ctx, req.GuardGenID)
	   if err != nil {
		   return nil, err
	  }
	  if err := TransitGuardInvalidate(g, req.Reason, now); err != nil {
		   return nil, fmt.Errorf("wdpartial: InvalidateGuardGen: %w", err)
	   }
	  if err := svc.guardGens.Update(ctx, g); err != nil {
		  return nil, fmt.Errorf("wdpartial: InvalidateGuardGen update: %w", err)
	   }
	   return g, nil
   }

// GetGuardGen 查询守卫世代。

   func (svc *Service) GetGuardGenStore(ctx context.Context, id string) (*BroadcastGuardGen, error) {
	  return svc.guardGens.Get(ctx, id)
  }

// GetRemReleaseLease 查询剩余释放租约。

   func (svc *Service) GetRemReleaseLeaseStore(ctx context.Context, id string) (*RemReleaseLease, error) {
	  return svc.leases.Get(ctx, id)
   }

// RevokeExpiredLeases 吊销过期但未终态的租约（Worker 调用）。

   func (svc *Service) RevokeExpiredLeasesStore(ctx context.Context, batch int) ([]string, error) {
	   now := svc.now()
	  expired,
		   err := svc.leases.ListExpired(ctx, batch)
	   if err != nil {
		  return nil, err
	  }
	  var revokedIDs []string
	   for _, l := range expired {
		  if err := TransitLeaseRevoked(l, now); err != nil {
			  continue
		   }
		   if err := svc.leases.Update(ctx, l); err != nil {
			   continue
		  }
		  revokedIDs = append(revokedIDs, l.ID)
	   }
	  return revokedIDs, nil
   }

// ClearFillMutexForPlan 清除指定 planID 的全部 FillMutexToken（ops 强制释放路径专用）。

  func (svc *Service) ClearFillMutexForPlan(planID string) {
	  svc.tokensMu.Lock()
	  defer svc.tokensMu.Unlock()
	  for id, t := range svc.tokens {
		  if t.PlanID == planID {
			   delete(svc.tokens, id)
		  }
	   }
  }

// ExportRemReleaseGen 导出当前 rem_release_gen（供他包只读引用）。
  func (svc *Service) ExportRemReleaseGenStore(ctx context.Context, planID string) (int64, error) {
	   l, err := svc.leases.GetOpenByPlan(ctx, planID)
	   if err != nil {
		   return 0, fmt.Errorf("wdpartial: ExportRemReleaseGen: %w", err)
	  }
	   return l.Generation, nil
  }

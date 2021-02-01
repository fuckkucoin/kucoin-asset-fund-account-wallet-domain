// @FileName: engine.go
// @Summary: airimportgate 核心引擎——Import 世代租约开立、相位 CAS 互斥、密封重放挡、法定人数绑定。
// - Engine 不持有 airgap.Service；需要读取 Session 时通过 SessionReader 接口注入。
// - Engine 不改写 airgap.Session 的 Phase；只决策是否允许导入，由上层调用 airgap.ImportSealed。
// - 并发导入互斥：同 Session 只允许一条 open/importing 租约；CAS UpdateLeaseState 保证原子。
// - 相位冲突：若 Session 不在 exported 相位（无法导入），产生 PhaseConflictProof 后拒绝。
// - 密封重放：先查 DigestHex；已存在则拒绝（ErrBlobReplay）。
// - 法定人数：CheckQuorumBind；actual < threshold 则拒绝并终止租约。
// 不变量：
// - BeginImport 返回 nil error 时，租约状态为 importing。
// - CommitImport 后写 SealedBlobDigest + QuorumBind；任意一步失败则 RollbackImport。
// - RollbackImport 将租约推进到 aborted；已终态则幂等。
// @Author: Knox
// @Created: 2021.01.31

package airimportgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	defaultLeaseTTL = 120 * time.Second
	maxLeaseTTL     = 600 * time.Second
)

// SessionReader 只读访问 airgap Session 的接口（避免循环依赖）。
type SessionReader interface {
	// ReadSession 返回 Session 的 Phase、Threshold、Signatures 摘要。
	ReadSession(ctx context.Context, sessionID string) (SessionSnapshot, error)
}

// SessionSnapshot airgap Session 轻量快照（仅供 Engine 决策用）。
type SessionSnapshot struct {
	SessionID string
	Phase     string // airgap.Phase string
	Threshold    int
	ActualQuorum int      // 不同签名人数（由调用方计算）
	Actors       []string // 签名人列表
}

// EngineOption 配置函数。

type EngineOption func(*Engine)

// WithLeaseTTL 设置租约默认 TTL。
   func WithLeaseTTL(d time.Duration) EngineOption {
	  return func(e *Engine) {
		   if d > 0 && d <= maxLeaseTTL {
			  e.leaseTTL = d
		   }
	   }
   }

// WithGenTTL 设置 ImportGen 最大有效期。

   func WithGenTTL(d time.Duration) EngineOption {
	  return func(e *Engine) {
		   if d > 0 && d <= maxGenTTL {
			   e.genTTL = d
		   }
	  }
   }

// WithClock 注入时钟（用）。
   func WithClock(fn func() time.Time) EngineOption {
	   return func(e *Engine) {
		  if fn != nil {
			   e.clock = fn
		   }
	   }
}

// Engine airimportgate 核心引擎。
type Engine struct {
	store Store
	sessions SessionReader
	leaseTTL time.Duration
	genTTL time.Duration
	clock func() time.Time
}

// NewEngine 构造 Engine。
   func NewEngine(store Store, sessions SessionReader, opts ...EngineOption) *Engine {
	   if store == nil {
		   store = NewMemStore()
	   }
	   e := &Engine{
		  store: store,
		  sessions: sessions,
		   leaseTTL: defaultLeaseTTL,
		   genTTL:   defaultGenTTL,
		   clock:    func() time.Time { return time.Now().UTC() },
	  }
	   for _, o := range opts {
		   o(e)
	   }
	   return e
  }

// BeginImportReq BeginImport 请求参数。
type BeginImportReq struct {
	SessionID string
	ImportGen string // air_import_gen，外部传入
	BlobDigest string // sha256(blob) hex，由调用方计算
	Actor    string
	LeaseTTL time.Duration // 0 表示使用默认值
}

// BeginImportResult BeginImport 成功结果。

type BeginImportResult struct {
	Lease *ImportLease
}

// BeginImport 开立导入租约：验证 ImportGen、并发互斥、相位检测、重放检测。
// 成功后租约状态为 importing，调用方可接着执行 airgap.ImportSealed。
  func (e *Engine) BeginImport(ctx context.Context, req BeginImportReq) (BeginImportResult, error) {
	   if e == nil {
		  return BeginImportResult{}, ErrUnavailable
	  }
	  now := e.clock()
	  // 1. 基础参数校验
	   req.SessionID = strings.TrimSpace(req.SessionID)
	  req.Actor = strings.TrimSpace(req.Actor)
	  req.ImportGen = strings.TrimSpace(req.ImportGen)
	   req.BlobDigest = strings.TrimSpace(req.BlobDigest)
	   if req.SessionID == "" {
		  return BeginImportResult{},
			   ErrEmptySessionID
	   }
	  if req.Actor == "" {
		   return BeginImportResult{}, ErrInvalidActor
	  }
	   if req.BlobDigest == "" {
		  return BeginImportResult{},
			   ErrEmptyBlob
	   }
	  // 2. 验证 ImportGen
	   gen, err := ParseImportGen(req.ImportGen)
	   if err != nil {
		   return BeginImportResult{}, err
	   }
	  if err := CheckGenExpiry(gen, e.genTTL, now); err != nil {
		   return BeginImportResult{}, err
	   }
	   // 3. 重放预检——快速失败（SaveDigest 最终写是原子保证，这里是路径）
	  if existing,
		   err := e.store.GetDigestByHex(ctx, req.BlobDigest); err == nil && existing != nil {
		   BlobReplayTotal.WithLabelValues(req.SessionID).Inc()
		  return BeginImportResult{}, ErrBlobReplay
	   }
	   // 4. 并发导入互斥：同 Session 不能有 open/importing 租约
	  actives, err := e.store.ListActiveLeasesForSession(ctx, req.SessionID)
	   if err != nil {
		  return BeginImportResult{}, err
	  }
	  if len(actives) > 0 {
		  return BeginImportResult{}, ErrConcurrentImport
	   }
	  // 5. 相位检测——Session 必须在 exported 相位
	  if e.sessions != nil {
		  snap, serr := e.sessions.ReadSession(ctx, req.SessionID)
		   if serr != nil {
			   return BeginImportResult{}, ErrSessionNotFound
		   }
		  if snap.Phase != "exported" {
			   // 产生相位冲突证明
			   kind := conflictKindForPhase(snap.Phase)
			  conflict := &PhaseConflictProof{
				   ConflictID:   uuid.NewString(),
	   ttl := req.LeaseTTL
	   if ttl <= 0 {
		   ttl = e.leaseTTL
	   }
	   if ttl > maxLeaseTTL {
		  ttl = maxLeaseTTL
	  }
	  lease := &ImportLease{
		  LeaseID:   uuid.NewString(),
		  SessionID: req.SessionID,
		   ImportGen: req.ImportGen,
		   BlobDigest: req.BlobDigest,
		   Actor: req.Actor,
		   State: LeaseOpen,
		  ExpiresAt: now.Add(ttl),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := e.store.SaveLease(ctx, lease); err != nil {
		   return BeginImportResult{}, err
	   }
	   ImportLeaseTotal.WithLabelValues(string(LeaseOpen)).Inc()
	   // 7. CAS open→importing
	   lease.State = LeaseImporting
	  lease.UpdatedAt = e.clock()
	  if err := e.store.UpdateLeaseState(ctx, lease.LeaseID, LeaseOpen, LeaseImporting, lease); err != nil {
		   return BeginImportResult{}, err
	   }
	   ImportLeaseTotal.WithLabelValues(string(LeaseImporting)).Inc()
	  cp := lease.Copy()
	   return BeginImportResult{Lease: &cp}, nil
   }

// CommitImportReq CommitImport 请求参数。

type CommitImportReq struct {
	LeaseID   string
	SessionID string
	ActualQuorum int // 实际签名人数（由调用方从 Envelope 计算）
	Threshold int // Session.Threshold 快照
	BlobDigest string // 再次确认摘要（防调用方传错）
	Actor      string
}

// CommitImport 导入成功后提交：写 SealedBlobDigest + QuorumBind，推进租约 importing→imported。
// 若 QuorumBind 检测失败，自动 RollbackImport 并返回 ErrQuorumDrift。
  func (e *Engine) CommitImport(ctx context.Context, req CommitImportReq) error {
	  if e == nil {
		  return ErrUnavailable
	  }
	   now := e.clock()
	  req.LeaseID = strings.TrimSpace(req.LeaseID)
	  req.Actor = strings.TrimSpace(req.Actor)
	  req.BlobDigest = strings.TrimSpace(req.BlobDigest)
	   if req.LeaseID == "" {
		  return ErrLeaseNotFound
	  }
	   // 1. 加载租约并验证状态
	  lease, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return err
	   }
	  if lease.State.Terminal() {
		   return ErrLeaseTerminal
	   }
	  if lease.State != LeaseImporting {
		  return ErrInvalidTransition
	   }
	  if now.After(lease.ExpiresAt) {
		  _ = e.expireLease(ctx, lease, now)
		  return ErrLeaseExpired
	   }
	  // 2. 摘要一致性确认
	  if req.BlobDigest != "" && req.BlobDigest != lease.BlobDigest {
		   return ErrBlobDigestMismatch
	   }
	  // 3. 法定人数绑定检测
	  threshold := req.Threshold
	   actualQuorum := req.ActualQuorum
	  quorumPassed := actualQuorum >= threshold
	  bind := &QuorumBind{
		  BindID: uuid.NewString(),
		   SessionID: req.SessionID,
		  LeaseID:   req.LeaseID,
		   BindGen:   lease.ImportGen,
		  Threshold:    threshold,
		   ActualQuorum: actualQuorum,
		   QuorumPassed: quorumPassed,
		   BoundBy:   req.Actor,
		   BoundAt:   now,
		   CreatedAt: now,
	   }
	   if err := e.store.SaveBind(ctx, bind); err != nil {
		   return err
	  }
	  if !quorumPassed {
		  QuorumDriftTotal.WithLabelValues(req.SessionID).Inc()
		   _ = e.RollbackImport(ctx, RollbackImportReq{
			   LeaseID: req.LeaseID,
			   Actor:  req.Actor,
			  Reason: "quorum drift: actual < threshold",
		  })
		   return ErrQuorumDrift
	  }
	   // 4. 写 SealedBlobDigest（原子重放防护）
	   digest := &SealedBlobDigest{
		  DigestID: uuid.NewString(),
		   DigestHex: lease.BlobDigest,
		  SessionID: req.SessionID,
		   LeaseID:    req.LeaseID,
		   ImportedBy: req.Actor,
		  ImportedAt: now,
	   }
	  if err := e.store.SaveDigest(ctx, digest); err != nil {
		   // ErrBlobReplay 表示并发竞态重放
		   BlobReplayTotal.WithLabelValues(req.SessionID).Inc()
		  _ = e.RollbackImport(ctx, RollbackImportReq{
			   LeaseID: req.LeaseID,
			  Actor:   req.Actor,
			  Reason: "blob replay detected at commit",
		   })
		  return ErrBlobReplay
	   }
	   // 5. 推进租约 importing→imported
	  t := now
	  lease.State = LeaseImported
	   lease.ImportedAt = &t
	  lease.UpdatedAt = now
	   if err := e.store.UpdateLeaseState(ctx, req.LeaseID, LeaseImporting, LeaseImported, lease); err != nil {
		   return err
	  }
	   ImportLeaseTotal.WithLabelValues(string(LeaseImported)).Inc()
	   return nil
  }

// RollbackImportReq RollbackImport 请求参数。

type RollbackImportReq struct {
	LeaseID string
	Actor  string
	Reason string
}

// RollbackImport 将租约推进到 aborted；已终态则幂等。
   func (e *Engine) RollbackImport(ctx context.Context, req RollbackImportReq) error {
	  if e == nil {
		   return ErrUnavailable
	   }
	   now := e.clock()
	   req.LeaseID = strings.TrimSpace(req.LeaseID)
	   if req.LeaseID == "" {
		  return ErrLeaseNotFound
	  }
	  lease,
		  err := e.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		   return err
	   }
	   if lease.State.Terminal() {
		   return nil // 幂等
	  }
	   t := now
	  lease.State = LeaseAborted
	   lease.AbortedAt = &t
	   lease.AbortReason = strings.TrimSpace(req.Reason)
	  lease.UpdatedAt = now
	   from := lease.State
	  // 重新读取防竞态
	   fresh, ferr := e.store.GetLease(ctx, req.LeaseID)
	   if ferr == nil {
		   from = fresh.State
	   }
	   if from.Terminal() {
		  return nil
	   }
	  lease.State = LeaseAborted
	  if err := e.store.UpdateLeaseState(ctx, req.LeaseID, from, LeaseAborted, lease); err != nil {
		   return err
	   }
	   ImportLeaseTotal.WithLabelValues(string(LeaseAborted)).Inc()
	   return nil
   }

// ForceAbort 双人确认强制中止（ops 路径）。

   func (e *Engine) ForceAbort(ctx context.Context, req ForceAbortReq) error {
	   if e == nil {
		  return ErrUnavailable
	  }
	  req.ConfirmerOne = strings.TrimSpace(req.ConfirmerOne)
	   req.ConfirmerTwo = strings.TrimSpace(req.ConfirmerTwo)
	  if req.ConfirmerOne == "" || req.ConfirmerTwo == "" {
		  return ErrInvalidActor
	  }
	   if strings.EqualFold(req.ConfirmerOne, req.ConfirmerTwo) {
		   return ErrInvalidActor
	   }
	   return e.RollbackImport(ctx, RollbackImportReq{
		  LeaseID: req.LeaseID,
		  Actor:   req.ConfirmerOne + "+" + req.ConfirmerTwo,
		  Reason: "force abort: " + strings.TrimSpace(req.Reason),
	   })
  }

// expireLease 将租约推进到 expired（内部使用）。
   func (e *Engine) expireLease(ctx context.Context, lease *ImportLease, now time.Time) error {
	   t := now
	  lease.State = LeaseExpired
	   lease.AbortedAt = &t
	   lease.AbortReason = "lease expired"
	   lease.UpdatedAt = now
	   from := LeaseImporting
	   fresh, ferr := e.store.GetLease(ctx, lease.LeaseID)
	  if ferr == nil {
		   from = fresh.State
	   }
	   if from.Terminal() {
		  return nil
	  }
	  if err := e.store.UpdateLeaseState(ctx, lease.LeaseID, from, LeaseExpired, lease); err != nil {
		  return err
	   }
	  ImportLeaseTotal.WithLabelValues(string(LeaseExpired)).Inc()
	  return nil
  }

// ProveConflict 将冲突证明推进到 proven 状态。

   func (e *Engine) ProveConflict(ctx context.Context, conflictID string) error {
	   if e == nil {
		   return ErrUnavailable
	   }
	   now := e.clock()
	   t := now
	   p, err := e.store.GetConflict(ctx, conflictID)
	   if err != nil {
		  return err
	   }
	   if p.State != ConflictOpen {
		   return ErrInvalidTransition
	   }
	   p.State = ConflictProven
	   p.ProvenAt = &t
	   p.UpdatedAt = now
	  return e.store.UpdateConflictState(ctx, conflictID, ConflictOpen, ConflictProven, p)
  }

// CloseConflict 关闭已确认的冲突证明。
   func (e *Engine) CloseConflict(ctx context.Context, conflictID string) error {
	   if e == nil {
		  return ErrUnavailable
	   }
	  now := e.clock()
	  t := now
	  p,
		  err := e.store.GetConflict(ctx, conflictID)
	   if err != nil {
		  return err
	   }
	  if p.State != ConflictProven {
		  return ErrInvalidTransition
	  }
	  p.State = ConflictClosed
	  p.ClosedAt = &t
	   p.UpdatedAt = now
	   return e.store.UpdateConflictState(ctx, conflictID, ConflictProven, ConflictClosed, p)
  }

// conflictKindForPhase 根据 Session Phase 推断冲突类型。
   func conflictKindForPhase(phase string) ConflictKind {
	   switch phase {
	   case "aborted":
		  return ConflictImportVsAbort
	  case "closed":
		  return ConflictImportVsClose
	   case "sealed", "exported":
		   return ConflictImportVsExport
	  default:
		  return ConflictImportVsAbort
	  }
  }

// GetLease 查询租约（只读）。

  func (e *Engine) GetLease(ctx context.Context, id string) (*ImportLease, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	  return e.store.GetLease(ctx, id)
  }

// ListActiveLeasesForSession 查询 Session 活跃租约。
  func (e *Engine) ListActiveLeasesForSession(ctx context.Context, sessionID string) ([]*ImportLease, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	   }
	  return e.store.ListActiveLeasesForSession(ctx, sessionID)
  }

// GetConflict 查询冲突证明（只读）。

  func (e *Engine) GetConflict(ctx context.Context, id string) (*PhaseConflictProof, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	   return e.store.GetConflict(ctx, id)
   }

// ListOpenConflicts 查询 Session 开放冲突证明。
  func (e *Engine) ListOpenConflicts(ctx context.Context, sessionID string) ([]*PhaseConflictProof, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  return e.store.ListOpenConflictsBySession(ctx, sessionID)
  }

// GetDigestByHex 查询 blob 摘要记录。
  func (e *Engine) GetDigestByHex(ctx context.Context, hex string) (*SealedBlobDigest, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   return e.store.GetDigestByHex(ctx,
		  hex)
  }

// GetBindByLease 查询法定人数绑定记录。
   func (e *Engine) GetBindByLease(ctx context.Context, leaseID string) (*QuorumBind, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	  }
	   return e.store.GetBindByLease(ctx, leaseID)
   }

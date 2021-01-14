package tracesnapgate

import (
	"context"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/investigate"
	"account-assets.kucoin.net/funding/internal/snap"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// Config Engine 配置项。
type Config struct {
	// SealTTL 快照从创建到必须密封的最大窗口（默认 10 分钟）。
	// 超出 TTL 的 open 快照拒绝 Seal（需重新 CreateSnapshot）。
	SealTTL time.Duration
	// ReplayBatchSize DriftPatrol 每次巡检的快照批量（默认 50）。
	ReplayBatchSize int
	// MaxProofsPerSnapshot 单快照最多保存漂移证明数（防暴力重放膨胀；默认 100）。
	MaxProofsPerSnapshot int
	// AllowLiveExport 是否允许 live 模式读活账（不作法证；仅标 non-forensic；默认 false）。
	// 必须 false；只读场景可设 true。
	AllowLiveExport bool
}

  func (c Config) withDefaults() Config {
	   if c.SealTTL <= 0 {
		  c.SealTTL = 10 * time.Minute
	  }
	   if c.ReplayBatchSize <= 0 {
		  c.ReplayBatchSize = 50
	   }
	  if c.MaxProofsPerSnapshot <= 0 {
		  c.MaxProofsPerSnapshot = 100
	   }
	  return c
  }

// ─── 外部依赖接口 ──────────────────────────────────────────────────────────
// TraceProvider 只读提供 TraceAsset 瞬时结果（不调用账本写路径）。

type TraceProvider interface {
	// TraceHops 获取用户指定资产的划转 hop 列表（只读）。
	TraceHops(ctx context.Context, userID, asset string) ([]investigate.Hop, error)
	// TraceBooks 获取用户指定资产的科目余额视图（只读）。
	TraceBooks(ctx context.Context, userID, asset string) ([]SnapPosition, error)
}

// ─── Engine ────────────────────────────────────────────────────────────────
// Engine 快照世代与法医重放一致性核心引擎。
type Engine struct {
	store    Store
	provider TraceProvider
	cfg Config
	clock func() time.Time
}

// NewEngine 初始化 Engine。
// 参数：
// - store：持久化实现（ SQLStore； MemStore）。
// - provider：只读 Trace 提供者（不写账本）。
// - cfg：Engine 配置。

  func NewEngine(store Store, provider TraceProvider, cfg Config) *Engine {
	   return &Engine{
		  store: store,
		   provider: provider,
		   cfg:      cfg.withDefaults(),
		  clock:    time.Now,
	   }
  }

// WithClock 替换时钟（注入）。

   func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
	   return e
  }

  func (e *Engine) now() time.Time {
	   if e.clock != nil {
		  return e.clock().UTC()
	   }
	  return time.Now().UTC()
  }

// ─── CreateSnapshot ────────────────────────────────────────────────────────
// CreateRequest 创建快照请求。

type CreateRequest struct {
	// UserID 被调查用户（必填）。
	UserID string
	// Asset 追踪资产代码（必填）。
	Asset string
	// Actor 执行操作的运维人员 ID（必填）。
	Actor string
	// IdemKey 幂等键（可选；非空时防重复创建）。
	IdemKey string
}

// CreateSnapshot 基于 TraceAsset 瞬时结果创建 open 快照。
// 幂等：同一 IdemKey 已存在快照时直接返回现有记录。
// 不密封：创建后处于 open，需调用 Seal 固化摘要根。
  func (e *Engine) CreateSnapshot(ctx context.Context, req CreateRequest) (TraceSnapshot, error) {
	  req.UserID = strings.TrimSpace(req.UserID)
	   req.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	  req.Actor = strings.TrimSpace(req.Actor)
	  req.IdemKey = strings.TrimSpace(req.IdemKey)
	   if req.UserID == "" {
		  return TraceSnapshot{}, ErrInvalidUserID
	   }
	   if req.Asset == "" {
		   return TraceSnapshot{}, ErrInvalidAsset
	  }
	  // 幂等检查。
	  if req.IdemKey != "" {
		  if existing, ok, err := e.store.SnapshotByIdemKey(ctx, req.IdemKey); err != nil {
			  return TraceSnapshot{}, err
		   } else if ok {
			   return existing, nil
		   }
	  }
	   // 读取当前 hops 和 books（只读）。
	   hops, err := e.provider.TraceHops(ctx, req.UserID, req.Asset)
	   if err != nil {
		  return TraceSnapshot{}, err
	  }
	  positions, err := e.provider.TraceBooks(ctx, req.UserID, req.Asset)
	   if err != nil {
		  return TraceSnapshot{}, err
	  }
	  now := e.now()
	   s := TraceSnapshot{
		  ID: uuid.NewString(),
		  SnapshotGen: uuid.NewString(),
		   UserID:      req.UserID,
		  Asset: req.Asset,
		  Status:  StatusOpen,
		   Hops:    HopsFromTrace(hops),
		   Books:   BooksFromPositions(positions),
		   Actor:   req.Actor,
		   IdemKey: req.IdemKey,
		   CreatedAt: now,
	  }
	  if err := e.store.PutSnapshot(ctx, s); err != nil {
		  return TraceSnapshot{}, err
	   }
	  return s, nil
  }

// ─── Seal ─────────────────────────────────────────────────────────────────
// SealRequest 密封快照请求。
type SealRequest struct {
	// SnapshotID 目标快照 ID（必填）。
	SnapshotID string
	// Actor 执行密封的操作员 ID（必填）。
	Actor string
}

// Seal 密封快照，固化 TransfersRoot / BooksRoot / AsOf。
// 副作用：
// 1. 计算摘要根并将快照转换为 sealed 状态。
// 2. 将同用户/同资产的其他 sealed/open 快照置 superseded（最多一个有效密封）。
// 失败模式：
// - 快照已密封 → ErrSealedAlready。
// - 快照不在 open → ErrNotOpen。
// - 超出 SealTTL → ErrNotOpen（告知已过期，需重新创建）。
  func (e *Engine) Seal(ctx context.Context, req SealRequest) (TraceSnapshot, error) {
	  req.SnapshotID = strings.TrimSpace(req.SnapshotID)
	   req.Actor = strings.TrimSpace(req.Actor)
	   if req.SnapshotID == "" {
		   return TraceSnapshot{}, ErrNotFound
	   }
	   s, err := e.store.GetSnapshot(ctx, req.SnapshotID)
	  if err != nil {
		   return TraceSnapshot{}, err
	   }
	  if err := CanSeal(&s); err != nil {
		   return TraceSnapshot{}, err
	  }
	  // TTL 检查：open 过期则拒绝，要求重新创建。
	  now := e.now()
	  if now.Sub(s.CreatedAt) > e.cfg.SealTTL {
		   return TraceSnapshot{}, ErrNotOpen
	  }
	   // 计算摘要根。
	   tRoot := ComputeTransfersRoot(s.Hops)
	  bRoot := ComputeBooksRoot(s.Books)
	   ApplySeal(&s, tRoot, bRoot, now)
	   if req.Actor != "" {
		   s.Actor = req.Actor
	   }
	  if err := e.store.PutSnapshot(ctx, s); err != nil {
		   return TraceSnapshot{}, err
	  }
	   // 取代同用户/同资产旧快照（异步友好，失败不回滚密封本身）。
	   _ = e.supersedePrevious(ctx, s.UserID, s.Asset, s.ID, now)
	  return s, nil
   }

// supersedePrevious 将同用户/同资产其他 sealed/open 快照置 superseded。
// 非核心路径，失败打印到错误但不回滚。
  func (e *Engine) supersedePrevious(ctx context.Context, userID, asset, newID string, now time.Time) error {
	  snaps, err := e.store.SnapshotsByUser(ctx, userID, 100)
	  if err != nil {
		  return err
	  }
	   for _, old := range snaps {
		  if old.ID == newID {
			   continue
		   }
		  if old.Asset != asset {
			   continue
		  }
		   if err := CanSupersede(&old); err != nil {
			  continue
		   }
		  ApplySupersede(&old, newID, now)
		   _ = e.store.PutSnapshot(ctx, old)
	   }
	  return nil
   }

// ─── Replay ────────────────────────────────────────────────────────────────
// Replay 按 snapshot_gen 对密封快照做重放复验。
// 行为：
// 1. 从活账重新计算 TransfersRoot / BooksRoot。
// 2. 与密封值核对；一致则返回 ReplayResult{Consistent: true}。
// 3. 不一致则生成 TraceDriftProof，返回 ErrDriftDetected。
// 绝不把活账视图作为法证结论返回给调用方。
// 幂等：相同快照多次 Replay 结果摘要一致（密封值不变）。

   func (e *Engine) Replay(ctx context.Context, snapshotID string) (ReplayResult, error) {
	   snapshotID = strings.TrimSpace(snapshotID)
	  s, err := e.store.GetSnapshot(ctx, snapshotID)
	   if err != nil {
		  return ReplayResult{}, err
	   }
	  if err := CanReplay(&s); err != nil {
		   return ReplayResult{}, err
	  }
	  // 从活账读取最新 hops / books（只读）。
	   liveHops, err := e.provider.TraceHops(ctx, s.UserID, s.Asset)
	  if err != nil {
		   return ReplayResult{}, err
	   }
	   livePositions, err := e.provider.TraceBooks(ctx, s.UserID, s.Asset)
	   if err != nil {
		  return ReplayResult{}, err
	  }
	  liveHopRecords := HopsFromTrace(liveHops)
	  liveBookRecords := BooksFromPositions(livePositions)
	  liveTRoot := ComputeTransfersRoot(liveHopRecords)
	   liveBRoot := ComputeBooksRoot(liveBookRecords)
	   now := e.now()
	   consistent := liveTRoot == s.TransfersRoot && liveBRoot == s.BooksRoot
	  if consistent {
		   return ReplayResult{
			   SnapshotID:  s.ID,
			  SnapshotGen: s.SnapshotGen,
			   Consistent:  true,
			   TransfersRoot: liveTRoot,
			   BooksRoot:     liveBRoot,
			   ReplayedAt:    now,
		   }, nil
	  }
	   // 漂移检测：生成 TraceDriftProof。
	  dt := classifyDrift(s.TransfersRoot, liveTRoot, s.BooksRoot, liveBRoot)
	  proofHash := ComputeProofHash(s.ID,
		  dt, s.TransfersRoot, liveTRoot, s.BooksRoot, liveBRoot)
	   // 幂等：相同 hash 已存在则直接返回现有证明 ID。
	   if existing, ok, perr := e.store.ProofByHash(ctx, proofHash); perr == nil && ok {
		   return ReplayResult{}, &DriftError{
			   ProofID:   existing.ID,
			  ProofHash: proofHash,
			   base:      ErrDriftDetected,
		  }
	  }
	  idem := uuid.NewString()
	   hopEntries := DiffHops(s.Hops, liveHopRecords)
	  bookEntries := DiffBooks(s.Books,
		   liveBookRecords)
	  entries := append(hopEntries, bookEntries...)
	  proof := TraceDriftProof{
		   ID:         uuid.NewString(),
		  SnapshotID: s.ID,
		   SnapshotGen: s.SnapshotGen,
		   UserID:    s.UserID,
		  Asset:     s.Asset,
		  DriftType: dt,
		   SealedTransfersRoot: s.TransfersRoot,
		  LiveTransfersRoot:   liveTRoot,
		   SealedBooksRoot: s.BooksRoot,
		   LiveBooksRoot:   liveBRoot,
		   Entries:         entries,
		   ProofHash: proofHash,
		   DetectedAt: now,
		   IdemKey: idem,
	  }
	  if err := e.store.PutProof(ctx, proof); err != nil {
		   return ReplayResult{}, err
	   }
	   return ReplayResult{}, &DriftError{
		   ProofID:   proof.ID,
		   ProofHash: proofHash,
		  base:      ErrDriftDetected,
	   }
   }

// classifyDrift 按两对根是否一致分类漂移类型。

   func classifyDrift(sealedTR, liveTR, sealedBR, liveBR string) DriftType {
	  tChanged := sealedTR != liveTR
	  bChanged := sealedBR != liveBR
	   switch {
	  case tChanged && bChanged:
		   return DriftBoth
	  case tChanged:
		  return DriftTransfers
	  default:
		   return DriftBooks
	   }
   }

// ─── DriftError ────────────────────────────────────────────────────────────
// DriftError 包装漂移检测错误，携带 ProofID 和 ProofHash。
type DriftError struct {
	ProofID   string
	ProofHash string
	base      error
}

  func (d *DriftError) Error() string {
	   if d.base != nil {
		   return d.base.Error() + " proof_id=" + d.ProofID
	   }
	   return "drift detected proof_id=" + d.ProofID
  }

  func (d *DriftError) Unwrap() error { return d.base }

// IsDriftError 检查 error 是否为漂移错误。
   func IsDriftError(err error) (*DriftError, bool) {
	  var de *DriftError
	   if errors.As(err, &de) {
		   return de, true
	  }
	   return nil, false
  }

// ─── Export ────────────────────────────────────────────────────────────────
// ExportRequest 法医导出请求。
type ExportRequest struct {
	// SnapshotID 目标快照 ID（必填）。
	SnapshotID string
	// Actor 执行导出的操作员 ID（必填）。
	Actor string
}

// Export 密封快照的法医导出（fail-closed）。
// 失败模式：
// - 快照不存在 → ErrNotFound。
// - 快照未密封 → ErrNotSealed。
// - 快照已被取代 → ErrSuperseded。
// - 快照缺 SnapshotGen → ErrNoSnapshot（fail-closed 核心）。
  func (e *Engine) Export(ctx context.Context, req ExportRequest) (ExportBundle, error) {
	   req.SnapshotID = strings.TrimSpace(req.SnapshotID)
	   req.Actor = strings.TrimSpace(req.Actor)
	  if req.SnapshotID == "" {
		   return ExportBundle{}, ErrNotFound
	   }
	  s,
		   err := e.store.GetSnapshot(ctx, req.SnapshotID)
	   if err != nil {
		  return ExportBundle{}, err
	  }
	  // fail-closed：缺 snapshot_gen 拒绝导出。
	   if strings.TrimSpace(s.SnapshotGen) == "" {
		  return ExportBundle{}, ErrNoSnapshot
	   }
	   if err := CanExport(&s); err != nil {
		  return ExportBundle{}, err
	   }
	  now := e.now()
	  ApplyExport(&s, now)
	   if err := e.store.PutSnapshot(ctx, s); err != nil {
		  return ExportBundle{}, err
	  }
	  proofs,
		  _ := e.store.ProofsBySnapshot(ctx, s.ID)
	   binds, _ := e.store.BindsBySnapshot(ctx, s.ID)
	  var bindPtr *InvestigateCaseBind
	   if len(binds) > 0 {
		  b := binds[0]
		   bindPtr = &b
	   }
	   return ExportBundle{
		  Snapshot: s,
		   Bind:     bindPtr,
		   DriftProofs: proofs,
		   ExportedAt:  now,
		  ExportedBy:  req.Actor,
	   }, nil
  }

// ─── BindCase ─────────────────────────────────────────────────────────────
// BindCaseRequest 案件与快照绑定请求。

type BindCaseRequest struct {
	// CaseID 调查案 ID（必填）。
	CaseID string
	// SnapshotID 目标密封快照 ID（必填）。
	SnapshotID string
	// BoundBy 执行绑定的操作员 ID（必填）。
	BoundBy string
}

// BindCase 将调查案与密封快照绑定（一案一绑，幂等）。
// 失败模式：
// - 案件已绑定 → ErrCaseBound。
// - 快照未密封 → ErrNotSealed。

  func (e *Engine) BindCase(ctx context.Context, req BindCaseRequest) (InvestigateCaseBind, error) {
	   req.CaseID = strings.TrimSpace(req.CaseID)
	   req.SnapshotID = strings.TrimSpace(req.SnapshotID)
	  req.BoundBy = strings.TrimSpace(req.BoundBy)
	   if req.CaseID == "" || req.SnapshotID == "" {
		   return InvestigateCaseBind{}, ErrNotFound
	  }
	   // 幂等检查：案件已绑定直接返回现有绑定。
	   if existing, ok, err := e.store.BindByCaseID(ctx, req.CaseID); err != nil {
		   return InvestigateCaseBind{}, err
	   } else if ok {
		   if existing.SnapshotID == req.SnapshotID {
			   return existing, nil
		  }
		   return InvestigateCaseBind{}, ErrCaseBound
	   }
	  s, err := e.store.GetSnapshot(ctx, req.SnapshotID)
	   if err != nil {
		  return InvestigateCaseBind{}, err
	   }
	   if s.Status != StatusSealed && s.Status != StatusExported {
		   return InvestigateCaseBind{}, ErrNotSealed
	  }
	  now := e.now()
	   b := InvestigateCaseBind{
		  ID: uuid.NewString(),
		  CaseID:     req.CaseID,
		   SnapshotID: s.ID,
		  SnapshotGen: s.SnapshotGen,
		  UserID:  s.UserID,
		   Asset:   s.Asset,
		   BoundAt: now,
		  BoundBy: req.BoundBy,
	   }
	  if err := e.store.PutBind(ctx, b); err != nil {
		   return InvestigateCaseBind{}, err
	   }
	  return b, nil
   }

// ─── 查询接口 ─────────────────────────────────────────────────────────────
// GetSnapshot 按 ID 读快照（只读）。

   func (e *Engine) GetSnapshot(ctx context.Context, id string) (TraceSnapshot, error) {
	   return e.store.GetSnapshot(ctx, id)
   }

// ListSnapshotsByUser 读用户快照列表（只读）。
   func (e *Engine) ListSnapshotsByUser(ctx context.Context, userID string, limit int) ([]TraceSnapshot, error) {
	  return e.store.SnapshotsByUser(ctx, userID, limit)
  }

// GetProof 按 ID 读漂移证明（只读）。
   func (e *Engine) GetProof(ctx context.Context, id string) (TraceDriftProof, error) {
	  return e.store.GetProof(ctx, id)
   }

// ListProofsBySnapshot 读快照关联的所有漂移证明（只读）。

   func (e *Engine) ListProofsBySnapshot(ctx context.Context, snapshotID string) ([]TraceDriftProof, error) {
	   return e.store.ProofsBySnapshot(ctx, snapshotID)
  }

// GetBind 按调查案 ID 读快照绑定（只读）。
   func (e *Engine) GetBindByCaseID(ctx context.Context, caseID string) (InvestigateCaseBind, bool, error) {
	   return e.store.BindByCaseID(ctx, caseID)
   }

// ─── LiveTrace（非法医；仅 AllowLiveExport=true 时可用）────────────────────
// LiveTraceResult 活账溯源结果（非法医，必须打标）。

type LiveTraceResult struct {
	// NonForensic 标记：true 表示结果为活账实时视图，不具备法证效力。
	NonForensic bool `json:"non_forensic"`
	// Hops 当前活账 hop 列表。
	Hops []investigate.Hop `json:"hops"`
	// Books 当前活账科目视图。
	Books []snap.Position `json:"books"`
}

// LiveTrace 读取活账溯源（非法医模式；AllowLiveExport=false 时返回 ErrLiveMode）。
// 不变量：返回结果标记 NonForensic=true，调用方禁止将其作为法证导出依据。
   func (e *Engine) LiveTrace(ctx context.Context, userID, asset string) (LiveTraceResult, error) {
	  if !e.cfg.AllowLiveExport {
		  return LiveTraceResult{},
			  ErrLiveMode
	   }
	  userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if userID == "" {
		  return LiveTraceResult{}, ErrInvalidUserID
	  }
	  if asset == "" {
		  return LiveTraceResult{},
			  ErrInvalidAsset
	   }
	   hops, err := e.provider.TraceHops(ctx, userID, asset)
	   if err != nil {
		   return LiveTraceResult{}, err
	   }
	   positions, err := e.provider.TraceBooks(ctx, userID, asset)
	  if err != nil {
		  return LiveTraceResult{}, err
	   }
	   snapPos := make([]snap.Position, 0, len(positions))
	  for _, p := range positions {
		   snapPos = append(snapPos, snap.Position{
			   Book: p.Book,
			  Subject: p.Subject,
			   Asset: p.Asset,
			   Available: p.Available,
			   Frozen: p.Frozen,
		   })
	   }
	   return LiveTraceResult{
		   NonForensic: true,
		   Hops:        hops,
		   Books:       snapPos,
	  }, nil
  }

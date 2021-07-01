package fiatbalgate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// JournalReader fiatjnl Entry 只读查询接口。
// Engine 通过此接口获取凭证列表；不写入 fiatjnl 包任何状态。
type JournalReader interface {
	// ListEntries 返回指定资产的凭证列表（含 Lines），按 created_at 升序。
	// sinceID 不为空时只返回 created_at > sinceEntry 的记录。
	// limit <= 0 时默认 200。
	ListEntries(ctx context.Context, asset string, sinceID string, limit int) ([]JournalEntry, error)
}

// JournalEntry JournalReader 向本包暴露的最小凭证视图（与 fiatjnl.Entry 同构，解耦依赖）。

type JournalEntry struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
	Kind     string `json:"kind"`
	SumDebit string `json:"sum_debit"`
	SumCredit string        `json:"sum_credit"`
	Balanced  bool          `json:"balanced"`
	Lines     []JournalLine `json:"lines"`
	CreatedAt time.Time     `json:"created_at"`
}

// JournalLine 凭证分录行（最小视图）。
type JournalLine struct {
	Asset string `json:"asset"`
	Debit  string `json:"debit"`
	Credit string `json:"credit"`
	Memo string `json:"memo"`
}

// EngineOption Engine 配置选项。

type EngineOption func(*Engine)

// WithClock 注入时钟（用）。

   func WithClock(fn func() time.Time) EngineOption {
	  return func(e *Engine) {
		   if fn != nil {
			  e.clock = fn
		  }
	   }
}

// WithOpenTTL 设置 open→blocking 的最大容忍时间（默认 30s）。
// open 状态超过此时间仍未被 Worker 推进，ExpiryWorker 将强制推进。
  func WithOpenTTL(d time.Duration) EngineOption {
	   return func(e *Engine) {
		   if d > 0 {
			   e.openTTL = d
		  }
	   }
   }

const (
	defaultOpenTTL       = 30 * time.Second
	proofDigestSeparator = "|"
)

// Engine fiatbalgate 核心引擎。

type Engine struct {
	store Store
	reader  JournalReader
	openTTL time.Duration
	clock   func() time.Time
}

// NewEngine 构造 Engine。
// reader 为 nil 时 Scan 操作将跳过（允许纯挡写模式部署）。
   func NewEngine(store Store, reader JournalReader, opts ...EngineOption) *Engine {
	  if store == nil {
		  store = NewMemStore()
	   }
	   e := &Engine{
		   store: store,
		  reader: reader,
		  openTTL: defaultOpenTTL,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
	   for _, o := range opts {
		   o(e)
	  }
	  return e
  }

// ── 世代同步 ──────────────────────────────────────────────────────────────────
// SyncGenReq 世代同步请求（由 fiatjnl.Post 成功后调用方传入）。
type SyncGenReq struct {
	Seq      uint64
	EntryID  string
	Kind     string
	Asset    string
	BumpedAt time.Time
}

// SyncGen 将 fiatjnl 世代快照同步写入 fiat_jnl_gens 表（幂等）。
// fiatjnl.Post 成功后异步调用；失败不影响 Post 主路径。
  func (e *Engine) SyncGen(ctx context.Context, req SyncGenReq) error {
	   if e == nil {
		  return ErrUnavailable
	   }
	  g := &JournalGeneration{
		   GenID:   uuid.NewString(),
		   Seq:     req.Seq,
		   EntryID: req.EntryID,
		  Kind:    req.Kind,
		  Asset: req.Asset,
		   BumpedAt: req.BumpedAt,
		   SyncedAt: e.clock(),
	  }
	  return e.store.SaveGeneration(ctx, g)
   }

// ── 挡写决策 ──────────────────────────────────────────────────────────────────
// CheckBlock 检查指定资产是否存在活跃不平衡案，决定是否挡写。
// fiatjnl.Post 在写入前必须调用此接口；Blocked=true 时必须返回错误给上层。
// 本方法不写任何表（只读），幂等，可高频调用。
  func (e *Engine) CheckBlock(ctx context.Context, asset string, genSeq uint64) (BlockDecision, error) {
	   if e == nil {
		   return BlockDecision{}, ErrUnavailable
	  }
	   if asset == "" {
		  return BlockDecision{}, ErrAssetRequired
	  }
	  has, err := e.store.HasActiveBlockingForAsset(ctx, asset)
	   if err != nil {
		  // fail-closed：Store 不可用时拒绝 Post
		   return BlockDecision{Blocked: true, GenSeq: genSeq, Reason: "store unavailable"}, err
	  }
	  if !has {
		  return BlockDecision{Blocked: false, GenSeq: genSeq}, nil
	   }
	   // 取最新 blocking 案的 ID 填充 Decision
	   cases, err2 := e.store.ListOpenOrBlockingCases(ctx, asset, 1)
	   caseID := ""
	   if err2 == nil && len(cases) > 0 {
		  caseID = cases[0].CaseID
	   }
	  return BlockDecision{
		   Blocked: true,
		  CaseID:  caseID,
		  GenSeq:  genSeq,
		   Reason: "imbalance case blocking",
	   }, &ErrBlockDetail{CaseID: caseID, Asset: asset, State: CaseBlocking}
  }

// ── 开案 ──────────────────────────────────────────────────────────────────────
// OpenCaseReq 开案请求，由扫描器在发现不平衡时构造。

type OpenCaseReq struct {
	Asset string
	ScanGenSeq      uint64
	DetectedEntryID string
	SumDebit        string
	SumCredit string
	ImbalanceKind ImbalanceKind
	ScannerID     string
}

// OpenCase 发现不平衡后开案（open 状态）。
// 若同 asset 已存在 open/blocking 案，返回 ErrAlreadyBlocking（幂等）。

   func (e *Engine) OpenCase(ctx context.Context, req OpenCaseReq) (*FiatImbalanceCase, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   if req.Asset == "" {
		   return nil, ErrAssetRequired
	   }
	   if req.ScannerID == "" {
		   return nil, ErrScannerIDRequired
	  }
	   // 幂等检查：同资产是否已有未清案
	   has,
		   err := e.store.HasActiveBlockingForAsset(ctx, req.Asset)
	  if err != nil {
		  return nil, fmt.Errorf("fiatbalgate open case check: %w", err)
	   }
	  if has {
		   return nil, ErrAlreadyBlocking
	   }
	   now := e.clock()
	   c := &FiatImbalanceCase{
		   CaseID: uuid.NewString(),
		  Asset: req.Asset,
		  ScanGenSeq:      req.ScanGenSeq,
		  DetectedEntryID: req.DetectedEntryID,
		   SumDebit:        req.SumDebit,
		   SumCredit:     req.SumCredit,
		  ImbalanceKind: req.ImbalanceKind,
		  State:     CaseOpen,
		   ScannerID: req.ScannerID,
		  OpenedAt: now,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := e.store.SaveCase(ctx, c); err != nil {
		   return nil, fmt.Errorf("fiatbalgate save case: %w", err)
	   }
	  return c, nil
  }

// ── 推进到 blocking ────────────────────────────────────────────────────────────
// PromoteToBlocking 将 open→blocking，激活挡写标志。由 Worker 或 HTTP ops API 调用。
   func (e *Engine) PromoteToBlocking(ctx context.Context, caseID string) (*FiatImbalanceCase, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	  }
	  now := e.clock()
	   patch := &FiatImbalanceCase{BlockingAt: &now}
	   if err := e.store.UpdateCaseState(ctx, caseID, CaseOpen, CaseBlocking, patch); err != nil {
		   return nil, fmt.Errorf("fiatbalgate promote blocking: %w", err)
	  }
	   return e.store.GetCase(ctx, caseID)
   }

// ── 四眼清障 ──────────────────────────────────────────────────────────────────
// ApproveFirst 第一眼确认：记录 ClearApproval，返回 ClearStepFirst。
// 要求 Case 处于 blocking 状态；若 actor 已确认过，返回 ErrAlreadyApproved。

   func (e *Engine) ApproveFirst(ctx context.Context, req ClearReq) (*ClearApproval, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.Note) == "" || len(strings.TrimSpace(req.Note)) < 10 {
		   return nil, ErrNoteRequired
	   }
	  c, err := e.store.GetCase(ctx, req.CaseID)
	  if err != nil {
		  return nil, err
	   }
	  if c.State == CaseCleared {
		  return nil, ErrCaseCleared
	   }
	  if c.State != CaseBlocking {
		   return nil, ErrCaseNotBlocking
	   }
	   if err := e.checkDuplicateActor(ctx, req.CaseID, req.Actor); err != nil {
		  return nil, err
	  }
	   a := &ClearApproval{
		  ApprovalID: uuid.NewString(),
		   CaseID: req.CaseID,
		   Step: ClearStepFirst,
		  Actor: req.Actor,
		  Note: req.Note,
		  CreatedAt: e.clock(),
	  }
	   if err := e.store.SaveApproval(ctx, a); err != nil {
		  return nil, fmt.Errorf("fiatbalgate save approval: %w", err)
	   }
	   return a, nil
  }

// ApproveSecond 第二眼确认：校验两眼不同操作人，写 Proof，Case→cleared。
   func (e *Engine) ApproveSecond(ctx context.Context, req ClearReq) (*FiatImbalanceProof, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.Note) == "" || len(strings.TrimSpace(req.Note)) < 10 {
		   return nil, ErrNoteRequired
	  }
	  c, err := e.store.GetCase(ctx, req.CaseID)
	   if err != nil {
		  return nil, err
	  }
	  if c.State == CaseCleared {
		  return nil, ErrCaseCleared
	  }
	   if c.State != CaseBlocking {
		   return nil, ErrCaseNotBlocking
	   }
	   approvals, err := e.store.ListApprovalsByCase(ctx, req.CaseID)
	   if err != nil {
		   return nil, err
	   }
	   if len(approvals) < 1 {
		  return nil, fmt.Errorf("fiatbalgate: first approval required before second")
	  }
	   // 禁止自我双签
	  for _, a := range approvals {
		   if a.Actor == req.Actor {
			   return nil, ErrAlreadyApproved
		   }
	  }
	   proof, err := e.buildAndSaveProof(ctx, c, req.Actor, req.Note)
	   if err != nil {
		   return nil, err
	  }
	   return proof, nil
  }

// buildAndSaveProof 构建 FiatImbalanceProof 并落地，然后推进 Case→cleared。
  func (e *Engine) buildAndSaveProof(ctx context.Context, c *FiatImbalanceCase, clearedBy, clearNote string) (*FiatImbalanceProof, error) {
	   var entriesSnap, linesSnap string
	   if e.reader != nil {
		  entries, err := e.reader.ListEntries(ctx, c.Asset, "", 0)
		   if err == nil {
			  eb, _ := json.Marshal(extractEntryIDs(entries))
			  lb, _ := json.Marshal(entries)
			   entriesSnap = string(eb)
			   linesSnap = string(lb)
		  }
	   }
	   digest := computeDigest(entriesSnap, linesSnap)
	   now := e.clock()
	   p := &FiatImbalanceProof{
		  ProofID: uuid.NewString(),
		  CaseID:  c.CaseID,
		  Asset:   c.Asset,
		  ScanGenSeq:      c.ScanGenSeq,
		   EntriesSnapshot: entriesSnap,
		   LinesSnapshot:   linesSnap,
		   SumDebit: c.SumDebit,
		  SumCredit: c.SumCredit,
		  DigestSHA256: digest,
		   State:        ProofPending,
		   CreatedAt:    now,
		  UpdatedAt:    now,
	  }
	   if err := e.store.SaveProof(ctx, p); err != nil {
		  return nil, fmt.Errorf("fiatbalgate save proof: %w", err)
	   }
	   // 推进 Case→cleared（Proof 已落地，Case 更新失败可重试）
	  patch := &FiatImbalanceCase{
		   ClearedAt: &now,
		   ClearedBy: clearedBy,
		  ClearNote: clearNote,
		   ProofID:   p.ProofID,
	  }
	  if err := e.store.UpdateCaseState(ctx, c.CaseID, CaseBlocking, CaseCleared, patch); err != nil {
		  return p, fmt.Errorf("fiatbalgate clear case state: %w", err)
	   }
	   // 同步审批记录第二眼
	  a := &ClearApproval{
		   ApprovalID: uuid.NewString(),
		  CaseID:     c.CaseID,
		   Step:  ClearStepDone,
		  Actor: clearedBy,
		  Note:      clearNote,
		   CreatedAt: now,
	  }
	   _ = e.store.SaveApproval(ctx, a)
	  return p, nil
   }

// checkDuplicateActor 检查同 actor 是否已提交过清障确认。
   func (e *Engine) checkDuplicateActor(ctx context.Context, caseID, actor string) error {
	  approvals,
		   err := e.store.ListApprovalsByCase(ctx, caseID)
	  if err != nil {
		  return err
	  }
	   for _, a := range approvals {
		  if a.Actor == actor {
			   return ErrAlreadyApproved
		  }
	  }
	   return nil
   }

// ── 证明导出 ──────────────────────────────────────────────────────────────────
// ExportProof 将证明状态推进到 exported（pending→exported），返回副本。
// 由 HTTP ops API 的导出端点调用；推进后调用方可序列化 Proof 导出为 JSON/CSV。
   func (e *Engine) ExportProof(ctx context.Context, proofID, actor string) (*FiatImbalanceProof, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   now := e.clock()
	   patch := &FiatImbalanceProof{ExportedAt: &now}
	   if err := e.store.UpdateProofState(ctx, proofID, ProofPending, ProofExported, patch); err != nil {
		  return nil, fmt.Errorf("fiatbalgate export proof: %w", err)
	  }
	  return e.store.GetProof(ctx, proofID)
   }

// VerifyProof 将证明状态推进到 verified（exported→verified）。由二次审计人调用。
   func (e *Engine) VerifyProof(ctx context.Context, proofID, verifiedBy string) (*FiatImbalanceProof, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	  now := e.clock()
	  patch := &FiatImbalanceProof{VerifiedAt: &now, VerifiedBy: verifiedBy}
	  if err := e.store.UpdateProofState(ctx, proofID, ProofExported, ProofVerified, patch); err != nil {
		   return nil, fmt.Errorf("fiatbalgate verify proof: %w", err)
	  }
	  return e.store.GetProof(ctx, proofID)
  }

// ── 辅助 ──────────────────────────────────────────────────────────────────────
// computeDigest 计算 entries + lines 快照的 SHA-256 摘要（hex）。
   func computeDigest(entriesSnap, linesSnap string) string {
	  h := sha256.New()
	  h.Write([]byte(entriesSnap))
	   h.Write([]byte(proofDigestSeparator))
	  h.Write([]byte(linesSnap))
	   return fmt.Sprintf("%x", h.Sum(nil))
  }

// extractEntryIDs 从凭证列表中提取 ID 切片（保持顺序）。
   func extractEntryIDs(entries []JournalEntry) []string {
	  ids := make([]string,
		  0, len(entries))
	   for _, e := range entries {
		  ids = append(ids, e.ID)
	  }
	  return ids
  }

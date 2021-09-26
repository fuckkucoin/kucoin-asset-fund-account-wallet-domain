package overwdlease

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
	"github.com/google/uuid"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// Config Engine 配置项。
type Config struct {
	// BindTTL 预检绑定租约有效期（默认 60s）。
	// PreCheck 通过后必须在此时间内发起 Commit，否则 bind 过期被吊销。
	BindTTL time.Duration
	// GenDriftTolerance LeaseGen 漂移容忍量（默认 0，即不允许任何漂移）。
	// 建议保持 0；仅在链上确认慢时可临时调至 1。
	GenDriftTolerance int64
	// AvailDriftTolerance Available 漂移容忍字符串比较（默认精确匹配）。
	// 为 true 时允许 Available 小幅波动（不推荐，仅用于集成）。
	AvailDriftTolerance bool
}

  func (c Config) withDefaults() Config {
	  if c.BindTTL <= 0 {
		   c.BindTTL = 60 * time.Second
	  }
	   return c
   }

// ─── Engine ────────────────────────────────────────────────────────────────
// Engine 超提屏障预检提交租约核心引擎。

type Engine struct {
	store Store
	cfg   Config
	clock func() time.Time
}

// NewEngine 装配 Engine。

  func NewEngine(store Store, cfg Config) *Engine {
	  return &Engine{
		  store: store,
		  cfg:   cfg.withDefaults(),
		   clock: time.Now,
	  }
   }

// WithClock 替换时钟（供注入）。
  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	  e.clock = fn
	  return e
  }

   // ─── IssueBind ────────────────────────────────────────────────────────────
   // IssueBindRequest 预检绑定签发请求。
  type IssueBindRequest struct {
	  UserID   string          // 发起用户
	   Asset    string          // 币种
	  Amount   string          // 本次出金请求金额（decimal string）
	   Decision BarrierDecision // overwdbar 屏障决策观测（只读）
	   IdemKey string // 幂等键（防重复）
   }

   // IssueBindReply 预检绑定签发结果。

   type IssueBindReply struct {
	  BindID    string    `json:"bind_id"`
	   LeaseGen  int64     `json:"lease_gen"`
	  ExpiresAt time.Time `json:"expires_at"`
	   State     BindState `json:"state"`
   }

  // IssueBind overwdbar.PreCheck 通过后签发预检绑定租约。
   // 前置：调用方已完成 overwdbar.PreCheck（dry-run）并确认 Decision.Allowed==true。
  // 后置：返回 BindID；调用方须将 BindID 随出金请求传递至 CommitBind。
  // 流程：
  // 1. 校验 UserID/Asset/Amount 非空；Decision.Allowed 必须为 true。
   // 2. 幂等检查（IdemKey 已存在则返回已有 bind）。
   // 3. 创建 PrecheckBind（state=open，LeaseGen/ExpectedAvail 锁定，ExpiresAt=now+BindTTL）。

  func (e *Engine) IssueBind(ctx context.Context, req IssueBindRequest) (*IssueBindReply, error) {
	   if req.UserID == "" || req.Asset == "" || req.Amount == "" {
		  return nil, fmt.Errorf("overwdlease: user_id, asset, amount required")
	   }
	  if !req.Decision.Allowed {
		  return nil, fmt.Errorf("overwdlease: cannot issue bind for failed precheck")
	   }
	  // 幂等检查
	   if req.IdemKey != "" {
		  existing, iErr := e.store.GetBindByIdem(ctx, req.IdemKey)
		   if iErr == nil && existing != nil {
			   return bindToReply(existing), nil
		  }
	  }
	   now := e.clock()
	  b := &PrecheckBind{
		   ID:            uuid.NewString(),
		  UserID:        req.UserID,
		  Asset:         req.Asset,
		  Amount:        req.Amount,
		   LeaseGen:      req.Decision.LeaseGen,
		   ExpectedAvail: req.Decision.Available,
		  State: BindStateOpen,
		   IdemKey:    req.IdemKey,
		  PreCheckAt: now,
		   ExpiresAt: now.Add(e.cfg.BindTTL),
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  b.BindHash = computeBindHash(b)
	  if err := e.store.InsertBind(ctx, b); err != nil {
		   if errors.Is(err, ErrDuplicate) && req.IdemKey != "" {
			   existing, _ := e.store.GetBindByIdem(ctx, req.IdemKey)
			   if existing != nil {
				  return bindToReply(existing),
					  nil
			   }
		  }
		  return nil, fmt.Errorf("overwdlease: insert bind: %w", err)
	   }
	   return bindToReply(b), nil
  }

// ─── CommitBind ───────────────────────────────────────────────────────────
// CommitBindRequest Commit 时复验预检绑定请求。

type CommitBindRequest struct {
	BindID string // 必须：预检绑定 ID
	UserID string // 发起用户（用于追加校验）
	Asset string // 币种（用于 open proof 挡写检查及 no_bind 案例）
	CommitRef string           // Commit 单据 ID（出金 ID 等）
	Current   LeaseObservation // Commit 时从 availcas 读取的当前观测值（只读）
}

// CommitBindReply Commit 复验结果。

type CommitBindReply struct {
	BindID   string    `json:"bind_id"`
	LeaseGen int64     `json:"lease_gen"`
	State    BindState `json:"state"`
}

// CommitBind 出金 Commit 前复验预检绑定。
// 安全语义：
// - BindID 为空 → 返回 ErrNoBind（Commit 必须携带 bind_id）。
// - bind 不存在 → 返回 ErrBindNotFound。
// - bind 已过期 → 返回 ErrBindExpired。
// - bind 已吊销 → 返回 ErrBindRevoked。
// - bind 已消费 → 返回 ErrBindConsumed（幂等 ok）。
// - LeaseGen 漂移 → bind→revoked + 升 VarianceProof + ErrOverdrawBlocked。
// - Available 漂移（且 AvailDriftTolerance=false）→ 同上。
// - 同用户同资产存在 open VarianceProof → 返回 ErrProofBlocked。
// - 一切正常 → bind→committed，返回 CommitBindReply。
// 注意：open VarianceProof 检查在读取 bind 之后进行，以便用 bind.Asset 作 key；
// 若调用方提供 req.Asset，则在读取 bind 前可先行快检；
// 若 bind 不存在则不做 proof 检查（bind 缺失本身是错误路径）。
  func (e *Engine) CommitBind(ctx context.Context, req CommitBindRequest) (*CommitBindReply, error) {
	  // 必须携带 bind_id
	  if req.BindID == "" {
		   // 记录无绑定漂移案例（用户直接 Commit 未 PreCheck）
		   _ = e.openDriftCaseNoBind(ctx, req.UserID, req.Asset)
		  return nil, ErrNoBind
	   }
	  // 若调用方已提供 Asset（快检路径），先检查是否存在 open VarianceProof
	   if req.Asset != "" {
		   if proof, pErr := e.store.GetOpenProofByUser(ctx, req.UserID, req.Asset); pErr == nil && proof != nil {
			   return nil, fmt.Errorf("%w: proof_id=%s user=%s asset=%s", ErrProofBlocked, proof.ID, req.UserID, req.Asset)
		   }
	  }
	  b, err := e.store.GetBindByID(ctx, req.BindID)
	   if err != nil {
		  return nil, fmt.Errorf("%w: bind_id=%s", ErrBindNotFound, req.BindID)
	  }
	  // 用 bind.Asset 做精确 open VarianceProof 检查（不依赖调用方是否提供 Asset）
	   if req.Asset == "" || req.Asset != b.Asset {
		   if proof, pErr := e.store.GetOpenProofByUser(ctx, b.UserID, b.Asset); pErr == nil && proof != nil {
			  return nil, fmt.Errorf("%w: proof_id=%s user=%s asset=%s", ErrProofBlocked, proof.ID, b.UserID, b.Asset)
		   }
	  }
	   // 用户一致性校验
	   if req.UserID != "" && b.UserID != req.UserID {
		   return nil, fmt.Errorf("%w: want=%s got=%s", ErrBindUserMismatch, req.UserID, b.UserID)
	   }
	   now := e.clock()
	   // 终态 bind 处理
	  switch b.State {
	  case BindStateCommitted:
		   return &CommitBindReply{BindID: b.ID, LeaseGen: b.LeaseGen, State: b.State}, nil
	  case BindStateExpired:
		   return nil, ErrBindExpired
	   case BindStateRevoked:
		  return nil, ErrBindRevoked
	   }
	   // 过期校验
	  if now.After(b.ExpiresAt) {
		  b.State = BindStateExpired
		  b.UpdatedAt = now
		  _ = e.store.UpdateBind(ctx, b)
		   return nil, ErrBindExpired
	   }
	   // LeaseGen 漂移检测
	   genDrift := req.Current.LeaseGen - b.LeaseGen
	   availDrift := req.Current.Available != b.ExpectedAvail
	   var trigger ProofTrigger
	  drifted := false
	   if genDrift > e.cfg.GenDriftTolerance || genDrift < -e.cfg.GenDriftTolerance {
		  drifted = true
		   if !availDrift || e.cfg.AvailDriftTolerance {
			   trigger = ProofTriggerGenDrift
		   } else {
			  trigger = ProofTriggerBothDrift
		  }
	  } else if availDrift && !e.cfg.AvailDriftTolerance {
		  drifted = true
		  trigger = ProofTriggerAvailDrift
	  }
	  if drifted {
		   // 吊销 bind
		  b.State = BindStateRevoked
		  b.RevokeReason = fmt.Sprintf("lease gen drift=%d avail_drift=%v commit_gen=%d bind_gen=%d",
			   genDrift,
			  availDrift, req.Current.LeaseGen, b.LeaseGen)
		  b.UpdatedAt = now
		  _ = e.store.UpdateBind(ctx, b)
		   // 升差异证明
		  proof,
			   proofErr := e.openVarianceProof(ctx, b, trigger, req.Current, genDrift, now)
		  if proofErr == nil && proof != nil {
			  b.ProofID = proof.ID
			   _ = e.store.UpdateBind(ctx, b)
		  }
		  return nil, fmt.Errorf("%w: bind_id=%s gen_drift=%d avail_drift=%v",
			  ErrOverdrawBlocked, b.ID, genDrift, availDrift)
	   }
	  // 复验通过：消费 bind
	   b.State = BindStateCommitted
	  b.CommittedAt = &now
	  b.CommitRef = req.CommitRef
	   b.UpdatedAt = now
	   if err := e.store.UpdateBind(ctx, b); err != nil {
		   return nil, fmt.Errorf("overwdlease: commit bind update: %w", err)
	  }
	  return &CommitBindReply{BindID: b.ID, LeaseGen: b.LeaseGen, State: b.State}, nil
  }

// ─── RevokeBind ───────────────────────────────────────────────────────────
// RevokeBind 手动吊销 open 状态的 PrecheckBind（异常路径或运维操作）。
   func (e *Engine) RevokeBind(ctx context.Context, bindID, reason string) error {
	  b, err := e.store.GetBindByID(ctx, bindID)
	  if err != nil {
		  return fmt.Errorf("overwdlease: revoke bind: %w", err)
	  }
	  if b.State != BindStateOpen {
		  return nil // 幂等：已是终态则跳过
	  }
	  now := e.clock()
	  b.State = BindStateRevoked
	   b.RevokeReason = reason
	  b.UpdatedAt = now
	  return e.store.UpdateBind(ctx, b)
  }

// ─── ConfirmProof / DismissProof ──────────────────────────────────────────
// ConfirmProofRequest 四眼确认差异证明请求。
type ConfirmProofRequest struct {
	ProofID     string
	ConfirmedBy string
	CloseReason string
}

// ConfirmProof 四眼确认差异证明，关闭证明并解除挡写。
// 四眼确认后，同用户同资产的 Commit 挡写解除；对应漂移案例推进至 closed。

   func (e *Engine) ConfirmProof(ctx context.Context, req ConfirmProofRequest) error {
	   p, err := e.store.GetProofByID(ctx, req.ProofID)
	  if err != nil {
		   return fmt.Errorf("overwdlease: confirm proof: %w", err)
	  }
	  if p.State == ProofStateClosed {
		  return nil // 幂等
	   }
	   if err := p.Transition(ProofStateClosed); err != nil {
		   return err
	   }
	  now := e.clock()
	  p.State = ProofStateClosed
	  p.ConfirmedBy = req.ConfirmedBy
	  p.ConfirmedAt = &now
	   p.CloseReason = req.CloseReason
	  p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		   return fmt.Errorf("overwdlease: confirm proof update: %w", err)
	   }
	   // 关联漂移案例推进
	   if p.DriftCaseID != "" {
		  _ = e.closeDriftCase(ctx, p.DriftCaseID, req.ConfirmedBy, "proof confirmed: "+req.CloseReason, now)
	   }
	  return nil
   }

// DismissProof 驳回差异证明（无需四眼，直接关闭）。
  func (e *Engine) DismissProof(ctx context.Context, proofID, dismissedBy, reason string) error {
	   p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return fmt.Errorf("overwdlease: dismiss proof: %w", err)
	   }
	   if p.State == ProofStateClosed {
		  return nil
	  }
	   if err := p.Transition(ProofStateClosed); err != nil {
		   return err
	  }
	  now := e.clock()
	  p.State = ProofStateClosed
	   p.ConfirmedBy = dismissedBy
	  p.ConfirmedAt = &now
	   p.CloseReason = "dismissed: " + reason
	  p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		  return fmt.Errorf("overwdlease: dismiss proof update: %w", err)
	  }
	   if p.DriftCaseID != "" {
		   _ = e.closeDriftCase(ctx, p.DriftCaseID, dismissedBy, "proof dismissed: "+reason, now)
	   }
	   return nil
  }

// ─── ConfirmDriftCase / DismissDriftCase ──────────────────────────────────
// ConfirmDriftCaseRequest 管理员确认漂移案例请求。

type ConfirmDriftCaseRequest struct {
	CaseID string
	ConfirmedBy string
	CloseReason string
}

// ConfirmDriftCase 管理员确认漂移案例（证明产出后关闭）。
  func (e *Engine) ConfirmDriftCase(ctx context.Context, req ConfirmDriftCaseRequest) error {
	  dc,
		   err := e.store.GetDriftCaseByID(ctx, req.CaseID)
	  if err != nil {
		   return fmt.Errorf("overwdlease: confirm drift case: %w", err)
	   }
	   if dc.State == DriftCaseOpen {
		  return fmt.Errorf("%w: case %s must be proven before confirm-close", ErrProofMissing, req.CaseID)
	  }
	  if dc.State == DriftCaseClosed {
		   return nil
	   }
	   if err := dc.Transition(DriftCaseClosed); err != nil {
		   return err
	  }
	  now := e.clock()
	  dc.State = DriftCaseClosed
	  dc.ConfirmedBy = req.ConfirmedBy
	  dc.ConfirmedAt = &now
	   dc.CloseReason = req.CloseReason
	  dc.UpdatedAt = now
	   return e.store.UpdateDriftCase(ctx, dc)
  }

// DismissDriftCase 驳回漂移案例（直接关闭，无需证明）。

  func (e *Engine) DismissDriftCase(ctx context.Context, caseID, dismissedBy, reason string) error {
	  dc, err := e.store.GetDriftCaseByID(ctx, caseID)
	  if err != nil {
		  return fmt.Errorf("overwdlease: dismiss drift case: %w", err)
	   }
	   if dc.State == DriftCaseClosed {
		  return nil
	  }
	  if err := dc.Transition(DriftCaseClosed); err != nil {
		   return err
	   }
	  now := e.clock()
	  dc.State = DriftCaseClosed
	   dc.ConfirmedBy = dismissedBy
	  dc.ConfirmedAt = &now
	   dc.CloseReason = "dismissed: " + reason
	   dc.UpdatedAt = now
	  return e.store.UpdateDriftCase(ctx, dc)
  }

// ─── ExportProof ──────────────────────────────────────────────────────────
// ProofExport 证明导出摘要（供审计/ops）。

type ProofExport struct {
	ProofID string `json:"proof_id"`
	BindID string `json:"bind_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Trigger string `json:"trigger"`
	BindLeaseGen int64  `json:"bind_lease_gen"`
	CommitGen    int64  `json:"commit_gen"`
	DriftGen     int64  `json:"drift_gen"`
	BindAvail    string `json:"bind_avail"`
	CommitAvail string `json:"commit_avail"`
	State       string `json:"state"`
	ProofHash   string `json:"proof_hash"`
	SealedAt string `json:"sealed_at"`
	DriftCaseID string `json:"drift_case_id"`
}

// ExportProof 导出差异证明摘要。

   func (e *Engine) ExportProof(ctx context.Context, proofID string) (*ProofExport, error) {
	   p, err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		  return nil, fmt.Errorf("overwdlease: export proof: %w", err)
	   }
	   return &ProofExport{
		   ProofID:      p.ID,
		   BindID:       p.BindID,
		   UserID:       p.UserID,
		  Asset:        p.Asset,
		   Trigger:      string(p.Trigger),
		  BindLeaseGen: p.BindLeaseGen,
		   CommitGen:   p.CommitGen,
		  DriftGen:    p.DriftGen,
		   BindAvail:   p.BindAvail,
		   CommitAvail: p.CommitAvail,
		  State:       string(p.State),
		  ProofHash: p.ProofHash,
		  SealedAt: p.SealedAt.Format(time.RFC3339),
		  DriftCaseID: p.DriftCaseID,
	   }, nil
  }

// ─── 查询接口 ─────────────────────────────────────────────────────────────
// GetBind 查询预检绑定租约。
   func (e *Engine) GetBind(ctx context.Context, bindID string) (*PrecheckBind, error) {
	  return e.store.GetBindByID(ctx, bindID)
  }

// GetProof 查询差异证明。
  func (e *Engine) GetProof(ctx context.Context,
	   proofID string) (*VarianceProof, error) {
	   return e.store.GetProofByID(ctx, proofID)
   }

// GetDriftCase 查询漂移案例。

  func (e *Engine) GetDriftCase(ctx context.Context, caseID string) (*OverwdDriftCase, error) {
	  return e.store.GetDriftCaseByID(ctx, caseID)
   }

// GetOpenProofByUser 查询同用户同资产的 open 差异证明（挡写检查）。
   func (e *Engine) GetOpenProofByUser(ctx context.Context, userID, asset string) (*VarianceProof, error) {
	   return e.store.GetOpenProofByUser(ctx, userID, asset)
   }

// ─── 内部工具 ─────────────────────────────────────────────────────────────
// openVarianceProof 开启差异证明（漂移路径）。

  func (e *Engine) openVarianceProof(
	   ctx context.Context,
	   b *PrecheckBind,
	   trigger ProofTrigger,
	  current LeaseObservation,
	  genDrift int64,
	   now time.Time,
  ) (*VarianceProof, error) {
	  idemKey := fmt.Sprintf("variance:%s:%s:%s:%d", b.UserID, b.Asset, b.ID, b.LeaseGen)
	  existing, iErr := e.store.GetProofByIdem(ctx, idemKey)
	   if iErr == nil && existing != nil {
		   return existing, nil // 幂等
	  }
	  sealedAt := now
	  proofID := uuid.NewString()
	  raw := fmt.Sprintf("%s:%s:%s:%d:%d", proofID, b.ID, string(trigger), genDrift, sealedAt.Unix())
	   h := sha256.Sum256([]byte(raw))
	   proofHash := fmt.Sprintf("%x", h)
	  p := &VarianceProof{
		  ID: proofID,
		  BindID: b.ID,
		  UserID: b.UserID,
		  Asset:  b.Asset,
		  Trigger: trigger,
		   BindLeaseGen: b.LeaseGen,
		  CommitGen:    current.LeaseGen,
		  DriftGen:     genDrift,
		   BindAvail:    b.ExpectedAvail,
		   CommitAvail: current.Available,
		  State:     ProofStateOpen,
		  ProofHash: proofHash,
		  SealedAt: sealedAt,
		   IdemKey:  idemKey,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := e.store.InsertProof(ctx, p); err != nil && !errors.Is(err, ErrDuplicate) {
		   return nil, fmt.Errorf("overwdlease: insert variance proof: %w", err)
	  }
	  // 开漂移案例
	   driftCase,
		   _ := e.openDriftCase(ctx, b, p, trigger, genDrift, now)
	   if driftCase != nil {
		   p.DriftCaseID = driftCase.ID
		  _ = e.store.UpdateProof(ctx, p)
	   }
	  return p, nil
  }

// openDriftCase 开启漂移案例（与差异证明关联）。
   func (e *Engine) openDriftCase(
	   ctx context.Context,
	  b *PrecheckBind,
	  proof *VarianceProof,
	   trigger ProofTrigger,
	  genDrift int64,
	   now time.Time,
   ) (*OverwdDriftCase, error) {
	  caseTrigger := proofTriggerToCaseTrigger(trigger)
	  idemKey := fmt.Sprintf("driftcase:%s:%s:%s:%d", b.UserID, b.Asset, b.ID, b.LeaseGen)
	   existing, iErr := e.store.GetDriftCaseByIdem(ctx, idemKey)
	   if iErr == nil && existing != nil {
		   return existing, nil
	   }
	  dc := &OverwdDriftCase{
		  ID: uuid.NewString(),
		  UserID: b.UserID,
		   Asset:   b.Asset,
		   Trigger: caseTrigger,
		   BindID:  b.ID,
		  ProofID: proof.ID,
		  ProofHash: proof.ProofHash,
		  State:     DriftCaseOpen,
		  DriftGen:  genDrift,
		  BlockedAt: now,
		   ScanNote:  fmt.Sprintf("bind_id=%s trigger=%s gen_drift=%d commit_gen=%d bind_gen=%d", b.ID, trigger, genDrift, proof.CommitGen, b.LeaseGen),
		  IdemKey:   idemKey,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := e.store.InsertDriftCase(ctx, dc); err != nil && !errors.Is(err, ErrDuplicate) {
		   return nil, fmt.Errorf("overwdlease: insert drift case: %w", err)
	   }
	  return dc, nil
   }

// openDriftCaseNoBind Commit 无 bind_id 时记录漂移案例。

  func (e *Engine) openDriftCaseNoBind(ctx context.Context, userID, asset string) error {
	  now := e.clock()
	  idemKey := fmt.Sprintf("no_bind:%s:%s:%d", userID, asset, now.UnixNano())
	   dc := &OverwdDriftCase{
		  ID: uuid.NewString(),
		  UserID: userID,
		   Asset:  asset,
		  Trigger:   DriftTriggerNoBind,
		  State:     DriftCaseOpen,
		   BlockedAt: now,
		  ScanNote:  "commit without precheck_bind_id",
		   IdemKey:   idemKey,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := e.store.InsertDriftCase(ctx, dc); err != nil && !errors.Is(err, ErrDuplicate) {
		  return fmt.Errorf("overwdlease: no_bind drift case: %w", err)
	   }
	  return nil
   }

// closeDriftCase 关闭漂移案例（证明关闭后联动）。
   func (e *Engine) closeDriftCase(ctx context.Context, caseID, operator, reason string, now time.Time) error {
	  dc, err := e.store.GetDriftCaseByID(ctx, caseID)
	   if err != nil {
		  return err
	  }
	  if dc.State == DriftCaseClosed {
		   return nil
	  }
	   // 尝试先推进到 proven（如果仍在 open）
	   if dc.State == DriftCaseOpen {
		   dc.State = DriftCaseProven
	   }
	   if tErr := dc.Transition(DriftCaseClosed); tErr != nil {
		   return tErr
	   }
	   dc.State = DriftCaseClosed
	   dc.ConfirmedBy = operator
	  dc.ConfirmedAt = &now
	   dc.CloseReason = reason
	   dc.UpdatedAt = now
	   return e.store.UpdateDriftCase(ctx, dc)
   }

// computeBindHash 计算绑定内容指纹。
   func computeBindHash(b *PrecheckBind) string {
	   raw := fmt.Sprintf("%s:%s:%d:%s:%d", b.UserID, b.Asset, b.LeaseGen, b.ExpectedAvail, b.PreCheckAt.UnixNano())
	  h := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", h)
  }

// bindToReply 将 PrecheckBind 转换为 IssueBindReply。

  func bindToReply(b *PrecheckBind) *IssueBindReply {
	  return &IssueBindReply{
		  BindID:    b.ID,
		   LeaseGen:  b.LeaseGen,
		  ExpiresAt: b.ExpiresAt,
		   State: b.State,
	  }
   }

// proofTriggerToCaseTrigger 将差异证明触发类型映射至漂移案例触发类型。

  func proofTriggerToCaseTrigger(t ProofTrigger) DriftCaseTrigger {
	   switch t {
	  case ProofTriggerGenDrift:
		  return DriftTriggerGenDrift
	   case ProofTriggerAvailDrift:
		  return DriftTriggerAvailDrift
	  default:
		   return DriftTriggerBothDrift
	   }
   }

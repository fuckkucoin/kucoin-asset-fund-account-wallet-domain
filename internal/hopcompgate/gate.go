// 资金路径同片 hop 补偿闸——补偿世代守卫与核心逻辑。
// Gate 是本包对外暴露的主服务接口。调用方（fundpath）在 BindTransfer 失败
// 后调用 Gate.OpenCase；补偿 worker 调用 AcquireLease + ExecuteCompensate；
// 巡检 worker 调用 DetectDoubleMove。
// 关键不变式：
// 1. SameShard=false 时拒绝 OpenCase，本包永远不调用 cross-shard CompensateMove。
// 2. 同一 hop_idem 下仅有一个 HopCompCase；重入时返回已有 Case。
// 3. CompensateMove 必须持有未过期且 State=bound 的租约才能执行。
// 4. 补偿成功后不得再次 CompensateMove（幂等关闭）。

// Author: Dax
// Date: 2021.07.20

package hopcompgate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// MoveExecutor 由 account.Service 提供的只读 hook，本包不直接依赖 account 包。
type MoveExecutor interface {
	// CompensateMove 执行反向 Move，将资金从 ToBook 归还 FromBook。
	// 幂等：相同 idem 多次调用只执行一次。
	CompensateMove(ctx context.Context, req MoveReq) (MoveResult, error)
}

// MoveReq 由本包定义的 Move 请求，与 account.MoveReq 结构相同但独立定义，
// 避免包循环依赖。
type MoveReq struct {
	UserID   string
	FromBook string
	ToBook   string
	Asset    string
	Amount   string
	IdempotencyKey string
}

// MoveResult Move 执行结果。

type MoveResult struct {
	TransferID string
}

// Gate 补偿闸主服务。

type Gate struct {
	store Store
	mover MoveExecutor
	clock func() time.Time
}

// New 创建 Gate，内部使用内存仓储（/开发）。
   func New() *Gate {
	   return &Gate{
		  store: NewMemStore(),
		   clock: time.Now,
	   }
  }

// BindStore 挂载持久化后端。
   func (g *Gate) BindStore(s Store) *Gate {
	  if g != nil && s != nil {
		  g.store = s
	   }
	   return g
   }

// BindMover 挂载 account.Service 的 Move 执行器。

   func (g *Gate) BindMover(m MoveExecutor) *Gate {
	  if g != nil && m != nil {
		   g.mover = m
	  }
	   return g
   }

  func (g *Gate) now() time.Time {
	  if g == nil || g.clock == nil {
		  return time.Now().UTC()
	   }
	   return g.clock().UTC()
  }

// ──────────────────────────────────────────────────────────────────────────────
// OpenCase：BindTransfer 失败时建仓（fundpath 调用）
// ──────────────────────────────────────────────────────────────────────────────
// OpenCase 在 BindTransfer 失败后建立 HopCompCase。
// 若同 hop_idem 已有仓，直接返回（幂等）。
// 跨片 Intent（req.SameShard=false）返回 ErrCrossShardIntent。
   func (g *Gate) OpenCase(ctx context.Context, req OpenCaseReq) (*HopCompCase, error) {
	   if g == nil || g.store == nil {
		   return nil, ErrUnavailable
	  }
	   if !req.SameShard {
		  return nil, ErrCrossShardIntent
	  }
	   if strings.TrimSpace(req.MoveID) == "" {
		  return nil, ErrMoveIDMissing
	  }
	   return g.store.OpenCase(ctx, req)
  }

// ──────────────────────────────────────────────────────────────────────────────
// AcquireLease：申请补偿世代租约
// ──────────────────────────────────────────────────────────────────────────────
// AcquireLease 申请补偿世代租约。
// 返回的 CompensateLease.Token 供 ExecuteCompensate 使用。
   func (g *Gate) AcquireLease(ctx context.Context, req AcquireLeaseReq) (*CompensateLease, error) {
	  if g == nil || g.store == nil {
		  return nil, ErrUnavailable
	   }
	   return g.store.AcquireLease(ctx, req, g.now())
   }

// ──────────────────────────────────────────────────────────────────────────────
// ExecuteCompensate：执行幂等补偿 Move
// ──────────────────────────────────────────────────────────────────────────────
// ExecuteCompensate 持租约执行补偿 Move。
// 调用链：
// 1. 检查 Case 状态（不得为 closed/blocked）。
// 2. 检查 Lease 有效性（未过期、generation 一致、token 一致）。
// 3. 将 Lease 迁移到 bound，Case 迁移到 compensating。
// 4. 调用 mover.CompensateMove（幂等 idem = case_id + gen）。
// 5. 成功：Lease→done，Case→closed。
// 6. 失败：Case→open（retry）或 blocked（超限）；Lease→expired。

  func (g *Gate) ExecuteCompensate(ctx context.Context, req CompensateReq) (*HopCompCase, error) {
	  if g == nil || g.store == nil {
		  return nil, ErrUnavailable
	  }
	   if g.mover == nil {
		  return nil, fmt.Errorf("hopcompgate: mover not bound")
	  }
	   c, err := g.store.GetCase(ctx, req.CaseID)
	  if err != nil {
		   return nil, err
	  }
	   if c.State == CaseClosed {
		  return c, ErrCaseAlreadyClosed
	  }
	   if c.State == CaseBlocked && c.ForcedBy == "" {
		   return c, ErrCaseBlocked
	   }
	  l, err := g.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		   return nil, err
	   }
	  if !l.Valid(g.now()) {
		  return nil, ErrLeaseExpired
	  }
	  if l.Generation != c.Generation {
		  return nil, ErrGenerationMismatch
	   }
	  if l.Token != req.Token {
		   return nil, ErrTokenMismatch
	  }
	  if req.Generation != c.Generation {
		   return nil, ErrGenerationMismatch
	  }
	  // Lease → bound
	  if _, err := g.store.BindLease(ctx, l.ID, l.Token); err != nil {
		   return nil, err
	   }
	  // Case → compensating
	   if _, err := g.store.UpdateCaseState(ctx, c.ID, c.State, CaseCompensating, c.Generation); err != nil {
		   // 回滚 Lease → expired
		  _, _ = g.expireLease(ctx, l.ID)
		   return nil, err
	   }
	  // 构造幂等 Move
	   compIdem := fmt.Sprintf("hopcomp:%s:gen%d", c.ID, c.Generation)
	  moveReq := MoveReq{
		  UserID:   c.UserID,
		   FromBook: c.ToBook,
		   ToBook:         c.FromBook,
		  Asset:          c.Asset,
		  Amount:         c.Amount,
		  IdempotencyKey: compIdem,
	  }
	   _, moveErr := g.mover.CompensateMove(ctx, moveReq)
	  if moveErr == nil {
		  // 成功路径
		   if _, err := g.store.CloseLease(ctx, l.ID); err != nil {
			   // Lease 关闭失败不影响 Case，记录但不阻断。
			   _ = err
		  }
		   updated, err := g.store.UpdateCaseState(ctx, c.ID, CaseCompensating, CaseClosed, c.Generation)
		   if err != nil {
			  return nil, err
		   }
		   return updated, nil
	  }
	   // 失败路径：记录重试，视次数决定下一状态
	   if _, expErr := g.expireLease(ctx, l.ID); expErr != nil {
		  _ = expErr
	  }
	  retried, _ := g.store.MarkRetry(ctx, c.ID)
	  if retried == nil {
		   retried = c
	  }
	   var nextState CaseState
	  curGen := retried.Generation
	  if retried.RetryCount >= MaxRetryCount {
		  nextState = CaseBlocked
	   } else {
		   nextState = CaseOpen
		  // 自增世代，使旧租约全部失效；读取自增后的世代供 UpdateCaseState 使用。
		  if bumped, berr := g.store.BumpGeneration(ctx, c.ID); berr == nil && bumped != nil {
			   curGen = bumped.Generation
		   }
	   }
	   updated, _ := g.store.UpdateCaseState(ctx, c.ID, CaseCompensating, nextState, curGen)
	  if updated == nil {
		  updated = retried
	  }
	   return updated, moveErr
   }

  func (g *Gate) expireLease(ctx context.Context, id string) (*CompensateLease, error) {
	   return g.store.ExpireLease(ctx, id, g.now())
  }

// ──────────────────────────────────────────────────────────────────────────────
// CheckReentry：hop 重入检查
// ──────────────────────────────────────────────────────────────────────────────
// CheckReentry 在 fundpath.Hop 入口处调用，若同 idem 下已有未关闭的 Case 则返回 ErrReentryRejected。
// 若无 Case 或 Case 已 closed，返回 nil（允许通过）。

   func (g *Gate) CheckReentry(ctx context.Context, hopIdem string) error {
	   if g == nil || g.store == nil {
		   return nil
	  }
	  c, err := g.store.GetCaseByIdem(ctx, hopIdem)
	   if err != nil {
		  if err == ErrCaseNotFound {
			   return nil
		  }
		  return err
	   }
	   if c.State == CaseClosed {
		  return nil
	  }
	  return ErrReentryRejected
   }

// ──────────────────────────────────────────────────────────────────────────────
// ForceCompensate：双人确认强制补偿
// ──────────────────────────────────────────────────────────────────────────────
// ForceCompensate 由 ops API 调用，双人确认后解除 blocked 状态并触发一次补偿。
   func (g *Gate) ForceCompensate(ctx context.Context, req ForceCompensateReq) (*HopCompCase, error) {
	  if g == nil || g.store == nil {
		   return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.Actor) == "" || strings.TrimSpace(req.Approver) == "" {
		  return nil, fmt.Errorf("hopcompgate: actor and approver required for force compensate")
	   }
	   if req.Actor == req.Approver {
		  return nil, ErrForceApproverSameAsActor
	  }
	  c, err := g.store.GetCase(ctx, req.CaseID)
	   if err != nil {
		   return nil, err
	  }
	   // 记录强制操作员
	   c, err = g.store.SetForceCompensate(ctx, c.ID, req.Actor, req.Approver, req.Reason)
	  if err != nil {
		  return nil, err
	  }
	  // blocked → open
	   if c.State == CaseBlocked {
		  c, err = g.store.UpdateCaseState(ctx, c.ID, CaseBlocked, CaseOpen, c.Generation)
		   if err != nil {
			  return nil, err
		  }
		   // 强制重试重置计数
		   c.RetryCount = 0
		   // 自增世代使旧租约失效
		  c,
			  err = g.store.BumpGeneration(ctx, c.ID)
		  if err != nil {
			   return nil, err
		  }
	   }
	  return c, nil
  }

// ──────────────────────────────────────────────────────────────────────────────
// DetectDoubleMove：双移巡检
// ──────────────────────────────────────────────────────────────────────────────
// DetectDoubleMove 检测给定 Case 是否存在双移或漏补场景，若发现则写入证明。
// 调用方（worker）传入已知的移动 ID 集合。

  func (g *Gate) DetectDoubleMove(ctx context.Context, caseID string, compensateMoveID, secondMoveID string) (*DoubleMoveProof, error) {
	   if g == nil || g.store == nil {
		   return nil, ErrUnavailable
	  }
	  c, err := g.store.GetCase(ctx, caseID)
	   if err != nil {
		   return nil, err
	  }
	  var kind DoubleMoveKind
	  var evidence string
	   switch {
	   case compensateMoveID != "" && secondMoveID != "":
		  kind = ProofDoubleMove
		   evidence = fmt.Sprintf(
			  "user=%s asset=%s amount=%s: move=%s compensate=%s second=%s; double-move detected",
			   c.UserID, c.Asset, c.Amount, c.MoveID, compensateMoveID, secondMoveID,
		   )
	  case compensateMoveID == "" && c.State != CaseClosed:
		   kind = ProofMissedCompensate
		   evidence = fmt.Sprintf(
			  "user=%s asset=%s amount=%s: move=%s compensate not executed; case=%s state=%s gen=%d",
			   c.UserID, c.Asset, c.Amount, c.MoveID, c.ID, c.State, c.Generation,
		   )
	   default:
		   return nil, nil
	   }
	  proof := &DoubleMoveProof{
		  ID: uuid.NewString(),
		  CaseID: c.ID,
		   Kind:    kind,
		   HopIdem: c.HopIdem,
		   UserID: c.UserID,
		  Asset:  c.Asset,
		   Amount: c.Amount,
		   MoveID:           c.MoveID,
		   CompensateMoveID: compensateMoveID,
		   SecondMoveID:     secondMoveID,
		   Evidence:         truncate(evidence, 2048),
		  DetectedAt:       g.now(),
	  }
	   if err := g.store.WriteProof(ctx, proof); err != nil {
		  return nil, err
	  }
	  return proof, nil
  }

// ──────────────────────────────────────────────────────────────────────────────
// Query helpers
// ──────────────────────────────────────────────────────────────────────────────
// QueryCases 列出补偿仓。

   func (g *Gate) QueryCases(ctx context.Context, q CaseQueryReq) ([]*HopCompCase, error) {
	   if g == nil || g.store == nil {
		  return nil, ErrUnavailable
	  }
	   return g.store.ListCases(ctx, q)
   }

// ExportProofs 列出双移证明，并将 ExportedAt 更新到返回对象中。

  func (g *Gate) ExportProofs(ctx context.Context, q ProofExportReq) ([]*DoubleMoveProof, error) {
	  if g == nil || g.store == nil {
		  return nil, ErrUnavailable
	  }
	  proofs, err := g.store.ListProofs(ctx, q)
	   if err != nil {
		  return nil, err
	  }
	   now := g.now()
	   for _, p := range proofs {
		  _ = g.store.MarkProofExported(ctx,
			   p.ID, now)
		   // 同步更新返回对象，避免调用方看到 nil ExportedAt。
		   p.ExportedAt = &now
	  }
	  return proofs, nil
   }

// GetCase 读取单个补偿仓。

  func (g *Gate) GetCase(ctx context.Context, id string) (*HopCompCase, error) {
	   if g == nil || g.store == nil {
		   return nil, ErrUnavailable
	   }
	   return g.store.GetCase(ctx, id)
   }

// GetLease 读取单个租约。

   func (g *Gate) GetLease(ctx context.Context, id string) (*CompensateLease, error) {
	   if g == nil || g.store == nil {
		  return nil, ErrUnavailable
	  }
	  return g.store.GetLease(ctx, id)
  }

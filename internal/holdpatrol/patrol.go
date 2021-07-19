package holdpatrol

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/holdrel"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// LedgerFreezeReader 读账本当前冻结量；注入接口，避免循环依赖。
// 返回 "0" 表示账本无冻结。
type LedgerFreezeReader func(ctx context.Context, userID, asset, book string) (frozenAmount string, err error)

// Patrol 缺口巡检执行器。

type Patrol struct {
	tokens *holdcat.TokenBook
	gaps   *holdrel.GapStore
	ledger LedgerFreezeReader // 可为 nil：跳过漂移检测，只做超时检测
	cfg    Config
	log    zerolog.Logger
}

// NewPatrol 构建 Patrol。

  func NewPatrol(
	  tokens *holdcat.TokenBook,
	  gaps *holdrel.GapStore,
	   ledger LedgerFreezeReader,
	  cfg Config,
	   log zerolog.Logger,
   ) *Patrol {
	  if cfg.ScanLimit <= 0 {
		   cfg.ScanLimit = DefaultConfig().ScanLimit
	   }
	   if cfg.ReleasingTimeout <= 0 {
		  cfg.ReleasingTimeout = DefaultConfig().ReleasingTimeout
	   }
	   if cfg.ActiveMaxAge <= 0 {
		   cfg.ActiveMaxAge = DefaultConfig().ActiveMaxAge
	   }
	  return &Patrol{tokens: tokens, gaps: gaps, ledger: ledger, cfg: cfg, log: log}
  }

// Run 执行一次全量巡检扫描，返回本次运行结果。
// 出错不中断扫描，逐 token 记录建议后接着。

  func (pt *Patrol) Run(ctx context.Context) PatrolRun {
	  run := PatrolRun{
		   RunID:     uuid.NewString(),
		  StartedAt: time.Now().UTC(),
	   }
	   toks, err := pt.tokens.List(ctx, "", true) // activeOnly=true
	   if err != nil {
		  run.Err = fmt.Sprintf("list tokens: %v", err)
		  run.FinishedAt = time.Now().UTC()
		  return run
	  }
	   limit := pt.cfg.ScanLimit
	  if len(toks) > limit {
		  toks = toks[:limit]
	   }
	  run.TokensScanned = len(toks)
	   now := time.Now().UTC()
	   advices := make([]DriftAdvice, 0)
	   for _, tok := range toks {
		   advs := pt.inspectToken(ctx, tok, now)
		   advices = append(advices, advs...)
		   for _, adv := range advs {
			  switch adv.Action {
			   case DriftActionOpenGap, DriftActionMarkBroken, DriftActionEscalate:
				  run.GapsOpened++
			  case DriftActionReleaseTag:
				  run.TagsReleased++
			   }
		  }
	  }
	  run.Escalated = run.GapsOpened // 告警数 = 开缺口数
	   run.Advices = advices
	  run.FinishedAt = time.Now().UTC()
	  return run
  }

// inspectToken 对单个 token 执行所有检测规则，返回建议列表。
  func (pt *Patrol) inspectToken(ctx context.Context, tok holdcat.HoldToken, now time.Time) []DriftAdvice {
	   var advices []DriftAdvice
	   // 解析创建时间
	  createdAt, err := time.Parse(time.RFC3339, tok.CreatedAt)
	   if err != nil {
		  createdAt = now // 解析失败则保守处理
	   }
	   // 规则 1：releasing 超时
	  if tok.State == holdcat.TokenStateReleasing {
		   updatedAt, err := time.Parse(time.RFC3339, tok.UpdatedAt)
		  if err != nil {
			   updatedAt = createdAt
		  }
		   if now.Sub(updatedAt) > pt.cfg.ReleasingTimeout {
			  adv := DriftAdvice{
				  TokenID: tok.ID,
				  UserID:  tok.UserID,
				  Asset:   tok.Asset,
				   Book:  tok.Book,
				   Layer: tok.Layer,
				   TokenAmount: tok.Amount,
				   Action:      DriftActionEscalate,
				   Reason:      fmt.Sprintf("releasing state for %s, exceeds timeout %s", now.Sub(updatedAt).Round(time.Second), pt.cfg.ReleasingTimeout),
			   }
			   advices = append(advices, adv)
			  pt.openGap(ctx, tok, holdrel.GapKindTimeout, adv.Reason)
		   }
		  return advices
	  }
	   // 规则 2：active 超龄
	  if tok.State == holdcat.TokenStateActive && now.Sub(createdAt) > pt.cfg.ActiveMaxAge {
		  adv := DriftAdvice{
			   TokenID: tok.ID,
			   UserID:  tok.UserID,
			   Asset: tok.Asset,
			   Book: tok.Book,
			   Layer:       tok.Layer,
			   TokenAmount: tok.Amount,
			  Action:      DriftActionEscalate,
			   Reason: fmt.Sprintf("active token age %s exceeds max %s",
				   now.Sub(createdAt).Round(time.Second), pt.cfg.ActiveMaxAge),
		  }
		   advices = append(advices, adv)
		  pt.openGap(ctx, tok, holdrel.GapKindTimeout, adv.Reason)
	   }
	   // 规则 3：漂移检测（需要账本读）
	   if pt.ledger != nil {
		  frozenStr,
			   err := pt.ledger(ctx, tok.UserID, tok.Asset, tok.Book)
		  if err != nil {
			  pt.log.Warn().Str("token_id", tok.ID).Err(err).Msg("patrol: ledger read failed")
		   } else {
			   advs := pt.checkDrift(ctx, tok, frozenStr)
			  advices = append(advices, advs...)
		   }
	  }
	  return advices
   }

// checkDrift 比较 token 金额与账本冻结量，生成漂移建议。

  func (pt *Patrol) checkDrift(ctx context.Context, tok holdcat.HoldToken, frozenStr string) []DriftAdvice {
	  var advices []DriftAdvice
	   tokenAmt, err := decimal.NewFromString(strings.TrimSpace(tok.Amount))
	  if err != nil || tokenAmt.IsZero() {
		   return advices
	   }
	   frozenAmt := decimal.Zero
	   if frozenStr != "" {
		   if fa, err2 := decimal.NewFromString(strings.TrimSpace(frozenStr)); err2 == nil {
			   frozenAmt = fa
		   }
	   }
	   if frozenAmt.IsZero() && tokenAmt.IsPositive() {
		   // 孤儿标签：账本无冻结但 token 仍 active
		  adv := DriftAdvice{
			   TokenID: tok.ID,
			  UserID:  tok.UserID,
			  Asset: tok.Asset,
			  Book:  tok.Book,
			  Layer: tok.Layer,
			  TokenAmount: tok.Amount,
			  LedgerFrozen: "0",
			  Action:       DriftActionOpenGap,
			   Reason:       "ledger frozen=0 but token is active (orphan tag)",
		   }
		   advices = append(advices, adv)
		  pt.openGap(ctx, tok, holdrel.GapKindOrphanTag, adv.Reason)
		  // 配置允许自动释放孤儿标签时发 ReleaseTag 建议
		  if pt.cfg.AutoReleaseOrphanTags {
			  adv2 := adv
			   adv2.Action = DriftActionReleaseTag
			   advices = append(advices, adv2)
		   }
		   return advices
	   }
	   if tokenAmt.GreaterThan(frozenAmt) {
		   // 金额漂移：token 金额超出账本冻结量
		   diff := tokenAmt.Sub(frozenAmt)
		   adv := DriftAdvice{
			   TokenID: tok.ID,
			   UserID:      tok.UserID,
			  Asset:       tok.Asset,
			   Book:        tok.Book,
			   Layer:       tok.Layer,
			   TokenAmount: tok.Amount,
			  LedgerFrozen: frozenStr,
			  Action: DriftActionOpenGap,
			  Reason: fmt.Sprintf(
				   "token amount %s > ledger frozen %s, drift=%s",
				  tok.Amount,
				   frozenStr, diff.String(),
			   ),
		  }
		  advices = append(advices, adv)
		   pt.openGap(ctx, tok, holdrel.GapKindAmountDrift, adv.Reason)
	   }
	   return advices
  }

// openGap 开缺口单；失败只记日志，不中断巡检。
  func (pt *Patrol) openGap(ctx context.Context, tok holdcat.HoldToken, kind holdrel.GapKind, detail string) {
	  if pt.gaps == nil {
		  return
	  }
	   gap := holdrel.HoldGap{
		   TokenID: tok.ID,
		   UserID:  tok.UserID,
		  Asset:   tok.Asset,
		   Book: tok.Book,
		   Kind:        kind,
		  Layer:       tok.Layer,
		   TokenAmount: tok.Amount,
		  Detail:      detail,
	  }
	  if _, err := pt.gaps.Open(ctx, gap); err != nil {
		  pt.log.Warn().Str("token_id", tok.ID).Str("kind", string(kind)).Err(err).Msg("patrol: open gap failed")
	   }
  }

// ScanOrphanHoldcatTags 扫描 holdcat.Book 中 open 的 bridge 标签，
// 与 TokenBook 核对，找出无对应 token 的孤儿标签，开缺口单。
// 不直接释放标签；建议由调用方决策。
   func (pt *Patrol) ScanOrphanHoldcatTags(ctx context.Context, holdBook *holdcat.Book) ([]holdrel.HoldGap, error) {
	   rows, err := holdBook.List(ctx, "", true)
	  if err != nil {
		  return nil, err
	  }
	   var opened []holdrel.HoldGap
	  for _, row := range rows {
		  // 只处理非 ParkedOnBridge 的标签（ParkedOnBridge 有自己的 SweepOrphans）
		  if holdcat.ParkedOnBridge(row.Reason) {
			   continue
		   }
		   // 按 ref 查找对应 token
		  toks, err := pt.tokens.List(ctx, row.UserID, true)
		   if err != nil {
			   continue
		  }
		   found := false
		  for _, t := range toks {
			   if t.LedgerRef == row.Ref && t.Asset == row.Asset {
				  found = true
				  break
			   }
		  }
		   if !found {
			  gap := holdrel.HoldGap{
				  TokenID: "", // 无对应 token
				  UserID:  row.UserID,
				  Asset:   row.Asset,
				  Book:    row.Book,
				  Kind:   holdrel.GapKindOrphanTag,
				  Detail: fmt.Sprintf("holdcat row id=%s reason=%s ref=%s has no matching HoldToken", row.ID, row.Reason, row.Ref),
			   }
			  // 用 row.ID 作 tokenID 避免重复开单
			  gap.TokenID = "holdcat:" + row.ID
			   if g, err := pt.gaps.Open(ctx, gap); err == nil {
				  opened = append(opened, g)
			  }
		  }
	  }
	  return opened, nil
   }

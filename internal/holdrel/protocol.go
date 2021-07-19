package holdrel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

const (
	kindAudit = "aa_hold_rel_audit"
)

// LedgerFreezeChecker 检查账本冻结量；由调用方注入以避免循环依赖。
// 返回给定用户/资产/账本的当前冻结金额（十进制字符串）。

type LedgerFreezeChecker func(ctx context.Context, userID, asset, book string) (frozenAmount string, err error)

// Protocol 释放协议处理器。

type Protocol struct {
	tokens *holdcat.TokenBook
	proofs *ProofStore
	audits  persist.KV
	checker LedgerFreezeChecker // 可为 nil（跳过账本冻结量校验）
}

// NewProtocol 构建 Protocol。
// kv 用于审计行持久化；tokens/proofs 各用自己的 KV（可共享同一 KV 实例）。

  func NewProtocol(tokens *holdcat.TokenBook, proofs *ProofStore, kv persist.KV, checker LedgerFreezeChecker) *Protocol {
	   return &Protocol{
		  tokens: tokens,
		  proofs: proofs,
		  audits: kv,
		  checker: checker,
	   }
  }

// RequestRelease 执行释放协议。
// 流程：
// 1. 幂等检查：IdemKey 已有 proof → 直接返回已有 proof + 审计（幂等）。
// 2. 获取 HoldToken，校验 CallerLayer ≥ FrozenLayer。
// 3. legal/aml 层必须提供 ApprovalRef。
// 4. 检查是否存在更高先级 active token（阻止低层释放高层）。
// 5. BeginRelease（→ releasing 中间态）。
// 6. 写 ReleaseProof。
// 7. CommitRelease（→ released）。
// 8. 写审计行。
// 注意：本函数不操作账本；账本解冻应在调用前由上层完成。
// 若账本解冻失败，调用方不应调用本函数；已 releasing 的 token 需人工确认。
  func (p *Protocol) RequestRelease(ctx context.Context, req ReleaseReq) (ReleaseProof, error) {
	  if p == nil {
		  return ReleaseProof{}, ErrStore
	   }
	  idemKey := strings.TrimSpace(req.IdemKey)
	   if idemKey == "" {
		  return ReleaseProof{}, ErrIdemKeyMissing
	  }
	   if strings.TrimSpace(req.Requester) == "" {
		   return ReleaseProof{}, ErrRequesterMissing
	   }
	   // 1. 幂等：已有 proof 直接返回
	   if existing, err := p.proofs.ByRef(ctx, idemKey); err == nil {
		  _ = p.writeAudit(ctx, req, existing.FrozenLayer, true, "", existing.ProofID)
		  return existing, nil
	   }
	  // 2. 获取 token
	  tok, err := p.tokens.Get(ctx, "", req.TokenID)
	   if err != nil {
		   if errors.Is(err, holdcat.ErrGone) {
			  return ReleaseProof{}, ErrTokenNotFound
		  }
		   return ReleaseProof{}, err
	  }
	  // 已 released：写审计并返回 proof（如有）
	  if tok.State == holdcat.TokenStateReleased {
		  if proof, err := p.proofs.ByRef(ctx, idemKey); err == nil {
			   _ = p.writeAudit(ctx, req, tok.Layer, true, "already_released", proof.ProofID)
			   return proof, nil
		  }
		  return ReleaseProof{}, ErrAlreadyReleased
	   }
	   // 3. 层权限校验
	   callerLayer, err := holdcat.ParseLayer(req.CallerLayer)
	  if err != nil {
		   denyReason := "invalid_caller_layer: " + req.CallerLayer
		   _ = p.writeAudit(ctx, req, tok.Layer, false, denyReason, "")
		  return ReleaseProof{}, ErrLayerDenied
	   }
	   frozenLayer, err := holdcat.ParseLayer(tok.Layer)
	  if err != nil {
		   return ReleaseProof{}, ErrLayerDenied
	   }
	  if !holdcat.CanRelease(callerLayer, frozenLayer) {
		  denyReason := callerLayer.Code() + " cannot release " + frozenLayer.Code()
		  _ = p.writeAudit(ctx, req, tok.Layer, false, denyReason, "")
		  return ReleaseProof{},
			   ErrLayerDenied
	   }
	   // 4. legal/aml 层必须提供 ApprovalRef
	   if (frozenLayer == holdcat.LayerLegal || frozenLayer == holdcat.LayerAML) &&
		  strings.TrimSpace(req.ApprovalRef) == "" {
		  denyReason := "approval_ref required for " + frozenLayer.Code()
		  _ = p.writeAudit(ctx, req, tok.Layer, false, denyReason, "")
		   return ReleaseProof{}, ErrApprovalRequired
	  }
	  // 5. 检查更高层冻结（同用户/资产存在更高层 active token 时拒绝）
	   // 注：此处仅检查 callerLayer < highestActiveLayer 情形；
	   // callerLayer == frozenLayer 的正常释放路径不受干扰。
	  blocked,
		   highestLayer, err := p.tokens.HasHigherLayerToken(
		   ctx, tok.UserID, tok.Asset, tok.Book, callerLayer,
	   )
	   if err != nil {
		  return ReleaseProof{}, err
	  }
	   if blocked {
		  denyReason := "higher layer " + highestLayer.Code() + " active; cannot release via " + callerLayer.Code()
		  _ = p.writeAudit(ctx, req, tok.Layer, false, denyReason, "")
		  return ReleaseProof{}, ErrLayerDenied
	  }
	   // 6. BeginRelease → releasing
	  if _, err := p.tokens.BeginRelease(ctx, tok.ID); err != nil {
		   return ReleaseProof{}, err
	   }
	   // 7. 写 ReleaseProof
	   info := releaseTokenInfo{
		   TokenID: tok.ID,
		   UserID: tok.UserID,
		   Asset:  tok.Asset,
		  Book:   tok.Book,
		   Amount: tok.Amount,
		   FrozenLayer: tok.Layer,
		  LedgerRef: tok.LedgerRef,
	   }
	  proof, err := p.proofs.Write(ctx, req, info)
	  if err != nil {
		  // proof 写失败：不回滚 releasing 态（由 Gap Patrol 检测超时后升级）
		  return ReleaseProof{}, err
	  }
	  // 8. CommitRelease → released
	  if _, err := p.tokens.CommitRelease(ctx, tok.ID); err != nil {
		   // proof 已写但 token 未 commit：Gap Patrol 会检测到 releasing 超时
		  return proof, err
	  }
	   _ = p.writeAudit(ctx, req, tok.Layer, true, "", proof.ProofID)
	  return proof, nil
  }

// QueryProof 按 ProofRef（幂等键）查询证明；幂等重放安全。
  func (p *Protocol) QueryProof(ctx context.Context, proofRef string) (ReleaseProof, error) {
	  return p.proofs.ByRef(ctx, proofRef)
  }

// QueryProofByID 按 ProofID 查询。
  func (p *Protocol) QueryProofByID(ctx context.Context, proofID string) (ReleaseProof, error) {
	  return p.proofs.ByID(ctx, proofID)
   }

// ListProofs 按 userID 列举证明。
  func (p *Protocol) ListProofs(ctx context.Context, userID string, limit int) ([]ReleaseProof, error) {
	  return p.proofs.List(ctx, userID, limit)
   }

   func (p *Protocol) writeAudit(
	   ctx context.Context,
	   req ReleaseReq,
	  frozenLayer string,
	  allowed bool,
	  denyReason, proofID string,
  ) error {
	  if p.audits == nil {
		   return nil
	   }
	   audit := ReleaseAudit{
		  AuditID: uuid.NewString(),
		   TokenID: req.TokenID,
		  CallerLayer: req.CallerLayer,
		  FrozenLayer: frozenLayer,
		   ReasonCode:  req.ReasonCode,
		  Requester:   req.Requester,
		   Allowed: allowed,
		  DenyReason: denyReason,
		   ProofID:    proofID,
		   At:         time.Now().UTC(),
	   }
	   body, err := json.Marshal(audit)
	  if err != nil {
		  return err
	   }
	  return p.audits.Put(ctx,
		   persist.Row{Kind: kindAudit, Key: audit.AuditID, Body: body},
		  persist.Fence{},
	   )
   }

// ListAudits 按 token ID 列出审计行（全量扫描，仅供 desk 使用）。

   func (p *Protocol) ListAudits(ctx context.Context, tokenID string, limit int) ([]ReleaseAudit, error) {
	   if p.audits == nil {
		  return nil, nil
	   }
	   if limit <= 0 || limit > 1000 {
		   limit = 200
	  }
	  rows, err := p.audits.List(ctx, kindAudit, limit)
	   if err != nil {
		   return nil, err
	   }
	   out := make([]ReleaseAudit, 0)
	  for _, r := range rows {
		   var a ReleaseAudit
		  if json.Unmarshal(r.Body, &a) != nil || a.AuditID == "" {
			  continue
		   }
		   if tokenID != "" && a.TokenID != tokenID {
			  continue
		  }
		  out = append(out, a)
	  }
	   return out, nil
  }

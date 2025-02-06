package holdrel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

const (
	kindProof    = "aa_hold_rel_proof"
	kindProofRef = "aa_hold_rel_proof_ref" // proof_ref → proof_id
)

// ProofStore 释放证明存储。

type ProofStore struct {
	kv persist.KV
}

// NewProofStore 创建 ProofStore。
  func NewProofStore(kv persist.KV) *ProofStore {
	   return &ProofStore{kv: kv}
  }

// Write 写入释放证明；ProofRef 已存在时幂等返回已有 proof，不覆盖。
   func (ps *ProofStore) Write(ctx context.Context, req ReleaseReq, tok releaseTokenInfo) (ReleaseProof, error) {
	  if ps == nil || ps.kv == nil {
		   return ReleaseProof{}, ErrStore
	  }
	   proofRef := strings.TrimSpace(req.IdemKey)
	   if proofRef == "" {
		  return ReleaseProof{}, ErrIdemKeyMissing
	  }
	   // 幂等：已有则直接返回
	   if prev, err := ps.ByRef(ctx, proofRef); err == nil {
		   return prev, nil
	   }
	   proof := ReleaseProof{
		  ProofID:  uuid.NewString(),
		   ProofRef: proofRef,
		  TokenID:  tok.TokenID,
		   UserID:   tok.UserID,
		   Asset:    tok.Asset,
		  Book: tok.Book,
		   Amount: tok.Amount,
		   FrozenLayer: tok.FrozenLayer,
		  CallerLayer: req.CallerLayer,
		   ReasonCode:  req.ReasonCode,
		   ApprovalRef: req.ApprovalRef,
		  LedgerRef: tok.LedgerRef,
		   Requester: req.Requester,
		  Note:       req.Note,
		  ReleasedAt: time.Now().UTC(),
	  }
	   // 先写 ref 索引，保证幂等语义
	   if err := ps.kv.Put(ctx,
		  persist.Row{Kind: kindProofRef,
			   Key: proofRef, Body: []byte(proof.ProofID)},
		   persist.Fence{},
	   ); err != nil {
		  // 并发竞争：读已有
		  if prev, e2 := ps.ByRef(ctx, proofRef); e2 == nil {
			   return prev, nil
		  }
		   return ReleaseProof{}, err
	  }
	  body, err := json.Marshal(proof)
	   if err != nil {
		   return ReleaseProof{}, err
	   }
	  if err := ps.kv.Put(ctx,
		   persist.Row{Kind: kindProof,
			  Key: proof.ProofID, Body: body},
		  persist.Fence{},
	   ); err != nil {
		  return ReleaseProof{}, err
	  }
	   return proof, nil
  }

// ByRef 按 ProofRef（幂等键）查询证明。

   func (ps *ProofStore) ByRef(ctx context.Context, proofRef string) (ReleaseProof, error) {
	  if ps == nil || ps.kv == nil {
		   return ReleaseProof{}, ErrStore
	  }
	   r,
		  err := ps.kv.Get(ctx, kindProofRef, strings.TrimSpace(proofRef))
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return ReleaseProof{}, ErrProofNotFound
		  }
		  return ReleaseProof{}, err
	   }
	  return ps.byID(ctx, string(r.Body))
   }

// ByID 按 ProofID 查询证明。

  func (ps *ProofStore) ByID(ctx context.Context, proofID string) (ReleaseProof, error) {
	  return ps.byID(ctx, proofID)
   }

  func (ps *ProofStore) byID(ctx context.Context, proofID string) (ReleaseProof, error) {
	  r, err := ps.kv.Get(ctx, kindProof, strings.TrimSpace(proofID))
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return ReleaseProof{}, ErrProofNotFound
		   }
		   return ReleaseProof{}, err
	  }
	   var proof ReleaseProof
	  if json.Unmarshal(r.Body, &proof) != nil || proof.ProofID == "" {
		   return ReleaseProof{}, ErrProofNotFound
	  }
	   return proof, nil
  }

// List 按 userID 列举证明（全量扫描，仅供 desk/运营审计使用）。

  func (ps *ProofStore) List(ctx context.Context, userID string, limit int) ([]ReleaseProof, error) {
	  if ps == nil || ps.kv == nil {
		  return nil, nil
	  }
	   if limit <= 0 || limit > 500 {
		  limit = 200
	   }
	  rows, err := ps.kv.List(ctx, kindProof, limit)
	   if err != nil {
		  return nil, err
	   }
	   out := make([]ReleaseProof, 0, len(rows))
	   for _, r := range rows {
		   var p ReleaseProof
		  if json.Unmarshal(r.Body, &p) != nil || p.ProofID == "" {
			  continue
		   }
		  if userID != "" && p.UserID != userID {
			   continue
		  }
		  out = append(out, p)
	  }
	  return out, nil
   }

// releaseTokenInfo 内部：从 HoldToken 提取写 proof 所需字段。

type releaseTokenInfo struct {
	TokenID string
	UserID string
	Asset string
	Book  string
	Amount string
	FrozenLayer string
	LedgerRef   string
}

package p2pabandon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ProofHashInput 收口证明哈希输入（决定哈希值的核心字段集合）。
// 只要 plan_id / order_id / outcome / committed_token_id / amount / asset
// 中任一字段被篡改，哈希将不符，巡检即可感知。

type ProofHashInput struct {
	PlanID  string       `json:"plan_id"`
	OrderID string       `json:"order_id"`
	Outcome CloseOutcome `json:"outcome"`
	CommittedTokenID string `json:"committed_token_id"`
	Amount           string `json:"amount"`
	Asset string `json:"asset"`
}

// ComputeProofHash 计算收口证明哈希。
// 输入各字段以 "|" 分隔后做 SHA256，结果为 hex 字符串。
  func ComputeProofHash(in ProofHashInput) string {
	  raw := strings.Join([]string{
		   in.PlanID,
		  in.OrderID,
		   string(in.Outcome),
		   in.CommittedTokenID,
		   in.Amount,
		   in.Asset,
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// VerifyProofHash 校验收口证明哈希与当前字段是否一致。
// 返回 nil 表示一致；否则返回 ErrProofTamper。
   func VerifyProofHash(proof CloseProof) error {
	  in := ProofHashInput{
		  PlanID:  proof.PlanID,
		  OrderID: proof.OrderID,
		   Outcome: proof.Outcome,
		  CommittedTokenID: proof.CommittedTokenID,
		   Amount:           proof.Amount,
		  Asset: proof.Asset,
	   }
	   want := ComputeProofHash(in)
	   if proof.ProofHash != want {
		   return fmt.Errorf("%w: stored=%s recomputed=%s", ErrProofTamper, proof.ProofHash, want)
	   }
	   return nil
   }

// MarshalProofFields 将 CloseProof 的关键字段序列化为 JSON（用于审计日志）。

   func MarshalProofFields(p CloseProof) ([]byte, error) {
	  return json.Marshal(struct {
		  ID string `json:"id"`
		  PlanID  string `json:"plan_id"`
		  OrderID string `json:"order_id"`
		   Outcome CloseOutcome `json:"outcome"`
		   CommittedTokenID string    `json:"committed_token_id"`
		   CommittedKind    TokenKind `json:"committed_kind"`
		   Amount string `json:"amount"`
		   Asset string `json:"asset"`
		   ProofHash string `json:"proof_hash"`
		   DecidedBy string `json:"decided_by"`
	  }{
		  ID: p.ID,
		   PlanID: p.PlanID,
		  OrderID: p.OrderID,
		  Outcome:          p.Outcome,
		   CommittedTokenID: p.CommittedTokenID,
		   CommittedKind:    p.CommittedKind,
		   Amount:    p.Amount,
		   Asset:     p.Asset,
		  ProofHash: p.ProofHash,
		  DecidedBy: p.DecidedBy,
	   })
  }

// TokenHashInput 互斥令牌状态哈希输入（用于审计日志防篡改）。

type TokenHashInput struct {
	TokenID string
	PlanID  string
	Kind    TokenKind
	Status TokenStatus
	Actor  string
}

// ComputeTokenHash 计算互斥令牌状态哈希（审计用）。

  func ComputeTokenHash(in TokenHashInput) string {
	   raw := strings.Join([]string{
		  in.TokenID,
		  in.PlanID,
		   string(in.Kind),
		  string(in.Status),
		  in.Actor,
	   }, "|")
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
  }

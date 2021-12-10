package stucksettle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// effectChecksum 计算效果证明指纹。
// 输入三段用 "|" 分隔后做 SHA-256。

  func effectChecksum(intent, action, ledgerRef string) string {
	   raw := intent + "|" + action + "|" + ledgerRef
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
  }

// intentOf 构造回收意图描述串。
// 形态：RecoverStuck:{transfer_id}:gen={generation}

  func intentOf(transferID string, gen int64) string {
	  return fmt.Sprintf("RecoverStuck:%s:gen=%d", transferID, gen)
   }

// BuildEffectProof 构造效果证明。
// action 来自 withdraw.RecoverStuck 返回的 StuckAction；
// ledgerRef 为广播记录 ID 或账本流水 ID（只读引用，不改写外部状态）。
	  }
	   if strings.TrimSpace(issuedBy) == "" {
		   return nil, fmt.Errorf("%w: issued_by empty", ErrInvalidInput)
	   }
	  intent := intentOf(lease.TransferID, lease.Generation)
	  checksum := effectChecksum(intent, action, ledgerRef)
	  return &EffectProof{
		  EffectProofID: uuid.NewString(),
		  TransferID: lease.TransferID,
		  LeaseID: lease.LeaseID,
		  Generation: lease.Generation,
		   Intent:     intent,
		  Action:     action,
		  LedgerRef: ledgerRef,
		   Checksum:  checksum,
		  IssuedBy:  issuedBy,
		   IssuedAt:  now,
	  }, nil
  }

// VerifyProof 校验效果证明。
// 返回 false 时调用方不得标记 transfer 为 Recovered。
  func VerifyProof(p *EffectProof) bool {
	   if p == nil {
		  return false
	   }
	  want := effectChecksum(p.Intent, p.Action, p.LedgerRef)
	   return want == p.Checksum
  }

// ProofSummary 证明摘要（用于审计导出）。
type ProofSummary struct {
	EffectProofID string `json:"effect_proof_id"`
	TransferID string `json:"transfer_id"`
	Generation int64  `json:"generation"`
	Action     string `json:"action"`
	LedgerRef string `json:"ledger_ref"`
	Valid     bool   `json:"valid"`
}

// Summarize 生成证明摘要。

   func Summarize(p *EffectProof) ProofSummary {
	   if p == nil {
		   return ProofSummary{}
	   }
	  return ProofSummary{
		   EffectProofID: p.EffectProofID,
		   TransferID: p.TransferID,
		   Generation: p.Generation,
		  Action:     p.Action,
		  LedgerRef:  p.LedgerRef,
		   Valid: VerifyProof(p),
	  }
   }

package writegate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/pricepoison"
)

// PoisonProof 毒价证明（tick ⊕ rule ⊕ write-scope hash）。

type PoisonProof struct {
	// Pair 交易对（已规范化）。
	Pair string `json:"pair"`
	// TickDigest tick 内容摘要（来自 GenToken.TickDigest）。
	TickDigest string `json:"tick_digest"`
	// Rule 触发的检测规则名称。
	Rule string `json:"rule"`
	// WriteScope 资金写路径作用域名称。
	WriteScope string `json:"write_scope"`
	// Grade 毒价等级标签。
	Grade string `json:"grade"`
	// IssuedAt 证明签发时刻（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// Digest SHA-256(tick_digest ‖ rule ‖ write_scope)，十六进制。
	Digest string `json:"digest"`
}

// BuildPoisonProof 从 GenToken 和 GradeDetail 构造毒价证明。
// scope 为写路径作用域名称。
// now 为签发时刻（零值使用 time.Now.UTC）。
   func BuildPoisonProof(tok pricepoison.GenToken, detail pricepoison.GradeDetail, scope WriteScope, now time.Time) (PoisonProof, error) {
	   if tok.Pair == "" {
		   return PoisonProof{}, fmt.Errorf("writegate: BuildPoisonProof: empty pair")
	  }
	  if tok.TickDigest == "" {
		   return PoisonProof{}, fmt.Errorf("writegate: BuildPoisonProof: empty tick digest")
	  }
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   scopeStr := strings.TrimSpace(string(scope))
	   digest := poisonDigest(tok.TickDigest, detail.Rule, scopeStr)
	  return PoisonProof{
		   Pair: tok.Pair,
		  TickDigest: tok.TickDigest,
		   Rule: detail.Rule,
		   WriteScope: scopeStr,
		  Grade:      detail.Grade.Label(),
		  IssuedAt:   now,
		   Digest: digest,
	  }, nil
  }

// BuildProofFromEvent 从 PoisonEvent 构造证明（PoisonEvent 包含了所有必要字段）。
   func BuildProofFromEvent(evt pricepoison.PoisonEvent, scope WriteScope, now time.Time) (PoisonProof, error) {
	  if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  // 重新计算 tick digest
	   tickDigest := pricepoison.ExportTickDigest(evt.Pair, evt.TickPrice, evt.TickAt)
	  tok := pricepoison.GenToken{
		  Pair:       evt.Pair,
		   TickDigest: tickDigest,
	   }
	  return BuildPoisonProof(tok, evt.Detail, scope, now)
  }

// Verify 验证证明摘要是否与字段一致。

  func (p PoisonProof) Verify() error {
	  want := poisonDigest(p.TickDigest, p.Rule, p.WriteScope)
	   if want != p.Digest {
		  return fmt.Errorf("writegate: proof digest mismatch: want=%s got=%s pair=%s",
			  want, p.Digest, p.Pair)
	   }
	   return nil
  }

// poisonDigest SHA-256(tick_digest ‖ "\x00" ‖ rule ‖ "\x00" ‖ write_scope)。

  func poisonDigest(tickDigest, rule, scope string) string {
	  h := sha256.New()
	   sep := []byte{0x00}
	  _, _ = h.Write([]byte(tickDigest))
	  _, _ = h.Write(sep)
	   _, _ = h.Write([]byte(rule))
	   _, _ = h.Write(sep)
	   _, _ = h.Write([]byte(scope))
	   return hex.EncodeToString(h.Sum(nil))
   }

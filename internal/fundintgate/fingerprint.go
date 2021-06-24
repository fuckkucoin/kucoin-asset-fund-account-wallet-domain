package fundintgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
)

// DefaultPolicySalt 闸面策略盐。换闸实现或策略翻转时应由调用方覆盖。
const DefaultPolicySalt = "fundint-gate-v1"

// CanonicalAmount 将金额规范为固定小数串，避免 1 与 1.00000000 被视作漂移。

   func CanonicalAmount(raw string) (string, error) {
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return "", fmt.Errorf("%w: empty amount", ErrInvalidReq)
	   }
	   d, err := decimal.NewFromString(raw)
	   if err != nil || !d.IsPositive() {
		   return "", fmt.Errorf("%w: amount", ErrInvalidReq)
	   }
	  return d.StringFixed(18), nil
  }

// CanonicalAsset 规范化资产代码。

  func CanonicalAsset(raw string) string {
	   return strings.ToUpper(strings.TrimSpace(raw))
  }

// ComposeToHint 将多标识提示压成稳定串。
  func ComposeToHint(toUID, toEmail, toPhone string) string {
	  return strings.Join([]string{
		  "uid=" + strings.TrimSpace(toUID),
		  "email=" + strings.ToLower(strings.TrimSpace(toEmail)),
		  "phone=" + strings.TrimSpace(toPhone),
	   }, "|")
   }

// GateFingerprintOf 计算闸面指纹。
  func GateFingerprintOf(implID string, halted, forbid bool, policySalt string, gen int64) string {
	   if strings.TrimSpace(policySalt) == "" {
		  policySalt = DefaultPolicySalt
	  }
	  material := fmt.Sprintf("impl=%s|halt=%t|forbid=%t|salt=%s|gen=%d",
		  strings.TrimSpace(implID), halted, forbid, policySalt, gen)
	  sum := sha256.Sum256([]byte(material))
	  return hex.EncodeToString(sum[:])
  }

// CounterpartFingerprint 对手与金额钉扎指纹（辅助证明字段，不替代逐字段复验）。
   func CounterpartFingerprint(from, to, channel, hint, asset, amount string) string {
	   material := fmt.Sprintf("from=%s|to=%s|ch=%s|hint=%s|asset=%s|amt=%s",
		  strings.TrimSpace(from), strings.TrimSpace(to), strings.TrimSpace(channel),
		   strings.TrimSpace(hint),
		  CanonicalAsset(asset), strings.TrimSpace(amount))
	  sum := sha256.Sum256([]byte(material))
	   return hex.EncodeToString(sum[:])
  }

// EqualFoldID 用户标识比较。

   func EqualFoldID(a, b string) bool {
	   return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
   }

// SurfaceFromGen 由世代记录构造闸面快照。

  func SurfaceFromGen(g GateGeneration) GateSurface {
	   return GateSurface{
		  ImplID:         g.GateImplID,
		  Halted:         g.Halted,
		   ForbidWithdraw: g.ForbidWithdraw,
		   Fingerprint: g.Fingerprint,
		  Gen:         g.Gen,
	  }
   }

// RecomputeSurfaceFP 按当前闸面字段重算指纹（换闸/Halt/禁提后必须重算）。

  func RecomputeSurfaceFP(implID string, halted, forbid bool, gen int64) string {
	   return GateFingerprintOf(implID, halted, forbid, DefaultPolicySalt, gen)
  }

package fiatrail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// LimitsFingerprint 限额策略指纹。Authorize 签发租约时钉扎；Settle 必须复验。
// 抬升 MaxSingle / DailySum 收紧 / HourlyCount 下降 / BIN 黑名单变化均改变指纹。
   func (l *Limits) Fingerprint() string {
	  if l == nil {
		  return sha256hex("limits:nil")
	   }
	   c := l.Caps()
	   l.mu.Lock()
	  bins := make([]string,
		   0, len(l.bins))
	  for b := range l.bins {
		   bins = append(bins, b)
	   }
	  l.mu.Unlock()
	  sort.Strings(bins)
	   raw := fmt.Sprintf("max=%s|daily=%s|hourly=%d|bins=%s",
		  strings.TrimSpace(c.MaxSingle), strings.TrimSpace(c.DailySum), c.HourlyCount, strings.Join(bins, ","))
	   return sha256hex(raw)
  }

// KYCFingerprint 用户档位与材料闸指纹。降档或材料失效必须使旧租约失效。
   func (g *Gate) KYCFingerprint(userID string) string {
	   if g == nil {
		  return sha256hex("kyc:nil")
	   }
	   p, err := g.Get(userID)
	   if err != nil {
		  return sha256hex("kyc:missing:" + strings.TrimSpace(userID))
	   }
	   raw := fmt.Sprintf("user=%s|tier=%s|rank=%d|name=%t|doc=%t|on=%s|off=%s|hi=%s",
		   p.UserID, p.Tier, p.Tier.Rank(), p.NameOK, p.DocOK, g.onNeed, g.offNeed, g.hiNeed)
	   return sha256hex(raw)
   }

// RequiredTierSnapshot 只读回放所需档位，不改闸配置。

  func (g *Gate) RequiredTierSnapshot(kind Kind, amount string) Tier {
	   need, err := g.Required(kind, amount)
	   if err != nil {
		   return TierNone
	   }
	  return need
   }

  func sha256hex(s string) string {
	  sum := sha256.Sum256([]byte(s))
	   return hex.EncodeToString(sum[:])
  }

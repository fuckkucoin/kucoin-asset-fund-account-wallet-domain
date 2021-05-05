package feequotelease

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// Tier 档位条目（镜像 feeeng.Tier，避免循环依赖）。

type Tier struct {
	Notional string `json:"notional"`
	BPS int `json:"bps"`
}

// TiersFingerprint 对 tiers 列表计算 SHA-256 指纹。
// 规则：
// 1. 按 Notional 升序排序，消除顺序差异。
// 2. 对规范化后的 JSON 数组计算 SHA-256。
// 3. 空列表返回空串。
   func TiersFingerprint(tiers []Tier) string {
	   if len(tiers) == 0 {
		   return ""
	  }
	  cp := make([]Tier, len(tiers))
	   copy(cp, tiers)
	  sort.SliceStable(cp, func(i, j int) bool {
		  return cp[i].Notional < cp[j].Notional
	  })
	  b, err := json.Marshal(cp)
	   if err != nil {
		   return fmt.Sprintf("err:%v", err)
	   }
	  h := sha256.Sum256(b)
	   return hex.EncodeToString(h[:])
   }

// FingerprintMatch 判断两个指纹是否一致。
// 均为空串视为一致（均未分档）。

   func FingerprintMatch(a, b string) bool {
	   return a == b
   }

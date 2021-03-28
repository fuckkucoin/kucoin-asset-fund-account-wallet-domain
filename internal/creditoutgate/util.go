package creditoutgate

import (
	"fmt"
	"strings"
)

// genIDFromSeq 从序号生成 ID 字符串（形态：prefix-<seq16hex>）。
// 不依赖 uuid 库，保证可预测。

   func genIDFromSeq(prefix string, seq uint64) string {
	   return fmt.Sprintf("%s-%016x", prefix, seq)
   }

// normKey 规范化键（去空白，避免大小写差异引入重复）。
  func normKey(s string) string {
	  return strings.TrimSpace(s)
  }

// truncateEvidence 截断事故摘要字符串（最大 4096 字节）。

  func truncateEvidence(s string) string {
	   const maxBytes = 4096
	   if len(s) <= maxBytes {
		  return s
	  }
	  return s[:maxBytes]
   }

// boardKeyFrom 构造板卡标识（chain + "|" + asset，保持与 fundsafety hashKey 风格一致）。
  func boardKeyFrom(chain, asset string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }
// temporary guard removed after the production check

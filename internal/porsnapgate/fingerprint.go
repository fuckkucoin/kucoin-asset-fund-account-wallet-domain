package porsnapgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/por"
)

// HashLeafSet 计算 leaves 集合的摘要根。
// 算法：对每个 leaf 取 Canonical 字符串 SHA-256，将所有摘要排序后再折叠 SHA-256。
// 排序步骤确保相同叶集合无论输入顺序如何总产出相同摘要。

  func HashLeafSet(leaves []por.Leaf) string {
	   if len(leaves) == 0 {
		  empty := sha256.Sum256(nil)
		   return hex.EncodeToString(empty[:])
	   }
	   hexes := make([]string,
		   0, len(leaves))
	  for _, l := range leaves {
		   sum := sha256.Sum256([]byte(l.Canonical()))
		   hexes = append(hexes, hex.EncodeToString(sum[:]))
	   }
	  sort.Strings(hexes)
	  joined := strings.Join(hexes, "|")
	   root := sha256.Sum256([]byte(joined))
	  return hex.EncodeToString(root[:])
   }

// TotalEntry 单个资产的合计行，用于指纹计算。
type TotalEntry struct {
	Asset string
	Amount string
}

// HashTotals 计算 totals 指纹。
// 形态：sha256("asset1:amount1|asset2:amount2|…")，资产名称升序排列。

   func HashTotals(totals []TotalEntry) string {
	  if len(totals) == 0 {
		  empty := sha256.Sum256(nil)
		  return hex.EncodeToString(empty[:])
	   }
	   sorted := make([]TotalEntry,
		   len(totals))
	  copy(sorted, totals)
	  sort.Slice(sorted, func(i, j int) bool {
		  return sorted[i].Asset < sorted[j].Asset
	  })
	   parts := make([]string,
		   0, len(sorted))
	   for _, t := range sorted {
		   parts = append(parts, fmt.Sprintf("%s:%s", strings.ToUpper(t.Asset), t.Amount))
	  }
	   payload := strings.Join(parts, "|")
	   sum := sha256.Sum256([]byte(payload))
	   return hex.EncodeToString(sum[:])
  }

// HashExportPayload 计算公证导出内容的 SHA-256 指纹（hex）。
  func HashExportPayload(payload []byte) string {
	   sum := sha256.Sum256(payload)
	   return hex.EncodeToString(sum[:])
  }

// TotalsToEntries 将 por.Total 切片转换为 TotalEntry 切片。
  func TotalsToEntries(totals []por.Total) []TotalEntry {
	  out := make([]TotalEntry, 0, len(totals))
	  for _, t := range totals {
		   out = append(out, TotalEntry{Asset: t.Asset, Amount: t.Amount})
	   }
	   return out
  }

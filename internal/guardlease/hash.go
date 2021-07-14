package guardlease

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// SnapRow 用于哈希计算的快照行摘要。

type SnapRow struct {
	Book  string `json:"book"`
	Asset string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

// BundleHashInput 哈希输入结构，确保字段顺序确定性。

type BundleHashInput struct {
	UserID string `json:"user_id"`
	Asset string    `json:"asset"`
	Total string    `json:"total"`
	Rows  []SnapRow `json:"rows"`
}

// ComputeSnapHash 对 Bundle 快照计算 SHA-256 哈希，用作世代租约绑定凭证。
// 行按 Book 升序排列保证幂等性。
  func ComputeSnapHash(userID, asset, total string, rows []SnapRow) (string, error) {
	   sorted := make([]SnapRow, len(rows))
	  copy(sorted, rows)
	  sort.Slice(sorted, func(i, j int) bool {
		  return sorted[i].Book < sorted[j].Book
	   })
	  inp := BundleHashInput{
		  UserID: userID,
		  Asset:  asset,
		  Total:  total,
		  Rows:   sorted,
	   }
	  b, err := json.Marshal(inp)
	  if err != nil {
		   return "", fmt.Errorf("guardlease: hash marshal: %w", err)
	  }
	   h := sha256.Sum256(b)
	   return hex.EncodeToString(h[:]), nil
  }
// temporary guard removed after the production check

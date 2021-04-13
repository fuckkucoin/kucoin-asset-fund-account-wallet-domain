package dispclaw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// LegHashInput 计算腿哈希的输入字段集合。
// 字段选取原则：覆盖所有业务关键属性；不含 UpdatedAt 等纯时间戳，防止重试后哈希变化。

type LegHashInput struct {
	PlanID string
	Seq     int
	LegType LegType
	UserID  string
	Asset   string
	Amount string
	Status LegStatus
}

// ComputeLegHash 计算单条追偿腿的防篡改哈希。
// 输入字段中任意一个被修改，哈希结果必定不同。

   func ComputeLegHash(in LegHashInput) string {
	  raw := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s",
		  in.PlanID, in.Seq, string(in.LegType),
		  in.UserID,
		  in.Asset, in.Amount, string(in.Status),
	  )
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// ComputeProofHash 按腿序号升序排列各腿哈希后拼接，计算收口证明根哈希。
// 巡检时重新计算此哈希与 CloseProof.ProofHash 核对，不符则记录篡改。
   func ComputeProofHash(legHashes []string) string {
	   sorted := make([]string, len(legHashes))
	   copy(sorted, legHashes)
	  sort.Strings(sorted)
	   raw := strings.Join(sorted, "|")
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
  }

// MarshalLegHashes 将腿哈希列表序列化为 JSON 数组字符串（存 CloseProof.LegHashes）。
  func MarshalLegHashes(hashes []string) (string, error) {
	  b, err := json.Marshal(hashes)
	   if err != nil {
		  return "", err
	  }
	   return string(b), nil
  }

// UnmarshalLegHashes 解析 CloseProof.LegHashes JSON 字符串。
   func UnmarshalLegHashes(s string) ([]string, error) {
	  if s == "" || s == "[]" {
		   return nil, nil
	   }
	   var out []string
	  if err := json.Unmarshal([]byte(s), &out); err != nil {
		  return nil, fmt.Errorf("dispclaw: unmarshal leg hashes: %w", err)
	   }
	  return out, nil
   }

// VerifyProof 用当前 legs 重新计算根哈希，与存储的 proof 核对。
// 返回 ErrProofTamper 表示检测到篡改。

   func VerifyProof(proof CloseProof, legs []RecoveryLeg) error {
	   currentHashes := make([]string, 0, len(legs))
	   for _, l := range legs {
		   h := ComputeLegHash(LegHashInput{
			   PlanID: l.PlanID,
			   Seq:     l.Seq,
			  LegType: l.LegType,
			  UserID: l.UserID,
			  Asset:  l.Asset,
			  Amount: l.Amount,
			   Status: l.Status,
		   })
		   currentHashes = append(currentHashes, h)
	   }
	  recomputed := ComputeProofHash(currentHashes)
	   if !strings.EqualFold(recomputed, proof.ProofHash) {
		  return ErrProofTamper
	  }
	  return nil
  }

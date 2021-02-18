package availcas

import (
	"fmt"
	"strings"
	"time"
)

// ProofSummary 供 desk 面板展示的竞态证明摘要（脱敏版）。

type ProofSummary struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Book string `json:"book"`
	WinnerIntentID string `json:"winner_intent_id"`
	LoserIntentID  string `json:"loser_intent_id"`
	WinnerAmount string `json:"winner_amount"`
	LoserAmount  string `json:"loser_amount"`
	AvailAtDecision  string     `json:"avail_at_decision"`
	RejectCode       RejectCode `json:"reject_code"`
	WindowDurationMs int64      `json:"window_duration_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	Note string `json:"note,omitempty"`
}

// ToSummary 将 RaceProof 转换为 desk 展示摘要。
   func (p *RaceProof) ToSummary() ProofSummary {
	   if p == nil {
		   return ProofSummary{}
	  }
	   ms := p.WindowEnd.Sub(p.WindowStart).Milliseconds()
	  return ProofSummary{
		   ID:     p.ID,
		   UserID: p.UserID,
		   Asset:          p.Asset,
		  Book:           p.Book,
		  WinnerIntentID: p.WinnerIntentID,
		   LoserIntentID: p.LoserIntentID,
		  WinnerAmount:  p.WinnerAmount.String(),
		  LoserAmount:   p.LoserAmount.String(),
		   AvailAtDecision: p.AvailAtDecision.String(),
		   RejectCode: p.RejectCode,
		   WindowDurationMs: ms,
		  CreatedAt:        p.CreatedAt,
		  Note:             p.Note,
	  }
  }

// Validate 检查 RaceProof 字段完整性（巡检 worker 调用）。
// 返回 nil 表示证明字段合法；否则返回描述性错误。

  func (p *RaceProof) Validate() error {
	   if p == nil {
		   return fmt.Errorf("raceproof: nil proof")
	   }
	   var missing []string
	   if strings.TrimSpace(p.ID) == "" {
		   missing = append(missing, "id")
	  }
	   if strings.TrimSpace(p.UserID) == "" {
		   missing = append(missing, "user_id")
	   }
	  if strings.TrimSpace(p.Asset) == "" {
		   missing = append(missing, "asset")
	   }
	  if strings.TrimSpace(p.LeaseID) == "" {
		  missing = append(missing, "lease_id")
	  }
	   if strings.TrimSpace(p.LoserIntentID) == "" {
		  missing = append(missing, "loser_intent_id")
	  }
	  if p.RejectCode == "" {
		  missing = append(missing, "reject_code")
	  }
	  if len(missing) > 0 {
		   return fmt.Errorf("raceproof validate: missing fields: %s", strings.Join(missing, ", "))
	   }
	  if p.WindowEnd.Before(p.WindowStart) {
		  return fmt.Errorf("raceproof validate: window_end %s before window_start %s",
			   p.WindowEnd.Format(time.RFC3339), p.WindowStart.Format(time.RFC3339))
	   }
	  return nil
   }

// RejectCodeDescription 返回 RejectCode 的人类可读描述。
  func RejectCodeDescription(code RejectCode) string {
	   switch code {
	   case RejectCodeCASConflict:
		   return "CAS 世代或期望可用不一致：同窗口存在先行意图，须申请新租约重试"
	   case RejectCodeLeaseExpired:
		  return "租约已过期：超出 TTL 窗口，须申请新租约重试"
	  case RejectCodeLeaseRevoked:
		  return "租约已撤销：余额被外部变更，须重新申请租约"
	  case RejectCodeInsufficientAvail:
		  return "超提：意图扣减额超过租约快照可用余额"
	  case RejectCodeDuplicateIdem:
		  return "幂等键重复：相同请求已处理，直接返回已有裁决"
	   default:
		   return fmt.Sprintf("未知拒绝码: %s", code)
	   }
   }

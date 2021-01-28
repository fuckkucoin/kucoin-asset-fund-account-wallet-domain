// 功能：summary.go
// 单笔输出的只读合页：相位、差额、回收单、输出槽，以及是否还能再贷记。
// 已冲正、未批准的回收单、巡检已锁定的输出，第二笔都必须是拒绝。
// CreateDate: 2025.03.06

// 作者：MilesFord
// 日期：2021.01.08

package deposit

// OutputSummary 客服与巡检看一笔输出时的合页。

type OutputSummary struct {
	Chain       string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	Phase string `json:"phase"`
	PhaseLabel  string `json:"phase_label"`
	Confirming  bool   `json:"confirming"`
	Reversed    bool   `json:"reversed"`
	Explain     string `json:"explain,omitempty"`
	AvailBefore string `json:"avail_before,omitempty"`
	AvailAfter   string `json:"avail_after,omitempty"`
	ReverseEntry string `json:"reverse_entry,omitempty"`
	DiffKind string `json:"diff_kind,omitempty"`
	TicketID string `json:"ticket_id,omitempty"`
	Blocked  bool   `json:"blocked"`
	SecondCredit  bool   `json:"second_credit_allowed"`
	RecoveryKind  string `json:"recovery_kind,omitempty"`
	RecoveryState string `json:"recovery_state,omitempty"`
	SlotState string `json:"slot_state,omitempty"`
	Hint string `json:"hint,omitempty"`
}

// Summarize 汇总一笔输出。没有记录时相位为 none，第二笔贷记仍按槽位判断。

   func (s *Service) Summarize(chain, hash string, output int) OutputSummary {
	  out := OutputSummary{Chain: CanonicalChain(chain), TxHash: hash, OutputIndex: output, Phase: PhaseNone}
	  if s == nil {
		   out.PhaseLabel = FaceLabel(PhaseNone)
		   out.Hint = "入金服务未装配"
		  return out
	  }
	  s.ensureGuards()
	  out.PhaseLabel = FaceLabel(out.Phase)
	   if face, err := s.ReadFace(chain, hash, output); err == nil {
		  out.Phase = face.Phase
		  out.PhaseLabel = face.PhaseLabel
		  out.Confirming = face.Confirming
		  out.Reversed = face.Reversed
		  out.Explain = face.Explain
		   out.AvailBefore = face.AvailBefore
		  out.AvailAfter = face.AvailAfter
		  out.ReverseEntry = face.ReverseEntry
	   }
	   if patrol, ok := s.patrol.Get(chain, hash, output); ok {
		  out.DiffKind = patrol.Kind
		   out.TicketID = patrol.TicketID
		  out.Blocked = patrol.Blocked
	  }
	   if slot, ok := s.slots.Get(chain, hash, output); ok {
		  out.SlotState = string(slot.State)
		  if slot.State == SlotLocked || slot.State == SlotReversed || slot.State == SlotCredited {
			   out.Blocked = true
		  }
	  }
	   if rec, ok := s.recover.Approved(chain, hash, output); ok {
		   out.RecoveryKind = rec.Kind
		   out.RecoveryState = rec.State
	  } else if err := s.recover.BlocksObserve(chain, hash, output); err != nil {
		   out.Blocked = true
		   out.RecoveryState = RecoverOpen
		   rows := s.recover.List("", 200)
		   for _, row := range rows {
			   if row.Chain == out.Chain && row.TxHash == hash && row.OutputIndex == output {
				  out.RecoveryKind = row.Kind
				   out.RecoveryState = row.State
				  break
			   }
		  }
	  }
	  err := s.slots.Allow(chain, hash, "", output)
	   out.SecondCredit = err == nil && !out.Blocked && !out.Reversed
	   if out.Reversed {
		  out.SecondCredit = false
		  out.Hint = "已冲正，不能再次贷记"
	   } else if out.Confirming {
		  out.Hint = "确认中，冲正前不能把同一输出再贷记一次"
	   } else if out.RecoveryState != "" && out.RecoveryState != RecoverApproved {
		  out.SecondCredit = false
		   out.Hint = "回收工单未批准，观察线程不能入账"
	  } else if out.DiffKind != "" {
		   out.SecondCredit = false
		   if copy, ok := CopyOf(out.DiffKind); ok {
			  out.Hint = copy.Hint
		  }
	  }
	   return out
   }

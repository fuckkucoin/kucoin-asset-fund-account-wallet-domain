package xferloop

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// Edge 扫描时对一条半截单的处理。

type Edge struct {
	Action string `json:"action"`
	Code   string `json:"code"`
	Reason string `json:"reason,omitempty"`
}

  func splitPhase(phase string) (base, comp string) {
	   phase = strings.TrimSpace(phase)
	   if i := strings.Index(phase, ":"); i >= 0 {
		   return phase[:i], phase[i+1:]
	  }
	   return phase, ""
  }

  func knownForward(phase string) bool {
	  switch phase {
	  case "init", "quota_reserved", "ledger_out", "overlay_in", "hopped", "overlay_out", "ledger_in":
		   return true
	   default:
		   return false
	   }
   }

// classifyHop 母子 hop 相位。done/aborted 视为已收口；未知相位升级。
  func classifyHop(phase string) Edge {
	   base, comp := splitPhase(phase)
	  switch base {
	   case "done", "aborted":
		   return Edge{Action: actionRecovered, Code: "hop_terminal"}
	  case "aborting":
		  if comp == "" {
			   return Edge{Action: actionDrive, Code: "hop_compensate_infer"}
		   }
		   if !knownForward(comp) {
			   return Edge{Action: actionEscalate, Code: "hop_comp_unknown", Reason: "compensate phase unknown"}
		   }
		  return Edge{Action: actionDrive, Code: "hop_compensate"}
	   case "":
		   return Edge{Action: actionEscalate, Code: "hop_phase_empty", Reason: "hop phase empty"}
	  default:
		  if knownForward(base) {
			  return Edge{Action: actionDrive, Code: "hop_forward"}
		  }
		  return Edge{Action: actionEscalate, Code: "hop_phase_unknown", Reason: "hop phase unknown"}
	  }
   }

// classifyInner 内部转。已入账的金额差异由调用方另行升级，这里拦自转、空对手和非正金额。
  func classifyInner(tr *ledger.Transfer) Edge {
	   if tr == nil || tr.Direction != ledger.SpotSend {
		   return Edge{Action: actionSkip, Code: "not_inner"}
	  }
	  if !tr.Amount.IsPositive() {
		  return Edge{Action: actionEscalate, Code: "inner_amount", Reason: "inner amount not positive"}
	  }
	   dest := strings.TrimSpace(tr.Counterparty())
	   if dest == "" {
		   return Edge{Action: actionEscalate, Code: "inner_no_dest", Reason: "counterparty missing"}
	  }
	   if strings.EqualFold(dest, tr.UserID) {
		   return Edge{Action: actionEscalate, Code: "inner_self", Reason: "self transfer"}
	  }
	  if tr.State == ledger.StateCredited || tr.State == ledger.StateRolledBack {
		  return Edge{Action: actionWait, Code: "inner_terminal_check"}
	  }
	  return Edge{Action: actionDrive, Code: "inner_open"}
  }

// CoverView 补账与主单、撤销单的关系，供分类，不在这里动账。

type CoverView struct {
	Cover  *ledger.Transfer
	Parent *ledger.Transfer
	ParentMissing bool
	Undo        *ledger.Transfer
	UndoMissing bool
	Now         time.Time
	MinAge time.Duration
}

// classifyCover 残留补账。主单仍在用则跳过；撤销已入账则收口；撤销未过账龄则等待。
  func classifyCover(v CoverView) Edge {
	   tr := v.Cover
	  if tr == nil || !ledger.IsAutoCoverKey(tr.IdempotencyKey) {
		   return Edge{Action: actionSkip, Code: "not_cover"}
	   }
	   if tr.State != ledger.StateCredited {
		   return Edge{Action: actionSkip, Code: "cover_not_credited"}
	   }
	  if !v.ParentMissing && v.Parent != nil &&
		   v.Parent.State != ledger.StateFailed && v.Parent.State != ledger.StateRolledBack {
		  return Edge{Action: actionSkip, Code: "cover_parent_live"}
	  }
	   if !v.UndoMissing && v.Undo != nil && v.Undo.State == ledger.StateCredited {
		   return Edge{Action: actionRecovered, Code: "cover_undone"}
	   }
	   if v.Undo != nil && v.Undo.State != ledger.StateCredited && v.Undo.State != ledger.StateFailed && v.Undo.State != ledger.StateRolledBack {
		  at := v.Undo.CreatedAt
		  now := v.Now
		   if now.IsZero() {
			   now = time.Now().UTC()
		   }
		   min := v.MinAge
		  if min <= 0 {
			   min = 14 * time.Second
		  }
		   if !at.IsZero() && now.Sub(at) < min {
			   return Edge{Action: actionWait, Code: "cover_undo_young", Reason: "undo still inside lock window"}
		  }
		  return Edge{Action: actionDrive, Code: "cover_undo_open"}
	  }
	   if v.Undo != nil && (v.Undo.State == ledger.StateFailed || v.Undo.State == ledger.StateRolledBack) {
		  return Edge{Action: actionDrive, Code: "cover_undo_failed", Reason: "undo did not credit"}
	  }
	  if v.ParentMissing {
		   return Edge{Action: actionDrive, Code: "cover_parent_missing", Reason: "parent missing"}
	   }
	  return Edge{Action: actionDrive, Code: "cover_stranded"}
   }

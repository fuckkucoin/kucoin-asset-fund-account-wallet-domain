// 金额用十进制字符串，时间用 RFC3339。kind 不在已知集合内的事件不得发出。

// Author: EliotVoss
// Date: 2021.01.08

package deposit

import (
	"strings"
)

const EventDepositDiff = "deposit.diff"

const (
	KindObserveNE     = "observe_ne_credit"
	KindConfirmedMiss = "confirmed_unccredited"
	KindDupOutput     = "dup_output_second_credit"
	KindEarlyRollback  = "early_rollback"
	KindWrongMemoEvent = "wrong_memo"
	KindWrongNetEvent = "wrong_network"
	KindUnattribEvent = "unattributed"
)

const (
	PhaseConfirming = "confirming"
	PhaseReversed   = "reversed"
	PhaseCredited   = "credited"
	PhaseRecovery   = "recovery"
	PhaseHeld = "held"
	PhaseNone = "none"
)

// DepositDiffEvent 观测与贷记差的字段级契约。
// 时间用 RFC3339。金额为十进制字符串，禁止科学计数。

type DepositDiffEvent struct {
	Event string `json:"event"`
	Kind   string `json:"kind"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int `json:"output_index"`
	Asset          string `json:"asset"`
	UserID         string `json:"user_id"`
	ObservedAmount string `json:"observed_amount"`
	CreditedAmount string `json:"credited_amount"`
	ObserveState string `json:"observe_state"`
	CreditState string `json:"credit_state"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	SecondCredit  bool   `json:"second_credit"`
	Blocked       bool   `json:"blocked"`
	TicketID      string `json:"ticket_id"`
	DepositTicket string `json:"deposit_ticket_id"`
	Phase         string `json:"phase"`
	ReverseEntry  string `json:"reverse_entry"`
	AvailBefore   string `json:"avail_before"`
	AvailAfter string `json:"avail_after"`
	Reason     string `json:"reason"`
}

// ContractFields 事件字段名，供读面与对账追加一致。

  func ContractFields() []string {
	  return []string{
		  "event", "kind", "chain", "tx_hash", "output_index", "asset", "user_id",
		  "observed_amount", "credited_amount", "observe_state", "credit_state",
		   "window_start", "window_end", "second_credit", "blocked",
		  "ticket_id", "deposit_ticket_id", "phase", "reverse_entry",
		  "avail_before", "avail_after", "reason",
	  }
  }

   func KnownDiffKind(kind string) bool {
	   switch strings.TrimSpace(kind) {
	   case KindObserveNE, KindConfirmedMiss, KindDupOutput, KindEarlyRollback,
		   KindWrongMemoEvent, KindWrongNetEvent, KindUnattribEvent:
		  return true
	  default:
		  return false
	   }
   }

   func KnownPhase(phase string) bool {
	  switch strings.TrimSpace(phase) {
	  case "", PhaseConfirming, PhaseReversed, PhaseCredited, PhaseRecovery, PhaseHeld, PhaseNone:
		   return true
	   default:
		   return false
	   }
  }

// NormalizeEvent 补默认事件名并清洗空白。不改金额精度。
   func NormalizeEvent(ev DepositDiffEvent) DepositDiffEvent {
	   ev.Event = EventDepositDiff
	  ev.Kind = strings.TrimSpace(ev.Kind)
	  ev.Chain = CanonicalChain(ev.Chain)
	   ev.TxHash = strings.TrimSpace(ev.TxHash)
	  ev.Asset = strings.ToUpper(strings.TrimSpace(ev.Asset))
	   ev.UserID = strings.TrimSpace(ev.UserID)
	  ev.ObservedAmount = strings.TrimSpace(ev.ObservedAmount)
	  ev.CreditedAmount = strings.TrimSpace(ev.CreditedAmount)
	  ev.ObserveState = strings.TrimSpace(ev.ObserveState)
	  ev.CreditState = strings.TrimSpace(ev.CreditState)
	   ev.Phase = strings.TrimSpace(ev.Phase)
	   ev.ReverseEntry = strings.TrimSpace(ev.ReverseEntry)
	  ev.AvailBefore = strings.TrimSpace(ev.AvailBefore)
	   ev.AvailAfter = strings.TrimSpace(ev.AvailAfter)
	  ev.Reason = strings.TrimSpace(ev.Reason)
	   ev.TicketID = strings.TrimSpace(ev.TicketID)
	  ev.DepositTicket = strings.TrimSpace(ev.DepositTicket)
	  return ev
  }

package replay

import "time"

const (
	EffNone   = "none"
	EffCredit = "credit"
	EffDebit  = "debit"
	EffFreeze = "freeze"
	EffUnfreeze = "unfreeze"
	EffOut = "xfer_out"
	EffIn = "xfer_in"
)

// Known 效果位是否被机接受。未知效果拒，避免静默丢掉一笔。
  func Known(effect string) bool {
	  switch effect {
	   case EffNone, EffCredit, EffDebit, EffFreeze, EffUnfreeze, EffOut, EffIn:
		  return true
	  default:
		  return false
	  }
   }

// Event 一条已入库事实在重放里的投影。
type Event struct {
	Seq    int    `json:"seq"`
	Kind   string `json:"kind"`
	Effect string `json:"effect"`
	UserID string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	RefID   string `json:"ref_id"`
	IdemKey string `json:"idem_key"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Snap 只读账面。重放拿它做差，不负责把它写回去。

type Snap struct {
	UserID string `json:"user_id"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

// Step 轨迹上的一步。Note 解释跳过、不足或钳制。

type Step struct {
	Ord int `json:"ord"`
	Kind  string `json:"kind"`
	RefID string `json:"ref_id,omitempty"`
	IdemKey   string `json:"idem_key,omitempty"`
	Amount    string `json:"amount"`
	Effect    string `json:"effect"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

// Report 重放报告。Verdict=diff 时调用方开差异告警，不推进账本。

type Report struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	Steps           []Step    `json:"steps"`
	ReplayAvailable string    `json:"replay_available"`
	ReplayFrozen    string    `json:"replay_frozen"`
	BookAvailable   string    `json:"book_available,omitempty"`
	BookFrozen      string    `json:"book_frozen,omitempty"`
	DiffAvailable string   `json:"diff_available,omitempty"`
	DiffFrozen    string   `json:"diff_frozen,omitempty"`
	Missing       []string `json:"missing,omitempty"`
	Verdict    string `json:"verdict"`
	CaseOpened bool   `json:"case_opened"`
	Note       string `json:"note,omitempty"`
}

// Input 一次重放。Snap 为空则只出轨迹，不和账面比。
type Input struct {
	UserID string
	Asset string
	From  time.Time
	To    time.Time
	Events []Event
	Snap   *Snap
	Expect []string
}

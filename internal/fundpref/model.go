package fundpref

import (
	"fmt"
	"time"
)

// ─────────────────────────────────────────────
// 枚举
// ─────────────────────────────────────────────
// AccountBook 资金科目枚举（只读消费 account 包的 Book 值）。
type AccountBook string

const (
	BookMain    AccountBook = "MAIN"
	BookTrade   AccountBook = "TRADE"
	BookMargin  AccountBook = "MARGIN"
	BookFutures AccountBook = "FUTURES"
	BookEarn    AccountBook = "EARN"
)

// ParseBook 解析科目字符串；未知时返回 ErrInvalidInput。
  func ParseBook(s string) (AccountBook, error) {
	  switch AccountBook(s) {
	   case BookMain,
		  BookTrade, BookMargin, BookFutures, BookEarn:
		   return AccountBook(s), nil
	  default:
		  return "", fmt.Errorf("%w: unknown account book %q", ErrInvalidInput, s)
	   }
   }

// PendingSwitchState 待切换状态机枚举。
type PendingSwitchState string

const (
	PendingSwitchQueued  PendingSwitchState = "queued"
	PendingSwitchApplied PendingSwitchState = "applied"
	PendingSwitchCancelled PendingSwitchState = "cancelled"
)

// ─────────────────────────────────────────────
// PrefGeneration
// ─────────────────────────────────────────────
// PrefGeneration 记录用户偏好世代号；Put DefaultAccount 每次递增。
// Route 路由消费端必须绑定未过期的世代快照，否则拒绝。

type PrefGeneration struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Generation  int64       `json:"generation"`
	DefaultBook AccountBook `json:"default_book"`
	BumpedBy    string      `json:"bumped_by"` // "put_default_account" | "init"
	SourceReqID string `json:"source_req_id"`
	BumpedAt time.Time `json:"bumped_at"`
}

// String 可读摘要。
  func (g PrefGeneration) String() string {
	   return fmt.Sprintf("PrefGeneration{user=%s gen=%d book=%s}", g.UserID, g.Generation, g.DefaultBook)
  }

// ─────────────────────────────────────────────
// DefaultAccountPref
// ─────────────────────────────────────────────
// DefaultAccountPref 用户当前 DefaultAccount 设置与代次。

type DefaultAccountPref struct {
	UserID  string      `json:"user_id"`
	Book    AccountBook `json:"book"`
	Version int64       `json:"version"` // 与 PrefGeneration.Generation 一致
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

// ─────────────────────────────────────────────
// PendingSwitch
// ─────────────────────────────────────────────
// PendingSwitch 在途贷记期间发起切换 DefaultAccount 时，申请进入队列等待。
// 当所有在途贷记完成后，worker 推进状态到 applied。
type PendingSwitch struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	FromBook AccountBook        `json:"from_book"`
	ToBook   AccountBook        `json:"to_book"`
	State    PendingSwitchState `json:"state"`
	RequestedBy string     `json:"requested_by"`
	RequestedAt time.Time  `json:"requested_at"`
	AppliedAt   *time.Time `json:"applied_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CancelNote string `json:"cancel_note,omitempty"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// ─────────────────────────────────────────────
// PutDefaultAccountReq
// ─────────────────────────────────────────────
// PutDefaultAccountReq Put DefaultAccount 请求参数。

type PutDefaultAccountReq struct {
	UserID      string
	Book        AccountBook
	RequestedBy string
	ReqID string // 幂等键
}

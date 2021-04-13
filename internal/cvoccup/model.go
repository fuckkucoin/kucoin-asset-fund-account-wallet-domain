package cvoccup

import (
	"errors"
	"strings"
	"time"
)

// OccStatus 路径占用状态机节点。
// quoting → frozen → partial → settled / rolled_back
type OccStatus string

const (
	// OccQuoting 已锁报价，尚未冻结。
	OccQuoting OccStatus = "QUOTING"
	// OccFrozen 冻结完成，holdcat token 已写入。
	OccFrozen OccStatus = "FROZEN"
	// OccPartial 已发生部分成交，剩余仍占用。
	OccPartial OccStatus = "PARTIAL"
	// OccSettled 全量成交，占用已清零。
	OccSettled OccStatus = "SETTLED"
	// OccRolledBack 失败/撤单/过期后回滚完成，占用已释放。
	OccRolledBack OccStatus = "ROLLED_BACK"
)

// LegKind 占用腿类型。
type LegKind string

const (
	LegSource LegKind = "SOURCE" // 源资产占用腿（from asset freeze）
	LegTarget LegKind = "TARGET" // 目标资产预分配腿（to asset pre-alloc）
)

// PathKind 路径类型（market/limit）。

type PathKind string

const (
	PathMarket PathKind = "MARKET"
	PathLimit PathKind = "LIMIT"
)

var (
	ErrOccNotFound = errors.New("occ path not found")
	ErrOccOwner    = errors.New("occ path owner mismatch")
	ErrOccTerminal = errors.New("occ path already in terminal state")
	ErrOccToken = errors.New("occ token mismatch")
	ErrOccNegative  = errors.New("occ available goes negative")
	ErrOccImbalance = errors.New("bilateral occ imbalance")
	ErrOccStore  = errors.New("occ store unavailable")
	ErrOccAmount = errors.New("occ amount invalid")
	ErrOccLeg    = errors.New("occ leg invalid")
)

// PathOcc 路径占用主记录。
// 对应 cv_path_occs 行；每条限价/市价路径对应一条。
type PathOcc struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	OrderID string `json:"order_id"`
	ClientOID string `json:"client_order_id"`
	PathKind PathKind `json:"path_kind"`
	FromAsset string `json:"from_asset"`
	ToAsset   string `json:"to_asset"`
	TotalAmount  string    `json:"total_amount"`
	FilledAmount string    `json:"filled_amount"`
	RemainAmount string    `json:"remain_amount"`
	Status       OccStatus `json:"status"`
	HoldToken    string    `json:"hold_token"`
	QuoteID string `json:"quote_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FrozenAt  *time.Time `json:"frozen_at,omitempty"`
	SettledAt *time.Time `json:"settled_at,omitempty"`
	RolledAt *time.Time `json:"rolled_at,omitempty"`
	ExpireAt time.Time `json:"expire_at"`
	AccountType string `json:"account_type"`
	RollbackID  string `json:"rollback_id,omitempty"`
	BreachID    string `json:"breach_id,omitempty"`
}

// IsTerminal 判断是否终态。

   func (p *PathOcc) IsTerminal() bool {
	  return p.Status == OccSettled || p.Status == OccRolledBack
   }

// IsActive 判断是否活跃（可成交）。

   func (p *PathOcc) IsActive() bool {
	   return p.Status == OccFrozen || p.Status == OccPartial
  }

// OccLeg 占用腿明细。
// 对应 cv_occ_legs 行；每条 PathOcc 至少有一条源资产腿。

type OccLeg struct {
	ID    string `json:"id"`
	OccID string `json:"occ_id"`
	UserID string `json:"user_id"`
	LegKind LegKind `json:"leg_kind"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Released string `json:"released"`
	HoldRef string `json:"hold_ref"`
	FillSeq string `json:"fill_seq,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Closed    bool      `json:"closed"`
}

// RemainingAmount 腿的剩余未释放量。
  func (l *OccLeg) RemainingAmount() string {
	  return l.Amount
  }

// BreachEvent 穿仓事件。
// 占用后可用为负或双边不平衡时触发，对应 cv_breaches 行。
type BreachEvent struct {
	ID string `json:"id"`
	UserID     string `json:"user_id"`
	OccID      string `json:"occ_id"`
	BreachKind string `json:"breach_kind"`
	Asset     string `json:"asset"`
	OccAmount string `json:"occ_amount"`
	AvailBefore string `json:"avail_before"`
	AvailAfter string     `json:"avail_after"`
	Detail     string     `json:"detail"`
	Resolved   bool       `json:"resolved"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// BreachKind 穿仓类型。

const (
	BreachAvailNeg  = "AVAIL_NEG" // 可用余额转负
	BreachBilateral = "BILATERAL" // 双边不平衡
	BreachTokenDrift = "TOKEN_DRIFT" // 占用令牌与hold不一致
)

// OccSummary 用户资产占用汇总（聚合只读视图）。
type OccSummary struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	TotalOcc string `json:"total_occ"`
	ActivePaths int       `json:"active_paths"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// statusTransitions 合法的状态跃迁。
var statusTransitions = map[OccStatus][]OccStatus{
	OccQuoting: {OccFrozen, OccRolledBack},
	OccFrozen: {OccPartial,
		OccSettled, OccRolledBack},
	OccPartial: {OccSettled, OccRolledBack},
}

// CanTransition 检查状态跃迁是否合法。
  func CanTransition(from, to OccStatus) bool {
	   allowed,
		  ok := statusTransitions[from]
	   if !ok {
		  return false
	   }
	   for _, a := range allowed {
		  if a == to {
			   return true
		  }
	  }
	  return false
  }

// NormalizeAsset 统一资产名大写。

   func NormalizeAsset(a string) string {
	  return strings.ToUpper(strings.TrimSpace(a))
   }

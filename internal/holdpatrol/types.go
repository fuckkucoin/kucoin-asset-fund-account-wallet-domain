package holdpatrol

import "time"

// DriftAction 漂移一致建议操作类型。

type DriftAction string

const (
	DriftActionReleaseTag DriftAction = "release_tag" // 账本已无冻结，建议释放分类标签
	DriftActionOpenGap DriftAction = "open_gap" // 账本冻结不足，建议开缺口单
	DriftActionEscalate   DriftAction = "escalate"    // 超时未释，建议升级处理
	DriftActionMarkBroken DriftAction = "mark_broken" // 令牌与账本严重不符，建议标 broken
)

// DriftAdvice 漂移一致建议单条目。
type DriftAdvice struct {
	TokenID string `json:"token_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Book  string `json:"book"`
	Layer string `json:"layer"`
	TokenAmount string `json:"token_amount"`
	LedgerFrozen string `json:"ledger_frozen"`
	Action DriftAction `json:"action"`
	Reason string `json:"reason"`
}

// PatrolRun 单次巡检结果。
type PatrolRun struct {
	RunID string `json:"run_id"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	TokensScanned int       `json:"tokens_scanned"`
	GapsOpened   int `json:"gaps_opened"`
	TagsReleased int `json:"tags_released"`
	Escalated    int `json:"escalated"`
	Advices []DriftAdvice `json:"advices,omitempty"`
	Err     string        `json:"err,omitempty"`
}

// Config 巡检配置参数。

type Config struct {
	// ReleasingTimeout 处于 releasing 中间态超过此时长升级
	ReleasingTimeout time.Duration
	// ActiveMaxAge active token 超过此时长开超时升级缺口
	ActiveMaxAge time.Duration
	// ScanLimit 单次扫描最大 token 数
	ScanLimit int
	// AutoReleaseOrphanTags 是否自动释放孤儿分类标签（账本已无冻结）
	AutoReleaseOrphanTags bool
}

// DefaultConfig 返回推荐默认配置。

   func DefaultConfig() Config {
	  return Config{
		  ReleasingTimeout:      15 * time.Minute,
		  ActiveMaxAge:          72 * time.Hour,
		  ScanLimit:             500,
		  AutoReleaseOrphanTags: false, // 默认不自动释放；需运营确认
	   }
   }

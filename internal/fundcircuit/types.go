package fundcircuit

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Path 资金写路径。总线只识别这三条入口，不进各域内部跃迁。

type Path string

const (
	// PathDepositObserve 入金观察写入口（deposit.Observe）。
	PathDepositObserve Path = "deposit_observe"
	// PathWithdrawRequest 出金申请入口（withdraw.Request）。
	PathWithdrawRequest Path = "withdraw_request"
	// PathTransferHop 内部划转科目 hop 入口（fundpath.Hop）。
	PathTransferHop Path = "transfer_hop"
)

// Paths 全部受管入口，按阻断矩阵展示顺序返回。

   func Paths() []Path {
	   return []Path{PathDepositObserve,
		   PathWithdrawRequest, PathTransferHop}
   }

// Valid 只认三条受管入口；未知入口一律拒绝，避免把未接闸的写路径当成已放行。
   func (p Path) Valid() bool {
	   switch p {
	  case PathDepositObserve, PathWithdrawRequest, PathTransferHop:
		   return true
	   }
	   return false
  }

   func (p Path) String() string { return string(p) }

// ParsePath 解析入口名。空值与未知值都返回 ErrPath，不落默认入口。
  func ParsePath(s string) (Path, error) {
	  p := Path(strings.ToLower(strings.TrimSpace(s)))
	  if !p.Valid() {
		   return "", ErrPath
	  }
	  return p, nil
   }

// Scope 熔断作用域。global 挡全部受管入口；user/asset/chain 只挡匹配目标。

type Scope string

const (
	ScopeUser Scope = "user"
	ScopeAsset  Scope = "asset"
	ScopeChain  Scope = "chain"
	ScopeGlobal Scope = "global"
)

// Scopes 作用域评估顺序：先窄后宽，读面与闸门共用同一顺序。

  func Scopes() []Scope {
	   return []Scope{ScopeUser, ScopeAsset, ScopeChain, ScopeGlobal}
  }

// Valid 作用域白名单。拼错的作用域不得落成新键位。
  func (s Scope) Valid() bool {
	   switch s {
	  case ScopeUser, ScopeAsset, ScopeChain, ScopeGlobal:
		   return true
	   }
	  return false
   }

  func (s Scope) String() string { return string(s) }

// ParseScope 解析作用域；空值按 global 处理，未知值返回 ErrScope。
  func ParseScope(s string) (Scope, error) {
	  v := Scope(strings.ToLower(strings.TrimSpace(s)))
	  if v == "" {
		   return ScopeGlobal, nil
	   }
	   if !v.Valid() {
		   return "", ErrScope
	  }
	  return v, nil
   }

// SignalKind 滥用信号来源。dep/wd/hop 速度由既有占用表桥接而来，键形不改。

type SignalKind string

const (
	// SigDepVelocity 入金速度，来源 usage 的 dep: 键。
	SigDepVelocity SignalKind = "dep_velocity"
	// SigWdVelocity 出金速度，来源 usage 出金占用与 wd: 跳闸键。
	SigWdVelocity SignalKind = "wd_velocity"
	// SigHopVelocity 划转 hop 速度，来源划转占用。
	SigHopVelocity SignalKind = "hop_velocity"
	// SigHTTPBurst HTTP 限流桶溢出，来源 ratelimit 读面。
	SigHTTPBurst SignalKind = "http_burst"
	// SigJobHalt 通用作业回压停写，来源 backpress。
	SigJobHalt SignalKind = "job_halt"
	// SigDestFanout 出金目标地址扩散。
	SigDestFanout SignalKind = "dest_fanout"
	// SigReorgChurn 观察回滚翻动。
	SigReorgChurn SignalKind = "reorg_churn"
	// SigManual 值班手工标注。
	SigManual SignalKind = "manual"
)

// SignalKinds 全部受理信号种类。

   func SignalKinds() []SignalKind {
	  return []SignalKind{
		  SigDepVelocity,
		   SigWdVelocity, SigHopVelocity, SigHTTPBurst,
		   SigJobHalt, SigDestFanout, SigReorgChurn, SigManual,
	   }
  }

// Valid 信号白名单。未登记的种类不进聚合，避免打分被任意来源撑高。

  func (k SignalKind) Valid() bool {
	   for _, v := range SignalKinds() {
		   if v == k {
			   return true
		  }
	   }
	  return false
  }

   func (k SignalKind) String() string { return string(k) }

// ParseSignalKind 解析信号种类。
  func ParseSignalKind(s string) (SignalKind, error) {
	  k := SignalKind(strings.ToLower(strings.TrimSpace(s)))
	   if !k.Valid() {
		   return "", ErrSignalKind
	   }
	   return k, nil
  }

// BaseWeight 各信号默认权重。入参未给权重时取此值；权重不得为负。

	  }
	  return 0
   }

// HintPath 信号的来源入口。用于跨路径联动判定（不同入口的信号才算跨路径）。

  func (k SignalKind) HintPath() Path {
	   switch k {
	  case SigDepVelocity, SigReorgChurn:
		   return PathDepositObserve
	   case SigWdVelocity, SigDestFanout:
		  return PathWithdrawRequest
	   case SigHopVelocity:
		  return PathTransferHop
	   }
	   return ""
  }

// AbuseSignal 一条滥用信号。SourceKey 保留原域键（dep:/wd:/rl:）原文，只读不改写。
type AbuseSignal struct {
	ID string `json:"id"`
	Kind   SignalKind `json:"kind"`
	Path   Path       `json:"path,omitempty"`
	UserID string     `json:"user_id,omitempty"`
	Chain string `json:"chain,omitempty"`
	Asset  string `json:"asset,omitempty"`
	Weight int    `json:"weight"`
	SourceKey string `json:"source_key,omitempty"`
	Note      string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

// CircuitState 熔断状态。closed→open→half_open→closed；半开失败回 open。
type CircuitState string

const (
	StateClosed CircuitState = "closed"
	StateOpen CircuitState = "open"
	StateHalfOpen CircuitState = "half_open"
)

// Valid 状态白名单。
  func (s CircuitState) Valid() bool {
	   switch s {
	  case StateClosed, StateOpen, StateHalfOpen:
		  return true
	   }
	  return false
  }

  func (s CircuitState) String() string { return string(s) }

// Blocking 是否处于拦截态。半开也算拦截，仅探针可穿过。
   func (s CircuitState) Blocking() bool {
	  return s == StateOpen || s == StateHalfOpen
   }

// Severity 分档。用于阻断矩阵取舍与是否升级全局停充提。

type Severity string

const (
	SevNone  Severity = "none"
	SevWatch Severity = "watch"
	SevWarn Severity = "warn"
	SevTrip Severity = "trip"
	SevHalt Severity = "halt"
)

// Rank 分档序号，用于矩阵比较；未知分档按 none 处理。
  func (s Severity) Rank() int {
	   switch s {
	  case SevWatch:
		   return 1
	  case SevWarn:
		   return 2
	  case SevTrip:
		   return 3
	  case SevHalt:
		   return 4
	   }
	  return 0
   }

   func (s Severity) String() string { return string(s) }

// Valid 分档白名单。

   func (s Severity) Valid() bool {
	  switch s {
	  case SevNone, SevWatch, SevWarn, SevTrip, SevHalt:
		   return true
	  }
	   return false
  }

// ParseSeverity 解析分档；空值按 trip 处理（矩阵默认在跳闸档生效）。
  func ParseSeverity(s string) (Severity, error) {
	  v := Severity(strings.ToLower(strings.TrimSpace(s)))
	   if v == "" {
		  return SevTrip, nil
	  }
	   if !v.Valid() {
		   return "", ErrSeverity
	  }
	   return v, nil
  }

// Circuit 一个作用域键上的熔断器。Key 恒为 fc: 前缀，与 dep:/wd: 键位互不覆盖。

type Circuit struct {
	Key string `json:"key"`
	Scope  Scope  `json:"scope"`
	UserID string `json:"user_id,omitempty"`
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
	State CircuitState `json:"state"`
	Score int `json:"score"`
	Severity Severity `json:"severity"`
	Reason string `json:"reason,omitempty"`
	Trips        int `json:"trips"`
	ProbesIssued int `json:"probes_issued"`
	Forced bool `json:"forced"`
	Actor     string `json:"actor,omitempty"`
	CaseID    string `json:"case_id,omitempty"`
	CrossPath bool   `json:"cross_path"`
	PolicyVer int    `json:"policy_ver"`
	OpenedAt   time.Time `json:"opened_at,omitempty"`
	HalfOpenAt time.Time `json:"half_open_at,omitempty"`
	ClosedAt   time.Time `json:"closed_at,omitempty"`
	NextAdvanceAt time.Time     `json:"next_advance_at,omitempty"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Fence         persist.Fence `json:"fence"`
}

// Blocking 熔断器当前是否拦截受管入口。
   func (c Circuit) Blocking() bool { return c.State.Blocking() }

// Target 作用域目标的可读值，供读面与审计。
  func (c Circuit) Target() string {
	  switch c.Scope {
	   case ScopeUser:
		   return c.UserID
	  case ScopeAsset:
		   return c.Asset
	  case ScopeChain:
		  return c.Chain
	  }
	   return "global"
  }

// ProbeState 探针状态。半开期内只允许一枚 pending 探针。

type ProbeState string

const (
	ProbePending ProbeState = "pending"
	ProbePassed ProbeState = "passed"
	ProbeFailed  ProbeState = "failed"
	ProbeExpired ProbeState = "expired"
)

   func (s ProbeState) String() string { return string(s) }

// Probe 半开探针。Token 与 ID 成对核验，避免旧探针回写覆盖新一轮结果。

type Probe struct {
	ID         string     `json:"id"`
	CircuitKey string     `json:"circuit_key"`
	Path       Path       `json:"path"`
	State      ProbeState `json:"state"`
	Token string `json:"token"`
	Actor string `json:"actor,omitempty"`
	Round int `json:"round"`
	Note      string    `json:"note,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	DecidedAt time.Time `json:"decided_at,omitempty"`
}

// Pending 探针是否仍在等待结论。

   func (p Probe) Pending() bool { return p.State == ProbePending }

// CaseState 案件状态。open→review→cleared；cleared 须双人复核。
type CaseState string

const (
	CaseOpen    CaseState = "open"
	CaseReview  CaseState = "review"
	CaseCleared CaseState = "cleared"
)

  func (s CaseState) String() string { return string(s) }

// CaseAudit 案件动作留痕。单人 Clear 被拒也会留一条，便于复盘。
type CaseAudit struct {
	Action string `json:"action"`
	Actor string `json:"actor"`
	From CaseState `json:"from"`
	To   CaseState `json:"to"`
	Note string    `json:"note,omitempty"`
	At time.Time `json:"at"`
}

// Case 熔断案件。与熔断器一对一（同一键同时只有一个未结案件）。

type Case struct {
	ID         string `json:"id"`
	CircuitKey string `json:"circuit_key"`
	Scope Scope `json:"scope"`
	UserID string `json:"user_id,omitempty"`
	Chain    string    `json:"chain,omitempty"`
	Asset    string    `json:"asset,omitempty"`
	State    CaseState `json:"state"`
	Score    int       `json:"score"`
	Severity Severity  `json:"severity"`
	Reason   string    `json:"reason,omitempty"`
	OpenedBy string `json:"opened_by,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
	ClearReq   string   `json:"clear_req,omitempty"`
	ClearVotes []string `json:"clear_votes,omitempty"`
	Audit []CaseAudit `json:"audit,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GateReq 入口闸入参。金额只作读面记录，不参与限额判定（限额仍归 limitgate/policy）。

type GateReq struct {
	Path Path `json:"path"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain,omitempty"`
	Asset  string `json:"asset,omitempty"`
	FromBook string `json:"from_book,omitempty"`
	ToBook string `json:"to_book,omitempty"`
	Amount string `json:"amount,omitempty"`
	ProbeID string `json:"probe_id,omitempty"`
	Token string `json:"token,omitempty"`
}

// Decision 闸决策。AutoUnfreeze 恒为 false、InflightPreserved 恒为 true：
// 熔断只挡新写，不得释放筛查中或已广播的在途单。

type Decision struct {
	Path       Path   `json:"path"`
	Allow      bool   `json:"allow"`
	Reason     string `json:"reason,omitempty"`
	CircuitKey string `json:"circuit_key,omitempty"`
	Scope Scope `json:"scope,omitempty"`
	State CircuitState `json:"state,omitempty"`
	Score    int      `json:"score"`
	Severity Severity `json:"severity"`
	PolicyVer int    `json:"policy_ver"`
	ProbeID   string `json:"probe_id,omitempty"`
	CaseID string `json:"case_id,omitempty"`
	AutoUnfreeze bool `json:"auto_unfreeze"`
	InflightPreserved bool      `json:"inflight_preserved"`
	At                time.Time `json:"at"`
}

// Unfreezes 恒为 false。调用方据此确认本决策不触发解冻。
  func (d Decision) Unfreezes() bool { return d.AutoUnfreeze }

// InflightRef 在途单引用。用于核验熔断不解冻筛查中/广播中的出金。
type InflightRef struct {
	TransferID string `json:"transfer_id"`
	UserID string `json:"user_id,omitempty"`
	Asset  string `json:"asset,omitempty"`
	FreezeState string `json:"freeze_state,omitempty"`
	Broadcast   string `json:"broadcast,omitempty"`
	TxHash      string `json:"tx_hash,omitempty"`
}

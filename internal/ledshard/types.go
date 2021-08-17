package ledshard

import "time"

const (
	SubBridge = "system.bridge"
	SubFee = "system.fee"
	SubCold = "system.cold"
	ShardBridge = "sh-sys-bridge"
	ShardFee = "sh-sys-fee"
	ShardCold = "sh-sys-cold"
	DefaultRing = 8
)

type ShardKind string

const (
	KindUser   ShardKind = "user"
	KindSystem ShardKind = "system"
)

type ShardStatus string

const (
	ShardActive ShardStatus = "active"
	ShardDraining ShardStatus = "draining"
	ShardSealed ShardStatus = "sealed"
)

type RouteStatus string

const (
	RouteDraft      RouteStatus = "draft"
	RoutePublishing RouteStatus = "publishing"
	RouteActive     RouteStatus = "active"
	RouteRetired RouteStatus = "retired"
)

type IntentState string

const (
	StateOpen      IntentState = "open"
	StatePreparing IntentState = "preparing"
	StateFrozen    IntentState = "frozen"
	StateVouchered IntentState = "vouchered"
	StateCredited  IntentState = "credited"
	StateCommitted IntentState = "committed"
	StateCompensating IntentState = "compensating"
	StateCompensated IntentState = "compensated"
	StateAborted     IntentState = "aborted"
	StateDesk        IntentState = "desk"
)

type StepKind string

const (
	StepPrepare StepKind = "prepare"
	StepFreeze StepKind = "freeze_source"
	StepMint StepKind = "mint_voucher"
	StepCredit StepKind = "credit_target"
	StepCommit     StepKind = "commit"
	StepCompensate StepKind = "compensate"
	StepAbort      StepKind = "abort"
	StepDesk     StepKind = "desk"
	StepReverse  StepKind = "reverse_credit"
	StepVoid     StepKind = "void_voucher"
	StepUnfreeze StepKind = "unfreeze_source"
)

type VoucherState string

const (
	VoucherMinted   VoucherState = "minted"
	VoucherConsumed VoucherState = "consumed"
	VoucherVoided VoucherState = "voided"
)

type DeskState string

const (
	DeskOpen DeskState = "open"
	DeskApproved DeskState = "approved"
	DeskExecuted DeskState = "executed"
	DeskRejected DeskState = "rejected"
)

// Shard 是分区目录上的一片。系统片只接受钉扎科目，不进用户环。

type Shard struct {
	ID         string      `json:"id"`
	Kind       ShardKind   `json:"kind"`
	Status     ShardStatus `json:"status"`
	Weight     int         `json:"weight"`
	PinSubject string      `json:"pin_subject,omitempty"`
	Region     string      `json:"region,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RouteVersion 是一份不可变的路由快照。发布窗内旧意图仍钉在自己的代次上。
type RouteVersion struct {
	Version    int64       `json:"version"`
	Status     RouteStatus `json:"status"`
	Modulus    int         `json:"modulus"`
	UserShards []string    `json:"user_shards"`
	Pins map[string]string `json:"pins"`
	Affinity map[string]string `json:"affinity,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	ActivatedAt time.Time `json:"activated_at,omitempty"`
	RetiredAt time.Time `json:"retired_at,omitempty"`
}

// Assignment 是一次解析结果。Pin 非空表示系统科目钉扎，不走用户哈希。

type Assignment struct {
	Version int64 `json:"version"`
	ShardID  string `json:"shard_id"`
	Subject  string `json:"subject"`
	Pin      string `json:"pin,omitempty"`
	Affinity string `json:"affinity,omitempty"`
	Slot   int  `json:"slot"`
	Hashed bool `json:"hashed"`
}

// Intent 是一笔跨分片转账。路由代次、源片、目标片在打开时钉死。
type Intent struct {
	ID           string `json:"id"`
	Idem         string `json:"idem"`
	BizHash      string `json:"biz_hash"`
	RouteVersion int64  `json:"route_version"`
	RouteFP string `json:"route_fp"`
	SrcUser string `json:"src_user"`
	SrcBook string `json:"src_book"`
	SrcSubject string `json:"src_subject"`
	SrcShard string `json:"src_shard"`
	DstUser string `json:"dst_user"`
	DstBook string `json:"dst_book"`
	DstSubject string `json:"dst_subject"`
	DstShard   string `json:"dst_shard"`
	Asset      string `json:"asset"`
	Amount string `json:"amount"`
	State IntentState `json:"state"`
	Halted       bool      `json:"halted"`
	Manual       bool      `json:"manual"`
	Deadline     time.Time `json:"deadline,omitempty"`
	FreezePosted bool      `json:"freeze_posted"`
	VoucherID string `json:"voucher_id,omitempty"`
	CreditPosted bool `json:"credit_posted"`
	CreditCount int `json:"credit_count"`
	ReverseCount int    `json:"reverse_count"`
	Reason       string `json:"reason,omitempty"`
	DeskID    string    `json:"desk_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Step 记录协议上的一步，以及只读调用 ledger.Apply 得到的镜像相位。
type Step struct {
	ID       string   `json:"id"`
	IntentID string   `json:"intent_id"`
	Seq      int      `json:"seq"`
	Kind     StepKind `json:"kind"`
	State    string   `json:"state"`
	ShardID    string `json:"shard_id,omitempty"`
	LedgerFrom string `json:"ledger_from,omitempty"`
	LedgerTo   string `json:"ledger_to,omitempty"`
	Detail string `json:"detail,omitempty"`
	At time.Time `json:"at"`
}

// BridgeVoucher 是源片冻结额与目标片贷记之间的桥接凭证。同一凭证只消费一次。

type BridgeVoucher struct {
	ID string `json:"id"`
	IntentID string `json:"intent_id"`
	Idem     string `json:"idem"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	SrcShard string `json:"src_shard"`
	DstShard string `json:"dst_shard"`
	BridgeShard  string       `json:"bridge_shard"`
	State        VoucherState `json:"state"`
	RouteVersion int64        `json:"route_version"`
	CreditCount int `json:"credit_count"`
	CreatedAt  time.Time `json:"created_at"`
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
	VoidedAt   time.Time `json:"voided_at,omitempty"`
}

// Balance 是分片进程内投影，不属于账本 Apply 的科目行。

type Balance struct {
	ShardID string `json:"shard_id"`
	Subject string `json:"subject"`
	Asset string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

// Posting 是分片投影上的一条分录。借方与贷方按资产轧平。

type Posting struct {
	ID        string `json:"id"`
	IntentID  string `json:"intent_id"`
	VoucherID string `json:"voucher_id,omitempty"`
	Seq       int    `json:"seq"`
	Kind StepKind `json:"kind"`
	ShardID    string `json:"shard_id"`
	Subject    string `json:"subject"`
	Asset      string `json:"asset"`
	DebitAvail string `json:"debit_avail"`
	CreditAvail  string    `json:"credit_avail"`
	DebitFrozen  string    `json:"debit_frozen"`
	CreditFrozen string    `json:"credit_frozen"`
	Memo         string    `json:"memo,omitempty"`
	At           time.Time `json:"at"`
}

// IdemRec 把业务指纹钉在第一次打开的意图上。重放不得换片。

type IdemRec struct {
	Key string `json:"key"`
	BizHash string `json:"biz_hash"`
	Intent string `json:"intent"`
}

// Alert 是路由漂移或收口异常。漂移只告警并进台，不改写已钉分片。

type Alert struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	IntentID string `json:"intent_id,omitempty"`
	Detail string `json:"detail"`
	Acked bool      `json:"acked"`
	At    time.Time `json:"at"`
}

// DeskCase 是冻死或漂移后的双人补偿单。申请人与批准人必须不同。

type DeskCase struct {
	ID       string `json:"id"`
	IntentID string `json:"intent_id"`
	State    DeskState `json:"state"`
	OpenedBy string    `json:"opened_by"`
	ApprovedBy string `json:"approved_by,omitempty"`
	ExecutedBy string    `json:"executed_by,omitempty"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Hint 是给后续因果图摄取的边提示。本包不保存图谱。
type Hint struct {
	ID string `json:"id"`
	Kind      string `json:"kind"`
	IntentID  string `json:"intent_id"`
	VoucherID string `json:"voucher_id,omitempty"`
	SrcShard string `json:"src_shard"`
	DstShard string `json:"dst_shard"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	At time.Time `json:"at"`
}

// OpenReq 打开一笔跨分片意图。RouteVersion 为 0 时用当前生效代次。
type OpenReq struct {
	Idem    string `json:"idempotency_key"`
	SrcUser string `json:"src_user"`
	SrcBook string `json:"src_book"`
	DstUser string `json:"dst_user"`
	DstBook string `json:"dst_book"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RouteVersion int64 `json:"route_version"`
	ExpectSrc string `json:"src_shard,omitempty"`
	ExpectDst string `json:"dst_shard,omitempty"`
}

// HopPlan 告诉调用方这笔 hop 是否可以留在单分片。
type HopPlan struct {
	SameShard bool `json:"same_shard"`
	SrcShard string `json:"src_shard"`
	DstShard string `json:"dst_shard"`
	Version int64 `json:"route_version"`
	IntentID string `json:"intent_id,omitempty"`
	State string `json:"state,omitempty"`
}

// ConserveReport 核对源出与目标入是否相等，桥和冻结是否回到零。

type ConserveReport struct {
	OK     bool   `json:"ok"`
	SrcOut string `json:"src_out"`
	DstIn string `json:"dst_in"`
	BridgeNet string `json:"bridge_net"`
	FrozenLeft string `json:"frozen_left"`
	VoucherID string `json:"voucher_id,omitempty"`
	Voucher   string `json:"voucher_state,omitempty"`
	CreditN   int    `json:"credit_count"`
}

// Dashboard 是跨分片在途与路由窗的运营面。
type Dashboard struct {
	ActiveVersion     int64          `json:"active_version"`
	PublishingVersion int64          `json:"publishing_version,omitempty"`
	WindowOpen        bool           `json:"window_open"`
	ByState           map[string]int `json:"by_state"`
	InFlight int `json:"in_flight"`
	OldestSec int64         `json:"oldest_sec"`
	Shards    []ShardHealth `json:"shards"`
	Alerts  []Alert  `json:"alerts"`
	Intents []Intent `json:"intents,omitempty"`
}

// ShardHealth 带上该片当前在途笔数。
type ShardHealth struct {
	Shard
	InFlight int `json:"in_flight"`
}

// TickReport 是一轮 worker 的计数。

type TickReport struct {
	Scanned  int `json:"scanned"`
	Advanced int `json:"advanced"`
	Compensated int `json:"compensated"`
	Aborted     int `json:"aborted"`
	Committed int `json:"committed"`
	Desk      int `json:"desk"`
	Drift     int `json:"drift"`
	Errors int `json:"errors"`
}

   func (s IntentState) Terminal() bool {
	   switch s {
	   case StateCommitted,
		   StateCompensated, StateAborted:
		  return true
	  default:
		   return false
	  }
   }

   func (s IntentState) InFlight() bool {
	  switch s {
	  case StateOpen, StatePreparing, StateFrozen, StateVouchered, StateCredited, StateCompensating, StateDesk:
		   return true
	  default:
		   return false
	  }
  }

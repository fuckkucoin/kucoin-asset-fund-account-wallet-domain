package wdrollback

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	KindFreeze = "freeze_release"
	KindFee    = "fee_release"
	KindOccupancy = "occupancy_release"
	KindShard = "shard_remainder"
	KindDeny = "deny_audit"
	KindSeal = "seal_ledger"
	ModeAuto  = "auto"
	ModeHuman = "human"
)

// Position 用户在单一资产上的回滚账快照。
// 功能：给补偿腿提供释放前后的余额。
// 按用户加资产聚合，不按链上地址拆。
// 不变量：Equity0 在建账时钉死，之后只允许桶间搬家。
type Position struct {
	UserID string
	Asset  string
	Available money.Amount
	Frozen money.Amount
	FeeReserved money.Amount
	Occupancy   money.Amount
	Available0 money.Amount
	Frozen0    money.Amount
	Fee0       money.Amount
	Equity0    money.Amount
}

   func (p Position) copy() Position { return p }

// Equity 当前权益。占用不计入。
   func (p Position) Equity() (money.Amount, error) {
	   sum,
		   err := p.Available.Add(p.Frozen)
	  if err != nil {
		   return money.Zero(), err
	  }
	  return sum.Add(p.FeeReserved)
  }

// Entry 一条回滚分录。
// 功能：留下释放或拒绝的幂等记录。
// Applied 为假的审计行不参与权益轧差。
// 不变量：同一幂等键只能对应一份指纹。
type Entry struct {
	ID string
	PlanID    string
	LegID     string
	FailureID string
	TransferID string
	UserID string
	Asset  string
	Kind string
	Amount string
	AvailableBefore string
	AvailableAfter  string
	FrozenBefore string
	FrozenAfter  string
	FeeBefore    string
	FeeAfter       string
	IdempotencyKey string
	Fingerprint string
	ReasonCode  string
	Path      string
	Applied   bool
	CreatedAt time.Time
}

   func (e Entry) copy() Entry { return e }

// Audit 自动解冻被拒的审计行。

type Audit struct {
	ID        string
	PlanID    string
	FailureID string
	TransferID string
	UserID     string
	Asset string
	Path   string
	TxHash string
	ReasonCode string
	Detail string
	At time.Time
}

// Request 一条补偿腿提交给回滚账的申请。

type Request struct {
	ID string
	PlanID string
	LegID     string
	FailureID string
	TransferID string
	UserID string
	Asset  string
	Kind string
	Mode     string
	Amount   money.Amount
	LiveHash bool
	ChainBlocked bool
	IdempotencyKey string
	ReasonCode     string
	Path           string
	TxHash         string
	Now time.Time
}

  func (r Request) fingerprint() string {
	  return strings.Join([]string{
		   r.Kind, r.UserID, r.Asset, r.TransferID, r.PlanID, r.Amount.String(),
	   }, "|")
  }

   func (r Request) validate() error {
	   if strings.TrimSpace(r.IdempotencyKey) == "" {
		  return ErrAmount
	   }
	  if strings.TrimSpace(r.UserID) == "" || strings.TrimSpace(r.Asset) == "" {
		  return ErrAmount
	   }
	  switch r.Kind {
	  case KindFreeze, KindFee, KindOccupancy, KindShard, KindDeny, KindSeal:
	  default:
		  return ErrKind
	   }
	  if r.Amount.IsNegative() {
		  return ErrNegative
	   }
	   return nil
   }

   func isRelease(kind string) bool {
	   return kind == KindFreeze || kind == KindFee || kind == KindOccupancy
  }

// Result 一次申请的落账结果。Replayed 为真时余额没有再次变动。

type Result struct {
	Entry    Entry
	Position Position
	Replayed bool
}

package fundpath

import (
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/persist"
)

type StepName string

const (
	StepAlloc  StepName = "alloc"
	StepAttrib StepName = "attrib"
	StepCredit StepName = "credit"
	StepHop StepName = "hop"
	StepAllow  StepName = "allowlist"
	StepKYT    StepName = "kyt"
	StepTravel StepName = "travel"
	StepWD  StepName = "wd_preflight"
	StepHot StepName = "hot"
)

type Step struct {
	Name StepName `json:"name"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

type State string

const (
	StateOpen State = "open"
	StateCredited State = "credited"
	StateHopped   State = "hopped"
	StateCleared  State = "cleared"
	StateBlocked  State = "blocked"
)

type Path struct {
	ID     string `json:"id"`
	Idem   string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Asset  string `json:"asset"`
	Amount string `json:"amount,omitempty"`
	TxHash string `json:"tx_hash,omitempty"`
	Address  addrpool.Binding `json:"address"`
	Attrib   deposit.Binding  `json:"attrib"`
	CreditID string           `json:"credit_id,omitempty"`
	HopID    string `json:"hop_id,omitempty"`
	TravelID string `json:"travel_id,omitempty"`
	Dest     string `json:"dest,omitempty"`
	ToBook   string `json:"to_book,omitempty"`
	Steps    []Step `json:"steps"`
	State    State  `json:"state"`
	Host   string `json:"host"`
	Region string `json:"region,omitempty"`
	Fence persist.Fence `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (p Path) Copy() Path {
	   cp := p
	   if p.Steps != nil {
		   cp.Steps = append([]Step(nil), p.Steps...)
	  }
	  return cp
   }

type RunReq struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	TxHash string `json:"tx_hash"`
	Memo string `json:"memo"`
	Tag  string `json:"tag"`
	UTXO string `json:"utxo"`
	ToBook string `json:"to_book"`
	Dest           string `json:"dest"`
	VASP           string `json:"vasp"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	WaitConfirm bool `json:"wait"`
	SkipCredit bool `json:"skip_credit"`
	SkipEgress bool `json:"skip_egress"`
}

type HopReq struct {
	UserID string `json:"user_id"`
	FromBook string `json:"from_book"`
	ToBook   string `json:"to_book"`
	Asset    string `json:"asset"`
	Amount string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type EgressReq struct {
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Dest  string `json:"dest"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	VASP   string `json:"vasp"`
	Actor  string `json:"actor"`
	MovementID string `json:"movement_id"`
}

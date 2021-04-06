package depoccup

import "time"

type Tier string

const (
	TierT0 Tier = "T0"
	TierT1 Tier = "T1"
	TierT2 Tier = "T2"
	TierT3 Tier = "T3"
	TierT4 Tier = "T4"
)

type LotState string

const (
	StateTransit LotState = "in_transit"
	StateAvail   LotState = "available"
	StateFailed  LotState = "failed"
	StateRolled LotState = "rolled_back"
)

type Source string

const (
	SourceSelf Source = "self"
	SourceVASP     Source = "vasp"
	SourceExchange Source = "exchange"
	SourceContract Source = "contract"
	SourceUnknown  Source = "unknown"
	SourceMixer    Source = "mixer"
)

type SignalKind string

const (
	SigMixer     SignalKind = "source_mixer"
	SigUnknown   SignalKind = "source_unknown"
	SigNewSource SignalKind = "source_changed"
	SigDustFloor SignalKind = "dust_below_floor"
	SigDustBurst SignalKind = "dust_repeat"
	SigKYTDeny   SignalKind = "kyt_deny"
	SigKYTReview SignalKind = "kyt_review"
	SigReorg     SignalKind = "reorg"
	SigFail      SignalKind = "fail"
	SigWindow    SignalKind = "window_breach"
	SigVelocity  SignalKind = "velocity"
	SigKYCDown   SignalKind = "kyc_down"
	SigKYCUp     SignalKind = "kyc_up"
	SigClean     SignalKind = "clean_settle"
	SigFullConfirm SignalKind = "full_confirm"
	SigContract    SignalKind = "source_contract"
)

type Policy struct {
	ID    string `json:"id"`
	KYC   string `json:"kyc"`
	Asset string `json:"asset"`
	Source    Source        `json:"source"`
	Chain     string        `json:"chain"`
	RatioBPS  int           `json:"ratio_bps"`
	MaxWindow time.Duration `json:"-"`
	MinWindow time.Duration `json:"-"`
	MaxSec    int           `json:"max_window_sec"`
	MinSec    int           `json:"min_window_sec"`
	TierFloor Tier `json:"tier_floor"`
	Version   int  `json:"version"`
	Priority int `json:"priority"`
	Enabled bool   `json:"enabled"`
	Note    string `json:"note,omitempty"`
}

type Lot struct {
	ID string `json:"id"`
	TransferID string `json:"transfer_id"`
	Idem       string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Asset string `json:"asset"`
	Source Source `json:"source"`
	KYC string `json:"kyc"`
	TxHash      string `json:"tx_hash"`
	FromAddress string `json:"from_address,omitempty"`
	Observed    string `json:"observed"`
	Occupied    string `json:"occupied"`
	Free        string `json:"free"`
	State    LotState `json:"state"`
	Tier     Tier     `json:"tier"`
	PolicyID string   `json:"policy_id"`
	RatioBPS int      `json:"ratio_bps"`
	MaxSec int `json:"max_window_sec"`
	MinSec int `json:"min_window_sec"`
	EarlyCredit bool      `json:"is_early_credit"`
	Confirms    int       `json:"confirms"`
	FullNeed    int       `json:"full_need"`
	ObservedAt  time.Time `json:"observed_at"`
	ReleaseAt   time.Time `json:"release_at,omitempty"`
	Deadline time.Time `json:"deadline"`
	Cause     string    `json:"cause,omitempty"`
	WindowHit bool      `json:"window_breach"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TierState struct {
	UserID string `json:"user_id"`
	Tier       Tier   `json:"tier"`
	Score      int    `json:"score"`
	CleanRun   int    `json:"clean_run"`
	KYC        string `json:"kyc"`
	LastSource Source `json:"last_source,omitempty"`
	Since    time.Time `json:"since"`
	LastUp   time.Time `json:"last_up,omitempty"`
	LastDown time.Time `json:"last_down,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Version int    `json:"version"`
}

type TierMove struct {
	UserID string `json:"user_id"`
	From Tier `json:"from"`
	To Tier `json:"to"`
	Score  int        `json:"score"`
	Kind   SignalKind `json:"kind,omitempty"`
	Reason string     `json:"reason"`
	At time.Time `json:"at"`
}

type Signal struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Kind   SignalKind `json:"kind"`
	Weight int        `json:"weight"`
	Ref    string     `json:"ref,omitempty"`
	Asset string `json:"asset,omitempty"`
	Chain  string    `json:"chain,omitempty"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

type Bucket struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	InTransit string `json:"in_transit"`
	Available string `json:"available"`
	Rolled string `json:"rolled"`
}

type Audit struct {
	ID string `json:"id"`
	LotID  string `json:"lot_id"`
	UserID string `json:"user_id"`
	Event string `json:"event"`
	FromState LotState  `json:"from_state,omitempty"`
	ToState   LotState  `json:"to_state"`
	Amount    string    `json:"amount"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
}

type Condition struct {
	Code string `json:"code"`
	Met  bool   `json:"met"`
	Detail string `json:"detail"`
}

type ObserveIn struct {
	TransferID string `json:"transfer_id"`
	Idem       string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount      string `json:"amount"`
	TxHash      string `json:"tx_hash"`
	SourceHint  string `json:"source"`
	FromAddress string `json:"from_address"`
	Confirms    int    `json:"confirms"`
	EarlyCredit bool   `json:"is_early_credit"`
	KYC         string `json:"kyc"`
	Actor       string `json:"actor"`
	DustRepeats int    `json:"dust_repeats"`
}

type ConfirmIn struct {
	TransferID  string `json:"transfer_id"`
	Confirms    int    `json:"confirms"`
	EarlyCredit bool   `json:"is_early_credit"`
	Actor string `json:"actor"`
}

type FailIn struct {
	TransferID string `json:"transfer_id"`
	Reason     string `json:"reason"`
	Actor string `json:"actor"`
}

type SignalIn struct {
	UserID string `json:"user_id"`
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	Asset  string `json:"asset"`
	Chain  string `json:"chain"`
	Detail string `json:"detail"`
	Actor  string `json:"actor"`
}

type PolicyIn struct {
	KYC    string `json:"kyc"`
	Asset  string `json:"asset"`
	Source string `json:"source"`
	Chain string `json:"chain"`
	RatioBPS int `json:"ratio_bps"`
	MaxSec int `json:"max_window_sec"`
	MinSec int `json:"min_window_sec"`
	TierFloor string `json:"tier_floor"`
	Note    string `json:"note"`
	Actor   string `json:"actor"`
	Enabled *bool  `json:"enabled"`
}

type HoldLine struct {
	Asset     string `json:"asset"`
	InTransit string `json:"in_transit"`
	Available string `json:"available"`
	Rolled    string `json:"rolled"`
}

type UserView struct {
	UserID string `json:"user_id"`
	Tier  Tier   `json:"tier"`
	Score int    `json:"score"`
	KYC   string `json:"kyc"`
	CleanRun int `json:"clean_run"`
	Holds      []HoldLine  `json:"holds"`
	Lots       []Lot       `json:"lots"`
	Conditions []Condition `json:"release_conditions"`
}

type CSView struct {
	User    UserView   `json:"user"`
	Signals []Signal   `json:"signals"`
	Moves   []TierMove `json:"tier_moves"`
	PolicyHits []MatchHit `json:"open_policies"`
}

type MatchHit struct {
	Policy      Policy `json:"policy"`
	Specificity int    `json:"specificity"`
	AppliedBPS  int    `json:"applied_bps"`
	Reason string `json:"reason"`
}

type Health struct {
	Lots int `json:"lots"`
	InTransit int  `json:"in_transit"`
	Policies  int  `json:"policies"`
	SQLBound  bool `json:"sql_bound"`
	RollbackBound bool   `json:"rollback_bound"`
	KYCBound      bool   `json:"kyc_bound"`
	DustBound     bool   `json:"dust_bound"`
	NoteErr       int    `json:"note_err"`
	SaveErr       string `json:"save_err,omitempty"`
	LoadErr string `json:"load_err,omitempty"`
}

   func (p Policy) copy() Policy { return p }

   func (l Lot) copy() Lot { return l }

  func (t TierState) copy() TierState { return t }

  func (b Bucket) copy() Bucket { return b }

  func (s Signal) copy() Signal { return s }

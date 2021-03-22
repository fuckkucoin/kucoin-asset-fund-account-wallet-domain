package closelatecredit

import (
	"fmt"
	"strings"
	"time"
)

// EpochState 关户世代相位。
// open ── CloseBegin ──▶ closing ── CloseCommit ──▶ closed
// closed ── Reopen ──▶ reopened ── ConfirmOpen ──▶ open
// reopened 亦可再次 CloseBegin。

type EpochState string

const (
	EpochOpen    EpochState = "open"
	EpochClosing EpochState = "closing"
	EpochClosed  EpochState = "closed"
	EpochReopened EpochState = "reopened"
)

// Terminal 关户终态（仍可 Reopen）。

   func (s EpochState) Terminal() bool {
	  return s == EpochClosed
  }

// AllowsLiveCredit 仅活户世代允许入活账。

   func (s EpochState) AllowsLiveCredit() bool {
	   return s == EpochOpen || s == EpochReopened
   }

// ClosingOrClosed 关户进行中或已关。
   func (s EpochState) ClosingOrClosed() bool {
	  return s == EpochClosing || s == EpochClosed
  }

// Valid 合法相位值。

  func (s EpochState) Valid() bool {
	  switch s {
	   case EpochOpen, EpochClosing, EpochClosed, EpochReopened:
		  return true
	   }
	   return false
   }

// ValidEpochTransitions 合法转换。
var ValidEpochTransitions = map[EpochState][]EpochState{
	EpochOpen: {EpochClosing},
	EpochClosing: {EpochClosed,
		EpochOpen},
	EpochClosed:   {EpochReopened},
	EpochReopened: {EpochOpen, EpochClosing},
}

// CanTransitEpoch 校验世代转换。
   func CanTransitEpoch(from, to EpochState) bool {
	   for _, t := range ValidEpochTransitions[from] {
		   if t == to {
			   return true
		  }
	  }
	  return false
  }

// LateState 迟贷记证明相位。
// open ── Reject ──▶ rejected
// open ── Dispute ──▶ disputed ── Close ──▶ closed
// rejected ── Close ──▶ closed

type LateState string

const (
	LateOpen LateState = "open"
	LateRejected LateState = "rejected"
	LateDisputed LateState = "disputed"
	LateClosed   LateState = "closed"
)

   func (s LateState) Terminal() bool {
	  return s == LateClosed
   }

   func (s LateState) Valid() bool {
	  switch s {
	   case LateOpen, LateRejected, LateDisputed, LateClosed:
		  return true
	  }
	  return false
  }

// ValidLateTransitions 迟贷证明合法转换。

var ValidLateTransitions = map[LateState][]LateState{
	LateOpen:     {LateRejected, LateDisputed, LateClosed},
	LateRejected: {LateClosed},
	LateDisputed: {LateClosed, LateRejected},
	LateClosed:   {},
}

// CanTransitLate 校验迟贷证明转换。

   func CanTransitLate(from, to LateState) bool {
	  for _, t := range ValidLateTransitions[from] {
		  if t == to {
			   return true
		  }
	   }
	   return false
   }

// HoldState 争议挂起仓相位。

type HoldState string

const (
	HoldOpen   HoldState = "open"
	HoldParked HoldState = "parked"
	HoldResolved HoldState = "resolved"
	HoldRejected HoldState = "rejected"
	HoldExported HoldState = "exported"
)

  func (s HoldState) Terminal() bool {
	   switch s {
	   case HoldResolved,
		  HoldRejected, HoldExported:
		  return true
	  }
	   return false
  }

   func (s HoldState) Valid() bool {
	  switch s {
	   case HoldOpen, HoldParked, HoldResolved, HoldRejected, HoldExported:
		  return true
	   }
	   return false
   }

// ValidHoldTransitions 争议仓合法转换。

var ValidHoldTransitions = map[HoldState][]HoldState{
	HoldOpen:     {HoldParked, HoldRejected},
	HoldParked:   {HoldResolved, HoldRejected, HoldExported},
	HoldResolved: {HoldExported},
	HoldRejected: {HoldExported},
	HoldExported: {},
}

// CanTransitHold 校验争议仓转换。

   func CanTransitHold(from, to HoldState) bool {
	  for _, t := range ValidHoldTransitions[from] {
		  if t == to {
			   return true
		   }
	   }
	  return false
   }

// CreditSource 迟达贷记来源。观察 / 出箱 / 法币回调同等受 epoch 约束。

type CreditSource string

const (
	SourceObserve CreditSource = "observe"
	SourceOutbox CreditSource = "outbox"
	SourceFiatCallback CreditSource = "fiat_callback"
)

   func (s CreditSource) Valid() bool {
	   switch s {
	   case SourceObserve, SourceOutbox, SourceFiatCallback:
		   return true
	  }
	  return false
  }

   func (s CreditSource) Label() string {
	  switch s {
	  case SourceObserve:
		   return "链观察贷记"
	   case SourceOutbox:
		   return "出箱迟达贷记"
	   case SourceFiatCallback:
		  return "法币回调贷记"
	   }
	   return string(s)
  }

// ParseCreditSource 解析来源；非法则空。
  func ParseCreditSource(s string) CreditSource {
	  v := CreditSource(strings.ToLower(strings.TrimSpace(s)))
	  if v.Valid() {
		  return v
	   }
	  return ""
   }

// LateReason 证明原因码。
type LateReason string

const (
	ReasonMissingEpoch  LateReason = "missing_epoch"
	ReasonEpochMismatch LateReason = "epoch_mismatch"
	ReasonClosedRefuse  LateReason = "closed_refuse"
	ReasonClosedDispute LateReason = "closed_dispute"
	ReasonStaleAfterOpen LateReason = "stale_after_reopen"
	ReasonWrongUser   LateReason = "wrong_user"
	ReasonForceDenied LateReason = "force_live_denied"
	ReasonIdemConflict LateReason = "idem_conflict"
	ReasonNoLiveCredit LateReason = "no_live_credit"
)

  func (r LateReason) Valid() bool {
	  switch r {
	   case ReasonMissingEpoch, ReasonEpochMismatch, ReasonClosedRefuse,
		  ReasonClosedDispute, ReasonStaleAfterOpen, ReasonWrongUser,
		  ReasonForceDenied,
		  ReasonIdemConflict, ReasonNoLiveCredit:
		  return true
	  }
	   return false
  }

// CreditDecision 闸门裁决。admit 仅表示允许入活账意图，本包仍不写 ledger。

type CreditDecision string

const (
	DecisionAdmit CreditDecision = "admit"
	DecisionReject  CreditDecision = "reject"
	DecisionDispute CreditDecision = "dispute"
)

  func (d CreditDecision) Allowed() bool { return d == DecisionAdmit }

  func (d CreditDecision) Parked() bool { return d == DecisionDispute }

// CloseEpoch 用户关户世代。N 单调递增；关户与重开均升世代。

type CloseEpoch struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	N      uint64 `json:"close_epoch"`
	State       EpochState `json:"state"`
	ClosedEpoch uint64     `json:"closed_epoch"`
	LiveEpoch   uint64     `json:"live_epoch"`
	CloseReason  string `json:"close_reason,omitempty"`
	ReopenReason string `json:"reopen_reason,omitempty"`
	IdemKey      string `json:"idem_key"`
	Actor     string    `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Clone 深拷贝。

  func (e *CloseEpoch) Clone() *CloseEpoch {
	  if e == nil {
		  return nil
	   }
	  c := *e
	  return &c
  }

// CurrentN 当前世代。
   func (e *CloseEpoch) CurrentN() uint64 {
	   if e == nil {
		  return 0
	  }
	  return e.N
  }

// Matches 调用方声明的 epoch 是否等于当前世代。

  func (e *CloseEpoch) Matches(claimed *uint64) bool {
	   if e == nil || claimed == nil {
		   return false
	  }
	  return e.N == *claimed
   }

// StaleAgainst 旧世代相对当前是否落后。
   func (e *CloseEpoch) StaleAgainst(claimed uint64) bool {
	   if e == nil {
		   return true
	  }
	   return claimed < e.N
  }

// IsLive 活户可贷。

  func (e *CloseEpoch) IsLive() bool {
	   return e != nil && e.State.AllowsLiveCredit()
   }

// IsClosed 已关户。
   func (e *CloseEpoch) IsClosed() bool {
	   return e != nil && e.State == EpochClosed
  }

// CloseLateCreditProof 关户后迟贷 / 错户证明。

type CloseLateCreditProof struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	ClaimedUser string `json:"claimed_user"`
	Book        string `json:"book"`
	Source CreditSource `json:"source"`
	Asset  string       `json:"asset"`
	Amount string       `json:"amount"`
	TxRef        string `json:"tx_ref"`
	ClaimedEpoch uint64 `json:"claimed_epoch"`
	ActualEpoch uint64 `json:"actual_epoch"`
	Reason LateReason `json:"reason"`
	State LateState `json:"state"`
	Decision CreditDecision `json:"decision"`
	IdemKey  string         `json:"idem_key"`
	Digest   string         `json:"digest"`
	HoldID string `json:"hold_id,omitempty"`
	WrongUser bool   `json:"wrong_user"`
	ForceLive bool   `json:"force_live"`
	Actor     string `json:"actor,omitempty"`
	Note string `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// Clone 深拷贝。

   func (p *CloseLateCreditProof) Clone() *CloseLateCreditProof {
	   if p == nil {
		  return nil
	   }
	  c := *p
	  if p.ClosedAt != nil {
		  t := *p.ClosedAt
		   c.ClosedAt = &t
	  }
	   return &c
   }

// Misroute 错户：声明用户与账本用户不一致，或世代落后于重开。

  func (p *CloseLateCreditProof) Misroute() bool {
	   if p == nil {
		   return false
	  }
	  if p.WrongUser {
		   return true
	  }
	  if p.Reason == ReasonWrongUser || p.Reason == ReasonStaleAfterOpen {
		   return true
	   }
	   return p.ClaimedUser != "" && p.UserID != "" && p.ClaimedUser != p.UserID
   }

// DisputeHoldCase 争议挂起仓：迟贷资金不得入已关户或新活户。

type DisputeHoldCase struct {
	ID string `json:"id"`
	ProofID string `json:"proof_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Source CreditSource `json:"source"`
	EpochAtPark uint64 `json:"epoch_at_park"`
	State HoldState `json:"state"`
	IdemKey     string `json:"idem_key"`
	ResolveNote string `json:"resolve_note,omitempty"`
	Actor string `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (h *DisputeHoldCase) Clone() *DisputeHoldCase {
	   if h == nil {
		  return nil
	   }
	  c := *h
	  return &c
   }

// EpochAudit 世代审计。

type EpochAudit struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Book string `json:"book"`
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	FromN     uint64 `json:"from_epoch"`
	ToN uint64 `json:"to_epoch"`
	Actor     string    `json:"actor"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

   func (a *EpochAudit) Clone() *EpochAudit {
	   if a == nil {
		  return nil
	  }
	   c := *a
	   return &c
   }

// PendingCredit 迟贷扫描入队项（不入活账）。
type PendingCredit struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Source CreditSource `json:"source"`
	Asset  string       `json:"asset"`
	Amount string       `json:"amount"`
	TxRef        string  `json:"tx_ref"`
	ClaimedEpoch *uint64 `json:"claimed_epoch"`
	SeenAt  time.Time `json:"seen_at"`
	Scanned bool      `json:"scanned"`
}

  func (p *PendingCredit) Clone() *PendingCredit {
	  if p == nil {
		   return nil
	   }
	  c := *p
	   if p.ClaimedEpoch != nil {
		  n := *p.ClaimedEpoch
		  c.ClaimedEpoch = &n
	   }
	   return &c
   }

// CreditAssertRequest 贷记断言。ClaimedEpoch 缺省则 fail-closed。

type CreditAssertRequest struct {
	UserID string
	ClaimedUser string
	Book   string
	Source CreditSource
	Asset  string
	Amount string
	TxRef string
	ClaimedEpoch *uint64
	IdemKey      string
	Actor        string
	ForceLive bool
	Note      string
}

// Normalize 清洗字段。

   func (r *CreditAssertRequest) Normalize() {
	   if r == nil {
		   return
	   }
	  r.UserID = strings.TrimSpace(r.UserID)
	  r.ClaimedUser = strings.TrimSpace(r.ClaimedUser)
	  if r.ClaimedUser == "" {
		  r.ClaimedUser = r.UserID
	   }
	   r.Book = strings.TrimSpace(r.Book)
	  r.Asset = strings.ToUpper(strings.TrimSpace(r.Asset))
	   r.Amount = strings.TrimSpace(r.Amount)
	  r.TxRef = strings.TrimSpace(r.TxRef)
	  r.IdemKey = strings.TrimSpace(r.IdemKey)
	  r.Actor = strings.TrimSpace(r.Actor)
	  r.Note = strings.TrimSpace(r.Note)
   }

// CloseRequest 关户升世代。

type CloseRequest struct {
	UserID  string
	Book    string
	Reason  string
	Actor   string
	IdemKey string
}

   func (r *CloseRequest) Normalize() {
	   if r == nil {
		   return
	  }
	   r.UserID = strings.TrimSpace(r.UserID)
	   r.Book = strings.TrimSpace(r.Book)
	  r.Reason = strings.TrimSpace(r.Reason)
	   r.Actor = strings.TrimSpace(r.Actor)
	  r.IdemKey = strings.TrimSpace(r.IdemKey)
   }

// ReopenRequest 重开户升世代。

type ReopenRequest struct {
	UserID string
	Book string
	Reason string
	Actor   string
	IdemKey string
}

  func (r *ReopenRequest) Normalize() {
	  if r == nil {
		  return
	   }
	  r.UserID = strings.TrimSpace(r.UserID)
	  r.Book = strings.TrimSpace(r.Book)
	   r.Reason = strings.TrimSpace(r.Reason)
	   r.Actor = strings.TrimSpace(r.Actor)
	   r.IdemKey = strings.TrimSpace(r.IdemKey)
  }

// DisputeResolveRequest 争议人工处置。不得把旧 epoch 资金贷入新活户。

type DisputeResolveRequest struct {
	HoldID  string
	ProofID string
	Actor   string
	Note    string
	Reject bool
	ExportOnly bool
}

  func (r *DisputeResolveRequest) Normalize() {
	  if r == nil {
		  return
	   }
	   r.HoldID = strings.TrimSpace(r.HoldID)
	   r.ProofID = strings.TrimSpace(r.ProofID)
	  r.Actor = strings.TrimSpace(r.Actor)
	   r.Note = strings.TrimSpace(r.Note)
   }

// GateResult 贷记闸结果。
type GateResult struct {
	Allowed  bool                  `json:"allowed"`
	Decision CreditDecision        `json:"decision"`
	Epoch    *CloseEpoch           `json:"epoch,omitempty"`
	Proof    *CloseLateCreditProof `json:"proof,omitempty"`
	Hold     *DisputeHoldCase      `json:"hold,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
}

  func (g *GateResult) FailClosed(reason string) *GateResult {
	  if g == nil {
		  g = &GateResult{}
	   }
	  g.Allowed = false
	  if g.Decision == "" {
		  g.Decision = DecisionReject
	  }
	  g.RejectReason = reason
	  return g
  }

// Epoch 用户世代游标（只读对外）。

type EpochCursor struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	NextN     uint64    `json:"next_epoch"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (c *EpochCursor) Clone() *EpochCursor {
	   if c == nil {
		   return nil
	   }
	  x := *c
	   return &x
   }

// BookDefault 缺省科目。funding 不可关户，默认 trade。
  func BookDefault(book string) string {
	  book = strings.TrimSpace(book)
	  if book == "" {
		   return "trading"
	   }
	  return book
   }

// AmountOK 金额须为非空十进制正数字符串，禁止浮点路径。
   func AmountOK(amount string) bool {
	  s := strings.TrimSpace(amount)
	   if s == "" || s == "0" || s == "0.0" || s == "0.00" {
		   return false
	   }
	  dot := 0
	   for i, c := range s {
		  if c == '.' {
			  dot++
			   if dot > 1 || i == 0 || i == len(s)-1 {
				  return false
			  }
			  continue
		  }
		   if c < '0' || c > '9' {
			   return false
		  }
	   }
	  return true
  }

// AssetOK 资产代码。
  func AssetOK(asset string) bool {
	  s := strings.ToUpper(strings.TrimSpace(asset))
	  if s == "" || len(s) > 32 {
		   return false
	  }
	  for _, c := range s {
		   if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			   return false
		  }
	  }
	   return true
  }

// DescribeEpoch 书面描述。
  func DescribeEpoch(e *CloseEpoch) string {
	   if e == nil {
		   return "close_epoch=<missing>"
	   }
	  return fmt.Sprintf("user=%s book=%s close_epoch=%d state=%s live=%d closed=%d",
		  e.UserID,
		   e.Book, e.N, e.State, e.LiveEpoch, e.ClosedEpoch)
   }

// DescribeProof 书面描述。
   func DescribeProof(p *CloseLateCreditProof) string {
	   if p == nil {
		  return "late_proof=<missing>"
	   }
	  return fmt.Sprintf("proof=%s user=%s source=%s claimed=%d actual=%d reason=%s decision=%s",
		   p.ID, p.UserID, p.Source, p.ClaimedEpoch, p.ActualEpoch, p.Reason, p.Decision)
   }

// IsolationNote 与邻接错户面隔离说明。

   func IsolationNote() string {
	   return "close_epoch 仅覆盖关户迟贷；不覆盖 addrpool recycle_epoch、Memo 找回工单、账户生命周期目录改写。"
   }

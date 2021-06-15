// 重点：同一幂等键绑定指纹与终态，禁止二次扣减。
// 风险点：键冲突不得覆盖已落库终态。

// Author: Hux
// Date: 2021.06.14

package fundmesh

import "time"

// OutboundRequest 出站提交意图。

type OutboundRequest struct {
	UserID string
	Actor  string
	Asset  string
	Amount string
	Chain  string
	Dest   string
	Memo   string
	Tag string
	IdempotencyKey string
	ClientKey      string
	WithdrawID string
	Entry      EntryKind
	Method string
	Path string
	HeaderKey string
}

// OutboundResult 出站回包。重复提交回同一终态。

type OutboundResult struct {
	WithdrawID     string        `json:"withdraw_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	Fingerprint    string        `json:"fingerprint"`
	State          OutboundState `json:"state"`
	Phase          WithdrawPhase `json:"phase"`
	Code string `json:"code"`
	Idempotent bool `json:"idempotent"`
	Debited    bool `json:"debited"`
	DebitCount int  `json:"debit_count"`
	Entry  EntryKind `json:"entry"`
	Reason string    `json:"reason"`
	At time.Time `json:"at"`
}

// OutboundRecord 出站幂等仓。

type OutboundRecord struct {
	RecordID string
	WithdrawID string
	UserID string
	Actor  string
	Asset  string
	Amount         string
	Chain          string
	Dest           string
	Memo           string
	Tag            string
	IdempotencyKey string
	Scheme string
	Kind   string
	Fingerprint string
	Canon string
	State    OutboundState
	Phase    WithdrawPhase
	Code     string
	RemoteOK bool
	DebitCount int
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OutboundIntent 远端扣减入参。

type OutboundIntent struct {
	WithdrawID string
	UserID string
	Asset  string
	Amount string
	Chain  string
	Dest   string
	Memo   string
	IdempotencyKey string
	Fingerprint string
}

// IdemKeyParts 统一幂等键解析结果。
type IdemKeyParts struct {
	Raw    string
	Scheme string
	Kind    string
	Body    string
	Derived bool
	Client bool
	Valid bool
}

// DebitLine 扣减流水。同一键只允许一行成功。
type DebitLine struct {
	LineID     string
	WithdrawID string
	IdempotencyKey string
	Fingerprint string
	Amount string
	Asset  string
	UserID string
	OK     bool
	Remote bool
	Note string
	At   time.Time
}

// OutboundProof 重复提交 / 冲突 / 二次扣减证明。
type OutboundProof struct {
	ProofID string `json:"proof_id"`
	Kind       string `json:"kind"`
	WithdrawID string `json:"withdraw_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Fingerprint  string `json:"fingerprint"`
	PriorPhase   string `json:"prior_phase"`
	PriorCode    string `json:"prior_code"`
	DebitAttempt int    `json:"debit_attempt"`
	Blocked      bool   `json:"blocked"`
	Detail       string `json:"detail"`
	At time.Time `json:"at"`
}

const (
	ProofReplay   = "outbound_replay"
	ProofConflict = "outbound_conflict"
	ProofDupDebit   = "outbound_dup_debit"
	ProofClosed     = "outbound_closed_replay"
	ProofKeyInvalid = "outbound_key_invalid"
	ProofPayload    = "outbound_payload"
	ProofRemoteFail = "outbound_remote_fail"
)

   func (r *OutboundRecord) Clone() *OutboundRecord {
	   if r == nil {
		  return nil
	  }
	   cp := *r
	   return &cp
   }

   func (r OutboundResult) SameTerminal(other OutboundResult) bool {
	  if r.WithdrawID != other.WithdrawID {
		   return false
	  }
	  if r.Phase != other.Phase {
		   return false
	   }
	   if r.Code != other.Code && !replayCodesEqual(r.Code, other.Code) {
		   return false
	  }
	   return r.IdempotencyKey == other.IdempotencyKey
   }

   func replayCodesEqual(a, b string) bool {
	   if a == b {
		  return true
	   }
	  pair := (a == CodeOutboundOK && b == CodeOutboundReplay) || (a == CodeOutboundReplay && b == CodeOutboundOK)
	   return pair
  }

  func (r OutboundRequest) Intent() OutboundIntent {
	  return OutboundIntent{
		   WithdrawID: r.WithdrawID,
		  UserID: r.UserID,
		   Asset:  r.Asset,
		   Amount: r.Amount,
		   Chain: r.Chain,
		  Dest:  r.Dest,
		  Memo:  r.Memo,
		  IdempotencyKey: r.IdempotencyKey,
	  }
  }

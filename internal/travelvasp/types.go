package travelvasp

import "time"

type Dir string

const (
	DirOut Dir = "outbound"
	DirIn  Dir = "inbound"
)

type OutState string

const (
	OutDraft  OutState = "draft"
	OutQueued OutState = "queued"
	OutSent   OutState = "sent"
	OutAcked  OutState = "acked"
	OutFailed OutState = "failed"
	OutExpired OutState = "expired"
)

type InState string

const (
	InReceived InState = "received"
	InScreened InState = "screened"
	InAccepted InState = "accepted"
	InRefused InState = "refused"
	InExpired InState = "expired"
)

// CaseStatus 资金账户出站旅行规则案件机：required → submitted → acknowledged / rejected / manual-review。
type CaseStatus string

const (
	CaseRequired CaseStatus = "required"
	CaseSubmitted    CaseStatus = "submitted"
	CaseAcknowledged CaseStatus = "acknowledged"
	CaseRejected     CaseStatus = "rejected"
	CaseManualReview CaseStatus = "manual-review"
)

const (
	ProtoLOCAL = "LOCAL"
	ProtoTRUST = "TRUST"
	ProtoSYGNA      = "SYGNA"
	ProtoNOTABENE   = "NOTABENE"
	ProtoCODE       = "CODE"
	ProtoUNHOSTED   = "UNHOSTED"
	CatalogUnhosted = "UNHOSTED"
)

type VASP struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Country  string `json:"country"`
	Endpoint string `json:"endpoint,omitempty"`
	DID string `json:"did,omitempty"`
	LEI string `json:"lei,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Enabled bool `json:"enabled"`
	Unhosted  bool      `json:"unhosted,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Party struct {
	Name    string `json:"name"`
	Account string `json:"account"`
	VASP string `json:"vasp"`
}

type Envelope struct {
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Chain       string `json:"chain,omitempty"`
	Originator  Party  `json:"originator"`
	Beneficiary Party  `json:"beneficiary"`
	Canonical    string `json:"canonical"`
	Checksum     string `json:"checksum"`
	PrevChecksum string `json:"prev_checksum,omitempty"`
	Note         string `json:"note"`
}

type Case struct {
	ID         string `json:"id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	Status CaseStatus `json:"status"`
	MsgID  string     `json:"msg_id,omitempty"`
	Chain  string     `json:"chain"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Dest     string `json:"dest,omitempty"`
	PeerVASP string `json:"peer_vasp"`
	Corridor string `json:"corridor"`
	Originator Party `json:"originator"`
	Beneficiary Party    `json:"beneficiary"`
	Checksum    string   `json:"checksum"`
	Envelope    Envelope `json:"envelope"`
	Actor     string    `json:"actor,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             string   `json:"id"`
	IdempotencyKey string   `json:"idempotency_key"`
	Dir            Dir      `json:"dir"`
	VASP           string   `json:"vasp"`
	OutState       OutState `json:"out_state,omitempty"`
	InState  InState  `json:"in_state,omitempty"`
	Envelope Envelope `json:"envelope"`
	Actor    string   `json:"actor"`
	Note     string   `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (v VASP) Copy() VASP { return v }

  func (e Envelope) Copy() Envelope { return e }

  func (m Message) Copy() Message { return m }

   func (c Case) Copy() Case { return c }

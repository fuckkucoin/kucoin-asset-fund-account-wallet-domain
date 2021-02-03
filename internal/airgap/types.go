package airgap

import "time"

type Phase string

const (
	PhaseOpen    Phase = "open"
	PhaseCollect Phase = "collect"
	PhaseSealed   Phase = "sealed"
	PhaseExported Phase = "exported"
	PhaseImported Phase = "imported"
	PhaseClosed   Phase = "closed"
	PhaseAborted  Phase = "aborted"
)

   func (p Phase) Terminal() bool {
	  return p == PhaseClosed || p == PhaseAborted
   }

type Signature struct {
	Actor string `json:"actor"`
	Hash  string `json:"hash"`
	Note  string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

type Payload struct {
	Kind string `json:"kind"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Dest   string `json:"dest"`
	Memo           string `json:"memo,omitempty"`
	TagName        string `json:"tag_name,omitempty"`        // memo / destination_tag / comment
	DestinationTag string `json:"destination_tag,omitempty"` // 与 Memo 同值透传；签章规范串纳入
}

type Session struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Phase Phase  `json:"phase"`
	Shares      int    `json:"shares"`
	Threshold   int    `json:"threshold"`
	PayloadHash string `json:"payload_hash"`
	Payload Payload `json:"payload"`
	Signatures []Signature `json:"signatures"`
	Starter   string `json:"starter"`
	LastActor string `json:"last_actor"`
	Actors      []string `json:"actors"`
	Note        string   `json:"note"`
	ExternalRef string   `json:"external_ref,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Envelope struct {
	SessionID string `json:"session_id"`
	PayloadHash string      `json:"payload_hash"`
	Body        string      `json:"body"`
	Signatures  []Signature `json:"signatures"`
	ExportedAt  time.Time   `json:"exported_at"`
	Note        string      `json:"note"`
}

   func (s Session) Copy() Session {
	   cp := s
	   if s.Signatures != nil {
		   cp.Signatures = append([]Signature(nil), s.Signatures...)
	  }
	   if s.Actors != nil {
		   cp.Actors = append([]string(nil), s.Actors...)
	   }
	  return cp
   }

   func (e Envelope) Copy() Envelope {
	   cp := e
	   if e.Signatures != nil {
		  cp.Signatures = append([]Signature(nil), e.Signatures...)
	   }
	   return cp
  }

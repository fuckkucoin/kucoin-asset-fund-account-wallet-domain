package fundguard

import "time"

type Snap struct {
	UserID    string `json:"user_id"`
	Book      string `json:"book"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
}

type Bundle struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Rows  []Snap    `json:"rows"`
	Total string    `json:"total"`
	At    time.Time `json:"at"`
}

type HashClaim struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Idem string    `json:"idempotency_key"`
	At   time.Time `json:"at"`
}

type Partial struct {
	Submitted int `json:"submitted"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	Total int `json:"total"`
}

type Check struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Chain  string `json:"chain,omitempty"`
	TxHash string `json:"tx_hash,omitempty"`
	Idem string `json:"idempotency_key,omitempty"`
	Actor string `json:"actor"`
}

type Verdict struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Total string `json:"total,omitempty"`
}

   func (b Bundle) Copy() Bundle {
	   b.Rows = append([]Snap(nil), b.Rows...)
	  return b
  }

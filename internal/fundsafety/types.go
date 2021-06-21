package fundsafety

import "time"

type CreditClaim struct {
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Idem   string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Ledger string `json:"ledger_id,omitempty"`
	Epoch int64 `json:"epoch,omitempty"`
	Token string    `json:"token,omitempty"`
	At    time.Time `json:"at"`
}

type Partial struct {
	Submitted string `json:"submitted"`
	Skipped   string `json:"skipped"`
	Failed    string `json:"failed"`
	Requested string `json:"requested"`
}

type ReorgReplay struct {
	TransferID string `json:"transfer_id"`
	Unwound    bool   `json:"unwound"`
	Replayed   bool   `json:"replayed"`
	Second bool   `json:"second_credit"`
	Reason string `json:"reason,omitempty"`
}

type Verdict struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Total  string `json:"total,omitempty"`
}

type BatchLine struct {
	Kind string `json:"kind"`
	Amount string `json:"amount"`
	Note   string `json:"note,omitempty"`
}

type BatchRun struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Requested string `json:"requested"`
	Submitted string `json:"submitted"`
	Skipped   string `json:"skipped"`
	Failed string      `json:"failed"`
	Lines  []BatchLine `json:"lines,omitempty"`
	Epoch  int64       `json:"epoch"`
	Token  string    `json:"token"`
	Closed bool      `json:"closed,omitempty"`
	At     time.Time `json:"at"`
}

type ConfirmWin struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Need int `json:"need"`
	Have int `json:"have"`
	Reorged bool      `json:"reorged,omitempty"`
	At      time.Time `json:"at"`
}

type CreditOutbox struct {
	Key string `json:"key"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Epoch  int64  `json:"epoch"`
	Token string `json:"token"`
	SentAt *time.Time `json:"sent_at,omitempty"`
}

type JournalLine struct {
	Side   string `json:"side"`
	Book   string `json:"book"`
	Amount string `json:"amount"`
}

type JournalCheck struct {
	Left string `json:"left"`
	Right string `json:"right"`
	OK    bool   `json:"ok"`
}

package balproof

import "time"

type Proof struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
	Total string `json:"total"`
	Nonce string `json:"nonce"`
	At         time.Time `json:"at"`
	Canonical  string    `json:"canonical"`
	Signature  string    `json:"signature"`
	Disclaimer string    `json:"disclaimer"`
	Deprecated bool      `json:"deprecated"`
	Solvency   bool      `json:"solvency"`
	Class string `json:"class"`
}

type IssueReq struct {
	UserID string `json:"user_id"`
	Book  string `json:"book"`
	Asset string `json:"asset"`
	Nonce string `json:"nonce"`
}

type VerifyReq struct {
	UserID string `json:"user_id"`
	Book string `json:"book"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Nonce     string `json:"nonce"`
	At        string `json:"at"`
	Signature string `json:"signature"`
}

// Disclaimer 余额签名快照的免责声明。该快照已废弃，不得当作偿付证明。

const Disclaimer = "deprecated: signed book snapshot for query-gateway; not a solvency attestation"

// PriorDisclaimer 改标前的原文。两种文案都不是偿付根。
const PriorDisclaimer = "signed book snapshot for query-gateway; not a public reserve attestation"

const EngineeringClass = "engineering_merkle"

  func (p Proof) Copy() Proof { return p }

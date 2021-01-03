package holdlayerapi

import "account-assets.kucoin.net/funding/internal/holdopsgate"

type PlaceBody struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	TokenID string `json:"token_id"`
	GapID   string `json:"gap_id"`
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
	Actor string `json:"actor"`
	Nonce string `json:"nonce"`
}

type ReleaseBody struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	TokenID string `json:"token_id"`
	GapID   string `json:"gap_id"`
	Actor   string `json:"actor"`
	Nonce   string `json:"nonce"`
	LeaseID string `json:"ops_lease_id"`
	Ticket  string `json:"ticket"`
}

type GapBody struct {
	UserID string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	TokenID string `json:"token_id"`
	GapID   string `json:"gap_id"`
	Actor string `json:"actor"`
	Nonce string `json:"nonce"`
	LeaseID string `json:"ops_lease_id"`
	Ticket  string `json:"ticket"`
}

type OpsReply struct {
	Lease *holdopsgate.HoldOpsLease     `json:"lease"`
	Proof *holdopsgate.HoldOpsRaceProof `json:"proof,omitempty"`
	Gen int64 `json:"hold_ops_gen"`
}

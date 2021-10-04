package prefroute

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// Store prefroute 全部持久化操作的统一接口。

type Store interface {
	// ── PrefGeneration ──────────────────────────────
	GetGeneration(ctx context.Context, userID string) (fundpref.PrefGeneration, error)
	SaveGeneration(ctx context.Context, g fundpref.PrefGeneration) error
	ListGenerations(ctx context.Context,
		userID string, limit int) ([]fundpref.PrefGeneration, error)
	// ── DefaultAccountPref ──────────────────────────
	GetDefaultPref(ctx context.Context, userID string) (fundpref.DefaultAccountPref, error)
	SaveDefaultPref(ctx context.Context, p fundpref.DefaultAccountPref) error
	// ── PendingSwitch ───────────────────────────────
	SavePendingSwitch(ctx context.Context,
		ps fundpref.PendingSwitch) error
	GetPendingSwitch(ctx context.Context, id string) (fundpref.PendingSwitch, error)
	ListPendingSwitches(ctx context.Context, userID string, state fundpref.PendingSwitchState) ([]fundpref.PendingSwitch, error)
	UpdatePendingSwitch(ctx context.Context, ps fundpref.PendingSwitch) error
	// ── PrefLease ───────────────────────────────────
	SavePrefLease(ctx context.Context, l PrefLease) error
	GetPrefLease(ctx context.Context, id string) (PrefLease, error)
	UpdatePrefLease(ctx context.Context, l PrefLease) error
	ListActiveLeases(ctx context.Context,
		userID string) ([]PrefLease, error)
	ExpireStaleLeases(ctx context.Context, userID string, newGen int64, now time.Time) ([]PrefLease, error)
	// ── RouteSnap ───────────────────────────────────
	SaveRouteSnap(ctx context.Context, r RouteSnap) error
	GetRouteSnap(ctx context.Context, id string) (RouteSnap, error)
	UpdateRouteSnap(ctx context.Context, r RouteSnap) error
	ListRouteSnapsByLease(ctx context.Context, leaseID string) ([]RouteSnap, error)
	// ── PrefDriftCase ───────────────────────────────
	SaveDriftCase(ctx context.Context, d PrefDriftCase) error
	GetDriftCase(ctx context.Context, id string) (PrefDriftCase, error)
	UpdateDriftCase(ctx context.Context, d PrefDriftCase) error
	ListOpenDriftCases(ctx context.Context, userID string) ([]PrefDriftCase, error)
	HasOpenDriftCase(ctx context.Context, userID string) (bool, string, error)
	// ── DriftProof ──────────────────────────────────
	SaveDriftProof(ctx context.Context,
		p DriftProof) error
	GetDriftProof(ctx context.Context, id string) (DriftProof, error)
	ListDriftProofs(ctx context.Context, driftCaseID string) ([]DriftProof, error)
	// ── 在途贷记追踪 ────────────────────────────────
	RegisterInflightCredit(ctx context.Context, ic InflightCredit) error
	GetInflightCredit(ctx context.Context, txnID string) (InflightCredit, error)
	UpdateInflightCredit(ctx context.Context, ic InflightCredit) error
	ListInflightCredits(ctx context.Context, userID string, state InflightCreditState) ([]InflightCredit, error)
}

// ─────────────────────────────────────────────
// InflightCredit 在途贷记追踪
// ─────────────────────────────────────────────
// InflightCreditState 在途贷记状态。
type InflightCreditState string

const (
	InflightStatePending   InflightCreditState = "pending"   // 贷记发起，尚未完成
	InflightStateCompleted InflightCreditState = "completed" // 贷记已完成
	InflightStateFailed InflightCreditState = "failed" // 贷记失败
)

// InflightCredit 追踪用户在途贷记（入金/划转入），供切换互斥检查使用。
type InflightCredit struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	TxnID string `json:"txn_id"`
	Asset string `json:"asset"`
	Amount  string              `json:"amount"`
	State   InflightCreditState `json:"state"`
	LeaseID string              `json:"lease_id,omitempty"`
	RouteSnapID string    `json:"route_snap_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

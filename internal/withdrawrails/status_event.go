// Package withdrawrails 提币状态变更通知 funding-bff（未配置客户端时跳过）。
package withdrawrails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"account-assets.kucoin.net/funding/internal/fundingbff"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/platformjava"
	"github.com/rs/zerolog"
)

// StatusEvent 入参（POST /internal/v1/withdraw-rails/status-events）。
type StatusEvent struct {
	EventID       string `json:"eventId"`
	WithdrawalID  string `json:"withdrawalId"`
	UID           string `json:"uid"`
	Status        string `json:"status"`
	Currency      string `json:"currency"`
	Chain         string `json:"chain"`
	Amount        string `json:"amount"`
	Fee           string `json:"fee,omitempty"`
	ToAddress     string `json:"toAddress"`
	Memo          string `json:"memo,omitempty"`
	TxHash        string `json:"txHash,omitempty"`
	Confirmations int    `json:"confirmations,omitempty"`
	FailCode      string `json:"failCode,omitempty"`
	FailMessage   string `json:"failMessage,omitempty"`
	OccurredAt    string `json:"occurredAt"`
}

// StatusEventData 出参 data。
type StatusEventData struct {
	Accepted      bool   `json:"accepted"`
	PlatformExtID string `json:"platformExtId,omitempty"`
}

var ErrStatusEvent = errors.New("withdraw-rails status-event rejected")

type Notifier struct {
	bff *fundingbff.Client
	log zerolog.Logger
}

func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	return &Notifier{bff: bff, log: log}
}

func (n *Notifier) NotifyTransfer(ctx context.Context, tr *ledger.Transfer, status string) error {
	if n == nil || n.bff == nil || !n.bff.Enabled() || tr == nil {
		return nil
	}
	if tr.Direction != ledger.WalletWithdraw {
		return nil
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		return nil
	}
	ev, err := eventFromTransfer(tr, status)
	if err != nil {
		return err
	}
	_, err = postStatusEvent(ctx, n.bff, ev)
	if err != nil && n.log.GetLevel() != zerolog.Disabled {
		n.log.Warn().Err(err).Str("wd", ev.WithdrawalID).Str("status", status).Msg("withdraw-rails status-event")
	}
	return err
}

// NotifyFromNotifyKind 由 kcfund outbox kind + transfer 复用，避免重复挂点。
func (n *Notifier) NotifyFromNotifyKind(ctx context.Context, tr *ledger.Transfer, kind string) error {
	st := statusFromNotifyKind(kind, tr)
	if st == "" {
		return nil
	}
	return n.NotifyTransfer(ctx, tr, st)
}

func statusFromNotifyKind(kind string, tr *ledger.Transfer) string {
	switch strings.TrimSpace(kind) {
	case notify.KindWithdrawBroadcast:
		if tr != nil && strings.TrimSpace(tr.TxHash) != "" {
			return "BROADCAST"
		}
		return "SUBMITTED"
	case notify.KindWithdrawConfirmed:
		return "SUCCESS"
	case notify.KindWithdrawFailed:
		return "FAILED"
	default:
		return ""
	}
}

func eventFromTransfer(tr *ledger.Transfer, status string) (StatusEvent, error) {
	addr := strings.TrimSpace(tr.ToAddressHint)
	if addr == "" {
		addr = strings.TrimSpace(tr.ToAddress)
	}
	ev := StatusEvent{
		EventID:      tr.ID + ":" + status,
		WithdrawalID: tr.ID,
		UID:          strings.TrimSpace(tr.UserID),
		Status:       status,
		Currency:     strings.TrimSpace(tr.Asset),
		Chain:        strings.TrimSpace(tr.Chain),
		Amount:       tr.Amount.String(),
		ToAddress:    addr,
		Memo:         tr.MemoHint,
		TxHash:       strings.TrimSpace(tr.TxHash),
		OccurredAt:   tr.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if ev.OccurredAt == "" || ev.OccurredAt == "0001-01-01T00:00:00Z" {
		ev.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if tr.FailReason != "" && status == "FAILED" {
		ev.FailMessage = tr.FailReason
	}
	if ev.UID == "" || ev.WithdrawalID == "" || ev.Currency == "" || ev.Chain == "" || !tr.Amount.IsPositive() {
		return StatusEvent{}, fmt.Errorf("withdraw status-event: missing required fields")
	}
	return ev, nil
}

func postStatusEvent(ctx context.Context, bff *fundingbff.Client, ev StatusEvent) (StatusEventData, error) {
	var zero StatusEventData
	code, raw, err := bff.PostJSON(ctx, platformjava.PathWithdrawRailsStatusEvents, ev)
	if err != nil {
		return zero, err
	}
	if code == 0 {
		return zero, nil
	}
	if code >= 400 {
		return zero, fmt.Errorf("%w: http %d", ErrStatusEvent, code)
	}
	var wrap struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return zero, err
		}
		if wrap.Code != "" && wrap.Code != "200000" {
			return zero, fmt.Errorf("%w: code %s", ErrStatusEvent, wrap.Code)
		}
		var data StatusEventData
		if len(wrap.Data) > 0 {
			_ = json.Unmarshal(wrap.Data, &data)
		}
		return data, nil
	}
	return zero, nil
}

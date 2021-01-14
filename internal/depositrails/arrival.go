// Package depositrails 链上入账后通知 Java funding-bff 落平台充值单。
package depositrails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"account-assets.kucoin.net/funding/internal/fundingbff"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/platformjava"
	"github.com/rs/zerolog"
)

// ArrivalRequest 与 Java POST /internal/v1/deposit-rails/arrivals 入参一致。
type ArrivalRequest struct {
	UID           string `json:"uid"`
	Chain         string `json:"chain"`
	Asset         string `json:"asset"`
	Amount        string `json:"amount"`
	TxHash        string `json:"txHash"`
	Address       string `json:"address"`
	Confirmations int    `json:"confirmations"`
	Credited      bool   `json:"credited"`
}

// ArrivalData 成功响应 data 段。
type ArrivalData struct {
	OrderID       string `json:"orderId"`
	UID           string `json:"uid"`
	Chain         string `json:"chain"`
	Asset         string `json:"asset"`
	Amount        string `json:"amount"`
	TxHash        string `json:"txHash"`
	Address       string `json:"address"`
	Confirmations int    `json:"confirmations"`
	Credited      bool   `json:"credited"`
	State         string `json:"state"`
	Notified      bool   `json:"notified"`
}

var (
	ErrArrivalTransport = errors.New("deposit-rails arrival transport failed")
	ErrArrivalRejected  = errors.New("deposit-rails arrival rejected")
)

// Notifier 入账后回调 Java；失败不反向改 ledger。
type Notifier struct {
	bff *fundingbff.Client
	log zerolog.Logger
}

func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	return &Notifier{bff: bff, log: log}
}

func (n *Notifier) NotifyArrival(ctx context.Context, tr *ledger.Transfer, confirmations int) error {
	if n == nil || n.bff == nil || !n.bff.Enabled() || tr == nil {
		return nil
	}
	if tr.Direction != ledger.WalletDeposit || tr.State != ledger.StateCredited {
		return nil
	}
	req, err := RequestFromTransfer(tr, confirmations, true)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArrivalRejected, err)
	}
	data, err := PostArrival(ctx, n.bff, req)
	if err != nil {
		if n.log.GetLevel() != zerolog.Disabled {
			n.log.Warn().Err(err).
				Str("user", req.UID).Str("chain", req.Chain).Str("tx", req.TxHash).
				Msg("deposit-rails arrival")
		}
		return err
	}
	if n.log.GetLevel() != zerolog.Disabled {
		n.log.Info().
			Str("orderId", data.OrderID).Str("state", data.State).Bool("notified", data.Notified).
			Str("tx", req.TxHash).Msg("deposit-rails arrival ok")
	}
	return nil
}

func PostArrival(ctx context.Context, bff *fundingbff.Client, req ArrivalRequest) (ArrivalData, error) {
	var zero ArrivalData
	code, raw, err := bff.PostJSON(ctx, platformjava.PathDepositRailsArrivals, req)
	if err != nil {
		return zero, fmt.Errorf("%w: %v", ErrArrivalTransport, err)
	}
	if code == 0 {
		return zero, nil
	}
	return decodeArrivalResponse(code, raw)
}

func decodeArrivalResponse(code int, raw []byte) (ArrivalData, error) {
	var zero ArrivalData
	if code >= 400 {
		return zero, fmt.Errorf("%w: http %d body %s", ErrArrivalRejected, code, truncate(raw, 512))
	}
	var wrap struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return zero, fmt.Errorf("%w: decode envelope: %v", ErrArrivalRejected, err)
	}
	if wrap.Code != "" && wrap.Code != "200000" {
		return zero, fmt.Errorf("%w: code %s msg %s", ErrArrivalRejected, wrap.Code, wrap.Msg)
	}
	var data ArrivalData
	if len(wrap.Data) > 0 {
		if err := json.Unmarshal(wrap.Data, &data); err != nil {
			return zero, fmt.Errorf("%w: decode data: %v", ErrArrivalRejected, err)
		}
	}
	return data, nil
}

func RequestFromTransfer(tr *ledger.Transfer, confirmations int, credited bool) (ArrivalRequest, error) {
	if tr == nil {
		return ArrivalRequest{}, fmt.Errorf("transfer required")
	}
	addr := strings.TrimSpace(tr.ToAddressHint)
	if addr == "" {
		addr = strings.TrimSpace(tr.ToAddress)
	}
	req := ArrivalRequest{
		UID:           strings.TrimSpace(tr.UserID),
		Chain:         strings.TrimSpace(tr.Chain),
		Asset:         strings.TrimSpace(tr.Asset),
		Amount:        tr.Amount.String(),
		TxHash:        strings.TrimSpace(tr.TxHash),
		Address:       addr,
		Confirmations: confirmations,
		Credited:      credited,
	}
	if req.UID == "" || req.Chain == "" || req.Asset == "" || req.TxHash == "" {
		return ArrivalRequest{}, fmt.Errorf("missing uid/chain/asset/txHash")
	}
	if !tr.Amount.IsPositive() {
		return ArrivalRequest{}, fmt.Errorf("amount must be positive")
	}
	if confirmations < 0 {
		return ArrivalRequest{}, fmt.Errorf("confirmations must be >= 0")
	}
	return req, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

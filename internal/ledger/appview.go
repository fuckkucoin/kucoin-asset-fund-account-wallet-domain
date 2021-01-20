// 账本对外展示字段。
// 作者: King
// 日期: 2021.01.05
package ledger

import (
	"context"
	"time"
	"github.com/shopspring/decimal"
)

// Subject 用户侧科目：现货资金账 / 钱包账；手续费和 bridge 不给用户接口。
   func (t AccountType) Subject() string {
	   switch t {
	  case AccountSpot:
		  return "funding"
	   case AccountWallet:
		  return "wallet"
	  case AccountEarn:
		  return "earn"
	  case AccountTrading:
		   return "trading"
	   case AccountMargin:
		  return "margin"
	   case AccountIsolated:
		   return "isolated"
	   case AccountFutures:
		  return "futures"
	  case AccountFee:
		   return "fee"
	   case AccountBridge:
		   return "bridge"
	   case AccountCold:
		   return "cold"
	  case AccountWarm:
		  return "warm"
	   default:
		   return string(t)
	  }
  }

   func (t AccountType) UserFacing() bool {
	   return t == AccountSpot || t == AccountWallet || t == AccountEarn ||
		  t == AccountTrading || t == AccountMargin || t == AccountIsolated || t == AccountFutures
   }

// AppStatus 给前端的单据状态，机内状态仍在 State。
  func (s State) AppStatus() string {
	   switch s {
	   case StateCredited:
		   return "success"
	  case StateRolledBack:
		  return "cancelled"
	   case StateFailed:
		  return "failed"
	  case StateHold:
		  return "hold"
	  case StateInitiated:
		  return "processing"
	  case StateSpotFrozen, StateWalletLocked:
		   return "frozen"
	   case StateChainSubmitted, StateConfirmed, StateReorged:
		   return "confirming"
	   default:
		   return string(s)
	   }
  }

   func (s State) AppStatusDesc() string {
	  switch s.AppStatus() {
	   case "success":
		   return "credited to destination book"
	  case "cancelled":
		   return "rolled back; freeze released"
	  case "failed":
		   return "failed; awaiting rollback if a freeze remains"
	   case "hold":
		   return "held for observation mismatch"
	  case "processing":
		  return "accepted, not yet frozen"
	   case "frozen":
		  return "source available moved to frozen"
	  case "confirming":
		   return "chain submitted or confirming"
	  default:
		   return string(s)
	   }
  }

  func (d Direction) FromAccount() string {
	   switch d {
	   case SpotToWallet:
		  return AccountSpot.Subject()
	  case WalletToSpot, WalletWithdraw:
		   return AccountWallet.Subject()
	   case WalletDeposit:
		   return "chain"
	  case SpotSend:
		   return AccountSpot.Subject()
	   case FiatOnramp:
		   return "fiat"
	  case ConvertDebit,
		  P2PDebit, SubDebit, FeeDebit:
		   return AccountSpot.Subject()
	  case ConvertCredit:
		  return "convert"
	  case P2PCredit:
		  return "p2p"
	  case SubCredit:
		   return "sub"
	   case EarnInterest:
		   return "earn"
	   case FiatOfframp:
		   return AccountSpot.Subject()
	  case EarnSubscribe:
		   return AccountSpot.Subject()
	   case EarnRedeem:
		   return AccountEarn.Subject()
	  case HotColdSweep, HotWarmSweep:
		  return AccountBridge.Subject()
	  case WarmColdSweep:
		  return AccountWarm.Subject()
	  case FundingToTrading, FundingToMargin, FundingToFutures, FundingToIsolated:
		   return AccountSpot.Subject()
	  case TradingToFunding:
		  return AccountTrading.Subject()
	  case MarginToFunding:
		  return AccountMargin.Subject()
	  case IsolatedToFunding:
		  return AccountIsolated.Subject()
	   case FuturesToFunding:
		   return AccountFutures.Subject()
	  default:
		   return ""
	   }
   }

  func (d Direction) ToAccount() string {
	   switch d {
	   case SpotToWallet, WalletDeposit:
		  return AccountWallet.Subject()
	  case WalletToSpot:
		  return AccountSpot.Subject()
	   case WalletWithdraw:
		   return "chain"
	  case SpotSend:
		  return "funding"
	   case FiatOnramp, ConvertCredit, P2PCredit, SubCredit, EarnInterest:
		  return AccountSpot.Subject()
	  case FiatOfframp:
		   return "fiat"
	   case ConvertDebit:
		   return "convert"
	  case FeeDebit:
		   return AccountFee.Subject()
	   case P2PDebit:
		  return "p2p"
	   case SubDebit:
		   return "sub"
	  case EarnSubscribe:
		   return AccountEarn.Subject()
	   case EarnRedeem:
		  return AccountSpot.Subject()
	  case HotColdSweep, WarmColdSweep:
		  return AccountCold.Subject()
	  case HotWarmSweep:
		  return AccountWarm.Subject()
	   case FundingToTrading:
		  return AccountTrading.Subject()
	   case FundingToMargin:
		   return AccountMargin.Subject()
	   case FundingToIsolated:
		   return AccountIsolated.Subject()
	  case FundingToFutures:
		  return AccountFutures.Subject()
	   case TradingToFunding, MarginToFunding, FuturesToFunding, IsolatedToFunding:
		   return AccountSpot.Subject()
	  default:
		   return ""
	   }
   }

// BookView 用户侧一个科目：available + frozen + total。

type BookView struct {
	Asset string `json:"asset"`
	Book    string `json:"book"`
	Subject string `json:"subject"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
	Total  string `json:"total"`
}

   func ViewBook(a Account) BookView {
	  return BookView{
		  Asset: a.Asset,
		  Book: string(a.AccountType),
		   Subject:   a.AccountType.Subject(),
		  Available: a.Available.String(),
		  Frozen:    a.Frozen.String(),
		   Total: a.Available.Add(a.Frozen).String(),
	  }
  }

  func UserBookViews(accs []Account) []BookView {
	  out := make([]BookView, 0, len(accs))
	   for _, a := range accs {
		  if a.UserID == "system" || !a.AccountType.UserFacing() {
			   continue
		   }
		  out = append(out, ViewBook(a))
	   }
	   return out
  }

// Accounts 资金/冻结/手续费科目，给 App 账户页。

   func (s *Service) Accounts(ctx context.Context, userID string) ([]BookView, error) {
	   accs, err := s.store.ListUserBooks(ctx, userID)
	  if err != nil {
		   return nil, err
	   }
	  return UserBookViews(accs), nil
   }

// TransferView 划转单给 App 看的字段；状态机字段仍保留给运维。
type TransferView struct {
	ID string `json:"id"`
	OrderID        string `json:"order_id"`
	ClientOid      string `json:"client_oid"`
	MovementID     string `json:"movement_id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Direction string `json:"direction"`
	Type       string `json:"type"`
	Settlement string `json:"settlement"`
	Kind   string `json:"kind"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	FilledAmount string `json:"filled_amount"`
	RemainAmount string `json:"remain_amount"`
	ObservedAmount string `json:"observed_amount,omitempty"`
	ObservedAsset string `json:"observed_asset,omitempty"`
	Chain string `json:"chain"`
	ToAddressHint string `json:"to_address_hint,omitempty"`
	FromAddressHint string `json:"from_address_hint,omitempty"`
	Memo            string `json:"memo,omitempty"`
	Tag             string `json:"tag,omitempty"`
	TagName  string `json:"tagName,omitempty"`
	NeedMemo bool   `json:"needMemo,omitempty"`
	TxHash      string `json:"tx_hash,omitempty"`
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
	Status      string `json:"status"`
	StatusDesc  string `json:"status_desc"`
	State       string `json:"state"`
	RiskFlags []string `json:"risk_flags,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
	FenceEpoch int64  `json:"fence_epoch,omitempty"`
	FenceToken  string    `json:"fence_token,omitempty"`
	KYTDecision string    `json:"kyt_decision,omitempty"`
	TraceID     string    `json:"trace_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedAtMs int64     `json:"created_at_ms"`
	UpdatedAtMs int64 `json:"updated_at_ms"`
}

   func ViewTransfer(tr *Transfer, traceID string) TransferView {
	   if tr == nil {
		  return TransferView{}
	  }
	  filled, remain := transferFill(tr)
	  obs := ""
	   if !tr.ObservedAmount.IsZero() {
		   obs = tr.ObservedAmount.String()
	   }
	   updated := tr.UpdatedAt
	  if updated.IsZero() {
		  updated = tr.CreatedAt
	   }
	  return TransferView{
		   ID: tr.ID, OrderID: tr.ID, ClientOid: tr.IdempotencyKey, MovementID: tr.ID,
		   IdempotencyKey: tr.IdempotencyKey, UserID: tr.UserID,
		  Direction: string(tr.Direction), Type: string(tr.Direction),
		  Settlement: tr.Direction.Settlement(), Kind: tr.Direction.Settlement(),
		   Asset: tr.Asset, Amount: tr.Amount.String(),
		   FilledAmount: filled.String(), RemainAmount: remain.String(),
		   ObservedAmount: obs, ObservedAsset: tr.ObservedAsset,
		   Chain: tr.Chain, ToAddressHint: tr.ToAddressHint, FromAddressHint: tr.FromAddressHint,
		  Memo: tr.MemoHint, Tag: tr.TagHint, TagName: tr.TagNameHint, NeedMemo: tr.TagNameHint != "" || tr.MemoHint != "",
		  TxHash:      tr.TxHash,
		  FromAccount: tr.Direction.FromAccount(), ToAccount: tr.Direction.ToAccount(),
		   Status: viewStatus(tr), StatusDesc: viewStatusDesc(tr),
		   State: string(tr.State), RiskFlags: tr.RiskFlags, FailReason: tr.FailReason,
		   FenceEpoch: tr.FenceEpoch, FenceToken: tr.FenceToken, KYTDecision: tr.KYTDecision, TraceID: traceID,
		  CreatedAt: tr.CreatedAt, UpdatedAt: updated,
		  CreatedAtMs: tr.CreatedAt.UnixMilli(), UpdatedAtMs: updated.UnixMilli(),
	  }
  }

   func viewStatus(tr *Transfer) string {
	   if st, ok := LoopRail(tr); ok && st != "" {
		   return st
	  }
	   return tr.State.AppStatus()
  }

  func viewStatusDesc(tr *Transfer) string {
	   if st, ok := LoopRail(tr); ok && st != "" {
		  switch st {
		   case RailRecovering:
			  return "close-loop recovering"
		   case RailEscalated:
			   return "close-loop escalated"
		  case RailCounterparty:
			   return "counterparty not credited"
		  case RailCoverStranded:
			   return "cover stranded"
		  case RailHopCompensate:
			   return "hop compensating"
		   default:
			  return st
		  }
	   }
	  return tr.State.AppStatusDesc()
   }

  func transferFill(tr *Transfer) (filled, remain decimal.Decimal) {
	   switch tr.State {
	   case StateCredited:
		  return tr.Amount, decimal.Zero
	  case StateRolledBack:
		  return decimal.Zero, decimal.Zero
	   default:
		   return decimal.Zero, tr.Amount
	  }
  }

  func MatchTransfer(tr *Transfer, status, direction, state string) bool {
	   if tr == nil {
		   return false
	   }
	  if direction != "" && string(tr.Direction) != direction {
		  return false
	  }
	   if state != "" && string(tr.State) != state {
		  return false
	   }
	   if status != "" && tr.State.AppStatus() != status && string(tr.State) != status {
		   return false
	  }
	   return true
   }

// FeePaid 用户可见手续费。

type FeePaid struct {
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Subject string `json:"subject"`
}

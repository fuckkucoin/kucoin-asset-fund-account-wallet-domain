// 账本科目、方向和状态机定义。
// 作者: Knox
// 日期: 2021.01.06
package ledger

import (
	"fmt"
	"time"
	"github.com/shopspring/decimal"
)

// 账本状态机：可用余额只能经 freeze/lock 离开，目标账只能经 credit 入账。
// failed 不是终态——冻结未回滚时对账必须能看见，不能当资金消失。
type State string

const (
	StateInitiated State = "initiated"
	StateSpotFrozen State = "spot_frozen"
	StateWalletLocked   State = "wallet_locked"
	StateChainSubmitted State = "chain_submitted"
	StateConfirmed      State = "confirmed"
	StateCredited State = "credited"
	StateFailed State = "failed"
	StateRolledBack State = "rolled_back"
	StateHold       State = "hold"
	StateReorged State = "reorged"
)

// Open 单据还欠冻结、确认、入账或回滚时为 true；credited/rolled_back 才算了结。
   func (s State) Open() bool {
	   switch s {
	  case StateCredited, StateRolledBack:
		  return false
	  default:
		  return true
	  }
   }

type Event string

const (
	EventFreezeSpot Event = "freeze_spot"
	EventLockWallet Event = "lock_wallet"
	EventSubmit     Event = "submit_chain"
	EventConfirm    Event = "confirm"
	EventCredit   Event = "credit"
	EventFail     Event = "fail"
	EventRollback Event = "rollback"
	EventHold Event = "hold"
	EventReorgUnwind Event = "reorg_unwind"
)

type Direction string

const (
	SpotToWallet Direction = "spot_to_wallet"
	WalletToSpot Direction = "wallet_to_spot"
	WalletWithdraw Direction = "wallet_withdraw"
	WalletDeposit  Direction = "wallet_deposit"
	SpotSend       Direction = "spot_send" // same-asset funding book, user → user, no chain
	FiatOnramp Direction = "fiat_onramp" // credit funding from system.bridge, no chain
	FiatOfframp Direction = "fiat_offramp" // freeze funding then park onto system.bridge, no chain
	ConvertDebit  Direction = "convert_debit"  // 闪兑扣源：资金账冻后停 bridge
	ConvertCredit Direction = "convert_credit" // 闪兑入目标：bridge → 资金账
	P2PDebit      Direction = "p2p_debit"      // P2P 卖出/保证金占用
	P2PCredit Direction = "p2p_credit" // P2P 买入入账或占用释放
	SubDebit Direction = "sub_debit" // 母户划出到子户 overlay
	SubCredit     Direction = "sub_credit"     // 子户划回母户账本
	EarnSubscribe Direction = "earn_subscribe" // funding → earn via bridge hop
	EarnRedeem    Direction = "earn_redeem"    // earn → funding; optional interest from bridge
	EarnInterest  Direction = "earn_interest"   // 理财利息：bridge → 资金账，不另铸
	FeeDebit      Direction = "fee_debit"       // 资金账扣手续费/罚金 → system.fee（非 bridge）
	HotColdSweep  Direction = "hot_cold_sweep"  // system.bridge excess → system.cold
	HotWarmSweep  Direction = "hot_warm_sweep"  // system.bridge excess → system.warm
	WarmColdSweep Direction = "warm_cold_sweep" // system.warm excess → system.cold
	FundingToTrading Direction = "funding_to_trading"
	TradingToFunding Direction = "trading_to_funding"
	FundingToMargin  Direction = "funding_to_margin"
	MarginToFunding Direction = "margin_to_funding"
	FundingToFutures Direction = "funding_to_futures"
	FuturesToFunding Direction = "futures_to_funding"
	FundingToIsolated Direction = "funding_to_isolated"
	IsolatedToFunding Direction = "isolated_to_funding"
)

// ParseDirection 认内部划转和链上充提词表；未知字符串直接失败。
   func ParseDirection(s string) (Direction, error) {
	  switch Direction(s) {
	   case SpotToWallet,
		   WalletToSpot, WalletWithdraw, WalletDeposit, SpotSend,
		  FiatOnramp, FiatOfframp, ConvertDebit, ConvertCredit, P2PDebit, P2PCredit,
		  SubDebit, SubCredit, EarnSubscribe, EarnRedeem, EarnInterest, FeeDebit,
		  HotColdSweep, HotWarmSweep, WarmColdSweep,
		  FundingToTrading, TradingToFunding, FundingToMargin, MarginToFunding,
		  FundingToFutures,
		  FuturesToFunding, FundingToIsolated, IsolatedToFunding:
		  return Direction(s), nil
	   default:
		   return "", ErrUnknownDirection
	  }
   }

// Internal 现货↔钱包、法币、理财、热冷归集：不上链。
   func (d Direction) Internal() bool {
	   switch d {
	   case SpotToWallet, WalletToSpot, FiatOnramp, FiatOfframp, ConvertDebit, ConvertCredit,
		   P2PDebit,
		  P2PCredit, SubDebit, SubCredit, EarnSubscribe, EarnRedeem, EarnInterest, FeeDebit,
		  HotColdSweep, HotWarmSweep, WarmColdSweep,
		  FundingToTrading, TradingToFunding, FundingToMargin, MarginToFunding,
		   FundingToFutures, FuturesToFunding, FundingToIsolated, IsolatedToFunding:
		   return true
	   default:
		   return false
	   }
   }

   func (d Direction) UserSend() bool {
	  return d == SpotSend
  }

// OnChain 只有提现和充值走链。
   func (d Direction) OnChain() bool {
	  return d == WalletWithdraw || d == WalletDeposit
   }

// Freezes 需要先冻源账的方向；充值不冻用户。
   func (d Direction) Freezes() bool {
	   switch d {
	   case SpotToWallet, WalletToSpot, WalletWithdraw, SpotSend, FiatOfframp, ConvertDebit, P2PDebit, SubDebit,
		  EarnSubscribe, EarnRedeem, FeeDebit,
		   FundingToTrading, TradingToFunding, FundingToMargin, MarginToFunding,
		   FundingToFutures, FuturesToFunding, FundingToIsolated, IsolatedToFunding:
		   return true
	  default:
		  return false
	  }
   }

// Settlement 产品分类：内部账、提现、充值、法币、理财、托管。

  func (d Direction) Settlement() string {
	   switch d {
	   case SpotToWallet, WalletToSpot:
		  return "internal"
	   case WalletWithdraw:
		  return "withdraw"
	   case WalletDeposit:
		   return "deposit"
	  case SpotSend:
		  return "internal_send"
	  case FiatOnramp, FiatOfframp:
		  return "fiat"
	  case ConvertDebit, ConvertCredit:
		   return "convert"
	   case P2PDebit, P2PCredit:
		  return "p2p"
	   case SubDebit, SubCredit:
		   return "sub"
	  case EarnSubscribe, EarnRedeem, EarnInterest:
		  return "earn"
	  case FeeDebit:
		  return "fee"
	   case HotColdSweep, HotWarmSweep, WarmColdSweep:
		  return "custody"
	   case FundingToTrading, TradingToFunding, FundingToMargin, MarginToFunding,
		  FundingToFutures, FuturesToFunding, FundingToIsolated, IsolatedToFunding:
		  return "book"
	   default:
		  return "unknown"
	   }
  }

   func InternalTxHash(id string) string { return "internal:" + id }

   func IsInternalTxHash(h string) bool {
	  return len(h) >= 9 && h[:9] == "internal:"
  }

type AccountType string

const (
	AccountSpot AccountType = "spot" // 托管资金账；App 侧科目 funding
	AccountWallet AccountType = "wallet" // 自托管钱包账
	AccountEarn   AccountType = "earn"   // 理财科目
	AccountTrading  AccountType = "trading"  // 现货交易占用账；本进程不跑撮合
	AccountMargin   AccountType = "margin"   // 全仓杠杆占用账；无强平引擎
	AccountIsolated AccountType = "isolated" // 逐仓杠杆占用账；按交易对记账，无强平引擎
	AccountFutures AccountType = "futures" // 合约占用账；无撮合引擎
	AccountBridge  AccountType = "bridge"  // 系统过渡：内部 hop / 链上 park，不对用户展示
	AccountFee  AccountType = "fee"  // 系统手续费，Swap 入账方
	AccountCold AccountType = "cold" // 系统冷端：desk 超额从 bridge 归集
	AccountWarm AccountType = "warm" // 系统温端：独立科目，热超额先入温再入冷
)

// Account 用户某科目某资产的余额。可用与冻结都不得为负。

type Account struct {
	ID int64
	UserID      string
	AccountType AccountType
	Asset     string
	Available decimal.Decimal
	Frozen decimal.Decimal
	// snap* 记下 GetAccount 读到的余额。条件更新要求行上仍是这对数字。
	// snapOK 为假表示这组余额不是本次从库里读出来的，事务内严禁按它写回。
	snapAvail  decimal.Decimal
	snapFrozen decimal.Decimal
	snapOK     bool
}

// stampBook 把当前可用、冻结记成快照，并标记这份余额来自本次读取。
// 只允许在 GetAccount 读回之后，或条件更新成功之后调用。严禁在手工填数后调用来绕过核对。
func (a *Account) stampBook() {
	if a == nil {
		return
	}
	a.snapAvail = a.Available
	a.snapFrozen = a.Frozen
	a.snapOK = true
}

// JournalEntry 复式分录的一边。同一行不能又借又贷，按资产必须轧平。
type JournalEntry struct {
	TransferID  string
	Seq         int
	UserID      string
	AccountType AccountType
	Asset       string
	Debit  decimal.Decimal
	Credit decimal.Decimal
	Memo   string
}

type Transfer struct {
	ID             string
	IdempotencyKey string
	UserID         string
	Direction Direction
	Asset     string
	Amount    decimal.Decimal
	Chain        string
	ToAddress    string
	ToAddressEnc []byte
	ToAddressHint   string
	FromAddressHint string // 链上付款方；观测可得时写入，供历史与核验回显
	MemoHint string // 观察 memo/tag 回显（FamMEMO）
	TagHint     string
	TagNameHint string // memo / destination_tag / comment
	TxHash      string
	OutputIndex int
	State      State
	RiskFlags  []string
	FailReason string
	CreatedAt  time.Time
	UpdatedAt time.Time
	FenceEpoch int64
	FenceToken string
	ObservedAmount decimal.Decimal
	ObservedAsset  string
	CreditedHeight int64
	CreditedBlock  string
	KYTDecision string
}

type AuditEvent struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Action string `json:"action"`
	Actor  string         `json:"actor"`
	Detail map[string]any `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// transitions is the only legal graph. Apply rejects anything else so credit
// cannot skip freeze, and rollback cannot run after credited (that would
// double-restore available).

var transitions = map[State]map[Event]State{
	StateInitiated: {
		EventFreezeSpot: StateSpotFrozen,
		EventLockWallet: StateWalletLocked,
		EventSubmit: StateChainSubmitted, // deposit: observe hash with no user freeze
		EventFail:   StateFailed,
	},
	StateSpotFrozen: {
		EventSubmit: StateChainSubmitted,
		EventFail: StateFailed,
		EventRollback: StateRolledBack,
	},
	StateWalletLocked: {
		EventSubmit:   StateChainSubmitted,
		EventFail:     StateFailed,
		EventRollback: StateRolledBack,
	},
	StateChainSubmitted: {
		EventConfirm:     StateConfirmed,
		EventFail:        StateFailed,
		EventRollback:    StateRolledBack,
		EventReorgUnwind: StateChainSubmitted,
		EventHold:        StateHold,
	},
	StateConfirmed: {
		EventCredit: StateCredited,
		EventFail: StateFailed,
		EventHold:        StateHold,
		EventReorgUnwind: StateChainSubmitted,
	},
	StateCredited: {
		EventReorgUnwind: StateReorged,
	},
	StateHold: {
		EventFail: StateFailed,
		EventRollback: StateRolledBack,
	},
	StateReorged: {
		EventSubmit: StateChainSubmitted,
		EventFail:   StateFailed,
	},
	StateFailed: {
		EventRollback: StateRolledBack,
	},
}

// Apply 状态机唯一出口；非法转移返回 ErrIllegalTransition，禁止跳步入账。

  func Apply(current State, ev Event) (State, error) {
	   next, ok := transitions[current][ev]
	  if !ok {
		  return current, fmt.Errorf("%w: %s + %s", ErrIllegalTransition, current, ev)
	  }
	  return next, nil
  }

  func CanApply(current State, ev Event) bool {
	   _, err := Apply(current, ev)
	  return err == nil
   }

// ValidateJournal 在落账前核对流水。金额不得为负，同一行不得又借又贷，同一资产借贷必须相等。
// 任一笔不合格则整批拒绝。严禁跳过本校验直接插入流水。

  func ValidateJournal(entries []JournalEntry) error {
	   if len(entries) == 0 {
		  return fmt.Errorf("empty journal")
	   }
	   type tot struct{ d, c decimal.Decimal }
	  byAsset := map[string]*tot{}
	  for i, e := range entries {
		  if e.Debit.IsNegative() || e.Credit.IsNegative() {
			  return fmt.Errorf("entry %d has negative amount", i)
		   }
		  if !e.Debit.IsZero() && !e.Credit.IsZero() {
			  return fmt.Errorf("entry %d debits and credits in one line", i)
		   }
		   if e.Debit.IsZero() && e.Credit.IsZero() {
			   return fmt.Errorf("entry %d is empty", i)
		   }
		  t := byAsset[e.Asset]
		  if t == nil {
			  t = &tot{}
			   byAsset[e.Asset] = t
		  }
		   t.d = t.d.Add(e.Debit)
		   t.c = t.c.Add(e.Credit)
	  }
	  for asset, t := range byAsset {
		  if !t.d.Equal(t.c) {
			  return fmt.Errorf("unbalanced journal for %s: debit %s credit %s", asset, t.d, t.c)
		   }
	   }
	   return nil
  }

// freezeLines moves value inside one book (available → frozen). Internal
// transfers credit the other book in the same lock; on-chain withdraw waits
// for confirmed credit (consume frozen only). Deposit never freezes.
// freezeLines 冻源账：available → frozen，划转/提现/兑换共用这组 memo。
   func freezeLines(tr *Transfer, book AccountType) []JournalEntry {
	   return []JournalEntry{
		   {TransferID: tr.ID, Seq: 1, UserID: tr.UserID, AccountType: book, Asset: tr.Asset, Debit: tr.Amount, Memo: "freeze_available"},
		   {TransferID: tr.ID, Seq: 2, UserID: tr.UserID, AccountType: book, Asset: tr.Asset, Credit: tr.Amount, Memo: "freeze_frozen"},
	  }
   }

// BuildFreezeLockLines 暴露既有 available→frozen 锁仓分录，供维持保证金强平路径复用，不另造账本产品。
  func BuildFreezeLockLines(tr *Transfer, book AccountType) []JournalEntry {
	  return freezeLines(tr, book)
   }

// BuildUnfreezeReleaseLines 暴露既有 frozen→available 释放分录，供部分强平成交按成交额释冻，不另造科目。

  func BuildUnfreezeReleaseLines(tr *Transfer, book AccountType) []JournalEntry {
	   return rollbackLines(tr, book, 1)
   }

// creditLines consumes frozen on the source book, hops through system.bridge,
// and credits the destination available. The bridge hop exists so both user
// books stay internally consistent while the conversion is still one asset.

  func creditLines(tr *Transfer, from, to AccountType, seq int) []JournalEntry {
	  if seq <= 0 {
		   seq = 1
	   }
	   return []JournalEntry{
		  {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: from, Asset: tr.Asset, Debit: tr.Amount, Memo: "consume_frozen"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: tr.Amount, Memo: "bridge_in"},
		  {TransferID: tr.ID, Seq: seq + 2, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: tr.Amount, Memo: "bridge_out"},
		  {TransferID: tr.ID, Seq: seq + 3, UserID: tr.UserID, AccountType: to, Asset: tr.Asset, Credit: tr.Amount, Memo: "credit_available"},
	  }
   }

// rollbackLines 解冻：frozen → available，失败回滚用。

   func rollbackLines(tr *Transfer, book AccountType, seq int) []JournalEntry {
	   if seq <= 0 {
		  seq = 1
	  }
	  return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: book, Asset: tr.Asset, Debit: tr.Amount, Memo: "unfreeze_frozen"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: tr.UserID, AccountType: book, Asset: tr.Asset, Credit: tr.Amount, Memo: "restore_available"},
	  }
   }

// SourceBook 冻/扣的源科目，给冻结原因和对账展示用。
   func (d Direction) SourceBook() AccountType { return sourceBook(d) }

// DestBook 入账目标科目。
  func (d Direction) DestBook() AccountType { return destBook(d) }

  func sourceBook(d Direction) AccountType {
	  switch d {
	  case SpotToWallet,
		  SpotSend, FiatOfframp, ConvertDebit, P2PDebit, SubDebit, EarnSubscribe, FeeDebit, FundingToTrading, FundingToMargin, FundingToFutures, FundingToIsolated:
		   return AccountSpot
	   case EarnRedeem:
		   return AccountEarn
	  case TradingToFunding:
		  return AccountTrading
	  case MarginToFunding:
		  return AccountMargin
	   case IsolatedToFunding:
		   return AccountIsolated
	   case FuturesToFunding:
		  return AccountFutures
	   default:
		   return AccountWallet
	  }
  }

  func destBook(d Direction) AccountType {
	  switch d {
	  case SpotToWallet, WalletDeposit:
		   return AccountWallet
	  case FeeDebit:
		   return AccountFee
	  case EarnSubscribe:
		   return AccountEarn
	  case HotColdSweep:
		  return AccountCold
	  case HotWarmSweep:
		   return AccountWarm
	  case WarmColdSweep:
		   return AccountCold
	   case FundingToTrading:
		   return AccountTrading
	   case FundingToMargin:
		  return AccountMargin
	   case FundingToIsolated:
		   return AccountIsolated
	   case FundingToFutures:
		  return AccountFutures
	  default:
		   return AccountSpot
	  }
   }

  func freezeEvent(d Direction) Event {
	   switch d {
	  case SpotToWallet, SpotSend, FiatOfframp, ConvertDebit, P2PDebit, SubDebit, EarnSubscribe, EarnRedeem, FeeDebit,
		   FundingToTrading, FundingToMargin, FundingToFutures, FundingToIsolated:
		  return EventFreezeSpot
	   default:
		  return EventLockWallet
	   }
  }

// withdrawSettleLines consumes the wallet freeze after a confirmed outbound
// tx. Contra sits on system.bridge (outflow park). Fee, if any, is carved
// from the same freeze onto system.fee so the chain amount is amount-fee.
  func withdrawSettleLines(tr *Transfer, seq int, fee decimal.Decimal) []JournalEntry {
	  if seq <= 0 {
		  seq = 1
	   }
	  if fee.IsNegative() {
		  fee = decimal.Zero
	  }
	   if fee.GreaterThanOrEqual(tr.Amount) {
		   fee = decimal.Zero
	  }
	   out := tr.Amount.Sub(fee)
	   lines := []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountWallet, Asset: tr.Asset, Debit: tr.Amount, Memo: "consume_frozen"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: out, Memo: "bridge_in"},
	  }
	  if !fee.IsZero() {
		   lines = append(lines, JournalEntry{TransferID: tr.ID, Seq: seq + 2, UserID: "system", AccountType: AccountFee, Asset: tr.Asset, Credit: fee, Memo: "fee_credit"})
	  }
	  return lines
  }

// depositCreditLines credits the wallet book from system.bridge after the
// inbound hash is confirmed. No user freeze: the user never locked inventory.

  func depositCreditLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		   seq = 1
	   }
	   amt := creditQty(tr)
	   return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: amt, Memo: "bridge_out"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: tr.UserID, AccountType: AccountWallet, Asset: tr.Asset, Credit: amt, Memo: "credit_available"},
	  }
   }

   func creditQty(tr *Transfer) decimal.Decimal {
	   if tr == nil {
		  return decimal.Zero
	   }
	   if tr.Direction == WalletDeposit && !tr.ObservedAmount.IsZero() {
		   return tr.ObservedAmount
	  }
	   return tr.Amount
  }

   func depositUnwindLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		   seq = 1
	  }
	  amt := creditQty(tr)
	  return []JournalEntry{
		  {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountWallet, Asset: tr.Asset, Debit: amt, Memo: "reorg_debit_available"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: amt, Memo: "bridge_in"},
	  }
  }

// withdrawUnwindLines 按入账分录反转：bridge 只退实发额，手续费从 fee 科目扣回，冻结按本金恢复。

  func withdrawUnwindLines(tr *Transfer, prev []JournalEntry, seq int) []JournalEntry {
	  if seq <= 0 {
		   seq = 1
	  }
	   fee := decimal.Zero
	  parked := tr.Amount
	  for _, e := range prev {
		   if e.Memo == "fee_credit" && e.AccountType == AccountFee {
			  fee = e.Credit
		   }
		  if e.Memo == "bridge_in" && e.UserID == "system" && e.AccountType == AccountBridge {
			  parked = e.Credit
		  }
	   }
	   if fee.IsNegative() {
		   fee = decimal.Zero
	  }
	  if !fee.IsZero() {
		  parked = tr.Amount.Sub(fee)
	   }
	   if !parked.IsPositive() {
		   parked = tr.Amount
		   fee = decimal.Zero
	   }
	   restore := parked.Add(fee)
	  lines := []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: parked, Memo: "bridge_out"},
	   }
	  n := seq + 1
	  if !fee.IsZero() {
		   lines = append(lines, JournalEntry{TransferID: tr.ID, Seq: n, UserID: "system", AccountType: AccountFee, Asset: tr.Asset, Debit: fee, Memo: "fee_debit"})
		  n++
	   }
	   return append(lines, JournalEntry{TransferID: tr.ID, Seq: n, UserID: tr.UserID, AccountType: AccountWallet, Asset: tr.Asset, Credit: restore, Memo: "reorg_restore_frozen"})
  }

  func reorgUnwindLines(tr *Transfer, prev []JournalEntry, seq int) []JournalEntry {
	   if tr.Direction == WalletDeposit {
		  return depositUnwindLines(tr, seq)
	  }
	   if tr.Direction == WalletWithdraw {
		  return withdrawUnwindLines(tr, prev, seq)
	   }
	   if tr.Direction == FeeDebit {
		   return feeUnwindLines(tr, prev, seq)
	  }
	  return nil
   }

// feeUnwindLines 已入账 FeeDebit 反向——system.fee 扣回、资金账恢复可用。
// 供运营冲正提前赎回罚金等；与出金 fee 切片共用 fee_debit memo。
// 仅 credited 且净侧仍为 credit 时可调；不得经 bridge。
   func feeUnwindLines(tr *Transfer, prev []JournalEntry, seq int) []JournalEntry {
	   if seq <= 0 {
		   seq = 1
	  }
	  amt := tr.Amount
	  for _, e := range prev {
		  if e.Memo == "fee_credit" && e.AccountType == AccountFee && e.UserID == "system" {
			   amt = e.Credit
			   break
		  }
	  }
	   if !amt.IsPositive() {
		   return nil
	   }
	  return []JournalEntry{
		  {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountFee, Asset: tr.Asset, Debit: amt, Memo: "fee_debit"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Credit: amt, Memo: "restore_available"},
	   }
  }

   func settleLines(tr *Transfer, seq int) []JournalEntry {
	  return settleLinesFee(tr, seq, decimal.Zero)
  }

// settleLinesFee 确认后入账分录：出金扣冻结、入金入钱包、内部划转入目标账。

  func settleLinesFee(tr *Transfer, seq int, fee decimal.Decimal) []JournalEntry {
	  switch tr.Direction {
	   case WalletWithdraw:
		   return withdrawSettleLines(tr, seq, fee)
	   case WalletDeposit:
		  return depositCreditLines(tr, seq)
	   case SpotSend:
		  return sendCreditLines(tr, seq)
	   case FiatOnramp, ConvertCredit, P2PCredit, SubCredit, EarnInterest:
		   return fiatCreditLines(tr, seq)
	  case FiatOfframp, ConvertDebit, P2PDebit, SubDebit:
		  return fiatDebitLines(tr, seq)
	   case FeeDebit:
		   return feeDebitLines(tr, seq)
	   case HotColdSweep:
		   return hotColdLines(tr, seq)
	  case HotWarmSweep:
		  return hotWarmLines(tr, seq)
	  case WarmColdSweep:
		  return warmColdLines(tr, seq)
	  case EarnRedeem:
		   return earnRedeemLines(tr, seq, fee)
	   default:
		   return creditLines(tr,
			   sourceBook(tr.Direction), destBook(tr.Direction), seq)
	   }
  }

// fiatCreditLines mints no units: desk inventory moves onto the funding book.
  func fiatCreditLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		   seq = 1
	  }
	   amt := tr.Amount
	   return []JournalEntry{
		  {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: amt, Memo: "bridge_out"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Credit: amt, Memo: "credit_available"},
	   }
  }

// fiatDebitLines parks a funding freeze onto system.bridge after rail payout.
   func fiatDebitLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		   seq = 1
	   }
	  return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Debit: tr.Amount, Memo: "consume_frozen"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: tr.Amount, Memo: "bridge_in"},
	  }
  }

// feeDebitLines 资金账冻结消耗贷记 system.fee（出金/理财罚金等同科目）。
// 与 ConvertDebit/fiatDebit 的 bridge 停泊区分；Conserve 两侧金额相等。
// 不得再经 bridge，避免罚金混入热账库存观察。
   func feeDebitLines(tr *Transfer, seq int) []JournalEntry {
	  if seq <= 0 {
		   seq = 1
	   }
	  return []JournalEntry{
		  {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Debit: tr.Amount, Memo: "consume_frozen"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountFee, Asset: tr.Asset, Credit: tr.Amount, Memo: "fee_credit"},
	   }
   }

// hotColdLines parks desk excess off the hot bridge onto system.cold.
   func hotColdLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		  seq = 1
	  }
	  return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: tr.Amount, Memo: "bridge_out"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountCold, Asset: tr.Asset, Credit: tr.Amount, Memo: "credit_available"},
	   }
   }

   func hotWarmLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		  seq = 1
	  }
	  return []JournalEntry{
		  {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: tr.Amount, Memo: "bridge_out"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountWarm, Asset: tr.Asset, Credit: tr.Amount, Memo: "credit_available"},
	  }
   }

  func warmColdLines(tr *Transfer, seq int) []JournalEntry {
	   if seq <= 0 {
		  seq = 1
	   }
	  return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: "system", AccountType: AccountWarm, Asset: tr.Asset, Debit: tr.Amount, Memo: "bridge_out"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountCold, Asset: tr.Asset, Credit: tr.Amount, Memo: "credit_available"},
	   }
   }

// earnRedeemLines hops earn freeze back to funding. fee here is interest
// paid from system.bridge (not a user fee); 0 skips the extra pair.

   func earnRedeemLines(tr *Transfer, seq int, interest decimal.Decimal) []JournalEntry {
	   lines := creditLines(tr,
		  AccountEarn, AccountSpot, seq)
	  if interest.IsNegative() || interest.IsZero() {
		  return lines
	   }
	  n := NextJournalSeq(lines)
	  return append(lines,
		  JournalEntry{TransferID: tr.ID, Seq: n, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: interest, Memo: "bridge_out"},
		  JournalEntry{TransferID: tr.ID, Seq: n + 1, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Credit: interest, Memo: "credit_available"},
	  )
  }

// sendCreditLines hops the freeze through system.bridge onto the counterparty
// funding book. Dest user is ToAddress (plaintext user_id, not a chain dest).

   func sendCreditLines(tr *Transfer, seq int) []JournalEntry {
	  if seq <= 0 {
		   seq = 1
	   }
	   dest := tr.Counterparty()
	   return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Debit: tr.Amount, Memo: "consume_frozen"},
		   {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: tr.Amount, Memo: "bridge_in"},
		  {TransferID: tr.ID, Seq: seq + 2, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Debit: tr.Amount, Memo: "bridge_out"},
		  {TransferID: tr.ID, Seq: seq + 3, UserID: dest, AccountType: AccountSpot, Asset: tr.Asset, Credit: tr.Amount, Memo: "credit_available"},
	  }
   }

  func (t *Transfer) Counterparty() string {
	   if t == nil {
		  return ""
	   }
	   if t.ToAddress != "" {
		   return t.ToAddress
	  }
	  return t.ToAddressHint
  }

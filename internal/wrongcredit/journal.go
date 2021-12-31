// @FileName: journal.go
// @Summary: 错贷记冲销分录构造——将原始贷记分录反转为冲销，支持全额和部分追回。
// - 仅负责构造 []JournalEntry；不操作账本状态机、不写 DB、不修改 Transfer。
// - 冲销方向：反转原始入账（credit_available → debit_available），经
// system.bad_debt 账户停泊剩余无法追回金额。
// - Partial 场景：追回金额 = min(available, creditedAmt)；余额不足部分
// 走 bad_debt 分录（debit system.bad_debt / credit 用户冻结余额）。
// - 所有输出分录须满足 ledger.ValidateJournal（借贷按资产轧平）。
// 不变量：
// - clawAmt + remainAmt = creditedAmt（全量守恒）。
// - clawAmt > 0；remainAmt ≥ 0。
// - 分录序号从 seq 开始单调递增。
// @Author: Kinsley
// @Created: 2021.12.29

package wrongcredit

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// ─── 冲销分录常量 ─────────────────────────────────────────────────────────────

const (
	// memoReversalDebitAvailable 从用户可用余额扣回（冲销贷记）。
	memoReversalDebitAvailable = "wrongcredit_reversal_debit_available"
	// memoReversalCreditBridge 回流至 system.bridge（与原始 deposit 路径对称）。
	memoReversalCreditBridge = "wrongcredit_reversal_credit_bridge"
	// memoPartialDebitAvailable 部分追回：扣用户可用。
	memoPartialDebitAvailable = "wrongcredit_partial_debit_available"
	// memoPartialCreditBridge 部分追回：流入 bridge。
	memoPartialCreditBridge = "wrongcredit_partial_credit_bridge"
	// memoBadDebtDebitBridge 坏账：从 bridge 转出（bridge 已承接部分追回）。
	memoBadDebtDebitBridge = "wrongcredit_baddebt_debit_bridge"
	// memoBadDebtCreditBadDebt 坏账：贷记 system.bad_debt（资金挂账）。
	memoBadDebtCreditBadDebt = "wrongcredit_baddebt_credit_baddebt"
	// memoFreezeForReversal 冻结用户可用余额以便冲销。
	memoFreezeForReversal = "wrongcredit_freeze_available"
	// memoFrozenForReversal 冻结后贷记冻结余额。
	memoFrozenForReversal = "wrongcredit_freeze_frozen"
	// memoUnfreezeAfterWaive 豁免后解冻：归还冻结余额至可用。
	memoUnfreezeAfterWaive = "wrongcredit_unfreeze_after_waive"
	// memoUnfreezeAvailable 豁免后解冻贷记可用。
	memoUnfreezeAvailableAfterWaive = "wrongcredit_unfreeze_available_after_waive"
)

// ─── 账户常量（system 账户） ──────────────────────────────────────────────────

const (
	// systemBadDebt 坏账挂账系统账户用户 ID（对应 AccountBadDebt 科目）。
	systemBadDebt = "system"
)

// AccountBadDebt 坏账科目类型（扩展 ledger.AccountType）；
// 本包内使用，不写入 ledger.types 以保持隔离。
const AccountBadDebt ledger.AccountType = "bad_debt"

// ─── 冻结分录 ─────────────────────────────────────────────────────────────────
// FreezeLines 构造"冻结用户可用余额"分录：available → frozen。
// transferID 为冲销专属 TransferID（由编排器生成，与原始入账 ID 不同）。
// 不变量：输出两行，借可用/贷冻结，金额相等；满足 ValidateJournal。
   func FreezeLines(transferID, userID, asset string, amt decimal.Decimal, seq int) []ledger.JournalEntry {
	  if seq <= 0 {
		  seq = 1
	   }
	  return []ledger.JournalEntry{
		  {
			   TransferID: transferID,
			  Seq:        seq,
			  UserID:     userID,
			  AccountType: ledger.AccountSpot,
			  Asset: asset,
			  Debit: amt,
			  Memo:  memoFreezeForReversal,
		  },
		  {
			  TransferID: transferID,
			  Seq:         seq + 1,
			   UserID:      userID,
			   AccountType: ledger.AccountSpot,
			  Asset: asset,
			  Credit: amt,
			  Memo: memoFrozenForReversal,
		  },
	  }
   }

// UnfreezeLines 豁免时解冻：归还冻结余额至可用（FreezeLines 的逆向）。
  func UnfreezeLines(transferID, userID, asset string, amt decimal.Decimal, seq int) []ledger.JournalEntry {
	  if seq <= 0 {
		   seq = 1
	   }
	   return []ledger.JournalEntry{
		   {
			  TransferID: transferID,
			   Seq:        seq,
			  UserID:     userID,
			  AccountType: ledger.AccountSpot,
			   Asset:       asset,
			   Debit: amt,
			  Memo:  memoUnfreezeAfterWaive,
		  },
		   {
			  TransferID: transferID,
			   Seq:    seq + 1,
			   UserID: userID,
			  AccountType: ledger.AccountSpot,
			  Asset:  asset,
			  Credit: amt,
			   Memo: memoUnfreezeAvailableAfterWaive,
		  },
	  }
   }

// ─── 全额冲销分录 ─────────────────────────────────────────────────────────────
// FullReversalLines 构造全额追回冲销分录。
// 路径：用户 spot.frozen -[consume]→ system.bridge.available（原始入账反转）。
// 前提：用户 spot.frozen ≥ clawAmt（由编排器在调用前校验）。
// 不变量：
// - 输出两行，借冻结/贷 bridge，金额相等。
// - 与 depositCreditLines 反向对称（bridge_out → credit 的逆）。
   func FullReversalLines(transferID, userID, asset string, clawAmt decimal.Decimal, seq int) []ledger.JournalEntry {
	   if seq <= 0 {
		  seq = 1
	   }
	   return []ledger.JournalEntry{
		   {
			  TransferID: transferID,
			  Seq:         seq,
			   UserID:      userID,
			   AccountType: ledger.AccountSpot,
			  Asset:       asset,
			  Debit:       clawAmt,
			   Memo:        memoReversalDebitAvailable,
		  },
		   {
			  TransferID: transferID,
			   Seq:        seq + 1,
			  UserID:     userID,
			   AccountType: ledger.AccountBridge,
			   Asset:  asset,
			   Credit: clawAmt,
			  Memo: memoReversalCreditBridge,
		   },
	   }
  }

// ─── 部分追回 + 坏账分录 ──────────────────────────────────────────────────────
// PartialClawLines 构造部分追回分录（可用余额不足时）。
// clawAmt = 实际可追回金额（≤ creditedAmt）。
// remainAmt = creditedAmt - clawAmt（剩余坏账）。
// 路径：
// 1) 部分追回：用户 spot.frozen -[consume]→ system.bridge
// 2) 坏账挂账：system.bridge -[park]→ system.bad_debt
// 不变量：
// - clawAmt + remainAmt = creditedAmt。
// - 输出 4 行（2 追回 + 2 坏账）；按资产借贷各轧平。
  func PartialClawLines(
	   transferID, userID, asset string,
	  clawAmt,
	  remainAmt decimal.Decimal,
	  seq int,
  ) ([]ledger.JournalEntry, error) {
	  if seq <= 0 {
		   seq = 1
	  }
	   if clawAmt.IsNegative() || remainAmt.IsNegative() {
		  return nil, fmt.Errorf("%w: claw=%s remain=%s both must be ≥ 0", ErrAmount, clawAmt, remainAmt)
	   }
	   if clawAmt.IsZero() && remainAmt.IsZero() {
		   return nil, fmt.Errorf("%w: both claw and remain are zero", ErrAmount)
	  }
	  var lines []ledger.JournalEntry
	   if clawAmt.IsPositive() {
		   lines = append(lines,
			   ledger.JournalEntry{
				  TransferID: transferID,
				   Seq:         seq,
				   UserID:      userID,
				   AccountType: ledger.AccountSpot,
				  Asset:       asset,
				  Debit: clawAmt,
				   Memo:  memoPartialDebitAvailable,
			   },
			  ledger.JournalEntry{
				  TransferID: transferID,
				   Seq:         seq + 1,
				  UserID:      systemBadDebt,
				  AccountType: ledger.AccountBridge,
				  Asset:       asset,
				  Credit: clawAmt,
				  Memo: memoPartialCreditBridge,
			   },
		  )
		  seq += 2
	  }
	  if remainAmt.IsPositive() {
		   lines = append(lines,
			   ledger.JournalEntry{
				   TransferID: transferID,
				   Seq: seq,
				  UserID:      systemBadDebt,
				   AccountType: ledger.AccountBridge,
				   Asset:       asset,
				   Debit: remainAmt,
				   Memo: memoBadDebtDebitBridge,
			   },
			  ledger.JournalEntry{
				   TransferID: transferID,
				  Seq:        seq + 1,
				  UserID: systemBadDebt,
				   AccountType: AccountBadDebt,
				  Asset:       asset,
				  Credit: remainAmt,
				   Memo: memoBadDebtCreditBadDebt,
			  },
		   )
	  }
	   return lines, nil
   }

// ─── 全坏账分录（无可用余额） ─────────────────────────────────────────────────
// FullBadDebtLines 当用户完全无可用/冻结余额时，将整笔错贷记金额全部挂坏账。
// 路径：system.bridge -[park]→ system.bad_debt（系统内部账目）。
// 不变量：
// - 输出两行，按 asset 借贷相等。
// - 调用方需保证 system.bridge 已有足够库存（原始入账时 bridge 已被 debit）。
   func FullBadDebtLines(transferID, asset string, amt decimal.Decimal, seq int) []ledger.JournalEntry {
	  if seq <= 0 {
		  seq = 1
	  }
	  return []ledger.JournalEntry{
		   {
			  TransferID:  transferID,
			   Seq:         seq,
			  UserID:      systemBadDebt,
			  AccountType: ledger.AccountBridge,
			  Asset:       asset,
			  Debit:       amt,
			   Memo: memoBadDebtDebitBridge,
		   },
		  {
			   TransferID: transferID,
			  Seq:        seq + 1,
			  UserID:     systemBadDebt,
			   AccountType: AccountBadDebt,
			  Asset: asset,
			   Credit: amt,
			  Memo:   memoBadDebtCreditBadDebt,
		  },
	   }
  }

// ─── 分录工具函数 ─────────────────────────────────────────────────────────────
// nextSeq 返回当前 entries 之后的下一个序号。
  func nextSeq(entries []ledger.JournalEntry) int {
	  if len(entries) == 0 {
		   return 1
	  }
	  max := 0
	  for _, e := range entries {
		  if e.Seq > max {
			  max = e.Seq
		  }
	   }
	   return max + 1
  }

// validateLines 校验分录集合借贷各资产轧平；复用 ledger.ValidateJournal。

   func validateLines(entries []ledger.JournalEntry) error {
	   if err := ledger.ValidateJournal(entries); err != nil {
		   return fmt.Errorf("%w: %v", ErrJournal, err)
	   }
	  return nil
   }

// BuildReversalLines 根据可用余额 available 与原始贷记额 credited 自动选择：
// - available ≥ credited → FullReversalLines（全额）
// - 0 < available < credited → PartialClawLines（部分 + 坏账）
// - available = 0 → FullBadDebtLines（全坏账）
// 返回分录列表、实际追回金额、坏账金额。
// 所有分录已通过 validateLines 校验。

  func BuildReversalLines(
	  transferID, userID, asset string,
	   credited, available decimal.Decimal,
	   seq int,
   ) (lines []ledger.JournalEntry, clawAmt, remainAmt decimal.Decimal, err error) {
	   if credited.IsNegative() || credited.IsZero() {
		   return nil, decimal.Zero, decimal.Zero, fmt.Errorf("%w: credited must be > 0", ErrAmount)
	   }
	   if available.IsNegative() {
		  available = decimal.Zero
	   }
	   switch {
	  case available.GreaterThanOrEqual(credited):
		   // 全额追回。
		  clawAmt = credited
		   remainAmt = decimal.Zero
		  lines = FullReversalLines(transferID, userID, asset, clawAmt, seq)
	   case available.IsZero():
		   // 无余额，全部挂坏账。
		  clawAmt = decimal.Zero
		  remainAmt = credited
		   lines = FullBadDebtLines(transferID, asset, remainAmt, seq)
	   default:
		  // 部分追回。
		  clawAmt = available
		  remainAmt = credited.Sub(available)
		  lines, err = PartialClawLines(transferID, userID, asset, clawAmt, remainAmt, seq)
		   if err != nil {
			   return nil,
				   decimal.Zero, decimal.Zero, err
		  }
	  }
	   if verr := validateLines(lines); verr != nil {
		   return nil, decimal.Zero, decimal.Zero, verr
	  }
	   return lines, clawAmt, remainAmt, nil
  }

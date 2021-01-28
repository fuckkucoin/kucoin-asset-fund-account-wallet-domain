package reorgacct

import (
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// ────────────────────────────────────────────────────────────────────────────
// 分录科目常量（与 book-subject.md 静态科目表对应，只读引用）
// ────────────────────────────────────────────────────────────────────────────

const (
	// SubjectUserAvail 用户可用余额科目（资产方）。
	SubjectUserAvail = "user.avail"
	// SubjectReorgReverse 重组冲正中间科目（负债方）。
	SubjectReorgReverse = "reorg.reverse"
	// SubjectOccupancyHold 风险占用科目（备抵方）。
	SubjectOccupancyHold = "deposit.occupancy"
	// SubjectOccupancyRelease 占用释放确认科目。
	SubjectOccupancyRelease = "deposit.occupancy.release"
	// SubjectFreezeHold 出金冻结科目（备抵方）。
	SubjectFreezeHold = "withdraw.freeze"
	// SubjectFreezeRelease 冻结释放科目。
	SubjectFreezeRelease = "withdraw.freeze.release"
	// SubjectHopTransit 划转在途科目。
	SubjectHopTransit = "transfer.hop.transit"
	// SubjectHopClose hop 收口科目。
	SubjectHopClose = "transfer.hop.close"
)

// ────────────────────────────────────────────────────────────────────────────
// JournalEntry 单条分录
// ────────────────────────────────────────────────────────────────────────────
// EntryDirection 借贷方向。
type EntryDirection string

const (
	// EntryDebit 借方（资产增加或负债减少）。
	EntryDebit EntryDirection = "debit"
	// EntryCredit 贷方（资产减少或负债增加）。
	EntryCredit EntryDirection = "credit"
)

// JournalEntry 一条冲正分录。
type JournalEntry struct {
	// IdemKey 幂等键（batch_id + "|" + impact_row_id + "|" + kind + "|" + direction）。
	IdemKey string `json:"idem_key"`
	// BatchID 关联批次 ID。
	BatchID string `json:"batch_id"`
	// ImpactRowID 关联影响面行 ID。
	ImpactRowID string `json:"impact_row_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Subject 科目代码。
	Subject string `json:"subject"`
	// Direction 借/贷方向。
	Direction EntryDirection `json:"direction"`
	// Amount 金额（正数）。
	Amount string `json:"amount"`
	// Kind ImpactKind（分录来源类型）。
	Kind string `json:"kind"`
	// Note 分录说明。
	Note string `json:"note"`
	// At 分录产生时间（UTC）。
	At time.Time `json:"at"`
}

// ────────────────────────────────────────────────────────────────────────────
// JournalPair 借贷对
// ────────────────────────────────────────────────────────────────────────────
// JournalPair 一对守恒的借贷分录。

type JournalPair struct {
	Debit JournalEntry `json:"debit"`
	Credit JournalEntry `json:"credit"`
}

// ────────────────────────────────────────────────────────────────────────────
// ReversalJournal 冲正分录生成器
// ────────────────────────────────────────────────────────────────────────────
// ReversalJournal 对 ImpactRow 列表产出守恒的借贷分录对。
type ReversalJournal struct {
	batchID string
}

// NewReversalJournal 构造 ReversalJournal；batchID 不得为空。
  func NewReversalJournal(batchID string) *ReversalJournal {
	  return &ReversalJournal{batchID: batchID}
  }

// Build 对 rows 产出 JournalPair 列表；KindLiveHash 行跳过（不产生分录）。
  func (rj *ReversalJournal) Build(rows []ImpactRow) ([]JournalPair, error) {
	  var pairs []JournalPair
	  now := time.Now().UTC()
	   for _, row := range rows {
		   if row.Kind == KindLiveHash {
			  continue
		   }
		   pair,
			  err := rj.buildPair(row, now)
		  if err != nil {
			  return nil, fmt.Errorf("ReversalJournal Build row %s kind %s: %w", row.ID, row.Kind, err)
		   }
		  pairs = append(pairs, pair)
	  }
	  return pairs, nil
  }

// buildPair 按 Kind 生成对应的借贷分录对。
  func (rj *ReversalJournal) buildPair(row ImpactRow, now time.Time) (JournalPair, error) {
	   amt := normalizeAmount(row.AffectedAmount)
	   if amt == "0" || amt == "" {
		   return JournalPair{}, fmt.Errorf("zero amount for row %s", row.ID)
	  }
	  var debitSubj, creditSubj string
	  var note string
	  switch row.Kind {
	   case KindCredited:
		   // 提前入账冲正：借 reorg.reverse，贷 user.avail（可用余额减少）。
		   debitSubj = SubjectReorgReverse
		  creditSubj = SubjectUserAvail
		  note = fmt.Sprintf("credited reversal for transfer %s user %s asset %s amount %s",
			  row.ReferenceID,
			  row.UserID, row.Asset, amt)
	  case KindOccupied:
		  // 占用释放：借 deposit.occupancy.release，贷 deposit.occupancy（占用归零）。
		  debitSubj = SubjectOccupancyRelease
		   creditSubj = SubjectOccupancyHold
		   note = fmt.Sprintf("occupancy release for occupancy %s user %s asset %s amount %s",
			   row.ReferenceID,
			   row.UserID, row.Asset, amt)
	  case KindFreeze:
		  // 冻结回滚：借 withdraw.freeze.release，贷 withdraw.freeze（冻结归零）。
		   debitSubj = SubjectFreezeRelease
		   creditSubj = SubjectFreezeHold
		   note = fmt.Sprintf("freeze rollback for freeze %s user %s asset %s amount %s",
			  row.ReferenceID,
			  row.UserID, row.Asset, amt)
	   case KindHopPartial:
		   // hop 收口：借 transfer.hop.close，贷 transfer.hop.transit（在途清零）。
		  debitSubj = SubjectHopClose
		   creditSubj = SubjectHopTransit
		  note = fmt.Sprintf("hop close for hop %s user %s asset %s amount %s",
			   row.ReferenceID, row.UserID, row.Asset, amt)
	   default:
		   return JournalPair{}, fmt.Errorf("unsupported kind %s for row %s", row.Kind, row.ID)
	   }
	   base := row.ID + "|" + row.Kind
	   debit := JournalEntry{
		  IdemKey: rj.batchID + "|" + base + "|debit",
		   BatchID: rj.batchID,
		  ImpactRowID: row.ID,
		  UserID: row.UserID,
		  Asset:     row.Asset,
		  Subject:   debitSubj,
		  Direction: EntryDebit,
		  Amount:    amt,
		   Kind: row.Kind,
		  Note: note,
		  At:   now,
	   }
	  credit := JournalEntry{
		   IdemKey: rj.batchID + "|" + base + "|credit",
		   BatchID:     rj.batchID,
		   ImpactRowID: row.ID,
		  UserID: row.UserID,
		   Asset:     row.Asset,
		  Subject:   creditSubj,
		   Direction: EntryCredit,
		  Amount: amt,
		   Kind: row.Kind,
		  Note: note,
		   At: now,
	  }
	  return JournalPair{Debit: debit, Credit: credit}, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// VerifyBalance 守恒校验
// ────────────────────────────────────────────────────────────────────────────
// BalanceResult 守恒校验结果。

type BalanceResult struct {
	// Balanced 是否守恒（∑借方 == ∑贷方）。
	Balanced bool `json:"balanced"`
	// TotalDebit 借方合计（按用户×资产聚合后再求和）。
	TotalDebit string `json:"total_debit"`
	// TotalCredit 贷方合计。
	TotalCredit string `json:"total_credit"`
	// Delta 差值（理论上为 0，若非零则存在守恒漏洞）。
	Delta string `json:"delta"`
	// ByAsset 按资产维度的守恒情况。
	ByAsset map[string]AssetBalance `json:"by_asset,omitempty"`
}

// AssetBalance 单资产的借贷合计。

type AssetBalance struct {
	Asset  string `json:"asset"`
	Debit  string `json:"debit"`
	Credit string `json:"credit"`
	Balanced bool `json:"balanced"`
}

// VerifyBalance 对 JournalPair 列表校验借贷守恒。
// 返回 BalanceResult；调用方应在 Balanced=false 时拒绝提交分录并告警。

  func VerifyBalance(pairs []JournalPair) BalanceResult {
	  totalDebit := decimal.Zero
	  totalCredit := decimal.Zero
	   byAsset := map[string]*struct{ debit, credit decimal.Decimal }{}
	  for _, p := range pairs {
		  dAmt, err := decimal.NewFromString(p.Debit.Amount)
		   if err != nil || dAmt.IsNegative() {
			  continue
		   }
		  cAmt, err := decimal.NewFromString(p.Credit.Amount)
		   if err != nil || cAmt.IsNegative() {
			  continue
		   }
		  totalDebit = totalDebit.Add(dAmt)
		   totalCredit = totalCredit.Add(cAmt)
		   asset := strings.ToUpper(p.Debit.Asset)
		  if byAsset[asset] == nil {
			   byAsset[asset] = &struct{ debit, credit decimal.Decimal }{}
		   }
		  byAsset[asset].debit = byAsset[asset].debit.Add(dAmt)
		  byAsset[asset].credit = byAsset[asset].credit.Add(cAmt)
	  }
	   delta := totalDebit.Sub(totalCredit)
	   balanced := delta.IsZero()
	   res := BalanceResult{
		   Balanced: balanced,
		   TotalDebit:  totalDebit.String(),
		  TotalCredit: totalCredit.String(),
		   Delta: delta.String(),
		   ByAsset: map[string]AssetBalance{},
	  }
	  for asset, v := range byAsset {
		  d := v.debit.Sub(v.credit)
		  res.ByAsset[asset] = AssetBalance{
			  Asset: asset,
			   Debit:  v.debit.String(),
			  Credit: v.credit.String(),
			  Balanced: d.IsZero(),
		   }
	   }
	  return res
   }

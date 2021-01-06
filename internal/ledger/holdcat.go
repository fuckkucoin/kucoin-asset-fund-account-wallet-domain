package ledger

import (
	"context"
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
)

// ParkedHold 已过账到 bridge 的占用标签（限价闪兑 / P2P），不是用户科目 Frozen。
type ParkedHold struct {
	Asset string
	Book  string
	Reason string
	Ref    string
	Amount string
}

   func parkDirection(reason string) Direction {
	   switch strings.ToLower(strings.TrimSpace(reason)) {
	   case "convert_limit":
		  return ConvertDebit
	  case "p2p_sell", "p2p_merchant_margin":
		  return P2PDebit
	   case "red_envelope", "lending_order":
		  return P2PDebit
	   default:
		   return ""
	   }
   }

type holdcatOrphan struct {
	Index int
	Need decimal.Decimal
	Have       decimal.Decimal
	InvalidAmt bool
}

   func holdcatOrphans(ctx context.Context, st Store, userID string, holds []ParkedHold) ([]holdcatOrphan, error) {
	   if st == nil || strings.TrimSpace(userID) == "" || len(holds) == 0 {
		  return nil, nil
	   }
	   trs,
		  err := st.ListUserTransfers(ctx, userID, 0)
	   if err != nil {
		  return nil, err
	  }
	  parked := map[string]decimal.Decimal{}
	   for _, tr := range trs {
		  if tr == nil || tr.State != StateCredited {
			  continue
		   }
		   if tr.Direction != ConvertDebit && tr.Direction != P2PDebit {
			   continue
		  }
		  k := string(tr.Direction) + "|" + strings.ToUpper(tr.Asset)
		  parked[k] = parked[k].Add(tr.Amount)
	  }
	  var out []holdcatOrphan
	  for i, h := range holds {
		  dir := parkDirection(h.Reason)
		  if dir == "" {
			   continue
		   }
		  amt, err := decimal.NewFromString(strings.TrimSpace(h.Amount))
		  if err != nil || !amt.IsPositive() {
			  out = append(out, holdcatOrphan{Index: i, InvalidAmt: true})
			   continue
		  }
		   k := string(dir) + "|" + strings.ToUpper(strings.TrimSpace(h.Asset))
		  left := parked[k]
		   if left.LessThan(amt) {
			   out = append(out, holdcatOrphan{Index: i, Need: amt, Have: left})
			   continue
		  }
		   parked[k] = left.Sub(amt)
	  }
	   return out, nil
   }

// HoldcatOrphanIndexes 与 HoldcatMismatches 同一套贪心 park 核销：返回缺少已入账借记的 hold 下标。
// 日记不可用时返回 error；金额非法的标签也视为孤儿下标（仅释标签，禁止补 Credit）。
  func HoldcatOrphanIndexes(ctx context.Context, st Store, userID string, holds []ParkedHold) ([]int, error) {
	  hits, err := holdcatOrphans(ctx, st, userID, holds)
	  if err != nil {
		   return nil, err
	  }
	   idxs := make([]int, 0, len(hits))
	   for _, h := range hits {
		  idxs = append(idxs, h.Index)
	  }
	  return idxs, nil
  }

// HoldcatMismatches 核已停 bridge 的占用：open hold 必须有对应已入账借记。
// 不得把这类占用加进 extraFrozen，否则 Frozen=0 会被误判为在途失踪。

  func HoldcatMismatches(ctx context.Context, st Store, userID string, holds []ParkedHold) []Mismatch {
	  hits,
		   err := holdcatOrphans(ctx, st, userID, holds)
	  if err != nil {
		   return []Mismatch{{
			   UserID: userID, Note: "holdcat park journal unavailable: " + err.Error(),
		  }}
	  }
	   var out []Mismatch
	   for _, hit := range hits {
		  if hit.Index < 0 || hit.Index >= len(holds) {
			   continue
		  }
		  h := holds[hit.Index]
		   if hit.InvalidAmt {
			   out = append(out, Mismatch{
				  UserID: userID, Asset: h.Asset, Book: h.Book,
				  Note: "holdcat occupancy amount invalid",
			  })
			   continue
		   }
		  out = append(out, Mismatch{
			   UserID: userID, Asset: h.Asset, Book: h.Book, TransferID: h.Ref,
			   Note: fmt.Sprintf("holdcat occupancy without park journal reason=%s need=%s have=%s", h.Reason, hit.Need, hit.Have),
		   })
	  }
	   return out
  }

   func parksOnBridge(d Direction) bool {
	   switch d {
	  case FiatOfframp, ConvertDebit, P2PDebit, SubDebit:
		  return true
	  default:
		  return false
	   }
   }

// ParkJournalMismatches 核无 holdcat 的 park（法币出金、母子划出等）：已入账必须有 bridge_in。
  func ParkJournalMismatches(ctx context.Context, st Store, userID string) []Mismatch {
	   if st == nil || strings.TrimSpace(userID) == "" {
		   return nil
	  }
	   trs,
		  err := st.ListUserTransfers(ctx, userID, 0)
	  if err != nil {
		  return []Mismatch{{UserID: userID, Note: "park journal list unavailable: " + err.Error()}}
	  }
	   var out []Mismatch
	   for _, tr := range trs {
		  if tr == nil || tr.State != StateCredited || !parksOnBridge(tr.Direction) {
			   continue
		   }
		  j, err := st.ListJournal(ctx, tr.ID)
		  if err != nil {
			  out = append(out, Mismatch{UserID: userID, TransferID: tr.ID, Asset: tr.Asset, Note: "park journal read failed"})
			  continue
		   }
		   hasBridge := false
		   for _, e := range j {
			  if e.Memo == "bridge_in" && e.UserID == "system" {
				   hasBridge = true
				   break
			   }
		  }
		  if !hasBridge {
			  out = append(out, Mismatch{
				   UserID: userID, TransferID: tr.ID, Asset: tr.Asset, State: string(tr.State),
				  Note: fmt.Sprintf("credited park missing bridge_in dir=%s", tr.Direction),
			   })
		  }
	   }
	   return out
  }

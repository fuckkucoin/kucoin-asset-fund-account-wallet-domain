package p2pgate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
)

  func viewHold(row holdcat.Row, idx transferIndex) HoldView {
	  kind := KindOfReason(row.Reason)
	  debitKey := DebitKeyFor(row.Reason, row.Ref)
	   relKey := ReleaseKeyFor(row.ID)
	  debit := idx.lookup(debitKey)
	  rel := idx.lookup(relKey)
	   v := HoldView{
		  ID:     row.ID,
		  UserID: row.UserID,
		   Asset: strings.ToUpper(strings.TrimSpace(row.Asset)),
		  Book: row.Book,
		  Amount: row.Amount,
		  Reason: row.Reason,
		  Kind:   kind,
		   Ref: row.Ref,
		  Open: row.Open,
		   Status: statusOfHold(row.Open),
		   Parked:      holdcat.ParkedOnBridge(row.Reason),
		  Subject:     subjectOf(row.Reason),
		   CreatedAt:   row.CreatedAt,
		  CreatedAtMs: CreatedAtMs(row.CreatedAt),
		   DebitKey:    debitKey,
		   ReleaseKey: relKey,
	   }
	  if debit != nil {
		  v.DebitID = debit.ID
		  v.DebitState = string(debit.State)
		  v.Matched = debit.State == ledger.StateCredited
	   }
	   if rel != nil {
		   v.ReleaseID = rel.ID
	   }
	   return v
   }

  func occupancyRows(rows []holdcat.Row) []holdcat.Row {
	  out := make([]holdcat.Row, 0, len(rows))
	   for _, r := range rows {
		  if IsOccupancyReason(r.Reason) {
			  out = append(out, r)
		  }
	  }
	  return out
  }

   func viewsOf(rows []holdcat.Row, idx transferIndex) []HoldView {
	  out := make([]HoldView, 0, len(rows))
	   for _, r := range occupancyRows(rows) {
		  out = append(out, viewHold(r, idx))
	   }
	   return out
   }

  func filterHolds(views []HoldView, q OccupancyQuery) []HoldView {
	  out := make([]HoldView, 0, len(views))
	   for _, v := range views {
		  if q.MatchHold(v) {
			  out = append(out, v)
		   }
	   }
	   return out
  }

  func filterFlows(flows []FlowEntry, q OccupancyQuery) []FlowEntry {
	  out := make([]FlowEntry, 0, len(flows))
	  for _, e := range flows {
		   if q.MatchFlow(e) {
			   out = append(out, e)
		  }
	  }
	   return out
  }

  func holdByID(views []HoldView, id string) (HoldView, bool) {
	   id = strings.TrimSpace(id)
	  if id == "" {
		   return HoldView{}, false
	   }
	  for _, v := range views {
		   if v.ID == id {
			   return v, true
		   }
	  }
	   return HoldView{}, false
  }

   func holdsByRef(views []HoldView, ref string) []HoldView {
	   ref = strings.TrimSpace(ref)
	   if ref == "" {
		  return nil
	  }
	   out := make([]HoldView, 0)
	   for _, v := range views {
		   if v.Ref == ref {
			   out = append(out, v)
		   }
	   }
	  return out
   }

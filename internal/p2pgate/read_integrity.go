package p2pgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func composeIntegrity(userID string, views []HoldView, idx transferIndex) IntegrityReport {
	  rep := IntegrityReport{
		  UserID:      userID,
		  OrphanHolds: []HoldView{},
		  UnmatchedParks: []FlowEntry{},
		   AmountDrift: []DriftRow{},
		  ParkCoverageOK: true,
		  CheckedHolds: len(views),
		  CheckedFlows: len(idx.flows),
	  }
	   seenDebit := map[string]bool{}
	   for _, v := range views {
		   if !v.Open {
			  continue
		  }
		  tr := idx.lookup(v.DebitKey)
		  if tr == nil || tr.State != ledger.StateCredited {
			   rep.OrphanHolds = append(rep.OrphanHolds, v)
			  rep.ParkCoverageOK = false
			   continue
		   }
		   seenDebit[tr.ID] = true
		   if !decEqual(v.Amount, tr.Amount.String()) {
			   rep.AmountDrift = append(rep.AmountDrift, DriftRow{
				  HoldID: v.ID,
				   Asset: v.Asset, Reason: v.Reason, Ref: v.Ref,
				   HoldAmt: v.Amount, DebitAmt: tr.Amount.String(),
				  Note: "open hold amount differs from credited debit",
			  })
			   rep.ParkCoverageOK = false
		  }
	  }
	  for _, e := range idx.flows {
		   if e.Adjacent || e.Sign != SignDebit || !flowCredited(e) {
			   continue
		  }
		  if e.Action != ActionSellHold && e.Action != ActionMerchantHold {
			  continue
		   }
		  if seenDebit[e.TransferID] {
			  continue
		   }
		  // 已释放占用仍应对得上标签（open=false），否则记 unmatched。
		  matchedHold := false
		   for _, v := range views {
			  if v.DebitID == e.TransferID || (v.DebitKey != "" && v.DebitKey == e.IdempotencyKey) {
				   matchedHold = true
				  if !decEqual(v.Amount, e.Amount) {
					   rep.AmountDrift = append(rep.AmountDrift, DriftRow{
						  HoldID: v.ID, Asset: v.Asset, Reason: v.Reason, Ref: v.Ref,
						   HoldAmt: v.Amount, DebitAmt: e.Amount,
						   Note: "hold amount differs from debit flow",
					  })
					  rep.ParkCoverageOK = false
				  }
				  break
			   }
		   }
		  if !matchedHold {
			  rep.UnmatchedParks = append(rep.UnmatchedParks, e)
			  rep.ParkCoverageOK = false
		  }
	   }
	  return rep
   }

// Integrity 占用读面对账报告。

  func (f *Funds) Integrity(ctx context.Context, userID string) (IntegrityReport, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return IntegrityReport{}, err
	  }
	  views,
		  idx, err := f.loadHoldViews(ctx, uid)
	   if err != nil {
		  return IntegrityReport{}, err
	  }
	   return composeIntegrity(uid,
		   views, idx), nil
  }

// CoverageOK 是否所有开放占用均匹配已入账借记。

  func (f *Funds) CoverageOK(ctx context.Context, userID string) (bool, error) {
	   rep, err := f.Integrity(ctx, userID)
	  if err != nil {
		   return false, err
	  }
	  return rep.ParkCoverageOK, nil
  }

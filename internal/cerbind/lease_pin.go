package cerbind

import "account-assets.kucoin.net/funding/internal/cerdecidegate"

// PinFromDecision 瞬时决策转钉扎。

   func PinFromDecision(d Decision) cerdecidegate.DecidePin {
	  return cerdecidegate.DecidePin{
		  Required:  d.Required,
		   PolicyID:  d.PolicyID,
		   Threshold: d.Threshold,
		   Shares:    d.Shares,
		   BindMode:  d.BindMode,
		   DestRisk:  d.DestRisk,
		  Reason: d.Reason,
	   }
  }

// LineFromDecision 决策字段转行事实骨架。

  func LineFromDecision(batchID string, d Decision) cerdecidegate.LineFact {
	  return cerdecidegate.LineFact{
		  BatchID: batchID,
		   Asset: d.Asset,
		  Chain:    d.Chain,
		   Amount:   d.Amount,
		  Dest:     d.Dest,
		   DestRisk: d.DestRisk,
		  Side: d.Side,
		   Purpose: d.Purpose,
		  Approved: d.Approved,
	   }
  }

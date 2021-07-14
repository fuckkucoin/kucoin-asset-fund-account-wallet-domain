package haltepochgate

import "strings"

  func ValidateEngage(req EngageRequest) error {
	   if strings.TrimSpace(req.Actor) == "" {
		  return ErrActorRequired
	  }
	  if strings.TrimSpace(req.Reason) == "" {
		  return ErrReasonRequired
	   }
	   sc := strings.ToLower(strings.TrimSpace(req.Scope))
	   if sc != "" && sc != "global" && sc != "chain" && sc != "asset" && sc != "pair" {
		   return ErrScopeInvalid
	  }
	   if sc == "chain" && strings.TrimSpace(req.Chain) == "" {
		   return ErrScopeInvalid
	   }
	  if sc == "asset" && strings.TrimSpace(req.Asset) == "" {
		  return ErrScopeInvalid
	   }
	   return nil
  }

  func ValidateClear(req ClearRequest) error {
	   if req.FromBridge {
		   return ErrBridgeClear
	  }
	  if strings.TrimSpace(req.Actor) == "" {
		  return ErrActorRequired
	  }
	  if strings.TrimSpace(req.Reason) == "" {
		   return ErrReasonRequired
	  }
	   return nil
  }

   func ValidateLane(req LaneAssertRequest) error {
	  if !req.Lane.Valid() {
		   return ErrLaneUnknown
	  }
	   if req.CarriedGen <= 0 {
		   return ErrEpochMissing
	  }
	  return nil
  }

   func NormalizeLane(s string) (FundLane, error) {
	  switch strings.ToLower(strings.TrimSpace(s)) {
	  case "deposit":
		  return LaneDeposit, nil
	  case "withdraw":
		   return LaneWithdraw, nil
	   case "sweep":
		  return LaneSweep, nil
	   default:
		   return "", ErrLaneUnknown
	   }
   }

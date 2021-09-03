package memoeffectgate

import (
	"strings"
	"time"
)

  func ValidateApprove(req ApproveMemoWithEffectReq) error {
	  if strings.TrimSpace(req.TicketID) == "" {
		   return ErrTicketRequired
	   }
	  if normalizeAsset(req.Asset) == "" {
		  return ErrAssetRequired
	   }
	   if req.OutputIndex < 0 {
		  return ErrOutputKeyInvalid
	  }
	  if req.TTL != "" {
		  d, err := time.ParseDuration(req.TTL)
		  if err != nil || d <= 0 {
			   return ErrTTLInvalid
		   }
	   }
	  return nil
  }

  func ValidateDecide(req DecideMemoGuardedReq) error {
	  if strings.TrimSpace(req.TicketID) == "" {
		   return ErrTicketRequired
	  }
	  st := normalizeStatus(req.Status)
	   if st != "rejected" && st != "approved" {
		  return ErrStatusInvalid
	  }
	   if strings.TrimSpace(req.Actor) == "" {
		   return ErrActorRequired
	  }
	  return nil
  }

   func FailClosedApprove(ticketPending bool, hasEffect bool, occupied bool, auto bool) error {
	   if !ticketPending {
		  return ErrTicketNotPending
	  }
	   if occupied && auto {
		  return ErrAutoClaimOccupied
	  }
	  if occupied {
		  return ErrOutputOccupied
	   }
	   if !hasEffect {
		   return ErrEffectRequired
	   }
	  return nil
  }

package cvquotelease

import (
	"strings"
	"time"
)

   func ValidateQuoteReq(req QuoteWithLeaseReq) error {
	   if strings.TrimSpace(req.UserID) == "" {
		   return ErrUserRequired
	  }
	   from := NormalizeAsset(req.From)
	   to := NormalizeAsset(req.To)
	  if from == "" || to == "" {
		  return ErrAssetRequired
	  }
	  if from == to {
		  return ErrSameAsset
	  }
	   if _, err := ParsePositiveDecimal(req.Amount); err != nil {
		  return ErrAmountInvalid
	   }
	   if _, err := NormalizeBook(req.Book); err != nil {
		  return err
	  }
	  if strings.TrimSpace(req.TTL) != "" {
		  d, err := time.ParseDuration(req.TTL)
		  if err != nil || d <= 0 {
			   return ErrTTLInvalid
		   }
	   }
	  return nil
   }

   func ValidateExecuteReq(req ExecuteWithLeaseReq) error {
	   if strings.TrimSpace(req.QuoteID) == "" {
		   return ErrQuoteIDRequired
	   }
	   if strings.TrimSpace(req.LeaseID) == "" {
		  return ErrLeaseRequired
	  }
	  return nil
  }

   func LeaseStillExecutable(l CvQuoteLease, now time.Time) error {
	   if l.ID == "" {
		   return ErrLeaseNotFound
	  }
	   if l.State == LeaseConsumed {
		  return ErrLeaseConsumed
	   }
	   if l.State == LeaseRevoked {
		   return ErrLeaseRevoked
	   }
	   if l.State == LeaseExpired || now.After(l.ExpiresAt) {
		   return ErrLeaseExpired
	  }
	  if l.State != LeaseIssued {
		  return ErrIllegalTransition
	   }
	  if l.Used {
		   return ErrQuoteUsedRevive
	   }
	  return nil
  }

  func AssertNoDoubleFill(a, b ExecuteWithLeaseResp) bool {
	  if a.Filled && b.Filled && !a.Reused && !b.Reused {
		   return a.OrderID == b.OrderID
	  }
	   return true
   }

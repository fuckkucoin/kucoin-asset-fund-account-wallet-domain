package holdopsgate

import (
	"strings"
	"github.com/shopspring/decimal"
)

  func normUser(s string) (string, error) {
	   s = strings.TrimSpace(s)
	   if s == "" || len(s) > 64 {
		   return "", ErrUserInvalid
	  }
	   return s, nil
  }

  func normAsset(s string) (string, error) {
	   s = strings.ToUpper(strings.TrimSpace(s))
	   if s == "" || len(s) > 16 {
		   return "", ErrAssetInvalid
	   }
	  return s, nil
  }

   func normAmount(s string) (string, error) {
	  s = strings.TrimSpace(s)
	  amt, err := decimal.NewFromString(s)
	   if err != nil || !amt.IsPositive() {
		   return "", ErrAmountInvalid
	   }
	  return amt.StringFixed(18), nil
   }

  func normActor(s string) (string, error) {
	   s = strings.TrimSpace(s)
	  if s == "" || len(s) > 64 {
		   return "", ErrActorInvalid
	   }
	   return s, nil
   }

  func normNonce(s string) (string, error) {
	  s = strings.TrimSpace(s)
	   if s == "" || len(s) > 128 {
		   return "", ErrNonceInvalid
	   }
	  return s, nil
   }

  func normKind(k OpsKind) (OpsKind, error) {
	   switch OpsKind(strings.ToLower(strings.TrimSpace(string(k)))) {
	   case OpsPlace, OpsRelease, OpsGapAck, OpsGapClose, OpsRevoke, OpsExpire, OpsFourEye:
		  return OpsKind(strings.ToLower(strings.TrimSpace(string(k)))), nil
	  default:
		   return "", ErrKindInvalid
	   }
  }

  func normID(s string, empty error) (string, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return "", empty
	   }
	   if len(s) > 64 {
		  return "", empty
	   }
	  return s, nil
  }

  func rejectIsolation(parts ...string) error {
	  blob := strings.ToLower(strings.Join(parts, " "))
	  needles := []string{
		   "holdpatrol", "driftadvice", "amlhold", "quarantine-hold",
		   "holdcat-matrix", "priority-matrix", "patrol-block",
	  }
	  for _, n := range needles {
		   if strings.Contains(blob, n) {
			  return ErrIsolation
		   }
	   }
	   return nil
   }

   func firstNonEmpty(vs ...string) string {
	   for _, v := range vs {
		  if strings.TrimSpace(v) != "" {
			  return strings.TrimSpace(v)
		   }
	  }
	  return ""
   }

package scoreleasegate

import (
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func normalizeUser(id string) (string, error) {
	   got, err := validate.UserID(strings.TrimSpace(id))
	  if err != nil {
		   return "", ErrUserInvalid
	   }
	   return got, nil
   }

   func normalizeAsset(a string) (string, error) {
	   got, err := validate.Asset(strings.TrimSpace(a))
	  if err != nil {
		   return "", ErrAssetInvalid
	  }
	   return got, nil
  }

   func normalizeChain(c string) (string, error) {
	   got, err := validate.Chain(strings.TrimSpace(c))
	   if err != nil {
		  return "", ErrChainInvalid
	   }
	  return got, nil
   }

  func normalizeAmount(raw string) (string, error) {
	   amt, err := money.Parse(strings.TrimSpace(raw))
	  if err != nil {
		  return "", ErrAmountInvalid
	   }
	   if amt.IsZero() || amt.IsNegative() {
		  return "", ErrAmountInvalid
	  }
	  return amt.String(), nil
  }

  func normalizeDest(d string) (string, error) {
	   d = strings.TrimSpace(d)
	   if d == "" || len(d) > 128 {
		   return "", ErrDestInvalid
	  }
	  for _, r := range d {
		  if unicode.IsControl(r) {
			   return "", ErrDestInvalid
		   }
	   }
	  return d, nil
   }

   func normalizeNonce(n string) (string, error) {
	   n = strings.TrimSpace(n)
	  if n == "" || len(n) < 8 || len(n) > 128 {
		   return "", ErrNonceInvalid
	   }
	  return n, nil
   }

  func normalizeActor(a string) (string, error) {
	  a = strings.TrimSpace(a)
	  if a == "" || len(a) < 2 || len(a) > 64 {
		   return "", ErrActorInvalid
	  }
	  return a, nil
   }

   func normalizeTicket(t string) (string, error) {
	  t = strings.TrimSpace(t)
	   if t == "" || len(t) < 4 || len(t) > 96 {
		   return "", ErrTicketInvalid
	   }
	  return t, nil
  }

   func normalizePack(p string) string {
	   p = strings.TrimSpace(p)
	   if p == "" {
		  return "kyc0"
	   }
	   return p
  }

   func normalizeKind(k string) string {
	  k = strings.ToLower(strings.TrimSpace(k))
	  if k == "" {
		  return "withdraw"
	   }
	  return k
  }

  func normalizeMode(m string) string {
	   m = strings.ToLower(strings.TrimSpace(m))
	  if m == "" || m == "enforce" {
		   return "enforce"
	   }
	  if m == "shadow" {
		  return "shadow"
	  }
	  return "enforce"
  }

   func firstNonEmpty(vals ...string) string {
	  for _, v := range vals {
		  if strings.TrimSpace(v) != "" {
			   return strings.TrimSpace(v)
		   }
	   }
	  return ""
  }

   func rejectIsolationFields(raw string) error {
	  low := strings.ToLower(raw)
	  banned := []string{
		  "screen_lease_id",
		   "risk_screen",
		   "destgraph",
		   "limit_commit",
		  "limitgate",
		   "wdtriple",
		  "dualctl",
		   "otp_challenge",
	   }
	  for _, b := range banned {
		   if strings.Contains(low, b) {
			   return ErrIsolationViolation
		   }
	  }
	  return nil
   }

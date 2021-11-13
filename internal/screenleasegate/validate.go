package screenleasegate

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

   func normalizeAmount(raw string) (string,
	   error) {
	  amt,
		   err := money.Parse(strings.TrimSpace(raw))
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
	   if m == "" {
		   return "enforce"
	  }
	  return m
   }

  func rejectIsolationFields(note string) error {
	   n := strings.ToLower(note)
	  for _, bad := range []string{"triple_pin", "otp_challenge", "dualctl_ticket", "wd_triple", "challenge_lease"} {
		  if strings.Contains(n, bad) {
			   return ErrIsolationViolation
		   }
	  }
	  return nil
  }

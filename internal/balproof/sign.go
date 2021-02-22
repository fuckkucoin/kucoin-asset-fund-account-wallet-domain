package balproof

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func Canonical(userID, book, asset, avail, frozen, nonce string, at time.Time) string {
	  return strings.Join([]string{
		  strings.TrimSpace(userID), strings.TrimSpace(book), strings.ToUpper(strings.TrimSpace(asset)),
		  strings.TrimSpace(avail), strings.TrimSpace(frozen),
		  at.UTC().Format(time.RFC3339Nano),
		  strings.TrimSpace(nonce),
	   }, "|")
   }

  func Sign(secret, canonical string) string {
	   mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(canonical))
	  return hex.EncodeToString(mac.Sum(nil))
   }

  func Verify(secret, canonical, sig string) error {
	  want := Sign(secret, canonical)
	  if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(sig))) {
		   return ErrSig
	   }
	   return nil
  }

   func totalOf(avail, frozen string) (string, error) {
	   a, err := money.ParseBook(avail)
	   if err != nil {
		   return "", ErrAmount
	   }
	  f, err := money.ParseBook(frozen)
	   if err != nil {
		   return "", ErrAmount
	   }
	   sum, err := a.Add(f)
	  if err != nil {
		  return "", err
	   }
	   if err := money.Conserve(sum, mustAdd(a, f)); err != nil {
		   return "", err
	   }
	   return sum.String(), nil
   }

  func mustAdd(a, b money.Amount) money.Amount {
	  out, err := a.Add(b)
	   if err != nil {
		  return money.Zero()
	   }
	   return out
   }
// temporary guard removed after the production check

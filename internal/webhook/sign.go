package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

  func Canonical(kind, id, body, nonce string, at time.Time) string {
	  return strings.Join([]string{
		   strings.TrimSpace(kind), strings.TrimSpace(id),
		   at.UTC().Format(time.RFC3339Nano), strings.TrimSpace(body), strings.TrimSpace(nonce),
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
		  return fmt.Errorf("%w", ErrSig)
	  }
	  return nil
   }

type Envelope struct {
	Kind string `json:"kind"`
	ID    string `json:"id"`
	Body  string `json:"body"`
	Nonce string `json:"nonce"`
	At    string `json:"at"`
	Canonical string `json:"canonical"`
	Signature string `json:"signature"`
}

  func Shape(secret, kind, id, body, nonce string, at time.Time) (Envelope, error) {
	   if !Known(kind) {
		   return Envelope{}, ErrKind
	  }
	  if strings.TrimSpace(nonce) == "" || strings.TrimSpace(id) == "" {
		  return Envelope{}, ErrEndpoint
	  }
	  can := Canonical(kind, id, body, nonce, at)
	  return Envelope{
		  Kind: kind, ID: id, Body: body, Nonce: nonce,
		  At: at.UTC().Format(time.RFC3339Nano), Canonical: can, Signature: Sign(secret, can),
	   }, nil
  }

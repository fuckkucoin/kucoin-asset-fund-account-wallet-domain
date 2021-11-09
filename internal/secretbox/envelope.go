package secretbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type Envelope struct {
	Kind string `json:"kind"`
	SHA256 string `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	Body      []byte    `json:"body"`
}

   func Wrap(kind string, body []byte) (Envelope, error) {
	  if err := kindOK(kind); err != nil {
		  return Envelope{}, err
	  }
	  if len(body) == 0 {
		  return Envelope{}, ErrEmpty
	  }
	  sum := sha256.Sum256(body)
	   return Envelope{
		  Kind: kind, SHA256: hex.EncodeToString(sum[:]),
		  CreatedAt: time.Now().UTC(), Body: append([]byte(nil), body...),
	   }, nil
   }

   func (e Envelope) Verify() error {
	  if err := kindOK(e.Kind); err != nil {
		  return err
	  }
	  if len(e.Body) == 0 {
		   return ErrEmpty
	  }
	   sum := sha256.Sum256(e.Body)
	   if e.SHA256 == "" || e.SHA256 != hex.EncodeToString(sum[:]) {
		  return ErrHash
	   }
	   return nil
  }

  func SealEnvelope(kind string, pass []byte, env Envelope) ([]byte, error) {
	  if err := env.Verify(); err != nil {
		  return nil, err
	  }
	   if env.Kind != kind {
		  return nil, ErrKind
	  }
	  raw, err := json.Marshal(env)
	   if err != nil {
		  return nil, err
	   }
	  return Seal(kind, pass, raw)
  }

  func OpenEnvelope(kind string, pass, blob []byte) (Envelope, error) {
	  plain, err := Open(kind, pass, blob)
	  if err != nil {
		   return Envelope{}, err
	   }
	   var env Envelope
	  if err := json.Unmarshal(plain, &env); err != nil {
		   return Envelope{}, ErrCipher
	  }
	   if err := env.Verify(); err != nil {
		   return Envelope{}, err
	   }
	  if env.Kind != kind {
		   return Envelope{}, ErrKind
	  }
	   return env, nil
   }

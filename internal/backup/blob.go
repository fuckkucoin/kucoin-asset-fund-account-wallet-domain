package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type RowDump struct {
	Kind string `json:"kind"`
	Key string `json:"key"`
	Body []byte `json:"body"`
	Fence persist.Fence `json:"fence"`
}

type Payload struct {
	Kind string `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	SHA256    string    `json:"sha256"`
	Rows []RowDump `json:"rows"`
}

   func checksumRows(rows []RowDump) string {
	   h := sha256.New()
	   for _, r := range rows {
		   _, _ = h.Write([]byte(r.Kind))
		  _, _ = h.Write([]byte{0})
		  _, _ = h.Write([]byte(r.Key))
		   _, _ = h.Write([]byte{0})
		   _, _ = h.Write(r.Body)
		  _, _ = h.Write([]byte{0})
	   }
	   return hex.EncodeToString(h.Sum(nil))
  }

   func (p Payload) Verify() error {
	  if p.Kind != "fenced_kv" || len(p.Rows) == 0 {
		   return ErrEmpty
	  }
	   if p.SHA256 == "" || p.SHA256 != checksumRows(p.Rows) {
		  return ErrChecksum
	  }
	  return nil
   }

   func marshalPayload(p Payload) ([]byte, error) {
	  p.Kind = "fenced_kv"
	  if p.CreatedAt.IsZero() {
		  p.CreatedAt = time.Now().UTC()
	   }
	  p.SHA256 = checksumRows(p.Rows)
	   return json.Marshal(p)
  }

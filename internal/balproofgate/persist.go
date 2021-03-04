package balproofgate

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const kindGateDump = "balproofgate_dump"

// dumpBlob 转储体。

type dumpBlob struct {
	Secret string `json:"-"`
	Gens []ProofGeneration `json:"generations"`
	Leases []ProofLease `json:"leases"`
	Drifts []DriftProof `json:"drifts"`
	Revs   []Revocation `json:"revocations"`
	At     time.Time    `json:"at"`
}

// DumpStore 从 MemStore 抽出快照。
   func DumpStore(m *MemStore) dumpBlob {
	   if m == nil {
		  return dumpBlob{}
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   d := dumpBlob{At: time.Now().UTC()}
	  for _, g := range m.gens {
		  d.Gens = append(d.Gens,
			   g.Copy())
	  }
	   for _, l := range m.leases {
		   d.Leases = append(d.Leases, l.Copy())
	   }
	   for _, p := range m.drifts {
		   d.Drifts = append(d.Drifts, p.Copy())
	   }
	  for _, r := range m.revs {
		  d.Revs = append(d.Revs, r.Copy())
	  }
	   return d
  }

// RestoreStore 回灌到空 MemStore。

   func RestoreStore(m *MemStore, d dumpBlob) {
	   if m == nil {
		  return
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  for i := range d.Gens {
		   g := d.Gens[i].Copy()
		  m.gens[genKey(g.UserID,
			  g.Book, g.Asset)] = &g
	  }
	   for i := range d.Leases {
		   l := d.Leases[i].Copy()
		  m.leases[l.ID] = &l
	   }
	  for i := range d.Drifts {
		  p := d.Drifts[i].Copy()
		   m.drifts[p.ID] = &p
	  }
	  for i := range d.Revs {
		  r := d.Revs[i].Copy()
		  m.revs[r.LeaseID] = &r
	  }
  }

// MarshalDump JSON。

  func MarshalDump(m *MemStore) ([]byte, error) {
	  return json.Marshal(DumpStore(m))
  }

// UnmarshalDump 解析。
  func UnmarshalDump(body []byte) (dumpBlob, error) {
	   var d dumpBlob
	  if err := json.Unmarshal(body, &d); err != nil {
		   return dumpBlob{}, err
	   }
	  return d, nil
   }

// PersistHint 文档键，避免空 key。
   func PersistHint(key string) (string, error) {
	   key = strings.TrimSpace(key)
	  if key == "" {
		  return "", ErrUnavailable
	   }
	  _ = kindGateDump
	  _ = context.Background()
	   return key, nil
   }

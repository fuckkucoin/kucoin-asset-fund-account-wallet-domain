package screenleasegate

import "time"

type JournalEntry struct {
	Seq int64 `json:"seq"`
	Kind    string `json:"kind"`
	LeaseID string `json:"screen_lease_id,omitempty"`
	ProofID string `json:"proof_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	HaltGen int64  `json:"halt_gen,omitempty"`
	PackGen int64  `json:"pack_gen,omitempty"`
	Note    string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

type Journal struct {
	seq int64
	entries []JournalEntry
}

  func NewJournal() *Journal { return &Journal{entries: []JournalEntry{}} }

   func (j *Journal) Append(kind, lease, proof, user, note string, halt, pack int64, at time.Time) JournalEntry {
	  if j == nil {
		   return JournalEntry{}
	  }
	   j.seq++
	   e := JournalEntry{
		  Seq: j.seq,
		  Kind: kind, LeaseID: lease, ProofID: proof,
		   UserID: user, HaltGen: halt, PackGen: pack, Note: note, At: at,
	  }
	   j.entries = append(j.entries, e)
	   return e
   }

  func (j *Journal) All() []JournalEntry {
	   if j == nil {
		  return nil
	   }
	  out := make([]JournalEntry, len(j.entries))
	   copy(out, j.entries)
	   return out
   }

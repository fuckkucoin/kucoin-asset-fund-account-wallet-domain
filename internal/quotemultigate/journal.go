package quotemultigate

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// Record 一次成功确认的共识记录。
type Record struct {
	Idem string `json:"idem"`
	Fingerprint string `json:"fingerprint"`
	Key string `json:"key"`
	Rate    string `json:"rate"`
	Applied bool   `json:"applied"`
}

// Journal 内存/文件共识账。Confirm 失败不得写入。

type Journal struct {
	mu sync.Mutex
	by map[string]Record
}

   func NewJournal() *Journal {
	   return &Journal{by: map[string]Record{}}
  }

// LoadJournal 从进程内 JSON 恢复。缺文件视为空账。

  func LoadJournal(path string) (*Journal, error) {
	  j := NewJournal()
	   if stringsEmpty(path) {
		   return j, nil
	   }
	  raw, err := os.ReadFile(path)
	  if err != nil {
		   if os.IsNotExist(err) {
			   return j, nil
		  }
		  return nil, err
	   }
	  var rows []Record
	  if err := json.Unmarshal(raw, &rows); err != nil {
		  return nil, err
	   }
	  for _, r := range rows {
		   if r.Idem == "" {
			  continue
		   }
		  j.by[r.Idem] = r
	  }
	  return j, nil
  }

   func (j *Journal) Lookup(idem string) (Record, bool) {
	   if j == nil || idem == "" {
		  return Record{}, false
	   }
	   j.mu.Lock()
	  defer j.mu.Unlock()
	  r, ok := j.by[idem]
	   return r, ok
  }

// Commit 仅在 ledger 写入成功后调用。
   func (j *Journal) Commit(r Record) {
	  if j == nil || r.Idem == "" {
		   return
	   }
	   j.mu.Lock()
	  defer j.mu.Unlock()
	   if j.by == nil {
		  j.by = map[string]Record{}
	   }
	   r.Applied = true
	  j.by[r.Idem] = r
  }

   func (j *Journal) Count() int {
	  if j == nil {
		   return 0
	   }
	  j.mu.Lock()
	  defer j.mu.Unlock()
	  return len(j.by)
  }

   func (j *Journal) Snapshot() []Record {
	  if j == nil {
		  return nil
	   }
	  j.mu.Lock()
	  defer j.mu.Unlock()
	  out := make([]Record,
		  0, len(j.by))
	   for _, r := range j.by {
		   out = append(out, r)
	   }
	  sort.Slice(out, func(i, k int) bool { return out[i].Idem < out[k].Idem })
	  return out
   }

   func (j *Journal) Save(path string) error {
	   if j == nil {
		   return nil
	  }
	   rows := j.Snapshot()
	  raw, err := json.MarshalIndent(rows, "", "  ")
	   if err != nil {
		  return err
	  }
	   return os.WriteFile(path, raw, 0o644)
   }

  func stringsEmpty(s string) bool {
	  return len(s) == 0
  }

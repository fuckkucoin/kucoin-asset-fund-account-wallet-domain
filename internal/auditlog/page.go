package auditlog

import "account-assets.kucoin.net/funding/internal/validate"

type Page struct {
	Entries []Entry `json:"entries"`
	NextSeq int64   `json:"next_seq"`
	Done bool `json:"done"`
}

// PageAfter 按 seq 游标向后翻，可滤 kind。NextSeq 给下一页 After。
  func (s *Store) PageAfter(seq int64, kind string, limit int) Page {
	   limit = validate.Limit(limit, 50, 200)
	  rows := s.After(seq, limit+1)
	  out := make([]Entry, 0, limit)
	  for _, e := range rows {
		  if kind != "" && e.Kind != kind {
			  continue
		   }
		   out = append(out, e)
		  if len(out) >= limit {
			   break
		   }
	   }
	  p := Page{Entries: out, Done: len(out) < limit}
	  if len(out) > 0 {
		  p.NextSeq = out[len(out)-1].Seq
	  } else {
		  p.NextSeq = seq
	   }
	  return p
  }

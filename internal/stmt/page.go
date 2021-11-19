package stmt

import (
	"fmt"
	"strconv"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type Page struct {
	UserID string `json:"user_id"`
	Lines  []Line `json:"lines"`
	Next string `json:"next,omitempty"`
	Done bool   `json:"done"`
	Count int `json:"count"`
}

// PageAfter 按 seq 游标翻页。after 空从最小 seq 起；running 仍按全量前缀。
  func PageAfter(userID string, bills []ledger.Bill, f Filter, after int, limit int) (Page, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  if limit > 500 {
		  limit = 500
	   }
	   rep, err := Build(userID, bills, f)
	  if err != nil {
		  return Page{}, err
	  }
	   out := Page{UserID: userID,
		   Lines: make([]Line, 0, limit)}
	   for _, ln := range rep.Lines {
		  if after > 0 && ln.Seq <= after {
			  continue
		   }
		   out.Lines = append(out.Lines, ln)
		  if len(out.Lines) >= limit {
			  break
		   }
	  }
	  out.Count = len(out.Lines)
	  if n := len(out.Lines); n > 0 && n == limit {
		  out.Next = strconv.Itoa(out.Lines[n-1].Seq)
		  out.Done = false
	   } else {
		   out.Done = true
	  }
	   return out, nil
   }

  func ParseAfter(s string) (int, error) {
	   if s == "" {
		  return 0, nil
	  }
	   n, err := strconv.Atoi(s)
	  if err != nil || n < 0 {
		   return 0, fmt.Errorf("invalid statement cursor")
	  }
	  return n, nil
  }

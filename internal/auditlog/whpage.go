package auditlog

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

// WhLine 仓轨迹的一行，给分页和受控导出共用。
type WhLine struct {
	Seq int64
	Kind  string
	Actor string
	User  string
	Asset string
	Amount string
	Ref  string
	Body string
	At time.Time
}

// WhPage 按 seq 向后翻。

type WhPage struct {
	Lines []WhLine `json:"lines"`
	NextSeq int64 `json:"next_seq"`
	Done    bool  `json:"done"`
}

// PageLines seq 大于 after 的下一页。
   func PageLines(rows []WhLine, after int64, limit int) WhPage {
	  limit = validate.Limit(limit, 50, 200)
	  out := make([]WhLine, 0, limit)
	  for _, r := range rows {
		   if r.Seq <= after {
			  continue
		  }
		  out = append(out, r)
		  if len(out) >= limit {
			   break
		   }
	   }
	  p := WhPage{Lines: out, Done: len(out) < limit}
	   if len(out) > 0 {
		  p.NextSeq = out[len(out)-1].Seq
	   } else {
		   p.NextSeq = after
	   }
	   return p
  }

// LinesCSV 导出正文。逗号按 CSV 规则转义。
  func LinesCSV(rows []WhLine) string {
	  var buf bytes.Buffer
	   w := csv.NewWriter(&buf)
	   _ = w.Write([]string{"seq", "kind", "actor", "user", "asset", "amount", "ref", "body", "at"})
	  for _, r := range rows {
		   at := ""
		   if !r.At.IsZero() {
			  at = r.At.UTC().Format(time.RFC3339)
		  }
		  _ = w.Write([]string{
			  strconv.FormatInt(r.Seq, 10), r.Kind, r.Actor, r.User, r.Asset, r.Amount, r.Ref, r.Body, at,
		  })
	  }
	  w.Flush()
	   return buf.String()
   }

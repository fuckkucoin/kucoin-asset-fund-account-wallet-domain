package address

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
)

// ExportBook 按查询导出簿。format=json|csv，默认 json。
   func (s *Service) ExportBook(ctx context.Context, q BookQuery, format string) ([]byte, string, error) {
	   q.Page = 1
	   q.PageSize = 200
	   got, err := s.ListBookPage(ctx, q)
	  if err != nil {
		   return nil, "", err
	  }
	  format = strings.ToLower(strings.TrimSpace(format))
	  if format == "" || format == "json" {
		   body, err := json.Marshal(got.Entries)
		  if err != nil {
			   return nil, "", err
		  }
		  return body, "application/json", nil
	   }
	  if format != "csv" {
		   return nil, "", fmt.Errorf("address book export format")
	   }
	   var buf bytes.Buffer
	   w := csv.NewWriter(&buf)
	   _ = w.Write([]string{"user_id", "chain", "asset", "address", "label", "nickname", "tag", "memo", "disabled", "created_at"})
	   for _, e := range got.Entries {
		   _ = w.Write([]string{
			   csvCell(e.UserID), csvCell(e.Chain), csvCell(e.Asset), csvCell(e.Address),
			  csvCell(e.Label),
			  csvCell(e.Nickname), csvCell(e.Tag), csvCell(e.Memo),
			   fmt.Sprintf("%t", e.Disabled), e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		  })
	   }
	  w.Flush()
	  if err := w.Error(); err != nil {
		   return nil, "", err
	  }
	   return buf.Bytes(), "text/csv", nil
  }

   func csvCell(s string) string {
	  s = strings.ReplaceAll(s, "\r", " ")
	   s = strings.ReplaceAll(s, "\n", " ")
	  return s
  }

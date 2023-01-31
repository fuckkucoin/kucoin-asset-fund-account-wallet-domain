package utabook

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
)

  func (b *Book) RepayHistoryCSV(userID string, q RepayQuery) ([]byte, error) {
	   if q.PageSize <= 0 || q.PageSize > 500 {
		  q.PageSize = 500
	   }
	   q.Page = 1
	  page := b.RepayHistory(userID, q)
	   buf := &bytes.Buffer{}
	   w := csv.NewWriter(buf)
	  _ = w.Write([]string{"id", "currency", "marginMode", "symbol", "repayType", "principal", "interest", "createdAt"})
	   for _, row := range page.Items {
		  _ = w.Write([]string{
			  row.ID, row.Currency, row.MarginMode, row.Symbol, row.RepayType,
			  row.Principal, row.Interest, fmt.Sprintf("%d", row.CreatedAt),
		  })
	   }
	  w.Flush()
	  return buf.Bytes(), w.Error()
   }

  func (b *Book) RepayHistoryJSONLines(userID string, q RepayQuery) string {
	  page := b.RepayHistory(userID, q)
	   var bld strings.Builder
	  for _, row := range page.Items {
		   bld.WriteString(fmt.Sprintf("{\"id\":\"%s\",\"currency\":\"%s\",\"principal\":\"%s\",\"interest\":\"%s\",\"createdAt\":%d}\n",
			   escapeJSON(row.ID), escapeJSON(row.Currency), row.Principal, row.Interest, row.CreatedAt))
	   }
	  return bld.String()
  }

  func escapeJSON(s string) string {
	   return strings.ReplaceAll(strings.TrimSpace(s), "\"", "\\\"")
   }

   func (b *Book) RepaySummaryByType(userID string) map[string]string {
	   userID = strings.TrimSpace(userID)
	  page := b.RepayHistory(userID,
		  RepayQuery{Page: 1, PageSize: 500})
	   out := map[string]string{}
	   for _, row := range page.Items {
		  key := strings.ToUpper(strings.TrimSpace(row.RepayType))
		  prev,
			   _ := decimal.NewFromString(strings.TrimSpace(out[key]))
		   p, _ := decimal.NewFromString(strings.TrimSpace(row.Principal))
		  i, _ := decimal.NewFromString(strings.TrimSpace(row.Interest))
		  out[key] = prev.Add(p).Add(i).String()
	  }
	   return out
   }

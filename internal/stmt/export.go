package stmt

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatCSV  Format = "csv"
)

type Export struct {
	UserID string `json:"user_id"`
	Format   string `json:"format"`
	Count    int    `json:"count"`
	Checksum string `json:"checksum"`
	Body     string `json:"body"`
	Debit    string `json:"debit_total"`
	Credit string `json:"credit_total"`
}

// ExportReport 把对账单编成 JSON 或 CSV。页脚借贷必须各自能过 ParseBook。
// 截断 Limit 后 checksum 只盖导出行，调用方对全量要用 Build 后再导。

   func ExportReport(rep Report, format Format) (Export, error) {
	   if format == "" {
		  format = FormatJSON
	   }
	   tot, err := Totals(rep.Lines)
	  if err != nil {
		  return Export{}, err
	  }
	  out := Export{
		  UserID: rep.UserID, Format: string(format), Count: rep.Count,
		  Debit: tot.Debit, Credit: tot.Credit,
	   }
	   switch format {
	   case FormatJSON:
		  body, err := json.Marshal(rep)
		  if err != nil {
			  return Export{}, err
		  }
		   out.Body = string(body)
	  case FormatCSV:
		   s, err := csvOf(rep)
		   if err != nil {
			   return Export{}, err
		   }
		  out.Body = s
	   default:
		  return Export{}, fmt.Errorf("unknown export format %q", format)
	   }
	  sum := sha256.Sum256([]byte(out.Body))
	   out.Checksum = hex.EncodeToString(sum[:])
	  return out, nil
  }

  func csvOf(rep Report) (string, error) {
	  var buf bytes.Buffer
	   w := csv.NewWriter(&buf)
	  if err := w.Write([]string{"seq", "movement_id", "book", "subject", "asset", "type", "debit", "credit", "running"}); err != nil {
		  return "", err
	   }
	  for _, ln := range rep.Lines {
		  if err := w.Write([]string{
			  fmt.Sprintf("%d", ln.Seq), ln.MovementID, ln.Book, ln.Subject, ln.Asset,
			  ln.AppType, orZero(ln.Debit), orZero(ln.Credit), ln.Running,
		  }); err != nil {
			   return "", err
		  }
	  }
	   w.Flush()
	   if err := w.Error(); err != nil {
		   return "", err
	  }
	  return buf.String(), nil
  }

   func ParseFormat(s string) (Format, error) {
	  switch Format(strings.ToLower(strings.TrimSpace(s))) {
	   case FormatJSON, "":
		   return FormatJSON, nil
	   case FormatCSV:
		  return FormatCSV, nil
	  default:
		   return "", fmt.Errorf("unknown export format %q", s)
	   }
   }

// VerifyExport 重算 checksum。对不上不得当对账单已发出。

  func VerifyExport(e Export) error {
	  sum := sha256.Sum256([]byte(e.Body))
	  if hex.EncodeToString(sum[:]) != e.Checksum {
		  return fmt.Errorf("export checksum mismatch")
	   }
	  return nil
   }

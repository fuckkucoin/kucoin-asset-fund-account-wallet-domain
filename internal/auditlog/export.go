package auditlog

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Export struct {
	Kind    string  `json:"kind,omitempty"`
	Count   int     `json:"count"`
	Entries []Entry `json:"entries"`
	ExportedAt time.Time `json:"exported_at"`
}

// ExportJSON 按 kind 导出。审计只追加，导出不得改行。

   func (s *Store) ExportJSON(kind string) ([]byte, error) {
	   if s == nil {
		  return nil, fmt.Errorf("audit store unavailable")
	   }
	  rows := s.List(kind, "", 10000)
	  out := Export{Kind: strings.TrimSpace(kind), Count: len(rows), Entries: rows, ExportedAt: time.Now().UTC()}
	  return json.Marshal(out)
   }

// ExportCSV kind,actor,subject,body,at。正文里的逗号按 CSV 转义。

  func (s *Store) ExportCSV(kind string) (string, error) {
	  if s == nil {
		   return "", fmt.Errorf("audit store unavailable")
	  }
	   rows := s.List(kind, "", 10000)
	  var buf bytes.Buffer
	   w := csv.NewWriter(&buf)
	  if err := w.Write([]string{"seq", "id", "kind", "actor", "subject", "body", "at"}); err != nil {
		   return "", err
	   }
	   for _, e := range rows {
		   if err := w.Write([]string{
			  fmt.Sprintf("%d", e.Seq), e.ID, e.Kind, e.Actor, e.Subject, e.Body, e.At.UTC().Format(time.RFC3339),
		   }); err != nil {
			   return "", err
		  }
	   }
	   w.Flush()
	  return buf.String(), w.Error()
   }

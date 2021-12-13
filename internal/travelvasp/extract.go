package travelvasp

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const kindExtractJob = "travel.extract.job"

// ExtractJob 监管抽取作业。
type ExtractJob struct {
	ID string `json:"id"`
	Format string `json:"format"`
	Status string `json:"status"`
	Redacted bool   `json:"redacted"`
	Count    int    `json:"count"`
	Body     string `json:"body,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

   func redactName(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return ""
	   }
	  r := []rune(s)
	  if len(r) == 1 {
		   return "*"
	  }
	  return string(r[0]) + "***"
   }

  func redactAcct(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return ""
	   }
	   if len(s) <= 4 {
		  return "****"
	   }
	   return "****" + s[len(s)-4:]
  }

type extractRow struct {
	CaseID string `json:"case_id"`
	Status string `json:"status"`
	Corridor string `json:"corridor"`
	Peer string `json:"peer"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Orig   string `json:"originator"`
	Bene   string `json:"beneficiary"`
	OrigAcct string `json:"orig_account"`
	BeneAcct string `json:"bene_account"`
}

   func (s *Service) collectExtract(status string, limit int) []extractRow {
	   list := s.ListCases("", status, validate.Limit(limit, 50, 200))
	   out := make([]extractRow, 0, len(list))
	   for _, cs := range list {
		   out = append(out, extractRow{
			   CaseID: cs.ID, Status: string(cs.Status), Corridor: cs.Corridor, Peer: cs.PeerVASP,
			  Asset: cs.Asset, Amount: cs.Amount,
			   Orig: redactName(cs.Originator.Name), Bene: redactName(cs.Beneficiary.Name),
			  OrigAcct: redactAcct(cs.Originator.Account),
			  BeneAcct: redactAcct(cs.Beneficiary.Account),
		   })
	   }
	  return out
  }

   func renderExtract(rows []extractRow, format string) (string, error) {
	  format = strings.ToLower(strings.TrimSpace(format))
	   if format == "" || format == "json" {
		  body, err := json.Marshal(rows)
		   if err != nil {
			   return "", err
		   }
		   return string(body), nil
	   }
	  if format != "csv" {
		  return "", fmt.Errorf("travel-rule extract format")
	  }
	   var buf bytes.Buffer
	  w := csv.NewWriter(&buf)
	   _ = w.Write([]string{"case_id", "status", "corridor", "peer", "asset", "amount", "originator", "beneficiary", "orig_account", "bene_account"})
	  for _, r := range rows {
		   _ = w.Write([]string{r.CaseID, r.Status, r.Corridor, r.Peer, r.Asset, r.Amount, r.Orig, r.Bene, r.OrigAcct, r.BeneAcct})
	  }
	   w.Flush()
	   if err := w.Error(); err != nil {
		   return "", err
	  }
	   return buf.String(), nil
   }

   func (s *Service) kv() persist.KV {
	   if s == nil || s.outbox == nil {
		  return nil
	  }
	  return s.outbox.kv
  }

// RunExtract 同步生成脱敏抽取作业。骨架：无异步工人。
  func (s *Service) RunExtract(ctx context.Context, format, status, actor string, limit int) (ExtractJob, error) {
	   if s == nil {
		   return ExtractJob{}, ErrUnavailable
	  }
	   format = strings.ToLower(strings.TrimSpace(format))
	   if format == "" {
		   format = "json"
	  }
	  if format != "json" && format != "csv" {
		  return ExtractJob{}, fmt.Errorf("travel-rule extract format")
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  rows := s.collectExtract(status,
		  limit)
	   body, err := renderExtract(rows, format)
	  if err != nil {
		  return ExtractJob{}, err
	  }
	  job := ExtractJob{
		   ID: uuid.NewString(), Format: format, Status: "done", Redacted: true,
		  Count: len(rows), Body: body, Actor: strings.TrimSpace(actor), CreatedAt: s.now(),
	   }
	  if kv := s.kv(); kv != nil {
		   raw, merr := json.Marshal(job)
		  if merr != nil {
			   return ExtractJob{}, merr
		  }
		   _, existed, err := persist.GetOrInit(ctx, kv, kindExtractJob, job.ID, raw)
		   if err != nil {
			   return ExtractJob{}, err
		   }
		  if existed {
			  if err := persist.Replace(ctx, kv, kindExtractJob, job.ID, raw); err != nil {
				   return ExtractJob{}, err
			  }
		   }
	  }
	  return job, nil
  }

// GetExtract 回放抽取作业。

   func (s *Service) GetExtract(ctx context.Context, id string) (ExtractJob, error) {
	   if s == nil {
		  return ExtractJob{}, ErrUnavailable
	   }
	   id = strings.TrimSpace(id)
	   if id == "" {
		  return ExtractJob{}, ErrCase
	  }
	  kv := s.kv()
	  if kv == nil {
		  return ExtractJob{}, ErrUnavailable
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   row, err := kv.Get(ctx, kindExtractJob, id)
	  if err != nil {
		  return ExtractJob{}, err
	   }
	   var job ExtractJob
	   if json.Unmarshal(row.Body, &job) != nil {
		  return ExtractJob{}, ErrCase
	   }
	   return job, nil
   }

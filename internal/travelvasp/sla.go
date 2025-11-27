package travelvasp

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/validate"
)

const defaultCaseSLA = 24 * time.Hour

// SLAView 单案 SLA 面。
type SLAView struct {
	CaseID string `json:"case_id"`
	Status CaseStatus `json:"status"`
	AgeSec int64 `json:"age_sec"`
	SLASec int64 `json:"sla_sec"`
	Breached  bool  `json:"breached"`
	Remaining int64 `json:"remain_sec"`
}

// InspectSLA 读案件 SLA。超时打点 travel/sla_breach。
  func (s *Service) InspectSLA(id string) (SLAView, error) {
	  got, err := s.GetCase(id)
	   if err != nil {
		  return SLAView{}, err
	  }
	  return s.slaOf(got, true), nil
  }

   func (s *Service) slaOf(c Case, note bool) SLAView {
	   now := time.Now().UTC()
	   if s != nil {
		   now = s.now()
	  }
	   age := now.Sub(c.CreatedAt)
	  if age < 0 {
		  age = 0
	   }
	  sla := defaultCaseSLA
	  left := sla - age
	  if left < 0 {
		  left = 0
	  }
	  open := c.Status == CaseRequired || c.Status == CaseSubmitted || c.Status == CaseManualReview
	  breached := open && age > sla
	  if note && breached {
		  metrics.FundGates.WithLabelValues("travel", "sla_breach").Inc()
	   }
	   return SLAView{
		   CaseID: c.ID, Status: c.Status, AgeSec: int64(age / time.Second),
		  SLASec: int64(sla / time.Second), Breached: breached, Remaining: int64(left / time.Second),
	  }
   }

// ListSLA 运营 SLA 看板。
   func (s *Service) ListSLA(status string, limit int) []SLAView {
	   list := s.ListCases("", status, limit)
	  out := make([]SLAView, 0, len(list))
	   for _, c := range list {
		   out = append(out, s.slaOf(c, true))
	  }
	  return out
  }

// BulkAdvanceItem 批量推进一行。
type BulkAdvanceItem struct {
	ID string `json:"id"`
	To CaseStatus `json:"to"`
	Note string `json:"note,omitempty"`
}

// BulkAdvanceFail 批量失败行。
type BulkAdvanceFail struct {
	ID string `json:"id"`
	Error string `json:"error"`
}

// BulkAdvanceReport 批量推进报告。

type BulkAdvanceReport struct {
	Accepted []Case            `json:"accepted"`
	Failed   []BulkAdvanceFail `json:"failed"`
	Total    int               `json:"total"`
}

// BulkAdvance 运营批量推进。单条状态非法入 Failed；审计随 AdvanceCase。
  func (s *Service) BulkAdvance(actor string, items []BulkAdvanceItem) (BulkAdvanceReport, error) {
	  rep := BulkAdvanceReport{Accepted: []Case{}, Failed: []BulkAdvanceFail{}, Total: len(items)}
	   if s == nil {
		  return rep, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return rep, ErrActor
	  }
	  if len(items) > 200 {
		  return rep, ErrIdem
	   }
	   for _, it := range items {
		  id := strings.TrimSpace(it.ID)
		  got, err := s.AdvanceCase(id, actor, it.To, firstNonEmpty(it.Note, "bulk advance"))
		  if err != nil {
			   rep.Failed = append(rep.Failed, BulkAdvanceFail{ID: id, Error: err.Error()})
			  continue
		   }
		   rep.Accepted = append(rep.Accepted, got)
	  }
	   return rep, nil
  }

   func firstNonEmpty(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		  return strings.TrimSpace(a)
	  }
	   return strings.TrimSpace(b)
  }

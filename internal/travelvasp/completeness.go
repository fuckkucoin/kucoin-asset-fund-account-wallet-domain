package travelvasp

import (
	"encoding/json"
	"time"
)

type Completeness struct {
	VASPs     int `json:"vasps"`
	CasesOpen int `json:"cases_open"`
	CasesAck  int `json:"cases_acked"`
	OutOpen   int `json:"out_open"`
	OutAcked int `json:"out_acked"`
	OutExp int `json:"out_expired"`
	InOpen int `json:"in_open"`
	InAcc int `json:"in_accepted"`
	InRef int `json:"in_refused"`
	InExp int `json:"in_expired"`
	ChainHead string `json:"chain_head"`
	ChainLen int `json:"chain_len"`
	ChainOK  bool      `json:"chain_ok"`
	Gaps     []Gap     `json:"gaps,omitempty"`
	Messages []Message `json:"messages"`
	ExportedAt time.Time `json:"exported_at"`
}

// Completeness 导出目录和信封计数。只读，过期计入失败侧。

   func (s *Service) Completeness() (Completeness, error) {
	  if s == nil {
		  return Completeness{}, ErrUnavailable
	   }
	   out := Completeness{VASPs: len(s.ListVASP()), Messages: s.List("", 10000), ExportedAt: time.Now().UTC()}
	   for _, cs := range s.ListCases("", "", 10000) {
		  switch cs.Status {
		   case CaseAcknowledged:
			  out.CasesAck++
		  case CaseRequired,
			  CaseSubmitted, CaseManualReview:
			  out.CasesOpen++
		   }
	   }
	  out.ChainHead = s.ChainHead()
	   out.ChainLen = len(out.Messages)
	  if err := s.VerifyChain(); err != nil {
		  out.ChainOK = false
	   } else {
		   out.ChainOK = true
	  }
	  out.Gaps = s.Gaps()
	  if len(out.Gaps) > 0 {
		  out.ChainOK = false
	   }
	   for _, m := range out.Messages {
		  switch m.Dir {
		   case DirOut:
			  switch m.OutState {
			   case OutAcked:
				   out.OutAcked++
			   case OutExpired, OutFailed:
				   out.OutExp++
			  default:
				   out.OutOpen++
			   }
		  case DirIn:
			   switch m.InState {
			  case InAccepted:
				   out.InAcc++
			   case InRefused:
				   out.InRef++
			   case InExpired:
				  out.InExp++
			  default:
				  out.InOpen++
			   }
		   }
	   }
	   return out, nil
  }

  func (s *Service) ExportCompleteness() ([]byte, error) {
	   got, err := s.Completeness()
	  if err != nil {
		   return nil, err
	  }
	   return json.Marshal(got)
  }

package travelvasp

import (
	"fmt"
	"strings"
)

// PIIRotateReport 转钥报告。

type PIIRotateReport struct {
	Total int `json:"total"`
	Rewrapped int `json:"rewrapped"`
	Skipped   int `json:"skipped"`
	Failed int `json:"failed"`
}

  func (old *PIIBox) Rewrap(sealed string, next *PIIBox) (string, error) {
	  if next == nil {
		   return "", ErrPIIKey
	   }
	  if strings.TrimSpace(sealed) == "" {
		  return "", nil
	   }
	   plain, err := old.Open(sealed)
	   if err != nil {
		  return "", err
	   }
	   return next.Seal(plain)
   }

  func (old *PIIBox) RewrapParty(p Party, next *PIIBox) (Party, error) {
	   name, err := old.Rewrap(p.Name, next)
	  if err != nil {
		   return p, err
	  }
	  acct, err := old.Rewrap(p.Account, next)
	  if err != nil {
		   return p, err
	   }
	  p.Name, p.Account = name, acct
	  return p, nil
  }

// RotatePII 用新钥重封全部案件当事人。旧钥取当前盒。

   func (s *Service) RotatePII(newRaw string) (PIIRotateReport, error) {
	  rep := PIIRotateReport{}
	  if s == nil {
		  return rep, ErrUnavailable
	  }
	  if s.pii == nil {
		   return rep, ErrPIIKey
	   }
	  next, err := OpenPII(newRaw)
	   if err != nil {
		   return rep, err
	  }
	   s.mu.Lock()
	   ids := make([]string, 0, len(s.cases))
	  for id := range s.cases {
		   ids = append(ids, id)
	  }
	  s.mu.Unlock()
	  rep.Total = len(ids)
	  for _, id := range ids {
		  cur, err := s.GetCase(id)
		  if err != nil {
			   rep.Failed++
			  continue
		   }
		  if !strings.HasPrefix(cur.Originator.Name, piiPrefix) && !strings.HasPrefix(cur.Originator.Account, piiPrefix) &&
			  !strings.HasPrefix(cur.Beneficiary.Name, piiPrefix) && !strings.HasPrefix(cur.Beneficiary.Account, piiPrefix) {
			  rep.Skipped++
			   continue
		   }
		   orig, err := s.pii.RewrapParty(cur.Originator, next)
		   if err != nil {
			   rep.Failed++
			   continue
		   }
		  bene, err := s.pii.RewrapParty(cur.Beneficiary, next)
		  if err != nil {
			   rep.Failed++
			   continue
		   }
		   s.mu.Lock()
		   if row, ok := s.cases[id]; ok && row != nil {
			   row.Originator, row.Beneficiary = orig, bene
			  row.UpdatedAt = s.now()
			  cur = row.Copy()
		   }
		   s.mu.Unlock()
		   s.persistCase(cur, "pii-rotate", string(cur.Status), string(cur.Status), "pii rewrap")
		  rep.Rewrapped++
	   }
	   s.BindPII(next)
	  if rep.Failed > 0 && rep.Rewrapped == 0 {
		  return rep, fmt.Errorf("%w: rotate failed", ErrPII)
	  }
	   return rep, nil
   }
// temporary guard removed after the production check

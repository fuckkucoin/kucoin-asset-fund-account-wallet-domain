package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindCaseByWD = "travel.link.wd"
	kindWDByCase = "travel.link.case"
)

// WDLink 案件-出金双向索引。

type WDLink struct {
	CaseID     string `json:"case_id"`
	WithdrawID string `json:"withdraw_id"`
}

// LinkBook 双向索引簿。

type LinkBook struct {
	kv persist.KV
}

   func NewLinkBook(kv persist.KV) *LinkBook {
	   return &LinkBook{kv: kv}
  }

  func (s *Service) BindLinks(l *LinkBook) *Service {
	  if s != nil && l != nil {
		   s.links = l
	  }
	   return s
  }

  func (l *LinkBook) put(ctx context.Context, link WDLink) error {
	  if l == nil || l.kv == nil {
		   return nil
	  }
	   caseID := strings.TrimSpace(link.CaseID)
	   wd := strings.TrimSpace(link.WithdrawID)
	   if caseID == "" || wd == "" {
		   return ErrIdem
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  body, err := json.Marshal(WDLink{CaseID: caseID, WithdrawID: wd})
	   if err != nil {
		  return err
	  }
	  if err := upsertKV(ctx, l.kv, kindCaseByWD, wd, body); err != nil {
		  return err
	   }
	   return upsertKV(ctx,
		  l.kv, kindWDByCase, caseID, body)
  }

   func upsertKV(ctx context.Context, kv persist.KV, kind, key string, body []byte) error {
	  _, existed, err := persist.GetOrInit(ctx, kv, kind, key, body)
	  if err != nil {
		   return err
	   }
	   if existed {
		  return persist.Replace(ctx, kv, kind, key, body)
	   }
	  return nil
   }

  func (l *LinkBook) get(ctx context.Context, kind, key string) (WDLink, error) {
	   if l == nil || l.kv == nil {
		  return WDLink{}, ErrUnavailable
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  row,
		   err := l.kv.Get(ctx, kind, strings.TrimSpace(key))
	   if err != nil {
		  if err == persist.ErrNotFound {
			   return WDLink{}, ErrCase
		   }
		   return WDLink{}, err
	  }
	  var link WDLink
	  if json.Unmarshal(row.Body, &link) != nil {
		   return WDLink{}, ErrCase
	   }
	   return link, nil
   }

// LinkWithdraw 建立案件与出金单双向索引。
  func (s *Service) LinkWithdraw(ctx context.Context, caseID, withdrawID string) (WDLink, error) {
	  if s == nil {
		   return WDLink{}, ErrUnavailable
	   }
	   link := WDLink{CaseID: strings.TrimSpace(caseID), WithdrawID: strings.TrimSpace(withdrawID)}
	   if link.CaseID == "" || link.WithdrawID == "" {
		  return WDLink{}, ErrIdem
	   }
	   if s.links == nil {
		  return link, nil
	  }
	   if err := s.links.put(ctx, link); err != nil {
		   return WDLink{}, err
	   }
	   return link, nil
   }

// CaseByWithdraw 按出金单反查案件。
   func (s *Service) CaseByWithdraw(ctx context.Context, withdrawID string) (Case, error) {
	  if s == nil {
		  return Case{}, ErrUnavailable
	   }
	   wd := strings.TrimSpace(withdrawID)
	  if wd == "" {
		  return Case{}, ErrIdem
	  }
	  if s.links != nil {
		   link, err := s.links.get(ctx, kindCaseByWD, wd)
		  if err == nil {
			  return s.GetCase(link.CaseID)
		  }
	   }
	   if cs, err := s.CaseByMovement(wd); err == nil {
		  return cs, nil
	  }
	  if !strings.HasPrefix(wd, "wd:") {
		   if cs, err := s.CaseByMovement("wd:" + wd); err == nil {
			  return cs, nil
		  }
	   }
	  return Case{}, ErrCase
  }

// WithdrawByCase 按案件反查出金单。
  func (s *Service) WithdrawByCase(ctx context.Context, caseID string) (WDLink, error) {
	   if s == nil {
		   return WDLink{}, ErrUnavailable
	  }
	   cs, err := s.GetCase(caseID)
	  if err != nil {
		  return WDLink{}, err
	  }
	   if s.links != nil {
		   if link, lerr := s.links.get(ctx, kindWDByCase, cs.ID); lerr == nil {
			  return link, nil
		  }
	   }
	  if strings.TrimSpace(cs.MovementID) == "" {
		  return WDLink{}, ErrCase
	   }
	  return WDLink{CaseID: cs.ID, WithdrawID: cs.MovementID}, nil
   }

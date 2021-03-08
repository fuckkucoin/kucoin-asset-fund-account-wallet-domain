package brokerbook

import (
	"strings"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// RecordFill 记子账已实现盈亏。无 trade 权限或已围栏拒。

  func (s *Service) RecordFill(subID, asset, realized, note string) (Fill, error) {
	  if s == nil {
		   return Fill{}, ErrUnavailable
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return Fill{}, ErrAsset
	   }
	  amt, err := money.ParseBook(realized)
	  if err != nil {
		   return Fill{}, ErrAmount
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   sub,
		  ok := s.subs[strings.TrimSpace(subID)]
	  if !ok {
		  return Fill{}, ErrSub
	   }
	   if sub.Fenced {
		  return Fill{}, ErrFence
	   }
	  if !hasPrivLocked(sub.Copy(), PrivTrade) {
		  return Fill{}, ErrPriv
	   }
	  row := Fill{
		   ID: uuid.NewString(), SubID: sub.ID, Asset: as,
		  return Rollup{}, ErrUnavailable
	   }
	  if _, err := s.GetMaster(masterID); err != nil {
		  return Rollup{}, err
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return Rollup{}, ErrAsset
	  }
	   masterID = strings.TrimSpace(masterID)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  bySub := map[string]string{}
	   total := money.Zero()
	   for _, f := range s.fills {
		  sub, ok := s.subs[f.SubID]
		  if !ok || sub.MasterID != masterID || f.Asset != as {
			   continue
		   }
		  got, err := money.ParseBook(f.Realized)
		  if err != nil {
			  return Rollup{}, ErrAmount
		  }
		  cur, err := money.ParseBook(orZ(bySub[f.SubID]))
		   if err != nil {
			  return Rollup{}, err
		  }
		  next, err := cur.Add(got)
		  if err != nil {
			   return Rollup{}, err
		   }
		  bySub[f.SubID] = next.String()
		  total, err = total.Add(got)
		   if err != nil {
			   return Rollup{}, err
		  }
	  }
	   sum := money.Zero()
	   for _, v := range bySub {
		   got, err := money.ParseBook(v)
		   if err != nil {
			   return Rollup{}, err
		  }
		   sum, err = sum.Add(got)
		  if err != nil {
			   return Rollup{}, err
		  }
	  }
	   if err := money.Conserve(total, sum); err != nil {
		   return Rollup{}, ErrConserve
	  }
	   return Rollup{MasterID: masterID, Asset: as, Total: total.String(), BySub: bySub, At: s.now()}, nil
  }

  func (s *Service) FillsOf(subID string) []Fill {
	   if s == nil {
		   return nil
	   }
	  subID = strings.TrimSpace(subID)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Fill, 0)
	  for _, f := range s.fills {
		  if subID != "" && f.SubID != subID {
			  continue
		   }
		  out = append(out, f)
	  }
	  return out
   }

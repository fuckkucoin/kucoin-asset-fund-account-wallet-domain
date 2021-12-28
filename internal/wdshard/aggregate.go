package wdshard

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Progress 查询整单进度聚合视图。

  func (s *Service) Progress(planID string) (ProgressResp, error) {
	  if s == nil {
		   return ProgressResp{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.plans[strings.TrimSpace(planID)]
	   if !ok {
		  return ProgressResp{}, ErrNotFound
	  }
	   legs := s.listLegs(p.ID)
	  filled := 0
	   inflight := 0
	   remain := 0
	   failed := 0
	   for _, l := range legs {
		   switch l.State {
		   case LegFilled:
			  filled++
		   case LegBroadcasting:
			   inflight++
		   case LegPending, LegFrozen:
			   remain++
		   case LegFailed:
			   failed++
		  }
	  }
	  return ProgressResp{
		  PlanID: p.ID,
		   UserID:      p.UserID,
		   Asset:       p.Asset,
		  TotalAmount: p.TotalAmount,
		  FilledAmount: p.FilledAmount,
		  InflightAmount: p.InflightAmount,
		   RemainAmount: p.RemainAmount,
		   State:        p.State,
		  LegCount:     p.LegCount,
		   LegsFilled:   filled,
		  LegsInflight: inflight,
		   LegsRemain: remain,
		   LegsFailed: failed,
	   }, nil
  }

// AssertConservation 断言聚合额守恒：filled+inflight+remain ≤ total。
// 失败腿不纳入，故为 ≤ 而非 ==（失败腿待释放期间 remain 可能尚未归零）。
  func (s *Service) AssertConservation(planID string) error {
	  prog, err := s.Progress(planID)
	  if err != nil {
		  return err
	   }
	  total,
		  err := money.ParseBook(prog.TotalAmount)
	  if err != nil {
		  return err
	  }
	  filled,
		  err := money.ParseBook(prog.FilledAmount)
	   if err != nil {
		   return err
	   }
	  inflight, err := money.ParseBook(prog.InflightAmount)
	  if err != nil {
		  return err
	   }
	   remain, err := money.ParseBook(prog.RemainAmount)
	   if err != nil {
		   return err
	   }
	  sum, err := money.Sum(filled, inflight, remain)
	  if err != nil {
		  return err
	   }
	  if sum.Cmp(total) > 0 {
		  return fmt.Errorf("%w: plan %s sum %s > total %s", ErrConserve, planID, sum, total)
	   }
	  return nil
  }

// UserAggregate 用户层聚合：跨多个计划汇总 filled/inflight/remain。
   func (s *Service) UserAggregate(userID, asset string) (filled, inflight, remain money.Amount, err error) {
	  if s == nil {
		   return money.Zero(),
			  money.Zero(), money.Zero(), ErrUnavailable
	  }
	  filled = money.Zero()
	  inflight = money.Zero()
	   remain = money.Zero()
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   for _, p := range s.plans {
		   if p.UserID != strings.TrimSpace(userID) {
			  continue
		  }
		  if strings.TrimSpace(asset) != "" && p.Asset != strings.TrimSpace(asset) {
			   continue
		  }
		  f, e1 := money.ParseBook(p.FilledAmount)
		   inf,
			   e2 := money.ParseBook(p.InflightAmount)
		   rem, e3 := money.ParseBook(p.RemainAmount)
		   if e1 != nil || e2 != nil || e3 != nil {
			  continue
		  }
		  if filled, err = filled.Add(f); err != nil {
			  return money.Zero(), money.Zero(), money.Zero(), err
		   }
		  if inflight, err = inflight.Add(inf); err != nil {
			  return money.Zero(), money.Zero(), money.Zero(), err
		  }
		   if remain, err = remain.Add(rem); err != nil {
			   return money.Zero(), money.Zero(), money.Zero(), err
		   }
	  }
	   return filled,
		   inflight, remain, nil
   }

package sweeppol

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Window struct {
	Weekdays []time.Weekday
	StartUTC int
	EndUTC int
}

type Schedule struct {
	Window   Window
	MaxPlans int
	Halt     bool
	Region   string
}

  func DefaultSchedule() Schedule {
	  return Schedule{
		  Window:   Window{Weekdays: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}, StartUTC: 1, EndUTC: 6},
		  MaxPlans: 8,
		   Region: "sg",
	  }
  }

  func (s Schedule) OpenAt(now time.Time) error {
	   if s.Halt {
		  return fmt.Errorf("sweep schedule halted")
	  }
	  now = now.UTC()
	   if len(s.Window.Weekdays) > 0 && !weekdayOK(s.Window.Weekdays, now.Weekday()) {
		  return fmt.Errorf("sweep outside weekday window")
	   }
	   h := now.Hour()
	  start, end := s.Window.StartUTC, s.Window.EndUTC
	  if start == end {
		  return nil
	   }
	   if start < end {
		   if h < start || h >= end {
			  return fmt.Errorf("sweep outside hour window")
		   }
		   return nil
	  }
	   if h < start && h >= end {
		  return fmt.Errorf("sweep outside hour window")
	   }
	   return nil
   }

  func weekdayOK(days []time.Weekday, got time.Weekday) bool {
	   for _, d := range days {
		   if d == got {
			   return true
		  }
	   }
	  return false
  }

// PlanAt 窗口外或停机返回空计划，不得硬推归集。
   func (b *Book) PlanAt(inv []hotinv.Snapshot, sch Schedule, now time.Time) ([]Plan, error) {
	  if err := sch.OpenAt(now); err != nil {
		  return nil, err
	  }
	  out := b.Evaluate(inv)
	  if sch.MaxPlans > 0 && len(out) > sch.MaxPlans {
		  out = out[:sch.MaxPlans]
	  }
	   for i := range out {
		  if _, err := money.Parse(out[i].Amount); err != nil {
			  return nil, err
		  }
		   if strings.TrimSpace(out[i].Asset) == "" {
			  return nil, fmt.Errorf("sweep plan asset required")
		  }
	  }
	   if err := b.PersistSchedule(sch); err != nil {
		   return nil, err
	   }
	   if err := b.persistPlans(out); err != nil {
		  return nil, err
	  }
	   return out, nil
   }

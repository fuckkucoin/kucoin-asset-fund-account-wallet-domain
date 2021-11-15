package solvency

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

// PutSchedule 登记或覆盖调度。窗口必须落在 1 毫秒到 24 小时之间，资产代码必须能通过校验。
   func (p *Pipeline) PutSchedule(ctx context.Context, sch Schedule) (Schedule, error) {
	   if p == nil || p.store == nil {
		  return Schedule{}, ErrUnavailable
	   }
	   sch.Name = strings.TrimSpace(sch.Name)
	   if sch.Name == "" || len(sch.Name) > 64 {
		   return Schedule{}, ErrSchedule
	   }
	  if sch.Window <= 0 || sch.Window > maxWindow {
		   return Schedule{}, ErrWindow
	  }
	  if sch.Interval < 0 || sch.Interval > 30*24*time.Hour {
		   return Schedule{}, ErrSchedule
	  }
	  assets := make([]string, 0, len(sch.Assets))
	   for _, a := range sch.Assets {
		   as, err := validate.Asset(a)
		   if err != nil {
			   return Schedule{}, ErrSchedule
		  }
		   assets = append(assets, as)
	   }
	   sch.Assets = assets
	  now := p.now()
	  if sch.ID == "" {
		   sch.ID = uuid.NewString()
		  sch.CreatedAt = now
		  if sch.NextDue.IsZero() {
			   sch.NextDue = now
		   }
	  } else if cur, err := p.store.GetSchedule(ctx, sch.ID); err == nil {
		  if sch.CreatedAt.IsZero() {
			  sch.CreatedAt = cur.CreatedAt
		  }
		   if sch.NextDue.IsZero() {
			  sch.NextDue = cur.NextDue
		   }
		  if sch.LastRunID == "" {
			  sch.LastRunID = cur.LastRunID
		  }
	  }
	   if sch.CreatedAt.IsZero() {
		   sch.CreatedAt = now
	  }
	   sch.UpdatedAt = now
	  sch.NextDue = utc(sch.NextDue)
	  if err := p.store.PutSchedule(ctx, sch); err != nil {
		   return Schedule{}, err
	  }
	   return p.store.GetSchedule(ctx, sch.ID)
  }

// RunSchedule 在 T+ 窗口内采集、并封存。窗口外的 building 不会被补封。
  func (p *Pipeline) RunSchedule(ctx context.Context, id string) (Attestation, error) {
	   sch, err := p.store.GetSchedule(ctx, id)
	  if err != nil {
		   return Attestation{}, err
	  }
	   if !sch.Enabled {
		  return Attestation{}, ErrSchedule
	   }
	   now := p.now()
	   if cur, ok, err := p.openWindow(ctx, sch, now); err != nil {
		  return Attestation{}, err
	  } else if ok {
		   return p.advance(ctx, cur)
	   }
	   att, err := p.Begin(ctx, sch)
	   if err != nil {
		  return Attestation{}, err
	  }
	  return p.advance(ctx, att)
   }

   func (p *Pipeline) openWindow(ctx context.Context, sch Schedule, now time.Time) (Attestation, bool, error) {
	  list, err := p.store.ListAttestations(ctx, sch.ID, 50)
	  if err != nil {
		  return Attestation{}, false, err
	  }
	   var best Attestation
	  found := false
	   for _, a := range list {
		  if a.ScheduleID != sch.ID {
			   continue
		  }
		  if now.Before(a.WindowStart) || now.After(a.WindowEnd) {
			  continue
		  }
		   switch a.Status {
		  case StatusBuilding, StatusSealed, StatusPublished:
		   default:
			  continue
		  }
		  if !found || a.CreatedAt.After(best.CreatedAt) {
			   best = a
			   found = true
		   }
	   }
	   return best, found, nil
   }

   func (p *Pipeline) advance(ctx context.Context, att Attestation) (Attestation, error) {
	  switch att.Status {
	   case StatusSealed, StatusPublished:
		   return att, nil
	   case StatusBuilding:
	  default:
		  return att, ErrTransition
	  }
	  var err error
	  if att.Root == "" {
		  att, err = p.Collect(ctx, att.ID)
		  if err != nil {
			  return att, err
		  }
	   }
	  if att.Coverage == "" {
		   att, err = p.Compare(ctx, att.ID)
		  if err != nil {
			   return att, err
		  }
	  }
	  if p.now().After(att.WindowEnd) {
		   _, _ = p.Abandon(ctx, att.ID, "window missed before seal")
		  return att, ErrWindow
	  }
	  return p.Seal(ctx, att.ID)
   }

   func (p *Pipeline) bump(ctx context.Context, att Attestation, now time.Time) error {
	   if att.ScheduleID == "" {
		   return nil
	   }
	  sch, err := p.store.GetSchedule(ctx, att.ScheduleID)
	  if err != nil {
		  return err
	   }
	   sch.LastRunID = att.ID
	  next := now.Add(sch.Interval)
	  if sch.Interval <= 0 || next.Before(att.WindowEnd) {
		  next = att.WindowEnd
	   }
	  sch.NextDue = next
	   sch.UpdatedAt = now
	  return p.store.PutSchedule(ctx, sch)
   }

// Tick 跑所有到期调度。单张失败不阻断其余调度，返回第一笔错误。
   func (p *Pipeline) Tick(ctx context.Context) error {
	  if p == nil || p.store == nil {
		  return ErrUnavailable
	   }
	  list, err := p.store.ListSchedules(ctx)
	   if err != nil {
		   return err
	  }
	  now := p.now()
	  var first error
	   for _, sch := range list {
		  if !sch.Enabled || sch.NextDue.After(now) {
			   continue
		   }
		   if _, err := p.RunSchedule(ctx, sch.ID); err != nil && first == nil {
			   first = err
		  }
	   }
	  return first
   }

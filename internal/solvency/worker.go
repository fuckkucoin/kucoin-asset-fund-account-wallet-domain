package solvency

import (
	"context"
	"time"
)

// Loop 按间隔跑调度、可发布档的自动发布和巡检。取消 ctx 后退出。
// 自动发布只处理 sealed 且类别为可发布档的证明单。

   func (p *Pipeline) Loop(ctx context.Context, every time.Duration) {
	  if p == nil {
		   return
	   }
	   if every <= 0 {
		   every = time.Minute
	   }
	  t := time.NewTicker(every)
	   defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-t.C:
			   _ = p.Tick(ctx)
			   _, _ = p.PublishDue(ctx)
			  _, _ = p.Patrol(ctx)
		  }
	   }
   }

// PublishDue 发布已封存、调度允许自动发布、且类别为可发布档的证明。
// building 与内控档（含短缺）不会被这条路径发布。
  func (p *Pipeline) PublishDue(ctx context.Context) (int, error) {
	  if p == nil || p.store == nil {
		  return 0, ErrUnavailable
	   }
	  list, err := p.store.ListAttestations(ctx, "", 200)
	   if err != nil {
		   return 0, err
	  }
	  n := 0
	  var first error
	   for _, att := range list {
		  if att.Status != StatusSealed || att.Class != ClassPublishable {
			   continue
		   }
		   if att.ScheduleID == "" {
			   continue
		  }
		  sch, err := p.store.GetSchedule(ctx, att.ScheduleID)
		   if err != nil || !sch.AutoPublish {
			  continue
		   }
		   if _, err := p.Publish(ctx, att.ID, false); err != nil {
			  if first == nil {
				  first = err
			  }
			  continue
		   }
		  n++
	   }
	   return n, first
  }

// Patrol 检查超时 building 与已封存根的完整性。
// 窗口结束后的 building，无论是否已经写下草稿根，都改为 abandoned，不会进入 published。
  func (p *Pipeline) Patrol(ctx context.Context) (PatrolReport, error) {
	   rep := PatrolReport{At: time.Time{}}
	  if p == nil || p.store == nil {
		   return rep, ErrUnavailable
	  }
	  now := p.now()
	  rep.At = now
	   list, err := p.store.ListAttestations(ctx, "", 200)
	  if err != nil {
		  return rep, err
	  }
	   for _, att := range list {
		  switch att.Status {
		  case StatusBuilding:
			   if !now.After(att.WindowEnd) {
				   continue
			  }
			   kind := "stale_building"
			  if att.Root != "" && att.SealDigest == "" {
				  kind = "partial_root"
			  }
			   if _, err := p.Abandon(ctx, att.ID, kind); err != nil {
				  rep.Findings = append(rep.Findings, Finding{AttestationID: att.ID, Kind: "abandon_failed", Detail: err.Error()})
				   notePatrol("abandon_failed")
				   continue
			  }
			  rep.Abandoned = append(rep.Abandoned, att.ID)
			  rep.Findings = append(rep.Findings, Finding{AttestationID: att.ID, Kind: kind})
			  notePatrol(kind)
		  case StatusSealed,
			  StatusPublished:
			   if err := p.recheck(ctx, att); err != nil {
				   rep.Findings = append(rep.Findings, Finding{AttestationID: att.ID, Kind: "integrity_fail", Detail: err.Error()})
				   notePatrol("integrity_fail")
				   continue
			  }
			   rep.Findings = append(rep.Findings, Finding{AttestationID: att.ID, Kind: "ok"})
			  notePatrol("ok")
		  }
	   }
	   return rep, nil
  }

package opsapi

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

type opsActor struct{ a perm.Actor }

  func (o opsActor) ID() string { return o.a.ID }

  func (o opsActor) CheckOps() error {
	  return o.a.Check(perm.ActOpsAdmin)
  }

// reconClearCheck 未关对账工单时挡 Clear，并暴露未关计数供 clear-check。
type reconClearCheck struct{ rt *app.Runtime }

  func (r reconClearCheck) AllowClear(ctx context.Context) error {
	  _ = ctx
	   d := r.ClearDetail(ctx)
	   if !d.Allow {
		  return custodyhalt.ErrClearBlocked
	   }
	   return nil
  }

  func (r reconClearCheck) ClearDetail(ctx context.Context) custodyhalt.ClearDetail {
	  _ = ctx
	  d := custodyhalt.ClearDetail{
		  Allow: true,
		  Policy: "human_only",
		   Host:      "custody-boundary-lab.kucoin.net",
		  ForceHint: "set force=true only after recon tickets closed and hot-wallet verified",
	  }
	  wf := reconWorkflow(r.rt)
	   agg := wf.Aggregate(time.Now().UTC())
	  d.ReconOpen = agg.Open
	   d.ReconAcked = agg.Acked
	  if agg.Open+agg.Acked > 0 {
		   d.Allow = false
		  d.Reason = custodyhalt.ErrClearBlocked.Error()
	   }
	   return d
   }

  func registerHalt(v1 *gin.RouterGroup, rt *app.Runtime, el *ha.Elector) {
	   if v1 == nil || rt == nil {
		   return
	   }
	  gate := haltGateOf(rt)
	   gate.BindClearCheck(reconClearCheck{rt: rt})
	   if rt.KV != nil {
		  custodyhalt.SharedHistory().BindAudit(custodyhalt.NewAuditStore(rt.KV))
	  }
	  custodyhalt.Mount(v1, gate, el, func(c *gin.Context) (custodyhalt.ActorChecker, bool) {
		  a, code, err := opsfc.Check(c.Request, perm.ActOpsAdmin)
		  if err != nil {
			   c.JSON(code, gin.H{"error": err.Error()})
			   return nil, false
		   }
		   return opsActor{a: a}, true
	  })
   }

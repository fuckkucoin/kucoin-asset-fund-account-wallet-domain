package opsapi

import (
	"net/http"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func haltGateOf(rt *app.Runtime) *custodyhalt.Gate {
	  if rt == nil {
		   return custodyhalt.New(nil)
	   }
	  if rt.FundSurf != nil {
		  if g := rt.FundSurf.HaltGate(); g != nil {
			   return g
		  }
	   }
	  return custodyhalt.New(rt.KV)
  }

// registerHaltStatus 挂充停提停/漂移熔断只读聚合。
   func registerHaltStatus(v1 *gin.RouterGroup, rt *app.Runtime, el *ha.Elector) {
	   if v1 == nil || rt == nil {
		  return
	  }
	   _ = el
	   v1.GET("/ops/admin/custody/status", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  gate := haltGateOf(rt)
		  // 与 Mount 共用 ClearChecker，避免读面与 clear-check 分叉。
		   gate.BindClearCheck(reconClearCheck{rt: rt})
		  ctx := c.Request.Context()
		  global, err := gate.Snapshot(ctx)
		  if err != nil {
			  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "fail_closed": true})
			   return
		  }
		  active, _ := gate.ActiveFlags(ctx)
		  // 充停/提停共用同一 custodyhalt；读失败视为应停。
		   depBlocked := global.On || len(active) > 0
		  wdBlocked := depBlocked
		  if err := gate.Check(ctx); err != nil {
			  depBlocked, wdBlocked = true, true
		   }
		  board := gate.Board(ctx, c.Query("chain"), c.Query("asset"))
		   pairs, _ := gate.ActivePairs(ctx)
		   wf := reconWorkflow(rt)
		   now := time.Now().UTC()
		  agg := wf.Aggregate(now)
		   slaBoard := wf.Board(now, 10)
		  clearDetail := gate.ProbeClear(ctx)
		   laneBlocked := map[string]bool{
			   "deposit":  board.Deposit.Blocked,
			  "withdraw": board.Withdraw.Blocked,
			   "sweep":    board.Sweep.Blocked,
		  }
		   c.JSON(http.StatusOK, gin.H{
			  "lanes":        board,
			  "lane_blocked": laneBlocked,
			  "global": global,
			  "active_scopes": active,
			   "active_pairs":    pairs,
			  "deposit_halted":  depBlocked,
			  "withdraw_halted": wdBlocked,
			  "sweep_blocked": depBlocked,
			  "recon_agg": agg,
			   "recon_sla_buckets": slaBoard.Buckets,
			   "clear_allowed":     clearDetail.Allow,
			  "clear_check":       clearDetail,
			   "history": custodyhalt.SharedHistory().Recent(20),
			  "host": "custody-boundary-lab.kucoin.net",
			   "clear_policy": "human_only",
			  "engage_allowed": true,
		  })
	  })
  }

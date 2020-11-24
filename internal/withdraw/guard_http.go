// @FileName: guard_http.go
// @Summary: 守恒、幂等柜台、刷提与人工推进的运营入口。
// 扫描漂移、查看阻断、跳闸、双人推进、只读幂等裁决。这些入口不贷记入金；live hash 拒绝留审计。把扫描接口做成解冻会绕过禁令。
// @Author: Hale
// @Created: 2020.10.29

package withdraw

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

  func registerGuard(v1 *gin.RouterGroup, svc *Service) {
	  if svc == nil {
		  svc = New(nil)
	   }
	  v1.POST("/withdrawals/conserve/scan", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, svc.ScanConserve())
	  })
	   v1.GET("/withdrawals/conserve/blocks", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  rep := svc.ScanConserve()
		  c.JSON(200, gin.H{"blocks": rep.Blocks, "alerts": rep.Alerts, "drifts": rep.Drifts})
	  })
	   v1.GET("/withdrawals/conserve/alerts", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{
			   "alerts":      []string{AlertFreezeBroadcastDrift, AlertHotIntentDrift, AlertUserAssetBlocked, AlertLiveHashDenied, AlertHalfFrozen, AlertAbuseTrip},
			  "live_denies": svc.LiveDenies(),
		   })
	   })
	   v1.POST("/withdrawals/abuse/trip", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Key string `json:"key"`
			   Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  hit, err := svc.TripAbuse(body.Key, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"hit": hit, "unfrozen": false})
	  })
	   v1.GET("/withdrawals/abuse", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"hits": svc.AbuseHits(), "prefix": "wd:"})
	  })
	  v1.POST("/withdrawals/:id/manual-advance", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   To string `json:"to"`
			  Actor string `json:"actor"`
			  Approver string `json:"approver"`
			   Reason   string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   rec, err := svc.ManualAdvance(c.Request.Context(), c.Param("id"), FreezeState(body.To), body.Actor, body.Approver, body.Reason)
		   if err != nil {
			   c.JSON(mw.ProblemOf(c, err).Status, gin.H{"error": err.Error(), "advance": rec})
			  return
		  }
		   c.JSON(200, rec)
	  })
	   v1.GET("/withdrawals/manual-advances", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"advances": svc.ManualAdvances()})
	   })
	  v1.POST("/withdrawals/idem/desk", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
			   Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			   Chain  string `json:"chain"`
			  ToAddress string `json:"to_address"`
			  Memo    string `json:"memo"`
			  TagName string `json:"tag_name"`
			   IdempotencyKey string `json:"idempotency_key"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   amt,
			  err := decimal.NewFromString(body.Amount)
		  if err != nil {
			  c.JSON(400, gin.H{"error": "amount invalid"})
			   return
		  }
		   view, err := svc.InspectIdem(c.Request.Context(), Request{
			   UserID: body.UserID, Asset: body.Asset, Amount: amt, Chain: body.Chain,
			  ToAddress: body.ToAddress, Memo: body.Memo, TagName: body.TagName, IdempotencyKey: body.IdempotencyKey,
		   })
		   if err != nil && view.Action == "" {
			  mw.WriteProblem(c, err)
			  return
		  }
		  code := 200
		  if err != nil {
			  code = 409
		   }
		   c.JSON(code, gin.H{"desk": view, "error": errString(err)})
	  })
	  v1.POST("/withdrawals/stuck/sweep", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   TTLSec int `json:"ttl_sec"`
			  Limit int `json:"limit"`
		   }
		   _ = c.ShouldBindJSON(&body)
		  ttl := time.Duration(body.TTLSec) * time.Second
		  n, err := svc.SweepStuck(time.Now().UTC(), ttl, body.Limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"released": n, "live_denies": svc.LiveDenies(), "alert": AlertLiveHashDenied})
	  })
  }

  func errString(err error) string {
	  if err == nil {
		  return ""
	  }
	   return err.Error()
  }

// MountConserveRead 给 fund-guard 挂只读面。不写账、不解冻、不挡入金。

   func MountConserveRead(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil {
		   return
	   }
	  v1.GET("/fundguard/withdraw-conserve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   body := gin.H{
			  "read_only": true,
			   "blocks_deposit_credit": false,
			   "alerts":                []string{AlertFreezeBroadcastDrift, AlertHotIntentDrift, AlertUserAssetBlocked, AlertLiveHashDenied},
			  "rules": []string{
				   "drift blocks new withdraw for the same user and asset",
				   "deposit credit is not blocked",
				   "live hash forbids auto and batch unfreeze",
				  "abuse keys use wd: prefix and do not unfreeze in-flight screening or broadcast",
			   },
		  }
		   if wd := FromRuntime(rt); wd != nil {
			  rep := wd.ScanConserve()
			   body["blocks"] = rep.Blocks
			   body["drifts"] = rep.Drifts
		  }
		   c.JSON(200, body)
	   })
   }

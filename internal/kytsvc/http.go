package kytsvc

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  screen := kyt.Screener(kyt.Static{Mode: kyt.Allow})
	   if rt != nil && rt.Cfg.KYT.Mode != "" {
		   if sc, err := kyt.NewScreener(rt.Cfg.KYT); err == nil {
			  screen = sc
		   }
	  }
	   pipe := compliance.New(screen, policy.NewRegistry(policy.DefaultV1()))
	   compliance.Mount(v1, pipe)
	   registerExtra(v1, rt, pipe)
   }

  func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, pipe *compliance.Pipeline) {
	  _ = rt
	   v1.GET("/kyt/health", func(c *gin.Context) {
		  c.JSON(200, gin.H{
			  "ok": true,
			  "host": "kyt.kucoin.net",
			   "label": "fail-closed screen",
		   })
	   })
	   v1.POST("/kyt/screen/batch", func(c *gin.Context) {
		  var body struct {
			  Items []kyt.Request `json:"items"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   if len(body.Items) == 0 || len(body.Items) > 20 {
			  c.JSON(400, gin.H{"error": "items must be 1..20"})
			  return
		  }
		   out := make([]any, 0, len(body.Items))
		   for _, it := range body.Items {
			  if _, err := validate.UserID(it.UserID); err != nil {
				   out = append(out, gin.H{"error": err.Error(), "movement_id": it.MovementID})
				  continue
			   }
			  if _, err := validate.Asset(it.Asset); err != nil {
				   out = append(out, gin.H{"error": err.Error(), "movement_id": it.MovementID})
				  continue
			   }
			  res, err := pipe.GateWithdraw(c.Request.Context(), compliance.GateReq{
				  UserID: it.UserID, MovementID: it.MovementID, Asset: it.Asset,
				   Amount: it.Amount, Chain: it.Chain, ToAddress: it.ToAddress, Allowlisted: true,
			   })
			  if err != nil && err != compliance.ErrReview {
				  out = append(out, gin.H{"error": err.Error(), "decision": res.Decision, "movement_id": it.MovementID})
				  continue
			  }
			  out = append(out, res)
		   }
		   c.JSON(200, gin.H{"results": out})
	   })
	  v1.POST("/kyt/failclosed", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			   Asset  string `json:"asset"`
			   Amount string `json:"amount"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if _, err := validate.UserID(body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  if _, err := validate.Asset(body.Asset); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   res, err := pipe.GateWithdraw(c.Request.Context(), compliance.GateReq{
			   UserID: body.UserID, MovementID: "failclosed", Asset: body.Asset,
			  Amount: body.Amount, Chain: "ETH", Allowlisted: false,
		  })
		  if err != nil && err != compliance.ErrReview {
			   c.JSON(200, gin.H{"allow": false, "error": err.Error(), "decision": res.Decision})
			  return
		   }
		  c.JSON(200, gin.H{"allow": err == nil, "decision": res.Decision, "host": "kyt.kucoin.net"})
	  })
   }

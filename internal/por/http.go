package por

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  v1.POST("/por/snapshot", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Users []string `json:"users"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if len(body.Users) == 0 {
			  uid, err := callerid.Bind(c, "", callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
			   if err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
			  body.Users = []string{uid}
		   }
		  snap, err := BuildFromStore(c.Request.Context(), rt.Store, body.Users, time.Now().UTC())
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.Header("Deprecation", "true")
		  c.Header("X-Solvency-Class", EngineeringClass)
		  c.JSON(200, gin.H{
			   "id": snap.ID, "root": snap.Root, "count": snap.Count,
			  "totals": snap.Totals, "disclaimer": snap.Disclaimer,
			  "deprecated": true, "solvency": false, "class": EngineeringClass,
			  "host": "query-gateway.kucoin.net",
		  })
	   })
	   v1.POST("/por/prove", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   Users []string `json:"users"`
			  UserID string `json:"user_id"`
			  Book string `json:"book"`
			   Asset string `json:"asset"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		   if _, err := validate.UserID(body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  if len(body.Users) == 0 {
			   body.Users = []string{body.UserID}
		  }
		  snap, err := BuildFromStore(c.Request.Context(), rt.Store, body.Users, time.Now().UTC())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   pr, err := snap.Prove(body.UserID, body.Book, body.Asset)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.Header("Deprecation", "true")
		   c.Header("X-Solvency-Class", EngineeringClass)
		   c.JSON(200, gin.H{
			   "proof": pr, "root": snap.Root, "disclaimer": Disclaimer,
			   "deprecated": true, "solvency": false, "class": EngineeringClass,
		  })
	  })
   }

package hmacfence

import (
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New(30 * time.Second)
	   secret := ""
	   if rt != nil && rt.Cfg != nil && len(rt.Cfg.Auth.HMACKeys) > 0 {
		   for _, v := range rt.Cfg.Auth.HMACKeys {
			   secret = v
			  break
		  }
	   }
	   v1.POST("/hmacfence/verify", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   KeyID string `json:"key_id"`
			   TS    string `json:"ts"`
			  Nonce string `json:"nonce"`
			  Method string `json:"method"`
			   Path   string `json:"path"`
			   Sig    string `json:"sig"`
			  Body   string `json:"body"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.TS == "" {
			   body.TS = strconv.FormatInt(time.Now().Unix(), 10)
		   }
		  if secret == "" {
			   c.JSON(503, gin.H{"error": "hmac key not configured"})
			   return
		   }
		   if err := svc.Verify(body.KeyID, secret, body.TS, body.Nonce, body.Method, body.Path, body.Sig, []byte(body.Body)); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true, "seen": svc.Seen(), "host": "iam-gateway.kucoin.net"})
	  })
  }

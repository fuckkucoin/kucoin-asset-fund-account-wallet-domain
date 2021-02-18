package backup

import (
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  store := persist.KV(persist.NewMemKV())
	   if rt != nil && rt.KV != nil {
		   store = rt.KV
	   }
	  v1.POST("/backup/put", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Kind string `json:"kind"`
			  Key  string `json:"key"`
			  Body string `json:"body"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := persist.PutNew(c.Request.Context(), store, persist.Row{Kind: body.Kind, Key: body.Key, Body: []byte(body.Body)}); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/backup/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Pass string `json:"passphrase"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  blob,
			  err := Export(c.Request.Context(), store, []byte(body.Pass))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"blob": base64.StdEncoding.EncodeToString(blob), "host": "iam-gateway.kucoin.net"})
	  })
	  v1.POST("/backup/import", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  Pass string `json:"passphrase"`
			  Blob string `json:"blob"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   raw, err := base64.StdEncoding.DecodeString(body.Blob)
		  if err != nil {
			  mw.WriteProblem(c, ErrMagic)
			   return
		   }
		  dst := persist.NewMemKV()
		   n, err := Import(c.Request.Context(), dst, []byte(body.Pass), raw)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"rows": n})
	  })
   }

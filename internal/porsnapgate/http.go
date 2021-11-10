package porsnapgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 porsnapgate HTTP 路由注册到 v1 RouterGroup。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service, reVerify *AttestReVerifyWorker) {
	   g := v1.Group("/porsnapgate")
	   // ── BuildWithGen ────────────────────────────────────────────────────────
	  g.POST("/build", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Users []string `json:"users"`
			  TTLSeconds int64 `json:"ttl_seconds"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  result, err := svc.BuildWithGen(c.Request.Context(), BuildWithGenOpts{
			  Users:      body.Users,
			   TTLSeconds: body.TTLSeconds,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, result)
	  })
	   // ── Attest ──────────────────────────────────────────────────────────────
	  g.POST("/attest", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  SnapGenID     string `json:"snap_gen_id"`
			  Requester     string `json:"requester"`
			   ExportPayload []byte `json:"export_payload"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if body.SnapGenID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": ErrNoGen.Error()})
			   return
		   }
		  a, err := svc.Attest(c.Request.Context(), AttestRequest{
			  SnapGenID: body.SnapGenID,
			   Requester: body.Requester,
			   ExportPayload: body.ExportPayload,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, a)
	   })
	  // ── PublishAttest ────────────────────────────────────────────────────────
	  g.POST("/attest/publish", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   AttestID string `json:"attest_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  a, err := svc.PublishAttest(c.Request.Context(), body.AttestID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, a)
	   })
	  // ── RevokeAttest ─────────────────────────────────────────────────────────
	  g.POST("/attest/revoke", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  AttestID string `json:"attest_id"`
			  Reason string `json:"reason"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if err := svc.RevokeAttest(c.Request.Context(), body.AttestID, body.Reason); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"revoked": true, "attest_id": body.AttestID})
	  })
	   // ── ProveAttested ────────────────────────────────────────────────────────
	  g.POST("/prove", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  SnapGenID string `json:"snap_gen_id"`
			   UserID string `json:"user_id"`
			   Book   string `json:"book"`
			  Asset  string `json:"asset"`
			   Users []string `json:"users"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.SnapGenID == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": ErrNoGen.Error()})
			  return
		   }
		  proof, err := svc.ProveAttested(c.Request.Context(), ProveAttestedOpts{
			  SnapGenID: body.SnapGenID,
			  UserID:    body.UserID,
			  Book:      body.Book,
			  Asset:     body.Asset,
			   Users: body.Users,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"proof": proof, "snap_gen_id": body.SnapGenID})
	  })
	   // ── CompareWithProof ─────────────────────────────────────────────────────
	   g.POST("/compare", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   SnapGenID string   `json:"snap_gen_id"`
			  Users     []string `json:"users"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.SnapGenID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": ErrNoGen.Error()})
			  return
		  }
		  result, err := svc.CompareWithProof(c.Request.Context(), CompareWithProofOpts{
			  SnapGenID: body.SnapGenID,
			   Users: body.Users,
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, result)
	   })
	  // ── LiveSnapshot ─────────────────────────────────────────────────────────
	  g.POST("/live", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Users []string `json:"users"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  result, err := svc.LiveSnapshot(c.Request.Context(), body.Users)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.Header("X-Por-Mode", "live")
		  c.Header("X-Por-NonAttestation", "true")
		   c.JSON(http.StatusOK, result)
	   })
	   // ── RevokeGen ────────────────────────────────────────────────────────────
	   g.POST("/gen/revoke", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  SnapGenID string `json:"snap_gen_id"`
			  Reason    string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if err := svc.RevokeGen(c.Request.Context(), body.SnapGenID, body.Reason); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"revoked": true, "snap_gen_id": body.SnapGenID})
	  })
	  // ── GetGen ───────────────────────────────────────────────────────────────
	  g.GET("/gen/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   gen, err := svc.GetGen(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gen)
	   })
	   // ── GetAttest ────────────────────────────────────────────────────────────
	  g.GET("/attest/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  a, err := svc.GetAttest(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, a)
	   })
	  // ── ListDriftProofsByGen ─────────────────────────────────────────────────
	   g.GET("/proof/:snap_gen_id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   proofs, err := svc.GetDriftProofsByGen(c.Request.Context(), c.Param("snap_gen_id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
	  })
	   // ── VerifyAttest（ops 工具）──────────────────────────────────────────────
	  if reVerify != nil {
		  g.POST("/reverify", func(c *gin.Context) {
			  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
				  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
				  return
			   }
			   var body struct {
				   AttestID string `json:"attest_id"`
			  }
			  if !mw.BindJSON(c, &body) {
				  return
			  }
			  result, err := reVerify.VerifyAttest(c.Request.Context(), body.AttestID)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
			   c.JSON(http.StatusOK, result)
		  })
	  }
	  _ = rt
   }

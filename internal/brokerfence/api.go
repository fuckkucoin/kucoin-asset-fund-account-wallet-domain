package brokerfence

import (
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// RegisterAPI 将围栏旁路引擎 API 注册到 Gin 路由组。
// 路由前缀：/v1/broker/fence

   func RegisterAPI(v1 *gin.RouterGroup, svc *Service) {
	  grp := v1.Group("/broker/fence")
	   // 围栏世代查询（只读）。
	   grp.GET("/generation", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   subID := c.Query("sub_id")
		   if subID == "" {
			   c.JSON(400, gin.H{"error": "sub_id required"})
			  return
		   }
		   resp, err := svc.QueryGeneration(subID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, resp)
	  })
	   // 激活围栏。
	   grp.POST("/activate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var req FenceReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		   lease, tok, err := svc.FenceActivate(req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{
			  "fence_lease": lease,
			   "mutex_token": tok,
		  })
	  })
	   // 申请解围。
	  grp.POST("/unfence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var req UnfenceReq
		   if !mw.BindJSON(c, &req) {
			   return
		  }
		  lease, proof, err := svc.UnfenceRequest(req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{
			  "fence_lease": lease,
			  "conserve_proof": proof,
		  })
	   })
	   // 查询围栏租约。
	  grp.GET("/lease", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  leaseID := c.Query("lease_id")
		  if leaseID == "" {
			  c.JSON(400, gin.H{"error": "lease_id required"})
			  return
		   }
		   lease, err := svc.GetLease(leaseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, lease)
	   })
	  // 颁发互斥令牌（运维手动）。
	   grp.POST("/token/issue", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   LeaseID string `json:"lease_id"`
			   SubID string `json:"sub_id"`
			   Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  tok, err := svc.IssueToken(body.LeaseID, body.SubID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, tok)
	  })
	   // 旁路案件查询。
	  grp.GET("/bypass/case", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  caseID := c.Query("case_id")
		  if caseID == "" {
			  c.JSON(400, gin.H{"error": "case_id required"})
			  return
		  }
		   bc, err := svc.GetBypassCase(caseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, bc)
	   })
	  // 列出子账未关闭旁路案件。
	  grp.GET("/bypass/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   subID := c.Query("sub_id")
		  cases, err := svc.ListOpenBypassCases(subID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"bypass_cases": cases, "count": len(cases)})
	  })
	   // 旁路案件双人确认（step=1 或 step=2）。
	   grp.POST("/bypass/confirm", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var req BypassConfirmReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		  bc, err := svc.ApproveBypassCase(req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, bc)
	   })
	   // 守恒证明导出（按 lease_id）。
	  grp.GET("/conserve/proof", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   leaseID := c.Query("lease_id")
		   if leaseID == "" {
			   c.JSON(400, gin.H{"error": "lease_id required"})
			  return
		  }
		  proof, err := svc.ExportConserveProof(leaseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, proof)
	  })
   }

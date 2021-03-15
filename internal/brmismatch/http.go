// 功能：稳定币桥失配检测与账户暂停管理的 HTTP 接口。
// http.go
// - POST /brmismatch/match 运营发起单次窗口匹配（需 OpsAdmin 权限）。
// - POST /brmismatch/suspend 暂停账户（需 OpsAdmin 权限）。
// - POST /brmismatch/clear 四眼清除（需 OpsAdmin 权限）。
// - GET /brmismatch/cases 列出案件（需 QueryRead 权限）。
// - GET /brmismatch/proof/:id 查询双边证明（需 QueryRead 权限）。
// - GET /brmismatch/suspense 查询账户暂停状态（需 QueryRead 权限）。
// 不变量：
// - 所有写操作需 OpsAdmin；读操作需 QueryRead。
// - 返回 HTTP 200 JSON 或口径 problem 形态错误。
// CreateDate: 2021.05.22

// 作者：Cason
// 日期：2021.03.06

package brmismatch

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 brmismatch HTTP 路由挂载到 v1 路由组。
   func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	  svc := New()
	  // POST /brmismatch/match — 单次窗口匹配
	   v1.POST("/brmismatch/match", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  Events []ChainEvent `json:"events"`
			   Entries []BookEntry `json:"entries"`
			   WindowKey string `json:"window_key"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   result, err := Match(body.Events, body.Entries, body.WindowKey, time.Now().UTC())
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   // 自动注册检测到的失配案件
		   registered := make([]MismatchCase, 0, len(result.Cases))
		  for _, mc := range result.Cases {
			  got, rerr := svc.RegisterCase(mc)
			   if rerr == ErrDuplicate {
				  continue
			   }
			   if rerr != nil {
				  mw.WriteProblem(c, rerr)
				  return
			   }
			  registered = append(registered, got)
		   }
		  c.JSON(200, gin.H{
			  "matched": result.Matched,
			   "mismatches": registered,
		  })
	   })
	   // POST /brmismatch/suspend — 暂停账户
	  v1.POST("/brmismatch/suspend", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body SuspendReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.SuspendAccount(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  // POST /brmismatch/clear — 四眼清除
	   v1.POST("/brmismatch/clear", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body FourEyesClearReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   got,
			   err := svc.FourEyesClear(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  // GET /brmismatch/cases — 列出案件
	  v1.GET("/brmismatch/cases", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  cases := svc.ListCases(
			  c.Query("user_id"),
			  c.Query("asset"),
			  MismatchState(c.Query("state")),
			  50,
		  )
		   c.JSON(200, gin.H{"cases": cases})
	   })
	  // GET /brmismatch/proof/:id — 查询双边证明
	   v1.GET("/brmismatch/proof/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   proof, err := svc.GetProof(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, proof)
	   })
	  // GET /brmismatch/suspense — 查询账户暂停状态
	  v1.GET("/brmismatch/suspense", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  acc, err := svc.GetSuspenseAccount(c.Query("user_id"), c.Query("asset"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, acc)
	  })
  }

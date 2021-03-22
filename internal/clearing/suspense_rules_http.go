package clearing

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterSuspenseRules 注册挂账规则建议路由。
  func RegisterSuspenseRules(v1 *gin.RouterGroup, sb *SuspenseBook, re *RulesEngine) {
	   if sb == nil {
		   sb = NewSuspenseBook()
	   }
	   if re == nil {
		  re = NewRulesEngine(DefaultRuleConfig())
	   }
	   // 所有 open 挂账的处置建议
	   v1.GET("/clearing/suspense/recommendations", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  recs := re.RecommendAll(sb)
		  c.JSON(http.StatusOK, gin.H{
			  "recommendations": recs,
			  "count":           len(recs),
		   })
	  })
	   // 单条挂账的处置建议
	  v1.GET("/clearing/suspense/:id/recommend", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  entry, err := sb.Get(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   rec := re.Recommend(entry)
		   c.JSON(http.StatusOK, gin.H{"recommendation": rec})
	   })
   }

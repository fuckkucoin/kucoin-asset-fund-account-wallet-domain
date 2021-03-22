package clearing

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterDayClose 注册日切操作台路由到 gin 路由组。
   func RegisterDayClose(v1 *gin.RouterGroup, reg *DayCloseRegistry) {
	  if reg == nil {
		  reg = NewDayCloseRegistry(nil)
	  }
	   // 开日
	   v1.POST("/clearing/day-closes", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Day string `json:"day"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  dc, err := reg.OpenDay(body.Day, actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"day_close": dc})
	  })
	  // 假日跳过登记
	   v1.POST("/clearing/day-closes/holiday", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   HolidayDay string `json:"holiday_day"`
			  NextBizDay string `json:"next_biz_day"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   actor := perm.FromRequest(c.Request, "").ID
		   dc, err := reg.OpenHolidayCatchUp(body.HolidayDay, body.NextBizDay, actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"day_close": dc})
	   })
	  // 冻结封锁窗
	   v1.POST("/clearing/day-closes/:id/freeze", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerFreeze); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Epoch int64 `json:"epoch"`
			   Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  actor := perm.FromRequest(c.Request, "").ID
		  dc, err := reg.Freeze(c.Param("id"), actor, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"day_close": dc})
	  })
	  // 关日
	  v1.POST("/clearing/day-closes/:id/close", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerFreeze); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   actor := perm.FromRequest(c.Request, "").ID
		  dc, err := reg.Close(c.Param("id"), actor, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"day_close": dc})
	  })
	  // 有限再开
	  v1.POST("/clearing/day-closes/:id/reopen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Epoch int64 `json:"epoch"`
			  Token string `json:"token"`
			  Reason string `json:"reason"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  dc, err := reg.Reopen(c.Param("id"), actor, body.Reason, body.Epoch, body.Token)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"day_close": dc})
	   })
	  // 查询单条
	  v1.GET("/clearing/day-closes/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  dc, err := reg.Get(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"day_close": dc})
	   })
	   // 列表（可按 day 过滤）
	   v1.GET("/clearing/day-closes", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  day := c.Query("day")
		   if day != "" {
			   dc, err := reg.GetByDay(day)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			   }
			   c.JSON(http.StatusOK, gin.H{"day_closes": []DayClose{dc}})
			  return
		   }
		  list := reg.List()
		  c.JSON(http.StatusOK, gin.H{"day_closes": list, "count": len(list)})
	  })
	  // 封锁状态查询
	   v1.GET("/clearing/day-closes/fence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   day := c.Query("day")
		  fenced := reg.IsFenced(day)
		  c.JSON(http.StatusOK, gin.H{"day": day, "fenced": fenced})
	  })
   }

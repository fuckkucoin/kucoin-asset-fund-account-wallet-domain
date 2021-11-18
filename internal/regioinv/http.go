package regioinv

import (
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
)

// Router 挂载区域库存域的全部路由。

  func Router(svc *Service) *gin.Engine {
	   r := gin.New()
	   r.GET("/healthz", func(c *gin.Context) {
		   c.JSON(http.StatusOK, gin.H{"service": "regio-inv", "ok": true})
	  })
	   h := &handler{svc: svc}
	  v1 := r.Group("/v1/regio")
	   // 区域管理
	   v1.POST("/regions", h.registerRegion)
	   v1.GET("/regions", h.listRegions)
	   v1.GET("/regions/:code", h.getRegion)
	   // 库存读写
	   v1.POST("/regions/:code/observe", h.observe)
	   v1.GET("/regions/:code/snapshot", h.snapshot)
	  // 水位
	   v1.POST("/marks", h.setMark)
	  v1.GET("/marks", h.listMarks)
	   v1.GET("/board", h.board)
	   v1.GET("/breaches", h.breaches)
	   // 隔离策略
	  v1.POST("/policies", h.setPolicy)
	  v1.GET("/policies/:code", h.getPolicy)
	  // 防火墙动作
	  v1.POST("/regions/:code/halt", h.engageHalt)
	   v1.POST("/regions/:code/clear", h.clearHalt)
	   v1.POST("/regions/:code/probe", h.applyProbe)
	   // 热取现防火墙校验（纯判定）
	   v1.POST("/check/hot-withdraw", h.checkHotWithdraw)
	  return r
  }

// Mount 把路由挂在 svcboot RouterGroup 上。
   func Mount(v1 *gin.RouterGroup, svc *Service) {
	  h := &handler{svc: svc}
	   g := v1.Group("/regio")
	  g.POST("/regions", h.registerRegion)
	  g.GET("/regions", h.listRegions)
	  g.GET("/regions/:code", h.getRegion)
	  g.POST("/regions/:code/observe", h.observe)
	  g.GET("/regions/:code/snapshot", h.snapshot)
	  g.POST("/marks", h.setMark)
	   g.GET("/marks", h.listMarks)
	   g.GET("/board", h.board)
	   g.GET("/breaches", h.breaches)
	   g.POST("/policies", h.setPolicy)
	  g.GET("/policies/:code", h.getPolicy)
	   g.POST("/regions/:code/halt", h.engageHalt)
	   g.POST("/regions/:code/clear", h.clearHalt)
	  g.POST("/regions/:code/probe", h.applyProbe)
	   g.POST("/check/hot-withdraw", h.checkHotWithdraw)
  }

type handler struct{ svc *Service }

   func (h *handler) registerRegion(c *gin.Context) {
	  var req RegisterReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	  }
	   r,
		  err := h.svc.RegisterRegion(c.Request.Context(), req)
	  if err != nil {
		   writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, r)
   }

  func (h *handler) listRegions(c *gin.Context) {
	  list,
		   err := h.svc.ListRegions(c.Request.Context())
	   if err != nil {
		   writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"regions": list})
   }

   func (h *handler) getRegion(c *gin.Context) {
	   r, err := h.svc.GetRegion(c.Request.Context(), c.Param("code"))
	  if err != nil {
		  writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, r)
   }

  func (h *handler) observe(c *gin.Context) {
	  var req ObserveReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	  req.RegionCode = c.Param("code")
	   snap, err := h.svc.ObserveInv(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, snap)
   }

   func (h *handler) snapshot(c *gin.Context) {
	  list, err := h.svc.Snapshot(c.Request.Context(), c.Param("code"))
	   if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"snapshot": list})
   }

  func (h *handler) setMark(c *gin.Context) {
	  var req MarkReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	   wm, err := h.svc.SetMark(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, wm)
   }

  func (h *handler) listMarks(c *gin.Context) {
	  code := c.Query("region_code")
	   r, err := h.svc.GetRegion(c.Request.Context(), code)
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   marks, err := h.svc.st.ListMarks(c.Request.Context(), r.ID)
	   if err != nil {
		   writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"marks": marks})
  }

  func (h *handler) board(c *gin.Context) {
	  b, err := h.svc.WatermarkBoard(c.Request.Context())
	   if err != nil {
		   writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, b)
   }

  func (h *handler) breaches(c *gin.Context) {
	   list, err := h.svc.InspectBreaches(c.Request.Context())
	   if err != nil {
		  writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"breaches": list})
  }

  func (h *handler) setPolicy(c *gin.Context) {
	   var req PolicyReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	   }
	   pol, err := h.svc.SetPolicy(c.Request.Context(), req)
	   if err != nil {
		   writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, pol)
   }

   func (h *handler) getPolicy(c *gin.Context) {
	  r, err := h.svc.GetRegion(c.Request.Context(), c.Param("code"))
	   if err != nil {
		   writeErr(c, err)
		  return
	  }
	  pol, err := h.svc.st.FindPolicy(c.Request.Context(), r.ID)
	   if err != nil {
		   writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, pol)
   }

   func (h *handler) engageHalt(c *gin.Context) {
	   var body struct {
		  Actor string `json:"actor"`
		  Reason string `json:"reason"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	   r, err := h.svc.EngageHalt(c.Request.Context(), c.Param("code"), body.Actor, body.Reason, time.Now().UTC())
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, r)
  }

  func (h *handler) clearHalt(c *gin.Context) {
	  var body struct {
		  Actor  string `json:"actor"`
		   Reason string `json:"reason"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	  }
	   res, err := h.svc.ClearHalt(c.Request.Context(), c.Param("code"), body.Actor, body.Reason, time.Now().UTC())
	  if err != nil {
		   writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, res)
   }

   func (h *handler) applyProbe(c *gin.Context) {
	   var rep ProbeReport
	   if err := c.ShouldBindJSON(&rep); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	  }
	  rep.RegionCode = c.Param("code")
	  r, err := h.svc.ApplyProbe(c.Request.Context(), rep, time.Now().UTC())
	  if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, r)
   }

   func (h *handler) checkHotWithdraw(c *gin.Context) {
	   var req HotWithdraw
	   if err := c.ShouldBindJSON(&req); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	   }
	  if err := h.svc.CheckHotWithdraw(c.Request.Context(), req); err != nil {
		  code := CodeOf(err)
		   c.JSON(http.StatusForbidden, gin.H{
			  "error": err.Error(),
			  "code":  code,
		  })
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"ok": true})
   }

  func writeErr(c *gin.Context, err error) {
	   code := CodeOf(err)
	  status := http.StatusInternalServerError
	  switch code {
	  case CodeNotFound:
		   status = http.StatusNotFound
	  case CodeInvalid, CodeCatalog, CodeSameRegion, CodeConserve, CodeMinLeg:
		   status = http.StatusBadRequest
	  case CodeHotBelow,
		   CodeRegionHalted, CodeBlocked, CodeCrossHot, CodeWrongRebalance,
		   CodeWindow, CodeQuota, CodeWatermark, CodeInsufficient:
		  status = http.StatusForbidden
	   }
	   c.JSON(status, gin.H{"error": err.Error(), "code": code})
  }

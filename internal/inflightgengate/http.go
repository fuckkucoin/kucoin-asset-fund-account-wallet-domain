// @FileName: http.go
// @Summary: 在途世代闸 HTTP API。
// 端点（出金广播路径）：
// POST /v1/destlife/entries/:entry_id/refs AcquireRef：签发在途世代租约
// POST /v1/destlife/refs/:lease_id/bind BindRef：绑定广播世代
// POST /v1/destlife/refs/:lease_id/release ReleaseRef：释放引用
// POST /v1/destlife/refs/:lease_id/verify-broadcast VerifyBroadcast：校验广播世代
// 端点（吊销路径）：
// POST /v1/destlife/entries/:entry_id/begin-revoking BeginRevoking：发起延迟吊销
// 端点（ops 查询）：
// GET /v1/destlife/entries/:entry_id/refs 列出活跃引用
// GET /v1/destlife/entries/:entry_id/revoke-gen 查询吊销世代
// GET /v1/destlife/refs/:lease_id 查询单个引用
// GET /v1/destlife/refs/:lease_id/proofs/:movement_id 查询在途证明
// GET /v1/destlife/drift-cases 列出未关闭漂移案例
// GET /v1/destlife/drift-cases/:case_id 查询单个漂移案例（入参 entry_id）
// POST /v1/destlife/drift-cases/:case_id/close 四眼收口漂移案例
// 设计约定：
// - 所有写路径须校验调用者 actor；只读路径允许无 actor。
// - drift-cases 收口须提供 closed_by 与 resolution；缺省返回 400。
// - 证明查询不存在时返回 404（ErrProofMissing）。
// @Author: Cason
// @Created: 2021.07.26

package inflightgengate

import (
	"errors"
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/destlife"
	"github.com/gin-gonic/gin"
)

// Register 挂载在途世代闸路由到已有 v1 路由组。
   func Register(v1 *gin.RouterGroup, gate *Gate) {
	  if v1 == nil || gate == nil {
		  return
	   }
	  dl := v1.Group("/destlife")
	  // 出金广播路径
	  dl.POST("/entries/:entry_id/refs", handleAcquireRef(gate))
	   dl.POST("/refs/:lease_id/bind", handleBindRef(gate))
	   dl.POST("/refs/:lease_id/release", handleReleaseRef(gate))
	   dl.POST("/refs/:lease_id/verify-broadcast", handleVerifyBroadcast(gate))
	  // 吊销路径
	  dl.POST("/entries/:entry_id/begin-revoking", handleBeginRevoking(gate))
	  // ops 查询
	   dl.GET("/entries/:entry_id/refs", handleListActiveRefs(gate))
	   dl.GET("/entries/:entry_id/revoke-gen", handleGetRevokeGen(gate))
	  dl.GET("/refs/:lease_id", handleGetRef(gate))
	  dl.GET("/refs/:lease_id/proofs/:movement_id", handleGetProof(gate))
	   dl.GET("/drift-cases", handleListDriftCases(gate))
	  dl.GET("/drift-cases/:case_id", handleGetDriftCase(gate))
	   dl.POST("/drift-cases/:case_id/close", handleCloseDriftCase(gate))
  }

// ─────────────────────────────────────────────────────────────────────────────
// 出金广播路径处理器
// ─────────────────────────────────────────────────────────────────────────────

  func handleAcquireRef(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id" binding:"required"`
			  Chain   string `json:"chain" binding:"required"`
			  Address string `json:"address" binding:"required"`
			   MovementID string `json:"movement_id" binding:"required"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  entryID := c.Param("entry_id")
		   resp, err := gate.AcquireRef(c.Request.Context(), AcquireRefInput{
			   EntryID: entryID,
			   UserID: body.UserID,
			   Chain:  body.Chain,
			   Address:    body.Address,
			  MovementID: body.MovementID,
		  })
		  if err != nil {
			  writeGateError(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, resp)
	  }
   }

  func handleBindRef(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   leaseID := c.Param("lease_id")
		   resp, err := gate.BindRef(c.Request.Context(), leaseID)
		   if err != nil {
			  writeGateError(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	   }
   }

  func handleReleaseRef(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  leaseID := c.Param("lease_id")
		   var body struct {
			  Actor string `json:"actor"`
		  }
		   _ = c.ShouldBindJSON(&body)
		  resp, err := gate.ReleaseRef(c.Request.Context(), leaseID, body.Actor)
		  if err != nil {
			  writeGateError(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	   }
   }

   func handleVerifyBroadcast(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   leaseID := c.Param("lease_id")
		  var body struct {
			  MovementID string `json:"movement_id" binding:"required"`
			   BroadcastTxID string `json:"broadcast_tx_id"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  resp, err := gate.VerifyBroadcast(c.Request.Context(), VerifyBroadcastInput{
			  LeaseID:    leaseID,
			  MovementID: body.MovementID,
			   BroadcastTxID: body.BroadcastTxID,
		  })
		  if err != nil {
			  writeGateError(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 吊销路径处理器
// ─────────────────────────────────────────────────────────────────────────────

  func handleBeginRevoking(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  entryID := c.Param("entry_id")
		   var body struct {
			  Actor string `json:"actor"`
		  }
		   _ = c.ShouldBindJSON(&body)
		  resp, err := gate.BeginRevoking(c.Request.Context(), BeginRevokingInput{
			   EntryID: entryID,
			   Actor:   body.Actor,
		   })
		  if err != nil {
			  writeGateError(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, resp)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// ops 查询处理器
// ─────────────────────────────────────────────────────────────────────────────

   func handleListActiveRefs(gate *Gate) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  entryID := c.Param("entry_id")
		  refs, err := gate.ListActiveRefs(c.Request.Context(), entryID)
		  if err != nil {
			   writeGateError(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"refs": refs, "count": len(refs)})
	   }
   }

  func handleGetRevokeGen(gate *Gate) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   entryID := c.Param("entry_id")
		   resp, err := gate.GetRevokeGen(c.Request.Context(), entryID)
		  if err != nil {
			   writeGateError(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	  }
  }

  func handleGetRef(gate *Gate) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  leaseID := c.Param("lease_id")
		  ref, err := gate.GetRef(c.Request.Context(), leaseID)
		  if err != nil {
			   writeGateError(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, ref)
	  }
   }

  func handleGetProof(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  leaseID := c.Param("lease_id")
		  movementID := c.Param("movement_id")
		   resp, err := gate.GetProof(c.Request.Context(), leaseID, movementID)
		   if err != nil {
			  writeGateError(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, resp)
	   }
  }

   func handleListDriftCases(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   limitStr := c.DefaultQuery("limit", "50")
		  limit, _ := strconv.Atoi(limitStr)
		  if limit <= 0 || limit > 200 {
			  limit = 50
		   }
		   cases, err := gate.ListOpenDriftCases(c.Request.Context(), limit)
		  if err != nil {
			  writeGateError(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"cases": cases, "count": len(cases)})
	  }
  }

  func handleGetDriftCase(gate *Gate) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   entryID := c.Query("entry_id")
		   if entryID == "" {
			  entryID = c.Param("case_id")
		  }
		   caseID := c.Param("case_id")
		  if entryID != caseID {
			   dc, err := gate.eng.GetOpenDriftCase(c.Request.Context(), entryID)
			  if err != nil {
				  writeGateError(c, err)
				   return
			   }
			  c.JSON(http.StatusOK, toDriftCaseResp(dc))
			  return
		  }
		  dc, err := gate.eng.GetOpenDriftCase(c.Request.Context(), entryID)
		   if err != nil {
			   writeGateError(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, toDriftCaseResp(dc))
	   }
  }

   func handleCloseDriftCase(gate *Gate) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   caseID := c.Param("case_id")
		   var body struct {
			  ClosedBy   string `json:"closed_by" binding:"required"`
			  Resolution string `json:"resolution" binding:"required"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  resp, err := gate.CloseDriftCase(c.Request.Context(), caseID, body.ClosedBy, body.Resolution)
		   if err != nil {
			  writeGateError(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, resp)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 错误映射
// ─────────────────────────────────────────────────────────────────────────────

   func writeGateError(c *gin.Context, err error) {
	   switch {
	   case errors.Is(err,
		  destlife.ErrEntryNotFound),
		  errors.Is(err, destlife.ErrLeaseNotFound),
		   errors.Is(err, destlife.ErrProofMissing):
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	  case errors.Is(err, destlife.ErrEntryRevoked),
		   errors.Is(err,
			  destlife.ErrEntryRevoking),
		  errors.Is(err, destlife.ErrStaleGen),
		   errors.Is(err, destlife.ErrDriftBlocked),
		  errors.Is(err, destlife.ErrRefConflict),
		  errors.Is(err,
			  destlife.ErrRefAlreadyReleased):
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	   case errors.Is(err, destlife.ErrMovementIDEmpty),
		   errors.Is(err, destlife.ErrEntryIDEmpty):
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	   case errors.Is(err, destlife.ErrStore):
		   c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	  case errors.Is(err, destlife.ErrInvalidState):
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	   default:
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	   }
  }

package casefencegate

import (
	"errors"
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 注册围栏引擎 HTTP 路由。
  func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	  if eng == nil {
		   return
	   }
	   g := v1.Group("/case-fence")
	   g.POST("/decide-fenced", handleDecideFenced(eng))
	   g.POST("/bare-decide", handleBareDecide(eng))
	  g.POST("/submit-check", handleSubmitCheck(eng))
	   g.POST("/expire", handleExpireStale(eng))
	   g.GET("/ticket/:id", handleGetTicket(eng))
	   g.GET("/ticket-by-movement/:movement_id", handleGetTicketByMovement(eng))
	  g.GET("/proofs", handleExportProofs(eng))
	  g.GET("/bare-alerts", handleExportBareAlerts(eng))
	   g.GET("/audit", handleAudit(eng))
   }

   func handleDecideFenced(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req DecideFencedReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  result, err := eng.DecideFencedOnly(c.Request.Context(), req)
		  if err != nil {
			  c.JSON(httpStatus(err), gin.H{"error": err.Error(), "result": result})
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"result": result})
	   }
  }

   func handleBareDecide(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req BareDecideReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  alert, err := eng.BareDecideBlock(c.Request.Context(), req)
		  if err != nil {
			  c.JSON(httpStatus(err), gin.H{"error": err.Error(), "alert": alert})
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"alert": alert})
	  }
  }

   func handleSubmitCheck(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req SubmitCheckReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   result, err := eng.SubmitCheck(c.Request.Context(), req)
		   if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error(), "result": result})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"result": result})
	   }
   }

  func handleExpireStale(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  result, err := eng.ExpireStaleTickets(c.Request.Context())
		  if err != nil {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"result": result})
	   }
   }

  func handleGetTicket(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  id := c.Param("id")
		   ticket,
			  err := eng.GetTicket(c.Request.Context(), id)
		  if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"ticket": ticket})
	  }
  }

   func handleGetTicketByMovement(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   movementID := c.Param("movement_id")
		  ticket, err := eng.GetTicketByMovement(c.Request.Context(), movementID)
		   if err != nil {
			  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"ticket": ticket})
	  }
  }

  func handleExportProofs(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req ProofExportReq
		  req.CaseID = c.Query("case_id")
		  req.MovementID = c.Query("movement_id")
		  proofs, err := eng.ExportProofs(c.Request.Context(), req)
		  if err != nil {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"proofs": proofs})
	  }
  }

   func handleExportBareAlerts(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  caseID := c.Query("case_id")
		   alerts,
			  err := eng.ExportBareAlerts(c.Request.Context(), caseID, 100)
		   if err != nil {
			  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"alerts": alerts})
	   }
   }

   func handleAudit(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  caseID := c.Query("case_id")
		  rows := eng.Audit(caseID, 200)
		  c.JSON(http.StatusOK, gin.H{"audit": rows})
	  }
  }

  func httpStatus(err error) int {
	  switch {
	  case errors.Is(err, ErrStaleFence):
		   return http.StatusConflict
	  case errors.Is(err,
		  ErrBareForbidden):
		   return http.StatusForbidden
	  case errors.Is(err,
		  ErrTicketNotFound), errors.Is(err, ErrCaseNotFound), errors.Is(err, ErrProofNotFound):
		  return http.StatusNotFound
	  case errors.Is(err, ErrTicketExpired), errors.Is(err, ErrTicketStale):
		   return http.StatusGone
	   case errors.Is(err,
		  ErrPolicyDrift):
		   return http.StatusConflict
	   case errors.Is(err, ErrMakerCheck), errors.Is(err, ErrActorSelf):
		   return http.StatusForbidden
	  case errors.Is(err, ErrInvalidDecision):
		  return http.StatusBadRequest
	  case errors.Is(err, ErrCaseNotOpen), errors.Is(err, ErrCaseAlreadyClosed), errors.Is(err, ErrExpireMutex):
		   return http.StatusConflict
	  case errors.Is(err, ErrMovementMismatch):
		  return http.StatusBadRequest
	   default:
		   return http.StatusInternalServerError
	  }
  }

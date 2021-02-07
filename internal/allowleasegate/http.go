package allowleasegate

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	   if eng == nil {
		   return
	   }
	  g := v1.Group("/allow-lease")
	   g.POST("/issue", handleIssue(eng))
	   g.POST("/submit-check", handleSubmitCheck(eng))
	  g.POST("/consume", handleConsume(eng))
	   g.POST("/revoke", handleRevoke(eng))
	   g.POST("/expire", handleExpireStale(eng))
	  g.POST("/patrol", handlePatrol(eng))
	  g.GET("/lease/:id", handleGetLease(eng))
	  g.GET("/lease-by-movement/:movement_id", handleGetLeaseByMovement(eng))
	   g.GET("/proofs", handleExportProofs(eng))
	   g.GET("/audit", handleAudit(eng))
   }

  func handleIssue(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req IssueReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  lease, err := eng.Issue(c.Request.Context(), req)
		  if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"lease": lease})
	   }
   }

   func handleSubmitCheck(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var req SubmitReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  result, err := eng.SubmitCheck(c.Request.Context(), req)
		   if err != nil {
			  status := httpStatus(err)
			  c.JSON(status, gin.H{"error": err.Error(), "result": result})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"result": result})
	  }
   }

  func handleConsume(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var body struct {
			  LeaseID string `json:"lease_id"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		   lease, err := eng.Consume(c.Request.Context(), body.LeaseID)
		  if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"lease": lease})
	  }
   }

   func handleRevoke(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var req RevokeReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  lease, err := eng.Revoke(c.Request.Context(), req)
		   if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"lease": lease})
	   }
   }

  func handleExpireStale(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   expired, err := eng.ExpireStale(c.Request.Context())
		  if err != nil {
			  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"expired": expired})
	  }
  }

  func handlePatrol(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   result, err := eng.PatrolCases(c.Request.Context())
		  if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"result": result})
	   }
   }

   func handleGetLease(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  lease, err := eng.GetLease(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"lease": lease})
	  }
   }

   func handleGetLeaseByMovement(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   lease, err := eng.GetLeaseByMovement(c.Request.Context(), c.Param("movement_id"))
		  if err != nil {
			  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"lease": lease})
	   }
  }

  func handleExportProofs(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   req := ProofExportReq{
			   MovementID: c.Query("movement_id"),
			  UserID:     c.Query("user_id"),
			   LeaseID:    c.Query("lease_id"),
		   }
		   proofs, err := eng.ExportProofs(c.Request.Context(), req)
		   if err != nil {
			  c.JSON(httpStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"proofs": proofs})
	  }
  }

   func handleAudit(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   leaseID := c.Query("lease_id")
		   movementID := c.Query("movement_id")
		   if movementID != "" {
			  c.JSON(http.StatusOK, gin.H{
				  "trail": eng.Trail().ListByMovement(movementID, 200),
			   })
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{
			   "trail": eng.Trail().List(leaseID, 200),
		  })
	   }
  }

  func httpStatus(err error) int {
	   if err == nil {
		  return http.StatusOK
	  }
	   switch err {
	   case ErrLeaseNotFound:
		   return http.StatusNotFound
	   case ErrLeaseExpired, ErrLeaseRevoked, ErrLeaseConsumed,
		  ErrNoLease, ErrCaseDenied, ErrCaseExpired, ErrCaseNotAllowed:
		   return http.StatusForbidden
	   case ErrMovementMismatch, ErrCaseMismatch, ErrPolicyMismatch,
		   ErrAlreadyBound, ErrBindConflict:
		  return http.StatusConflict
	  case ErrEngineNil, ErrStoreNil:
		  return http.StatusServiceUnavailable
	   case ErrInvalidTTL,
		   ErrLeaseIllegal:
		   return http.StatusBadRequest
	  default:
		  return http.StatusInternalServerError
	   }
  }

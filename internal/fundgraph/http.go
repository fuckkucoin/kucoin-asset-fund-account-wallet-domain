package fundgraph

import (
	"errors"
	"net/http"
	"strconv"
	"time"
	"github.com/gin-gonic/gin"
)

// Router 挂上因果图的读面和摄取面。
  func Router(svc *Service) http.Handler {
	   r := gin.New()
	   r.GET("/healthz", func(c *gin.Context) {
		  c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "fund-graph", "write_path": WritePath})
	   })
	  h := &handler{svc: svc}
	   v1 := r.Group("/v1/fund-graph")
	  v1.GET("/catalog", h.catalog)
	   v1.GET("/metrics", h.metrics)
	  v1.GET("/cursors", h.cursors)
	   v1.POST("/ingest", h.ingest)
	  v1.POST("/envelopes", h.envelopes)
	   v1.POST("/lineage", h.lineage)
	  v1.GET("/lineage", h.lineageQuery)
	  v1.GET("/nodes", h.nodes)
	   v1.GET("/nodes/:id", h.node)
	  v1.GET("/edges", h.edges)
	  v1.POST("/inspect", h.inspect)
	   v1.GET("/incidents", h.incidents)
	   v1.POST("/incidents/:id/ack", h.ack)
	   v1.POST("/incidents/:id/close", h.close)
	  v1.POST("/prune/plan", h.prunePlan)
	  v1.POST("/prune/apply", h.pruneApply)
	  return r
   }

// Listen 在 addr 上提供 HTTP。空地址使用 :8094。
   func Listen(addr string, svc *Service) error {
	  if addr == "" {
		  addr = ":8094"
	  }
	   srv := &http.Server{
		   Addr: addr,
		   Handler: Router(svc),
		   ReadHeaderTimeout: 5 * time.Second,
	   }
	  return srv.ListenAndServe()
  }

type handler struct {
	svc *Service
}

   func (h *handler) catalog(c *gin.Context) {
	  c.JSON(http.StatusOK, gin.H{
		  "edge_types": EdgeCatalog(),
		   "node_kinds": NodeCatalog(),
		   "write_path": WritePath,
	  })
   }

   func (h *handler) metrics(c *gin.Context) {
	  m, err := h.svc.Metrics(c.Request.Context())
	  if err != nil {
		  writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, m)
   }

   func (h *handler) cursors(c *gin.Context) {
	   list, err := h.svc.store.ListCursors(c.Request.Context())
	   if err != nil {
		  writeErr(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"cursors": list})
   }

   func (h *handler) ingest(c *gin.Context) {
	   var b IngestBatch
	   if err := c.ShouldBindJSON(&b); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	   res, err := h.svc.Ingest(c.Request.Context(), b)
	  if errors.Is(err, ErrConflict) {
		   c.JSON(http.StatusConflict,
			   res)
		  return
	   }
	   if err != nil {
		  writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, res)
   }

  func (h *handler) envelopes(c *gin.Context) {
	  var envs []BusEnvelope
	   if err := c.ShouldBindJSON(&envs); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	  batch, skipped, err := AdaptEnvelopes(envs)
	  if err != nil && !errors.Is(err, ErrSkip) {
		  writeErr(c, err)
		  return
	  }
	   if errors.Is(err, ErrSkip) {
		  c.JSON(http.StatusOK, gin.H{"skipped": skipped, "inserted_edges": 0})
		   return
	  }
	  res, err := h.svc.Ingest(c.Request.Context(), batch)
	   if errors.Is(err, ErrConflict) {
		   c.JSON(http.StatusConflict, res)
		   return
	   }
	   if err != nil {
		  writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "skipped":        skipped,
		  "inserted_nodes": res.InsertedNodes,
		   "inserted_edges": res.InsertedEdges,
		   "replay_edges": res.ReplayEdges,
	   })
  }

  func (h *handler) lineage(c *gin.Context) {
	  var q LineageQuery
	  if err := c.ShouldBindJSON(&q); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		  return
	  }
	  res, err := h.svc.Lineage(c.Request.Context(), q)
	   if err != nil {
		  writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, res)
  }

   func (h *handler) lineageQuery(c *gin.Context) {
	  q := LineageQuery{
		   StartNodeID: c.Query("start_node_id"),
		   StartRef:    c.Query("start_ref"),
		  StartKind: NodeKind(c.Query("start_kind")),
		  UserID:    c.Query("user_id"),
		  Asset:     c.Query("asset"),
		  Direction: c.Query("direction"),
		   TargetNodeID: c.Query("target_node_id"),
		  TargetKind:   NodeKind(c.Query("target_kind")),
		  TargetRef:    c.Query("target_ref"),
		  PageToken: c.Query("page_token"),
	   }
	   if v := c.Query("max_hops"); v != "" {
		   n, err := strconv.Atoi(v)
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "max_hops"})
			  return
		   }
		  q.MaxHops = n
	   }
	   if v := c.Query("limit"); v != "" {
		  n, err := strconv.Atoi(v)
		  if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "limit"})
			  return
		   }
		  q.Limit = n
	  }
	  res, err := h.svc.Lineage(c.Request.Context(), q)
	  if err != nil {
		  writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, res)
   }

  func (h *handler) nodes(c *gin.Context) {
	  f := NodeFilter{
		  UserID: c.Query("user_id"),
		  Asset:  c.Query("asset"),
		  Ref:    c.Query("ref"),
		  Kind: NodeKind(c.Query("kind")),
	   }
	   if v := c.Query("limit"); v != "" {
		  n, err := strconv.Atoi(v)
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "limit"})
			   return
		  }
		  f.Limit = n
	   }
	   list, err := h.svc.Nodes(c.Request.Context(), f)
	   if err != nil {
		  writeErr(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"nodes": list})
   }

   func (h *handler) node(c *gin.Context) {
	  n, err := h.svc.Node(c.Request.Context(), c.Param("id"))
	   if err != nil {
		   writeErr(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, n)
   }

   func (h *handler) edges(c *gin.Context) {
	   f := EdgeFilter{
		   Ref:    c.Query("ref"),
		  FromID: c.Query("from_id"),
		  ToID:     c.Query("to_id"),
		   EdgeType: EdgeType(c.Query("edge_type")),
		   UserID:   c.Query("user_id"),
		  Asset: c.Query("asset"),
	   }
	   if v := c.Query("limit"); v != "" {
		   n, err := strconv.Atoi(v)
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "limit"})
			  return
		  }
		   f.Limit = n
	   }
	  list, err := h.svc.Edges(c.Request.Context(), f)
	  if err != nil {
		   writeErr(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"edges": list})
   }

   func (h *handler) inspect(c *gin.Context) {
	  rep, err := h.svc.Inspect(c.Request.Context())
	  if err != nil {
		  writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, rep)
   }

   func (h *handler) incidents(c *gin.Context) {
	  f := IncidentFilter{
		  Status: c.Query("status"),
		  Kind: c.Query("kind"),
		   Ref: c.Query("ref"),
	   }
	   list, err := h.svc.Incidents(c.Request.Context(), f)
	   if err != nil {
		   writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"incidents": list})
  }

  func (h *handler) ack(c *gin.Context) {
	   if err := h.svc.AckIncident(c.Request.Context(), c.Param("id")); err != nil {
		  writeErr(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"status": StatusAck})
   }

   func (h *handler) close(c *gin.Context) {
	   if err := h.svc.CloseIncident(c.Request.Context(), c.Param("id")); err != nil {
		  writeErr(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"status": StatusClosed})
   }

   func (h *handler) prunePlan(c *gin.Context) {
	  plan, err := h.svc.PlanPrune(c.Request.Context())
	  if err != nil {
		   writeErr(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, plan)
  }

  func (h *handler) pruneApply(c *gin.Context) {
	  var plan PrunePlan
	   if err := c.ShouldBindJSON(&plan); err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		   return
	  }
	  res, err := h.svc.ApplyPrune(c.Request.Context(), plan)
	   if err != nil {
		   writeErr(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, res)
   }

  func writeErr(c *gin.Context, err error) {
	   switch {
	   case errors.Is(err, ErrNotFound):
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrConflict):
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	  case errors.Is(err,
		   ErrDeleteDisabled):
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrInvalid), errors.Is(err, ErrPage), errors.Is(err, ErrStatus), errors.Is(err, ErrConfirm):
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	   default:
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	  }
   }

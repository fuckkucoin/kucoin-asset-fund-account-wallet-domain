package fundcircuit

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/config"
	"github.com/gin-gonic/gin"
)

// 运营读面主机名取风控门户，与限流/风控值班同一入口。

const opsHost = config.HostRiskPortal

// LeaderChecker 选主约束；实现由 ha.Elector 提供。

type LeaderChecker interface {
	RequireLeader() error
}

// ActorChecker 运营鉴权。
type ActorChecker interface {
	ID() string
	CheckOps() error
}

type actorFunc func(*gin.Context) (ActorChecker, bool)

// Mount 挂熔断总线运营接口。写操作要求运营权限；解除还要求双人票齐。
   func Mount(v1 *gin.RouterGroup, bus *Bus, leader LeaderChecker, actorFn actorFunc) {
	   if v1 == nil {
		   return
	  }
	   if bus == nil {
		  bus = New()
	  }
	  if actorFn == nil {
		  actorFn = func(*gin.Context) (ActorChecker, bool) { return nil, false }
	  }
	  h := &handler{bus: bus, leader: leader, actorFn: actorFn}
	  v1.GET("/ops/fundcircuit/stats", h.stats)
	  v1.GET("/ops/fundcircuit/policy", h.getPolicy)
	  v1.POST("/ops/fundcircuit/policy", h.putPolicy)
	   v1.GET("/ops/fundcircuit/matrix", h.matrix)
	  v1.GET("/ops/fundcircuit/circuits", h.circuits)
	   v1.GET("/ops/fundcircuit/circuit", h.circuit)
	  v1.GET("/ops/fundcircuit/board", h.board)
	   v1.GET("/ops/fundcircuit/signals", h.listSignals)
	  v1.POST("/ops/fundcircuit/signals", h.postSignal)
	   v1.POST("/ops/fundcircuit/force-open", h.forceOpen)
	  v1.POST("/ops/fundcircuit/clear", h.singleClear)
	   v1.POST("/ops/fundcircuit/clear/request", h.clearRequest)
	  v1.POST("/ops/fundcircuit/clear/vote", h.clearVote)
	   v1.POST("/ops/fundcircuit/clear/execute", h.clearExecute)
	  v1.GET("/ops/fundcircuit/clear/pending", h.clearPending)
	   v1.GET("/ops/fundcircuit/cases", h.cases)
	   v1.POST("/ops/fundcircuit/cases/review", h.caseReview)
	   v1.GET("/ops/fundcircuit/probes", h.probes)
	   v1.GET("/ops/fundcircuit/probe-config", h.probeConfig)
	  v1.POST("/ops/fundcircuit/probe-config", h.putProbeConfig)
	   v1.POST("/ops/fundcircuit/probes/issue", h.probeIssue)
	   v1.POST("/ops/fundcircuit/probes/settle", h.probeSettle)
	  v1.GET("/ops/fundcircuit/unfreeze-check", h.unfreezeCheck)
	   v1.GET("/ops/fundcircuit/audit", h.audit)
	  v1.POST("/ops/fundcircuit/tick", h.tick)
   }

type handler struct {
	bus *Bus
	leader  LeaderChecker
	actorFn actorFunc
}

// actor 取运营身份并校验权限。

  func (h *handler) actor(c *gin.Context) (ActorChecker, bool) {
	   a, ok := h.actorFn(c)
	   if !ok {
		  return nil, false
	   }
	  if a == nil {
		  c.JSON(http.StatusUnauthorized, gin.H{"error": ErrActor.Error()})
		   return nil, false
	  }
	  if err := a.CheckOps(); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return nil, false
	   }
	   return a, true
   }

// writeActor 写操作：运营权限 + 选主。

  func (h *handler) writeActor(c *gin.Context) (ActorChecker, bool) {
	  a, ok := h.actor(c)
	   if !ok {
		  return nil, false
	  }
	   if h.leader != nil {
		  if err := h.leader.RequireLeader(); err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			   return nil, false
		   }
	   }
	   if strings.TrimSpace(a.ID()) == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": ErrActor.Error()})
		   return nil, false
	  }
	   return a, true
  }

   func fail(c *gin.Context, err error) {
	   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "host": opsHost, "auto_unfreeze": false})
  }

   func (h *handler) stats(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "stats": h.bus.Stats(), "sql_bound": h.bus.SQLBound(),
		   "keyspace": Prefix, "host": opsHost, "auto_unfreeze": false,
	   })
  }

  func (h *handler) getPolicy(c *gin.Context) {
	   if _, ok := h.actor(c); !ok {
		  return
	  }
	  if v := strings.TrimSpace(c.Query("version")); v != "" {
		   n, err := strconv.Atoi(v)
		  if err != nil {
			  fail(c, ErrPolicyVersion)
			   return
		   }
		  doc, gerr := h.bus.Policy().Get(n)
		  if gerr != nil {
			  fail(c, gerr)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"policy": policyView(doc), "host": opsHost})
		   return
	  }
	  doc := h.bus.Policy().Current()
	  c.JSON(http.StatusOK, gin.H{
		  "policy": policyView(doc), "versions": h.bus.Policy().Versions(),
		   "matrix": doc.MatrixView(), "host": opsHost, "auto_unfreeze": false,
	  })
   }

// policyBody 策略落版入参。时长用秒，避免运营手填纳秒。

type policyBody struct {
	WindowSec    int `json:"window_sec"`
	SignalTTLSec int `json:"signal_ttl_sec"`
	WatchScore int `json:"watch_score"`
	WarnScore  int `json:"warn_score"`
	TripScore  int `json:"trip_score"`
	HaltScore   int `json:"halt_score"`
	CooldownSec int `json:"cooldown_sec"`
	CooldownMaxSec int `json:"cooldown_max_sec"`
	HalfOpenProbes int `json:"half_open_probes"`
	ProbeTTLSec    int `json:"probe_ttl_sec"`
	CrossPathNum   int `json:"cross_path_num"`
	CrossPathDen   int `json:"cross_path_den"`
	PathFloor int               `json:"path_floor"`
	Matrix    map[string]string `json:"matrix"`
	ScopeGate map[string]bool   `json:"scope_gate"`
	ScopeMinUsers map[string]int `json:"scope_min_users"`
	AutoUnfreeze bool `json:"auto_unfreeze"`
	EscalateHalt bool `json:"escalate_halt"`
	Note string `json:"note"`
}

// toPolicy 把入参折成策略文档。未给的字段沿用当前代次。
   func (b policyBody) toPolicy(cur BlockPolicy) (BlockPolicy, error) {
	  doc := cur
	  doc.Note = strings.TrimSpace(b.Note)
	  doc.AutoUnfreeze = b.AutoUnfreeze
	  doc.EscalateHalt = b.EscalateHalt
	   if b.WindowSec > 0 {
		  doc.Window = time.Duration(b.WindowSec) * time.Second
	  }
	   if b.SignalTTLSec > 0 {
		   doc.SignalTTL = time.Duration(b.SignalTTLSec) * time.Second
	  }
	   if b.WatchScore > 0 {
		   doc.WatchScore = b.WatchScore
	  }
	   if b.WarnScore > 0 {
		  doc.WarnScore = b.WarnScore
	   }
	   if b.TripScore > 0 {
		  doc.TripScore = b.TripScore
	   }
	  if b.HaltScore > 0 {
		   doc.HaltScore = b.HaltScore
	  }
	  if b.CooldownSec > 0 {
		  doc.Cooldown = time.Duration(b.CooldownSec) * time.Second
	  }
	   if b.CooldownMaxSec > 0 {
		  doc.CooldownMax = time.Duration(b.CooldownMaxSec) * time.Second
	   }
	   if b.HalfOpenProbes > 0 {
		   doc.HalfOpenProbes = b.HalfOpenProbes
	  }
	   if b.ProbeTTLSec > 0 {
		   doc.ProbeTTL = time.Duration(b.ProbeTTLSec) * time.Second
	  }
	   if b.CrossPathNum > 0 {
		   doc.CrossPathNum = b.CrossPathNum
	   }
	   if b.CrossPathDen > 0 {
		   doc.CrossPathDen = b.CrossPathDen
	   }
	   if b.PathFloor > 0 {
		   doc.PathFloor = b.PathFloor
	  }
	   if len(b.Matrix) > 0 {
		   m := map[Path]Severity{}
		  for k, v := range b.Matrix {
			  path, err := ParsePath(k)
			  if err != nil {
				   return BlockPolicy{}, err
			  }
			   if strings.EqualFold(strings.TrimSpace(v), "off") || strings.TrimSpace(v) == SevNone.String() {
				  continue
			  }
			  sev, err := ParseSeverity(v)
			   if err != nil {
				   return BlockPolicy{}, err
			  }
			  m[path] = sev
		  }
		  doc.Matrix = m
	   }
	  if len(b.ScopeGate) > 0 {
		   g := map[Scope]bool{}
		  for k, v := range b.ScopeGate {
			  scope, err := ParseScope(k)
			   if err != nil {
				   return BlockPolicy{}, err
			  }
			   g[scope] = v
		   }
		  doc.ScopeGate = g
	  }
	  if len(b.ScopeMinUsers) > 0 {
		  m := map[Scope]int{}
		   for k, v := range b.ScopeMinUsers {
			   scope, err := ParseScope(k)
			  if err != nil {
				  return BlockPolicy{}, err
			   }
			  m[scope] = v
		   }
		   doc.ScopeMinUsers = m
	   }
	  return doc, nil
  }

  func (h *handler) putPolicy(c *gin.Context) {
	  a, ok := h.writeActor(c)
	   if !ok {
		  return
	  }
	   var body policyBody
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, ErrPolicy)
		  return
	  }
	  doc, err := body.toPolicy(h.bus.Policy().Current())
	   if err != nil {
		   fail(c, err)
		   return
	   }
	  saved, err := h.bus.Policy().Put(a.ID(), doc)
	   if err != nil {
		   fail(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "policy": policyView(saved), "matrix": saved.MatrixView(),
		   "actor": a.ID(), "host": opsHost, "auto_unfreeze": false,
	   })
  }

   func (h *handler) matrix(c *gin.Context) {
	   if _, ok := h.actor(c); !ok {
		   return
	   }
	  doc := h.bus.Policy().Current()
	   c.JSON(http.StatusOK, gin.H{
		  "matrix": doc.MatrixView(), "policy_ver": doc.Version, "scopes": Scopes(),
		   "protected_inflight_states": InflightStates(), "auto_unfreeze": UnfreezeAllowed(),
		  "host": opsHost,
	   })
  }

   func (h *handler) circuits(c *gin.Context) {
	   if _, ok := h.actor(c); !ok {
		  return
	  }
	  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	   c.JSON(http.StatusOK, gin.H{
		   "circuits": h.bus.Circuits().List(limit), "active": h.bus.Circuits().Active(),
		  "stats": h.bus.Circuits().Stats(), "host": opsHost, "auto_unfreeze": false,
	  })
  }

  func (h *handler) circuit(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		  return
	  }
	   scope, err := ParseScope(c.Query("scope"))
	   if err != nil {
		   fail(c, err)
		  return
	  }
	  key, err := ScopeKey(scope, c.Query("user"), c.Query("chain"), c.Query("asset"))
	  if err != nil {
		  fail(c, err)
		   return
	   }
	   row := h.bus.Circuits().Snapshot(key)
	   probe,
		  hasProbe := h.bus.Probes().InFlight(key)
	  body := gin.H{
		  "circuit": row, "key": key, "case": nil, "probe": nil,
		   "host": opsHost, "auto_unfreeze": false,
	  }
	   if cs, ok := h.bus.Cases().OpenFor(key); ok {
		  body["case"] = cs
	   }
	   if hasProbe {
		   body["probe"] = probe
	   }
	  c.JSON(http.StatusOK, body)
  }

   func (h *handler) board(c *gin.Context) {
	   if _, ok := h.actor(c); !ok {
		   return
	  }
	   board := h.bus.Board(c.Request.Context(), c.Query("user"), c.Query("chain"), c.Query("asset"))
	   c.JSON(http.StatusOK, gin.H{"board": board, "host": opsHost})
   }

   func (h *handler) listSignals(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		  return
	   }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  key := strings.TrimSpace(c.Query("key"))
	   if key != "" {
		  if err := GuardKey(key); err != nil {
			   fail(c, err)
			   return
		  }
		  win := h.bus.Policy().Current().Window
		  rows := h.bus.Signals().Window(key, win)
		  agg := Score(key, rows, h.bus.Policy().Current(), time.Time{})
		   c.JSON(http.StatusOK, gin.H{"signals": rows, "aggregate": agg, "host": opsHost})
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "signals": h.bus.Signals().Recent(limit), "stats": h.bus.Signals().Stats(),
		   "kinds": SignalKinds(), "host": opsHost,
	   })
   }

   func (h *handler) postSignal(c *gin.Context) {
	  a, ok := h.writeActor(c)
	   if !ok {
		   return
	   }
	   var body struct {
		   Kind string `json:"kind"`
		  Path string `json:"path"`
		   UserID string `json:"user_id"`
		  Chain  string `json:"chain"`
		  Asset  string `json:"asset"`
		  Weight int    `json:"weight"`
		   SourceKey string `json:"source_key"`
		   Note string `json:"note"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, ErrSignal)
		   return
	   }
	   kind, err := ParseSignalKind(body.Kind)
	  if err != nil {
		  fail(c, err)
		   return
	  }
	   sig := AbuseSignal{
		   Kind: kind, UserID: body.UserID, Chain: body.Chain, Asset: body.Asset,
		  Weight: body.Weight, SourceKey: body.SourceKey,
		  Note: strings.TrimSpace(body.Note) + " (actor " + a.ID() + ")",
	  }
	  if strings.TrimSpace(body.Path) != "" {
		  path, perr := ParsePath(body.Path)
		  if perr != nil {
			  fail(c, perr)
			  return
		   }
		   sig.Path = path
	  }
	   row, trs, err := h.bus.Observe(c.Request.Context(), sig)
	   if err != nil {
		  fail(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{
		  "signal": row, "transitions": trs, "actor": a.ID(), "host": opsHost,
	   })
   }

  func (h *handler) forceOpen(c *gin.Context) {
	   a, ok := h.writeActor(c)
	  if !ok {
		   return
	   }
	   var body struct {
		  Scope string `json:"scope"`
		  UserID string `json:"user_id"`
		   Chain  string `json:"chain"`
		   Asset  string `json:"asset"`
		  Reason string `json:"reason"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, ErrReason)
		  return
	   }
	   scope, err := ParseScope(body.Scope)
	  if err != nil {
		  fail(c, err)
		  return
	  }
	  row, err := h.bus.Desk().ForceOpen(c.Request.Context(), a.ID(), scope, body.UserID, body.Chain, body.Asset, body.Reason)
	  if err != nil {
		   fail(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "circuit": row, "actor": a.ID(), "action": "force_open",
		  "auto_unfreeze": false, "inflight_preserved": true, "host": opsHost,
	   })
   }

// singleClear 单人解除入口：恒拒绝，提示走双控三步。

   func (h *handler) singleClear(c *gin.Context) {
	  a, ok := h.writeActor(c)
	   if !ok {
		   return
	  }
	   var body struct {
		  Key string `json:"key"`
		  Reason string `json:"reason"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, ErrReason)
		  return
	  }
	   err := h.bus.Desk().ForceClear(c.Request.Context(), a.ID(), body.Key, body.Reason)
	   if err == nil {
		   // 不应出现：单人解除必须失败。
		  fail(c, ErrDualControl)
		  return
	   }
	   c.JSON(apierr.HTTPStatus(err), gin.H{
		  "error": err.Error(), "actor": a.ID(), "need": h.bus.Desk().Need(),
		  "hint": "POST /ops/fundcircuit/clear/request then vote by another desk, then execute",
		   "host": opsHost, "auto_unfreeze": false,
	   })
   }

   func (h *handler) clearRequest(c *gin.Context) {
	   a, ok := h.writeActor(c)
	  if !ok {
		  return
	  }
	   var body struct {
		  Key string `json:"key"`
		   Reason string `json:"reason"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, ErrReason)
		  return
	   }
	   req, err := h.bus.Desk().OpenClear(c.Request.Context(), a.ID(), body.Key, body.Reason)
	   if err != nil {
		  fail(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "request": req, "need": h.bus.Desk().Need(), "actor": a.ID(), "host": opsHost,
	   })
   }

   func (h *handler) clearVote(c *gin.Context) {
	   a, ok := h.writeActor(c)
	   if !ok {
		   return
	   }
	   var body struct {
		   ReqID string `json:"req_id"`
		  Allow bool   `json:"allow"`
		  Epoch int64  `json:"epoch"`
		  Token string `json:"token"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, ErrDualControl)
		   return
	  }
	   req, err := h.bus.Desk().VoteClear(c.Request.Context(), body.ReqID, a.ID(), body.Allow, body.Epoch, body.Token)
	   if err != nil {
		  fail(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"request": req, "actor": a.ID(), "host": opsHost})
   }

  func (h *handler) clearExecute(c *gin.Context) {
	   a, ok := h.writeActor(c)
	   if !ok {
		  return
	   }
	  var body struct {
		  ReqID string `json:"req_id"`
		   Epoch int64 `json:"epoch"`
		   Token string `json:"token"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, ErrDualControl)
		   return
	  }
	  row, cs, err := h.bus.Desk().ExecuteClear(c.Request.Context(), body.ReqID, a.ID(), body.Epoch, body.Token)
	   if err != nil {
		  fail(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "circuit": row, "case": cs, "actor": a.ID(), "action": "clear_executed",
		  "auto_unfreeze": false, "inflight_preserved": true, "host": opsHost,
	  })
  }

  func (h *handler) clearPending(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		   return
	   }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   c.JSON(http.StatusOK, gin.H{
		   "pending": h.bus.Desk().PendingClears(limit), "need": h.bus.Desk().Need(), "host": opsHost,
	   })
   }

  func (h *handler) cases(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		  return
	  }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  state := CaseState(strings.TrimSpace(c.Query("state")))
	   c.JSON(http.StatusOK, gin.H{
		   "cases": h.bus.Cases().List(state, limit), "stats": h.bus.Cases().Stats(), "host": opsHost,
	  })
   }

   func (h *handler) caseReview(c *gin.Context) {
	   a, ok := h.writeActor(c)
	   if !ok {
		   return
	   }
	   var body struct {
		   CaseID string `json:"case_id"`
		  Note string `json:"note"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, ErrCaseMissing)
		   return
	   }
	   cs,
		  err := h.bus.Cases().Review(body.CaseID, a.ID(), body.Note)
	  if err != nil {
		   fail(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"case": cs, "actor": a.ID(), "host": opsHost})
  }

  func (h *handler) probes(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		  return
	   }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	   c.JSON(http.StatusOK, gin.H{
		  "probes": h.bus.Probes().List(limit), "stats": h.bus.Probes().Stats(), "host": opsHost,
	  })
   }

  func (h *handler) probeConfig(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		   return
	  }
	  doc := h.bus.Policy().Current()
	  c.JSON(http.StatusOK, gin.H{
		  "half_open_probes": doc.HalfOpenProbes, "probe_ttl_sec": int(doc.ProbeTTL / time.Second),
		   "cooldown_sec": int(doc.Cooldown / time.Second), "cooldown_max_sec": int(doc.CooldownMax / time.Second),
		   "policy_ver": doc.Version, "stats": h.bus.Probes().Stats(), "host": opsHost,
	   })
   }

// putProbeConfig 只调探针相关字段，其余沿用当前代次，落成新策略代次。
  func (h *handler) putProbeConfig(c *gin.Context) {
	  a, ok := h.writeActor(c)
	   if !ok {
		   return
	   }
	  var body struct {
		  HalfOpenProbes int `json:"half_open_probes"`
		   ProbeTTLSec    int `json:"probe_ttl_sec"`
		   CooldownSec    int `json:"cooldown_sec"`
		   CooldownMaxSec int `json:"cooldown_max_sec"`
		   Note string `json:"note"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, ErrPolicy)
		  return
	  }
	  doc := h.bus.Policy().Current()
	   if body.HalfOpenProbes > 0 {
		  doc.HalfOpenProbes = body.HalfOpenProbes
	  }
	   if body.ProbeTTLSec > 0 {
		  doc.ProbeTTL = time.Duration(body.ProbeTTLSec) * time.Second
	  }
	   if body.CooldownSec > 0 {
		   doc.Cooldown = time.Duration(body.CooldownSec) * time.Second
	   }
	   if body.CooldownMaxSec > 0 {
		   doc.CooldownMax = time.Duration(body.CooldownMaxSec) * time.Second
	   }
	   if n := strings.TrimSpace(body.Note); n != "" {
		  doc.Note = n
	  }
	  saved, err := h.bus.Policy().Put(a.ID(), doc)
	   if err != nil {
		   fail(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "half_open_probes": saved.HalfOpenProbes, "probe_ttl_sec": int(saved.ProbeTTL / time.Second),
		  "policy_ver": saved.Version, "actor": a.ID(), "host": opsHost,
	  })
  }

   func (h *handler) probeIssue(c *gin.Context) {
	  a, ok := h.writeActor(c)
	   if !ok {
		   return
	   }
	  var body struct {
		  Key string `json:"key"`
		   Path string `json:"path"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, ErrProbeState)
		  return
	   }
	   path, err := ParsePath(body.Path)
	   if err != nil {
		  fail(c, err)
		  return
	   }
	  if err := GuardKey(body.Key); err != nil {
		  fail(c, err)
		  return
	  }
	  p, err := h.bus.Probe(c.Request.Context(), strings.TrimSpace(body.Key), path, a.ID())
	   if err != nil {
		   fail(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"probe": p, "actor": a.ID(), "host": opsHost})
  }

   func (h *handler) probeSettle(c *gin.Context) {
	  a, ok := h.writeActor(c)
	  if !ok {
		  return
	  }
	   var body struct {
		  ProbeID string `json:"probe_id"`
		  Token string `json:"token"`
		   Pass  bool   `json:"pass"`
		   Note string `json:"note"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, ErrProbeState)
		  return
	   }
	   p, row, err := h.bus.SettleProbe(c.Request.Context(), body.ProbeID, body.Token, body.Pass, a.ID(), body.Note)
	   if err != nil {
		  fail(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "probe": p, "circuit": row, "actor": a.ID(),
		   "auto_unfreeze": false, "inflight_preserved": true, "host": opsHost,
	  })
   }

// unfreezeCheck 读面自检：证明在途相位一律拒绝解冻。

   func (h *handler) unfreezeCheck(c *gin.Context) {
	   if _, ok := h.actor(c); !ok {
		   return
	  }
	   ref := InflightRef{
		  TransferID: strings.TrimSpace(c.Query("transfer_id")),
		   UserID:      strings.TrimSpace(c.Query("user")),
		  Asset:       strings.TrimSpace(c.Query("asset")),
		  FreezeState: strings.TrimSpace(c.Query("freeze_state")),
		  Broadcast:   strings.TrimSpace(c.Query("broadcast")),
		  TxHash: strings.TrimSpace(c.Query("tx_hash")),
	   }
	   err := h.bus.AdmitUnfreeze(c.Request.Context(), ref)
	   body := gin.H{
		   "ref": ref, "auto_unfreeze": false, "refused": err != nil,
		  "protected_inflight_states": InflightStates(), "host": opsHost,
	  }
	   if err != nil {
		   body["error"] = err.Error()
	   }
	  c.JSON(http.StatusOK, body)
   }

   func (h *handler) audit(c *gin.Context) {
	  if _, ok := h.actor(c); !ok {
		   return
	   }
	   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	  c.JSON(http.StatusOK, gin.H{
		   "events": h.bus.Desk().Audit(limit), "need": h.bus.Desk().Need(), "host": opsHost,
	  })
  }

// tick 手工跑一轮作业，便于事故期立即推进冷却与聚合。
   func (h *handler) tick(c *gin.Context) {
	   a, ok := h.writeActor(c)
	  if !ok {
		   return
	   }
	   rep := h.bus.Tick(c.Request.Context())
	  c.JSON(http.StatusOK, gin.H{"report": rep, "actor": a.ID(), "host": opsHost})
  }

// policyView 策略读面：时长换成秒，便于运营核对。

  func policyView(doc BlockPolicy) gin.H {
	   matrix := map[string]string{}
	  for path, sev := range doc.Matrix {
		  matrix[path.String()] = sev.String()
	   }
	  gate := map[string]bool{}
	  for scope, on := range doc.ScopeGate {
		  gate[scope.String()] = on
	  }
	   breadth := map[string]int{}
	  for scope, n := range doc.ScopeMinUsers {
		   breadth[scope.String()] = n
	   }
	   return gin.H{
		  "version": doc.Version, "window_sec": int(doc.Window / time.Second),
		  "signal_ttl_sec": int(doc.SignalTTL / time.Second),
		   "watch_score":    doc.WatchScore, "warn_score": doc.WarnScore,
		  "trip_score": doc.TripScore, "halt_score": doc.HaltScore,
		  "cooldown_sec": int(doc.Cooldown / time.Second), "cooldown_max_sec": int(doc.CooldownMax / time.Second),
		   "half_open_probes": doc.HalfOpenProbes, "probe_ttl_sec": int(doc.ProbeTTL / time.Second),
		  "cross_path_num": doc.CrossPathNum, "cross_path_den": doc.CrossPathDen,
		  "path_floor": doc.PathFloor,
		   "matrix": matrix, "scope_gate": gate, "scope_min_users": breadth, "auto_unfreeze": false,
		   "escalate_halt": doc.EscalateHalt, "note": doc.Note,
		   "actor": doc.Actor, "updated_at": doc.UpdatedAt,
	  }
   }

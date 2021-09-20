package outeffect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

// ────────────────────────────────────────────────────────────────────────────
// Handler 聚合体
// ────────────────────────────────────────────────────────────────────────────
// DeskAPI 聚合所有 desk/ops HTTP handler。
type DeskAPI struct {
	store fxonce.Store
	dupSvc    *fxonce.DupProofService
	engine    *fxonce.CASEngine
	policyMgr *AuditedPolicyManager
}

// NewDeskAPI 构造 DeskAPI。
   func NewDeskAPI(
	  store fxonce.Store,
	  dupSvc *fxonce.DupProofService,
	  engine *fxonce.CASEngine,
	  policyMgr *AuditedPolicyManager,
  ) *DeskAPI {
	   return &DeskAPI{
		  store: store,
		   dupSvc: dupSvc,
		  engine: engine,
		   policyMgr: policyMgr,
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 效果查询
// ────────────────────────────────────────────────────────────────────────────
// ListEffects GET /internal/fund-effects?business_key=&limit=&offset=

  func (d *DeskAPI) ListEffects(w http.ResponseWriter, r *http.Request) {
	  businessKey := r.URL.Query().Get("business_key")
	   limit := queryInt(r, "limit", 20)
	  offset := queryInt(r, "offset", 0)
	   effects,
		   err := d.store.ListEffects(r.Context(), businessKey, limit, offset)
	  if err != nil {
		  writeError(w, http.StatusInternalServerError, fmt.Sprintf("list effects: %v", err))
		  return
	  }
	  writeJSON(w, http.StatusOK, map[string]interface{}{
		  "effects": effects,
		  "count": len(effects),
		  "limit":  limit,
		  "offset": offset,
	  })
   }

// GetEffectByBusiness GET /internal/fund-effects/:business_key

  func (d *DeskAPI) GetEffectByBusiness(w http.ResponseWriter, r *http.Request) {
	  businessKey := pathLast(r.URL.Path)
	  if businessKey == "" {
		  writeError(w, http.StatusBadRequest, "business_key required")
		  return
	  }
	   effects, err := d.store.GetEffectsByBusiness(r.Context(), businessKey)
	   if err != nil {
		  writeError(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	  writeJSON(w, http.StatusOK, map[string]interface{}{"effects": effects, "count": len(effects)})
  }

// ────────────────────────────────────────────────────────────────────────────
// 双花证明
// ────────────────────────────────────────────────────────────────────────────
// GetDupProof GET /internal/dup-proofs/:id
   func (d *DeskAPI) GetDupProof(w http.ResponseWriter, r *http.Request) {
	  id := pathLast(r.URL.Path)
	  if id == "" {
		   writeError(w, http.StatusBadRequest, "dup proof id required")
		  return
	  }
	  dp, err := d.dupSvc.Get(r.Context(), id)
	   if err == fxonce.ErrEffectNotFound {
		  writeError(w, http.StatusNotFound, "dup proof not found")
		   return
	   }
	   if err == fxonce.ErrWindowExpired {
		   writeJSON(w, http.StatusOK, map[string]interface{}{"dup_proof": dp, "warning": "beyond retention window"})
		   return
	   }
	  if err != nil {
		   writeError(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusOK, map[string]interface{}{"dup_proof": dp})
  }

// ListUnreviewedDups GET /internal/dup-proofs/unreviewed?limit=

  func (d *DeskAPI) ListUnreviewedDups(w http.ResponseWriter, r *http.Request) {
	   limit := queryInt(r, "limit", 50)
	  dups, err := d.dupSvc.ListUnreviewed(r.Context(), limit)
	  if err != nil {
		   writeError(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusOK, map[string]interface{}{"dup_proofs": dups, "count": len(dups)})
   }

// ListDupProofsByBusiness GET /internal/fund-effects/:business_key/dup-proofs?limit=
  func (d *DeskAPI) ListDupProofsByBusiness(w http.ResponseWriter, r *http.Request) {
	   // URL: /internal/fund-effects/{business_key}/dup-proofs
	   parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	  var businessKey string
	  for i, p := range parts {
		   if p == "fund-effects" && i+1 < len(parts) {
			   businessKey = parts[i+1]
			  break
		  }
	   }
	   if businessKey == "" {
		  writeError(w, http.StatusBadRequest, "business_key required")
		  return
	  }
	   limit := queryInt(r, "limit", 20)
	   dups, err := d.dupSvc.ListForBusiness(r.Context(), businessKey, limit)
	  if err != nil {
		   writeError(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	   summary, _ := d.dupSvc.Summarize(r.Context(), businessKey)
	  writeJSON(w, http.StatusOK, map[string]interface{}{
		  "dup_proofs": dups,
		  "count": len(dups),
		  "summary": summary,
	   })
  }

// ReviewDupProof POST /internal/dup-proofs/:id/review

   func (d *DeskAPI) ReviewDupProof(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		   writeError(w, http.StatusMethodNotAllowed, "POST required")
		  return
	   }
	   id := pathSegment(r.URL.Path, -2)
	  actor := r.Header.Get("X-Actor")
	   if actor == "" {
		   writeError(w, http.StatusBadRequest, "X-Actor header required")
		   return
	   }
	   if err := d.dupSvc.MarkReviewed(r.Context(), id, actor); err != nil {
		  if err == fxonce.ErrEffectNotFound {
			  writeError(w, http.StatusNotFound, "dup proof not found")
			  return
		   }
		  writeError(w, http.StatusInternalServerError, err.Error())
		   return
	   }
	  writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id, "reviewed_by": actor})
   }

// ────────────────────────────────────────────────────────────────────────────
// Kind 策略
// ────────────────────────────────────────────────────────────────────────────
// ListKindPolicies GET /internal/kind-policies

  func (d *DeskAPI) ListKindPolicies(w http.ResponseWriter, r *http.Request) {
	   policies := d.policyMgr.inner.ListPolicies()
	  audits := d.policyMgr.ListAudits(20)
	  writeJSON(w, http.StatusOK, map[string]interface{}{
		  "policies": policies,
		   "count":    len(policies),
		  "recent_edits": audits,
	  })
  }

// GetKindPolicy GET /internal/kind-policies/:kind
  func (d *DeskAPI) GetKindPolicy(w http.ResponseWriter, r *http.Request) {
	   kind := pathLast(r.URL.Path)
	  if kind == "" {
		  writeError(w, http.StatusBadRequest, "kind required")
		  return
	  }
	  p := d.policyMgr.inner.GetPolicy(kind)
	  writeJSON(w, http.StatusOK, map[string]interface{}{"policy": p})
   }

// UpdateKindPolicy PUT /internal/kind-policies/:kind

   func (d *DeskAPI) UpdateKindPolicy(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPut {
		   writeError(w, http.StatusMethodNotAllowed, "PUT required")
		  return
	  }
	  kind := pathLast(r.URL.Path)
	   actor := r.Header.Get("X-Actor")
	  if actor == "" {
		   writeError(w, http.StatusBadRequest, "X-Actor header required")
		  return
	  }
	   var body struct {
		  GateRequired bool `json:"gate_required"`
		  Blocked  bool   `json:"blocked"`
		  MaxRetry int    `json:"max_retry"`
		  Memo     string `json:"memo"`
	  }
	   if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		  writeError(w, http.StatusBadRequest, fmt.Sprintf("decode body: %v", err))
		  return
	   }
	   p := &fxonce.KindPolicy{
		  Kind:         kind,
		  GateRequired: body.GateRequired,
		  Blocked:      body.Blocked,
		   MaxRetry: body.MaxRetry,
		  Memo:     body.Memo,
	  }
	   if err := d.policyMgr.SetPolicy(r.Context(), p, actor); err != nil {
		  writeError(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "kind": kind, "policy": p})
   }

// ────────────────────────────────────────────────────────────────────────────
// 强制重放
// ────────────────────────────────────────────────────────────────────────────
// ReplayCheck GET /internal/fund-effects/:id/replay-check
   func (d *DeskAPI) ReplayCheck(w http.ResponseWriter, r *http.Request) {
	   effectID := pathSegment(r.URL.Path, -2)
	   if effectID == "" {
		  writeError(w, http.StatusBadRequest, "effect id required")
		   return
	  }
	  dupSvc := fxonce.NewDupProofService(d.store)
	  result, err := dupSvc.CheckForceReplay(r.Context(), effectID)
	  if err != nil {
		  writeError(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusOK, map[string]interface{}{"check": result})
   }

// ForceReplay POST /internal/fund-effects/:id/force-replay
// 需要 X-Actor 和 X-Actor2 头（双人授权）。

  func (d *DeskAPI) ForceReplay(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		   writeError(w, http.StatusMethodNotAllowed, "POST required")
		  return
	   }
	   effectID := pathSegment(r.URL.Path,
		   -2)
	   actor1 := r.Header.Get("X-Actor")
	  actor2 := r.Header.Get("X-Actor2")
	   if actor1 == "" || actor2 == "" {
		   writeError(w, http.StatusBadRequest, "X-Actor and X-Actor2 headers required for dual approval")
		  return
	   }
	  if actor1 == actor2 {
		   writeError(w, http.StatusBadRequest, "X-Actor and X-Actor2 must be different persons")
		  return
	  }
	  actors := actor1 + "+" + actor2
	   if err := d.engine.ForceReplay(r.Context(), effectID, actors); err != nil {
		   if err == fxonce.ErrForceReplayDenied {
			   writeError(w, http.StatusForbidden, "force replay denied: "+err.Error())
			  return
		  }
		  if err == fxonce.ErrEffectConflict {
			   writeError(w, http.StatusConflict, "effect is not in poisoned state")
			   return
		  }
		   if err == fxonce.ErrEffectNotFound {
			   writeError(w, http.StatusNotFound, "effect not found")
			  return
		   }
		  writeError(w, http.StatusInternalServerError, err.Error())
		  return
	   }
	   writeJSON(w, http.StatusOK, map[string]interface{}{
		  "ok":        true,
		  "effect_id": effectID,
		  "actors":    actors,
	   })
   }

// ────────────────────────────────────────────────────────────────────────────
// HTTP 工具
// ────────────────────────────────────────────────────────────────────────────

  func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	   w.Header().Set("Content-Type", "application/json")
	   w.WriteHeader(code)
	   _ = json.NewEncoder(w).Encode(v)
   }

   func writeError(w http.ResponseWriter, code int, msg string) {
	   writeJSON(w, code, map[string]interface{}{"error": msg, "code": code})
  }

  func queryInt(r *http.Request, key string, def int) int {
	  s := r.URL.Query().Get(key)
	  if s == "" {
		  return def
	  }
	   n, err := strconv.Atoi(s)
	  if err != nil || n < 0 {
		  return def
	  }
	  return n
   }

// pathLast 取 URL path 最后一段（按 / 分割）。
  func pathLast(path string) string {
	  parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	  if len(parts) == 0 {
		   return ""
	   }
	   return parts[len(parts)-1]
   }

// pathSegment 按索引取 path 分段（负数从尾部计）。
   func pathSegment(path string, idx int) string {
	   parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/"), "/")
	  if idx < 0 {
		   idx = len(parts) + idx
	  }
	   if idx < 0 || idx >= len(parts) {
		  return ""
	  }
	   return parts[idx]
   }

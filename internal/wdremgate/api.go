package wdremgate

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// Handler HTTP 处理器。
type Handler struct {
	gate *RemReleaseGate
}

// NewHandler 构造 Handler。
   func NewHandler(gate *RemReleaseGate) *Handler {
	  return &Handler{gate: gate}
  }

// RegisterRoutes 注册路由到 ServeMux。
  func (h *Handler) RegisterRoutes(mux *http.ServeMux, prefix string) {
	  mux.HandleFunc(prefix+"/hold/open", h.handleHoldOpen)
	  mux.HandleFunc(prefix+"/lease/issue", h.handleIssueLease)
	   mux.HandleFunc(prefix+"/release", h.handleRelease)
	   mux.HandleFunc(prefix+"/fill/confirm", h.handleConfirmFill)
	  mux.HandleFunc(prefix+"/conserve/check", h.handleConserveCheck)
	  mux.HandleFunc(prefix+"/conserve/", h.handleConserveDetail)
	  mux.HandleFunc(prefix+"/force-release", h.handleForceRelease)
	  mux.HandleFunc(prefix+"/guard/invalidate", h.handleInvalidateGuard)
   }

// ──────────────────────────────────────────────
// Hold 开立
// ──────────────────────────────────────────────

   func (h *Handler) handleHoldOpen(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		   writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	  }
	   var req GateOpenHoldReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		  return
	  }
	  if req.PlanID == "" || req.LegID == "" {
		   writeErr(w, http.StatusBadRequest, "plan_id and leg_id required")
		  return
	   }
	   hold, err := h.gate.OpenHold(req)
	   if err != nil {
		  writeErr(w, http.StatusBadRequest, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusCreated, hold)
   }

// ──────────────────────────────────────────────
// 租约颁发
// ──────────────────────────────────────────────

   func (h *Handler) handleIssueLease(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		   writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	   }
	  var req GateIssueLeaseReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		   return
	  }
	   if req.PlanID == "" || req.UserID == "" {
		   writeErr(w, http.StatusBadRequest, "plan_id and user_id required")
		  return
	  }
	  resp, err := h.gate.IssueLease(req)
	   if err != nil {
		  writeErr(w,
			   http.StatusConflict, err.Error())
		  return
	   }
	   writeJSON(w, http.StatusCreated, resp)
  }

// ──────────────────────────────────────────────
// 带世代剩余释放
// ──────────────────────────────────────────────

  func (h *Handler) handleRelease(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	   }
	  var req GateReleaseReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		   writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		   return
	  }
	  if req.PlanID == "" || req.LeaseID == "" {
		   writeErr(w, http.StatusBadRequest, "plan_id and lease_id required")
		  return
	  }
	  resp, err := h.gate.Release(req)
	   if err != nil {
		  code := http.StatusBadRequest
		   if isConflictErr(err) {
			   code = http.StatusConflict
		  }
		  writeErr(w, code, err.Error())
		   return
	  }
	   writeJSON(w, http.StatusOK, resp)
  }

// ──────────────────────────────────────────────
// Fill 确认
// ──────────────────────────────────────────────

   func (h *Handler) handleConfirmFill(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	  }
	   var req wdpartial.ConfirmFillReq
	  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		   writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		  return
	  }
	   if req.FillID == "" {
		   writeErr(w, http.StatusBadRequest, "fill_id required")
		   return
	  }
	   fill, err := h.gate.ConfirmFill(req)
	  if err != nil {
		   writeErr(w, http.StatusBadRequest, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusOK, fill)
  }

// ──────────────────────────────────────────────
// 守恒校验与案件管理
// ──────────────────────────────────────────────

  func (h *Handler) handleConserveCheck(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	  }
	  var req GateConserveReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		  return
	   }
	   if req.PlanID == "" {
		   writeErr(w, http.StatusBadRequest, "plan_id required")
		   return
	  }
	   resp, err := h.gate.Conserve(req)
	  if err != nil {
		  if errors.Is(err, wdpartial.ErrConservationFail) {
			   w.Header().Set("Content-Type", "application/json")
			   w.WriteHeader(http.StatusUnprocessableEntity)
			   _ = json.NewEncoder(w).Encode(resp)
			   return
		  }
		  writeErr(w, http.StatusInternalServerError, err.Error())
		   return
	   }
	   writeJSON(w, http.StatusOK, resp)
  }

  func (h *Handler) handleConserveDetail(w http.ResponseWriter, r *http.Request) {
	   path := strings.TrimSuffix(r.URL.Path, "/")
	  parts := strings.Split(path, "/")
	  var caseID, action string
	   for i, p := range parts {
		   if p == "conserve" && i+1 < len(parts) {
			   caseID = parts[i+1]
			   if i+2 < len(parts) {
				   action = parts[i+2]
			   }
			  break
		   }
	   }
	   if caseID == "" {
		  writeErr(w, http.StatusBadRequest, "missing case_id")
		   return
	  }
	   switch action {
	  case "prove":
		  h.handleConserveProve(w, r, caseID)
	   case "close":
		  h.handleConserveClose(w,
			  r, caseID)
	   default:
		   if r.Method != http.MethodGet {
			  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			   return
		  }
		  resp, err := h.gate.ExportConserveCase(caseID)
		   if err != nil {
			   writeErr(w, http.StatusNotFound, err.Error())
			  return
		   }
		   writeJSON(w, http.StatusOK, resp)
	  }
  }

  func (h *Handler) handleConserveProve(w http.ResponseWriter, r *http.Request, caseID string) {
	   if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	   }
	   var req GateProveConserveReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		   return
	  }
	   req.CaseID = caseID
	  if req.ProofPayload == "" || req.Approver == "" || req.Approver2 == "" {
		  writeErr(w, http.StatusBadRequest, "proof_payload, approver, approver2 required")
		  return
	   }
	   c, err := h.gate.ProveConserve(req)
	  if err != nil {
		   writeErr(w,
			  http.StatusBadRequest, err.Error())
		   return
	   }
	   writeJSON(w, http.StatusOK, c)
   }

  func (h *Handler) handleConserveClose(w http.ResponseWriter, r *http.Request, caseID string) {
	   if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		   return
	   }
	   var body struct {
		   Approver string `json:"approver"`
	  }
	   if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		   return
	  }
	   c,
		   err := h.gate.CloseConserve(caseID, body.Approver)
	  if err != nil {
		  writeErr(w, http.StatusBadRequest, err.Error())
		   return
	   }
	   writeJSON(w, http.StatusOK, c)
   }

// ──────────────────────────────────────────────
// 强制释放
// ──────────────────────────────────────────────

   func (h *Handler) handleForceRelease(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		   return
	   }
	  var req GateForceReleaseReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		   writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		   return
	   }
	  if req.PlanID == "" || req.CaseID == "" || req.Approver == "" || req.Approver2 == "" || req.ProofPayload == "" {
		   writeErr(w, http.StatusBadRequest, "plan_id, case_id, proof_payload, approver, approver2 required")
		  return
	  }
	  resp, err := h.gate.ForceRelease(req)
	  if err != nil {
		  writeErr(w, http.StatusBadRequest, err.Error())
		   return
	  }
	  writeJSON(w, http.StatusOK, resp)
   }

// ──────────────────────────────────────────────
// 守卫世代管理
// ──────────────────────────────────────────────

  func (h *Handler) handleInvalidateGuard(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	   }
	  var req GateInvalidateGuardReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		   return
	   }
	   if req.GuardGenID == "" {
		  writeErr(w, http.StatusBadRequest, "guard_gen_id required")
		  return
	  }
	  g, err := h.gate.InvalidateGuard(req)
	  if err != nil {
		   writeErr(w, http.StatusBadRequest, err.Error())
		   return
	  }
	  writeJSON(w, http.StatusOK, g)
  }

// ──────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────

  func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	   w.Header().Set("Content-Type", "application/json")
	   w.WriteHeader(code)
	  _ = json.NewEncoder(w).Encode(v)
   }

  func writeErr(w http.ResponseWriter, code int, msg string) {
	   writeJSON(w, code, map[string]string{"error": msg})
   }

   func isConflictErr(err error) bool {
	   if err == nil {
		  return false
	  }
	  return errors.Is(err,
		  wdpartial.ErrLeaseConflict) ||
		   errors.Is(err, wdpartial.ErrGuardGenInvalid) ||
		   errors.Is(err, wdpartial.ErrConserveCaseOpen)
   }

package tradepwgen

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Handler HTTP 处理器（挂载到 Gin/ServeMux 路由前缀下）。

type Handler struct {
	svc *Service
}

// NewHandler 构造 Handler。
   func NewHandler(svc *Service) *Handler {
	   return &Handler{svc: svc}
   }

// RegisterRoutes 挂载路由到口径 ServeMux。
// prefix 例如 "/internal/tradepwgen"。

   func (h *Handler) RegisterRoutes(mux *http.ServeMux, prefix string) {
	   prefix = strings.TrimRight(prefix, "/")
	   mux.HandleFunc(prefix+"/generation/", h.handleGeneration)
	   mux.HandleFunc(prefix+"/lease/", h.handleLease)
	  mux.HandleFunc(prefix+"/drift", h.handleDriftList)
	  mux.HandleFunc(prefix+"/drift/", h.handleDriftDetail)
  }

// ──────────────────────────────────────────────
// 世代查询
// ──────────────────────────────────────────────

   func (h *Handler) handleGeneration(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodGet {
		   writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		   return
	  }
	  userID := lastPathSeg(r.URL.Path)
	  if userID == "" {
		   writeErr(w, http.StatusBadRequest, "user_id required")
		   return
	   }
	  resp, err := h.svc.QueryGeneration(userID)
	   if err != nil {
		  writeErr(w, http.StatusInternalServerError, err.Error())
		  return
	  }
	   writeJSON(w, http.StatusOK, resp)
  }

// ──────────────────────────────────────────────
// 核验令牌查询
// ──────────────────────────────────────────────

  func (h *Handler) handleLease(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodGet {
		   writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		  return
	   }
	   leaseID := lastPathSeg(r.URL.Path)
	  if leaseID == "" {
		  writeErr(w, http.StatusBadRequest, "lease_id required")
		   return
	  }
	   lease, err := h.svc.GetVerifyLease(leaseID)
	   if err != nil {
		  if errors.Is(err, ErrLeaseNotFound) {
			  writeErr(w, http.StatusNotFound, "lease not found")
			   return
		   }
		  writeErr(w, http.StatusInternalServerError, err.Error())
		   return
	  }
	   writeJSON(w, http.StatusOK, lease)
  }

// ──────────────────────────────────────────────
// 漂移案件列表
// ──────────────────────────────────────────────

   func (h *Handler) handleDriftList(w http.ResponseWriter,
	   r *http.Request) {
	  if r.Method != http.MethodGet {
		  writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		   return
	  }
	   limitStr := r.URL.Query().Get("limit")
	  limit := 50
	   if limitStr != "" {
		   if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			   limit = n
		  }
	  }
	  cases, err := h.svc.ListOpenDriftCases(limit)
	   if err != nil {
		  writeErr(w, http.StatusInternalServerError, err.Error())
		   return
	  }
	   writeJSON(w, http.StatusOK, map[string]interface{}{
		   "cases": cases,
		   "count": len(cases),
	  })
   }

// ──────────────────────────────────────────────
// 漂移案件详情 / 操作
// ──────────────────────────────────────────────

  func (h *Handler) handleDriftDetail(w http.ResponseWriter, r *http.Request) {
	  // 路由拆分：/drift/:case_id、/drift/:case_id/prove、/drift/:case_id/close、/drift/:case_id/force_close
	  parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	   // 取 drift 之后的段。
	  driftIdx := -1
	   for i, p := range parts {
		   if p == "drift" {
			  driftIdx = i
			  break
		  }
	   }
	   if driftIdx < 0 || driftIdx+1 >= len(parts) {
		   writeErr(w, http.StatusBadRequest, "case_id required")
		  return
	  }
	   caseID := parts[driftIdx+1]
	   action := ""
	   if driftIdx+2 < len(parts) {
		  action = parts[driftIdx+2]
	  }
	   switch {
	  case r.Method == http.MethodGet && action == "":
		  h.getDriftCase(w, r, caseID)
	   case r.Method == http.MethodPost && action == "prove":
		  h.proveCase(w, r, caseID)
	   case r.Method == http.MethodPost && action == "close":
		   h.closeCase(w, r, caseID)
	   case r.Method == http.MethodPost && action == "force_close":
		  h.forceCloseCase(w, r, caseID)
	  default:
		  writeErr(w, http.StatusMethodNotAllowed, "method or action not allowed")
	   }
  }

  func (h *Handler) getDriftCase(w http.ResponseWriter, _ *http.Request, caseID string) {
	  dc,
		   err := h.svc.GetDriftCase(caseID)
	  if err != nil {
		  writeErr(w, http.StatusInternalServerError, err.Error())
		   return
	   }
	   if dc == nil {
		  writeErr(w, http.StatusNotFound, "drift case not found")
		  return
	  }
	   writeJSON(w, http.StatusOK, dc)
  }

type proveReqBody struct {
	ProofPayload string `json:"proof_payload"`
	Approver  string `json:"approver"`
	Approver2 string `json:"approver2"`
}

   func (h *Handler) proveCase(w http.ResponseWriter, r *http.Request, caseID string) {
	   var body proveReqBody
	   if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid request body")
		   return
	   }
	  dc, err := h.svc.ProveGenDrift(ProveGenDriftReq{
		  CaseID:       caseID,
		   ProofPayload: body.ProofPayload,
		  Approver: body.Approver,
		   Approver2: body.Approver2,
	  })
	   if err != nil {
		   statusCode := http.StatusInternalServerError
		  if errors.Is(err, ErrInvalidStatus) || errors.Is(err, ErrDriftAlreadyClosed) {
			   statusCode = http.StatusConflict
		   }
		   writeErr(w, statusCode, err.Error())
		   return
	   }
	  writeJSON(w, http.StatusOK, dc)
  }

type closeReqBody struct {
	Approver string `json:"approver"`
}

  func (h *Handler) closeCase(w http.ResponseWriter, r *http.Request, caseID string) {
	  var body closeReqBody
	   if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid request body")
		   return
	  }
	  dc, err := h.svc.CloseGenDrift(CloseDriftReq{
		  CaseID: caseID,
		  Approver: body.Approver,
	   })
	  if err != nil {
		  statusCode := http.StatusInternalServerError
		  if errors.Is(err, ErrDriftNotProven) || errors.Is(err, ErrDriftAlreadyClosed) || errors.Is(err, ErrInvalidStatus) {
			   statusCode = http.StatusConflict
		  }
		  writeErr(w,
			  statusCode, err.Error())
		   return
	  }
	  writeJSON(w, http.StatusOK, dc)
  }

type forceCloseReqBody struct {
	Approver string `json:"approver"`
	Proof    string `json:"proof"`
}

  func (h *Handler) forceCloseCase(w http.ResponseWriter, r *http.Request, caseID string) {
	  var body forceCloseReqBody
	   if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		  writeErr(w, http.StatusBadRequest, "invalid request body")
		   return
	  }
	   dc, err := h.svc.ForceCloseGenDrift(CloseDriftReq{
		   CaseID:   caseID,
		   Approver: body.Approver,
	   }, body.Proof)
	   if err != nil {
		   statusCode := http.StatusInternalServerError
		   if errors.Is(err, ErrDriftAlreadyClosed) || errors.Is(err, ErrInvalidStatus) {
			  statusCode = http.StatusConflict
		  }
		  writeErr(w, statusCode, err.Error())
		  return
	  }
	  writeJSON(w, http.StatusOK, dc)
   }

// ──────────────────────────────────────────────
// 工具函数
// ──────────────────────────────────────────────

   func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	  w.Header().Set("Content-Type", "application/json; charset=utf-8")
	  w.WriteHeader(status)
	  _ = json.NewEncoder(w).Encode(v)
   }

   func writeErr(w http.ResponseWriter, status int, msg string) {
	   writeJSON(w, status, map[string]string{"error": msg})
  }

  func lastPathSeg(path string) string {
	  path = strings.TrimRight(path, "/")
	  idx := strings.LastIndex(path, "/")
	  if idx < 0 {
		  return path
	  }
	   return path[idx+1:]
   }

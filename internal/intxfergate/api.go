package intxfergate

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/shopspring/decimal"
)

// ─── 响应结构 ─────────────────────────────────────────────────────────────

type apiResp struct {
	Code int `json:"code"`
	Message string `json:"message"`
	Data interface{} `json:"data,omitempty"`
}

  func writeJSON(w http.ResponseWriter, status int, payload apiResp) {
	   w.Header().Set("Content-Type", "application/json; charset=utf-8")
	  w.WriteHeader(status)
	  _ = json.NewEncoder(w).Encode(payload)
   }

   func ok(w http.ResponseWriter, data interface{}) {
	  writeJSON(w, http.StatusOK, apiResp{Code: 0, Message: "ok", Data: data})
  }

   func fail(w http.ResponseWriter, status, code int, msg string) {
	   writeJSON(w, status, apiResp{Code: code, Message: msg})
   }

// ─── Handler ──────────────────────────────────────────────────────────────
// Handler 持有 Engine 的 HTTP 处理器集合。

type Handler struct {
	engine *Engine
}

// NewHandler 创建 Handler。
  func NewHandler(engine *Engine) *Handler {
	   return &Handler{engine: engine}
   }

// Register 向 ServeMux 注册所有路由。

  func (h *Handler) Register(mux *http.ServeMux) {
	   mux.HandleFunc("/intxfergate/snap", h.handleSnap)
	  mux.HandleFunc("/intxfergate/proof", h.handleProof)
	  mux.HandleFunc("/intxfergate/misroutes", h.handleMisroutes)
	  mux.HandleFunc("/intxfergate/misroutes/confirm", h.handleConfirmMisroute)
	  mux.HandleFunc("/intxfergate/send", h.handleSend)
   }

// ─── GET /intxfergate/snap ────────────────────────────────────────────────
// snapView 快照响应视图。

type snapView struct {
	ID         string `json:"id"`
	FromUserID string `json:"from_user_id"`
	ToUserID string `json:"to_user_id"`
	Channel  string `json:"channel"`
	Channels string `json:"channels"`
	IdemKey  string `json:"idem_key"`
	Gen int64 `json:"gen"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	State  string `json:"state"`
	ResolvedAt string `json:"resolved_at"`
	ExpiresAt  string `json:"expires_at"`
}

   func (h *Handler) handleSnap(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodGet {
		   fail(w, http.StatusMethodNotAllowed, 405, "method not allowed")
		   return
	   }
	   idemKey := strings.TrimSpace(r.URL.Query().Get("idem_key"))
	   if idemKey == "" {
		  fail(w, http.StatusBadRequest, 400, "idem_key required")
		  return
	  }
	  snap,
		  err := h.engine.GetSnap(r.Context(), idemKey)
	  if err != nil {
		   if errors.Is(err, fundint.ErrSnapNotFound) {
			   fail(w, http.StatusNotFound, 404, "snap not found")
			   return
		   }
		   fail(w, http.StatusInternalServerError, 500, "internal error")
		   return
	   }
	  ok(w, snapView{
		  ID: snap.ID,
		   FromUserID: snap.FromUserID,
		  ToUserID:   snap.ToUserID,
		  Channel:    snap.Channel,
		   Channels: snap.Channels,
		   IdemKey: snap.IdemKey,
		   Gen:   snap.Gen,
		   Asset: snap.Asset,
		   Amount: snap.Amount,
		   State: string(snap.State),
		  ResolvedAt: snap.ResolvedAt.Format("2006-01-02T15:04:05.000Z"),
		  ExpiresAt:  snap.ExpiresAt.Format("2006-01-02T15:04:05.000Z"),
	  })
  }

// ─── GET /intxfergate/proof ───────────────────────────────────────────────
// proofView 效果证明响应视图。

type proofView struct {
	ID string `json:"id"`
	SnapID string `json:"snap_id"`
	LeaseID string `json:"lease_id"`
	LedgerID string `json:"ledger_id"`
	FromUserID string `json:"from_user_id"`
	ToUserID   string `json:"to_user_id"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`
	IdemKey    string `json:"idem_key"`
	ProofHash  string `json:"proof_hash"`
	SealedAt   string `json:"sealed_at"`
	FenceEpoch int64  `json:"fence_epoch"`
}

  func (h *Handler) handleProof(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodGet {
		   fail(w, http.StatusMethodNotAllowed, 405, "method not allowed")
		   return
	  }
	  snapID := strings.TrimSpace(r.URL.Query().Get("snap_id"))
	  ledgerID := strings.TrimSpace(r.URL.Query().Get("ledger_id"))
	   if snapID == "" || ledgerID == "" {
		  fail(w, http.StatusBadRequest, 400, "snap_id and ledger_id required")
		   return
	   }
	   proof,
		   err := h.engine.ExportProof(r.Context(), snapID, ledgerID)
	   if err != nil {
		   if errors.Is(err, ErrNotFound) {
			   fail(w, http.StatusNotFound, 404, "proof not found")
			   return
		   }
		   fail(w, http.StatusInternalServerError, 500, err.Error())
		  return
	   }
	   ok(w, proofView{
		  ID: proof.ID,
		  SnapID: proof.SnapID,
		   LeaseID: proof.LeaseID,
		   LedgerID:   proof.LedgerID,
		   FromUserID: proof.FromUserID,
		  ToUserID:   proof.ToUserID,
		   Asset: proof.Asset,
		   Amount:    proof.Amount,
		   IdemKey:   proof.IdemKey,
		  ProofHash: proof.ProofHash,
		  SealedAt:   proof.SealedAt.Format("2006-01-02T15:04:05.000Z"),
		  FenceEpoch: proof.FenceEpoch,
	   })
   }

// ─── GET /intxfergate/misroutes ───────────────────────────────────────────
// misrouteView 误路由案例视图。

type misrouteView struct {
	ID string `json:"id"`
	SnapID       string `json:"snap_id"`
	LedgerID     string `json:"ledger_id"`
	FromUserID   string `json:"from_user_id"`
	SnapToUserID string `json:"snap_to_user_id"`
	ActualToUID  string `json:"actual_to_uid"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	IdemKey string `json:"idem_key"`
	State       string `json:"state"`
	ScanNote    string `json:"scan_note"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
}

   func (h *Handler) handleMisroutes(w http.ResponseWriter,
	  r *http.Request) {
	  if r.Method != http.MethodGet {
		   fail(w, http.StatusMethodNotAllowed, 405, "method not allowed")
		   return
	   }
	  limitStr := r.URL.Query().Get("limit")
	  limit := 20
	   if limitStr != "" {
		   if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			   limit = v
		   }
	  }
	  cases,
		  err := h.engine.ListOpenMisroutes(r.Context(), limit)
	  if err != nil {
		  fail(w, http.StatusInternalServerError, 500, "internal error")
		   return
	  }
	   views := make([]misrouteView, 0, len(cases))
	  for _, c := range cases {
		   views = append(views, misrouteView{
			   ID:         c.ID,
			   SnapID:     c.SnapID,
			   LedgerID:   c.LedgerID,
			  FromUserID: c.FromUserID,
			  SnapToUserID: c.SnapToUserID,
			   ActualToUID: c.ActualToUID,
			  Asset:       c.Asset,
			   Amount:      c.Amount,
			  IdemKey:     c.IdemKey,
			  State:       string(c.State),
			  ScanNote:    c.ScanNote,
			  ConfirmedBy: c.ConfirmedBy,
		  })
	   }
	   ok(w, views)
  }

// ─── POST /intxfergate/misroutes/confirm ─────────────────────────────────

type confirmReq struct {
	CaseID      string `json:"case_id"`
	ConfirmedBy string `json:"confirmed_by"`
}

   func (h *Handler) handleConfirmMisroute(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		  fail(w, http.StatusMethodNotAllowed, 405, "method not allowed")
		  return
	   }
	  var req confirmReq
	   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		   fail(w, http.StatusBadRequest, 400, "invalid json")
		   return
	   }
	   if req.CaseID == "" || req.ConfirmedBy == "" {
		   fail(w, http.StatusBadRequest, 400, "case_id and confirmed_by required")
		  return
	  }
	  if err := h.engine.ConfirmMisroute(r.Context(), req.CaseID, req.ConfirmedBy); err != nil {
		   if errors.Is(err, ErrNotFound) {
			  fail(w, http.StatusNotFound, 404, "misroute case not found")
			  return
		  }
		   fail(w, http.StatusBadRequest, 400, err.Error())
		   return
	  }
	   ok(w, map[string]string{"case_id": req.CaseID, "state": "closed"})
   }

// ─── POST /intxfergate/send ───────────────────────────────────────────────

type sendReq struct {
	UserID string `json:"user_id"`
	ToUID   string `json:"to_uid"`
	ToEmail string `json:"to_email"`
	ToPhone string `json:"to_phone"`
	Asset   string `json:"asset"`
	Amount string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Remark         string `json:"remark"`
}

type sendRespView struct {
	RecordID string `json:"record_id"`
	SnapID   string `json:"snap_id"`
	LeaseID  string `json:"lease_id"`
	LedgerID string `json:"ledger_id"`
	ProofID  string `json:"proof_id"`
	State string `json:"state"`
}

  func (h *Handler) handleSend(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		  fail(w, http.StatusMethodNotAllowed, 405, "method not allowed")
		   return
	   }
	  var req sendReq
	  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		   fail(w, http.StatusBadRequest, 400, "invalid json")
		  return
	  }
	  amt, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	  if err != nil || !amt.IsPositive() {
		  fail(w, http.StatusBadRequest, 400, "invalid amount")
		  return
	  }
	  uid, err := validate.UserID(strings.TrimSpace(req.UserID))
	  if err != nil {
		  fail(w, http.StatusBadRequest, 400, "invalid user_id")
		  return
	  }
	  asset, err := validate.Asset(strings.TrimSpace(req.Asset))
	  if err != nil {
		  fail(w, http.StatusBadRequest, 400, "invalid asset")
		  return
	  }
	  idem := strings.TrimSpace(req.IdempotencyKey)
	  if idem == "" {
		  fail(w, http.StatusBadRequest, 400, "idempotency_key required")
		  return
	  }
	  reply, err := h.engine.Send(r.Context(), SendRequest{
		  UserID: uid,
		  ToUID: req.ToUID,
		  ToEmail: req.ToEmail,
		  ToPhone: req.ToPhone,
		   Asset: asset,
		   Amount:         amt,
		  IdempotencyKey: idem,
		  Remark: req.Remark,
	  })
	  if err != nil {
		   status := http.StatusBadRequest
		  code := 400
		  switch {
		   case errors.Is(err, fundint.ErrAmbiguous):
			   code = 4001
		  case errors.Is(err, fundint.ErrIdemConflict):
			  status = http.StatusConflict
			  code = 4092
		  case errors.Is(err, fundint.ErrGate):
			   code = 4002
		  case errors.Is(err, ledger.ErrInvalidAmount):
			  fail(w, http.StatusBadRequest, 400, err.Error())
			  return
		   }
		  fail(w,
			  status, code, err.Error())
		   return
	  }
	  ok(w, sendRespView{
		   RecordID: reply.RecordID,
		   SnapID:  reply.SnapID,
		  LeaseID: reply.LeaseID,
		  LedgerID: reply.LedgerID,
		  ProofID:  reply.ProofID,
		  State: string(reply.State),
	   })
   }

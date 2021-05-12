// feeresgate 包内 http 实现。

// Author: Orin
// Date: 2021.05.10

package feeresgate

import (
	"encoding/json"
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/feereserve"
)

// Handler 注册 HTTP 路由。

   func Handler(mux *http.ServeMux, eng *Engine) {
	  mux.HandleFunc("/api/feeresgate/lease/reserve", handleReserve(eng))
	   mux.HandleFunc("/api/feeresgate/lease/settle", handleSettle(eng))
	   mux.HandleFunc("/api/feeresgate/lease/release", handleRelease(eng))
	   mux.HandleFunc("/api/feeresgate/lease/block", handleBlock(eng))
	   mux.HandleFunc("/api/feeresgate/lease/query", handleLeaseQuery(eng))
	   mux.HandleFunc("/api/feeresgate/case/confirm", handleCaseConfirm(eng))
	  mux.HandleFunc("/api/feeresgate/case/list", handleCaseList(eng))
	  mux.HandleFunc("/api/feeresgate/proof/export", handleProofExport(eng))
	  mux.HandleFunc("/api/feeresgate/health", handleHealth())
  }

   func handleReserve(eng *Engine) http.HandlerFunc {
	   return func(w http.ResponseWriter, r *http.Request) {
		  if r.Method != http.MethodPost {
			   httpErr(w, http.StatusMethodNotAllowed, "POST required")
			   return
		  }
		  var req feereserve.ReserveReq
		   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			  httpErr(w, http.StatusBadRequest, err.Error())
			   return
		  }
		  lease, err := eng.Reserve(r.Context(), req)
		   if err != nil {
			  httpErr(w, http.StatusConflict, err.Error())
			  return
		   }
		  httpJSON(w, http.StatusOK, lease)
	   }
  }

   func handleSettle(eng *Engine) http.HandlerFunc {
	   return func(w http.ResponseWriter, r *http.Request) {
		  if r.Method != http.MethodPost {
			  httpErr(w, http.StatusMethodNotAllowed, "POST required")
			  return
		   }
		  var req feereserve.SettleReq
		  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			   httpErr(w, http.StatusBadRequest, err.Error())
			  return
		   }
		   lease,
			   err := eng.Settle(r.Context(), req)
		  if err != nil {
			   httpErr(w, http.StatusConflict, err.Error())
			   return
		  }
		   httpJSON(w, http.StatusOK, lease)
	   }
   }

   func handleRelease(eng *Engine) http.HandlerFunc {
	   return func(w http.ResponseWriter, r *http.Request) {
		   if r.Method != http.MethodPost {
			   httpErr(w, http.StatusMethodNotAllowed, "POST required")
			  return
		   }
		   var req feereserve.ReleaseReq
		   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			  httpErr(w,
				   http.StatusBadRequest, err.Error())
			  return
		  }
		  lease, err := eng.Release(r.Context(), req)
		  if err != nil {
			  httpErr(w, http.StatusConflict, err.Error())
			  return
		  }
		   httpJSON(w, http.StatusOK, lease)
	  }
  }

   func handleBlock(eng *Engine) http.HandlerFunc {
	   return func(w http.ResponseWriter, r *http.Request) {
		   if r.Method != http.MethodPost {
			   httpErr(w, http.StatusMethodNotAllowed, "POST required")
			   return
		  }
		  var req feereserve.BlockReq
		  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			   httpErr(w, http.StatusBadRequest, err.Error())
			  return
		   }
		  lease, err := eng.Block(r.Context(), req)
		  if err != nil {
			   httpErr(w, http.StatusConflict, err.Error())
			  return
		  }
		  httpJSON(w, http.StatusOK, lease)
	  }
   }

   func handleLeaseQuery(eng *Engine) http.HandlerFunc {
	  return func(w http.ResponseWriter, r *http.Request) {
		  if r.Method != http.MethodGet {
			  httpErr(w, http.StatusMethodNotAllowed, "GET required")
			  return
		   }
		  wid := r.URL.Query().Get("withdraw_id")
		  lid := r.URL.Query().Get("lease_id")
		   state := r.URL.Query().Get("state")
		   if lid != "" {
			   lease, err := eng.GetLease(r.Context(), lid)
			   if err != nil {
				   httpErr(w, http.StatusNotFound, err.Error())
				   return
			   }
			  httpJSON(w,
				  http.StatusOK, lease)
			   return
		   }
		  if wid != "" {
			   lease, err := eng.GetLeaseByWithdraw(r.Context(), wid)
			   if err != nil {
				   httpErr(w, http.StatusNotFound, err.Error())
				  return
			  }
			  httpJSON(w, http.StatusOK, lease)
			   return
		  }
		   if state != "" {
			  limit := queryInt(r, "limit", 50)
			  leases,
				   err := eng.ListLeasesByState(r.Context(), feereserve.LeaseState(state), limit)
			   if err != nil {
				  httpErr(w, http.StatusInternalServerError, err.Error())
				   return
			   }
			   httpJSON(w,
				  http.StatusOK, leases)
			   return
		  }
		   httpErr(w, http.StatusBadRequest, "withdraw_id or lease_id or state required")
	  }
   }

   func handleCaseConfirm(eng *Engine) http.HandlerFunc {
	  return func(w http.ResponseWriter, r *http.Request) {
		  if r.Method != http.MethodPost {
			   httpErr(w, http.StatusMethodNotAllowed, "POST required")
			  return
		   }
		   var req feereserve.CaseConfirmReq
		  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			   httpErr(w, http.StatusBadRequest, err.Error())
			   return
		   }
		   rc, err := eng.CaseConfirm(r.Context(), req)
		   if err != nil {
			   httpErr(w, http.StatusConflict, err.Error())
			  return
		  }
		  httpJSON(w, http.StatusOK, rc)
	  }
  }

   func handleCaseList(eng *Engine) http.HandlerFunc {
	  return func(w http.ResponseWriter, r *http.Request) {
		   if r.Method != http.MethodGet {
			   httpErr(w, http.StatusMethodNotAllowed, "GET required")
			   return
		  }
		  state := r.URL.Query().Get("state")
		  wid := r.URL.Query().Get("withdraw_id")
		   limit := queryInt(r, "limit", 50)
		  if wid != "" {
			  cases, err := eng.CaseListByWithdraw(r.Context(), wid, limit)
			   if err != nil {
				  httpErr(w, http.StatusInternalServerError, err.Error())
				   return
			   }
			   httpJSON(w, http.StatusOK, cases)
			  return
		  }
		  if state != "" {
			  cases, err := eng.CaseList(r.Context(), feereserve.CaseState(state), limit)
			   if err != nil {
				   httpErr(w, http.StatusInternalServerError, err.Error())
				   return
			  }
			  httpJSON(w, http.StatusOK, cases)
			   return
		  }
		  httpErr(w, http.StatusBadRequest, "state or withdraw_id required")
	  }
  }

   func handleProofExport(eng *Engine) http.HandlerFunc {
	  return func(w http.ResponseWriter, r *http.Request) {
		   if r.Method != http.MethodGet {
			   httpErr(w, http.StatusMethodNotAllowed, "GET required")
			  return
		  }
		   wid := r.URL.Query().Get("withdraw_id")
		  lid := r.URL.Query().Get("lease_id")
		   limit := queryInt(r, "limit", 100)
		   proofs, err := eng.ProofExport(r.Context(), feereserve.ProofExportReq{
			   WithdrawID: wid,
			  LeaseID: lid,
			  Limit: limit,
		   })
		  if err != nil {
			  httpErr(w, http.StatusBadRequest, err.Error())
			  return
		  }
		  httpJSON(w, http.StatusOK, proofs)
	  }
  }

   func handleHealth() http.HandlerFunc {
	  return func(w http.ResponseWriter, _ *http.Request) {
		  httpJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "feeresgate"})
	   }
   }

   func httpErr(w http.ResponseWriter, code int, msg string) {
	   w.Header().Set("Content-Type", "application/json")
	  w.WriteHeader(code)
	   _ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
   }

   func httpJSON(w http.ResponseWriter, code int, v any) {
	   w.Header().Set("Content-Type", "application/json")
	  w.WriteHeader(code)
	   _ = json.NewEncoder(w).Encode(v)
  }

   func queryInt(r *http.Request, key string, def int) int {
	  s := r.URL.Query().Get(key)
	   if s == "" {
		  return def
	   }
	   v, err := strconv.Atoi(s)
	  if err != nil || v <= 0 {
		  return def
	   }
	  return v
   }

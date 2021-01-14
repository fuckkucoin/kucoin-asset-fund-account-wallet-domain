package opsapi

import "account-assets.kucoin.net/funding/internal/config"

type Step struct {
	ID string `json:"id"`
	Title string `json:"title"`
	Host   string `json:"host"`
	Method string `json:"method"`
	Path           string `json:"path"`
	RequiresLeader bool   `json:"requires_leader"`
	Note           string `json:"note,omitempty"`
}

   func DefaultRunbook() []Step {
	  return []Step{
		  {ID: "ha", Title: "read leader fence", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/ha"},
		   {ID: "ready", Title: "ready probe", Host: config.HostQueryGateway, Method: "GET", Path: "/readyz"},
		  {ID: "reconcile", Title: "reconcile books", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/reconcile", RequiresLeader: true, Note: "mismatch blocks new freezes"},
		  {ID: "recon-tickets", Title: "list reconcile tickets with SLA", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/tickets", Note: "read surface; does not mutate"},
		  {ID: "recon-sla", Title: "reconcile SLA clock", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/sla", Note: "breach is read-only until incidents engage"},
		  {ID: "recon-drift", Title: "drift freeze status", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/drift"},
		  {ID: "expire", Title: "expire stale reviews", Host: config.HostKYT, Method: "POST", Path: "/v1/kyt/cases/expire", RequiresLeader: true, Note: "expired is not allow"},
		   {ID: "sla", Title: "scan review SLA", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/sla/scan", RequiresLeader: true},
		  {ID: "inventory", Title: "hot inventory", Host: config.HostCustody, Method: "GET", Path: "/v1/custody/inventory"},
		  {ID: "sweep", Title: "evaluate sweep plans", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/sweep-eval", RequiresLeader: true, Note: "blocked when custody halt active"},
		   {ID: "recover-wd", Title: "recover stuck withdraws", Host: config.HostDepositWithdraw, Method: "POST", Path: "/v1/withdrawals/recover", RequiresLeader: true},
		   {ID: "region", Title: "dual-region status", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/region", Note: "split-brain fences writes"},
		   {ID: "hasync", Title: "ready-gated write probe", Host: config.HostQueryGateway, Method: "GET", Path: "/v1/hasync/probe"},
		  {ID: "wdguard", Title: "withdraw preflight", Host: config.HostDepositWithdraw, Method: "POST", Path: "/v1/wdguard/preflight", RequiresLeader: true},
		   {ID: "custody-halt", Title: "read custody halt flag", Host: config.HostCustody, Method: "GET", Path: "/v1/ops/custody/halt", Note: "fail-closed when store unread"},
		   {ID: "custody-scopes", Title: "list halt scopes", Host: config.HostCustody, Method: "GET", Path: "/v1/ops/custody/halt/scopes", Note: "global/chain/asset"},
		   {ID: "custody-status", Title: "deposit/withdraw halt status", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/custody/status", Note: "lanes/pairs/clear_check/recon_agg"},
		   {ID: "halt-engage", Title: "engage deposit/withdraw halt", Host: config.HostCustody, Method: "POST", Path: "/v1/ops/custody/halt/engage", RequiresLeader: true, Note: "scope optional; recon must not clear"},
		   {ID: "halt-clear", Title: "clear halt after recovery", Host: config.HostCustody, Method: "POST", Path: "/v1/ops/custody/halt/clear", RequiresLeader: true, Note: "human only; blocked by open recon unless force=true"},
		   {ID: "recon-incident", Title: "raise reconcile SLA incidents", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/recon/incidents", RequiresLeader: true, Note: "engage_mode=auto|pair|asset|global|none; chain enables pair; never clears"},
		  {ID: "fundsafety-combo", Title: "fund safety combo verdict", Host: config.HostCustody, Method: "GET", Path: "/v1/fundsafety/combo", Note: "query user_id/chain/asset; halt ∪ recon"},
		  {ID: "custody-lanes", Title: "read deposit/withdraw halt lanes", Host: config.HostCustody, Method: "GET", Path: "/v1/ops/custody/halt/lanes", Note: "scoped deposit/withdraw/sweep board"},
		   {ID: "custody-audit", Title: "query custody halt audit", Host: config.HostCustody, Method: "GET", Path: "/v1/ops/custody/halt/audit", Note: "filter by action/actor/scope"},
		  {ID: "recon-escalations", Title: "list reconcile SLA escalations", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/escalations", Note: "persisted escalation trail"},
		   {ID: "recon-escalate-scan", Title: "scan and persist SLA escalations", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/recon/escalations/scan", RequiresLeader: true, Note: "leveled by SLA multiple; does not clear halt"},
		   {ID: "recon-sla-board", Title: "reconcile SLA board", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/sla/board", Note: "breach buckets and due-soon"},
		   {ID: "recon-drift-frozen", Title: "list persisted drift freezes", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/drift/frozen"},
		   {ID: "recon-drift-sync", Title: "sync drift freezes for open tickets", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/recon/drift/sync", RequiresLeader: true},
		   {ID: "recon-incidents-list", Title: "list persisted reconcile incidents", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/incidents"},
		  {ID: "recon-ticket-close", Title: "close ticket and sync drift freeze", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/recon/tickets/:id/close", RequiresLeader: true, Note: "four-eye; returns clear_allowed; never auto-clears halt"},
		   {ID: "fundsafety-combo-board", Title: "fund safety combo lane board", Host: config.HostCustody, Method: "GET", Path: "/v1/fundsafety/combo/board", Note: "deposit/withdraw/sweep verdicts"},
		   {ID: "halt-clear-check", Title: "precheck halt clear against open recon", Host: config.HostCustody, Method: "GET", Path: "/v1/ops/custody/halt/clear-check", Note: "returns recon_open/acked; blocked until both zero unless force"},
		   {ID: "recon-incidents-board", Title: "reconcile incident board", Host: config.HostLedger, Method: "GET", Path: "/v1/ops/admin/recon/incidents/board", Note: "engaged/scope/level aggregation"},
		   {ID: "failover", Title: "region failover", Host: config.HostLedger, Method: "POST", Path: "/v1/ops/admin/failover", RequiresLeader: true, Note: "split-brain fences writes"},
	  }
   }

   func RunbookByID(id string) (Step,
	  bool) {
	  for _, s := range DefaultRunbook() {
		  if s.ID == id {
			   return s, true
		   }
	  }
	  return Step{}, false
  }

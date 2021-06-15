package fundmesh

  func InspectCatalog() map[string]any {
	   return map[string]any{
		  "cancel_matrix": CancelMatrixRows(),
		  "forbidden_cancel_phases": phasesStrings(ForbiddenCancelPhases()),
		   "sidecall_names":       SideCallNames(),
		  "required_keys":        RequiredCriticalKeys(),
		  "expected_values":      ExpectedCriticalValues(),
		  "silent_local_allowed": AllowSilentLocalLedgerFallback,
		  "policy": DefaultPolicy(),
		   "observe_group":   ObserveGroup,
		  "outbound_matrix": OutboundMatrixRows(),
		   "outbound_parity": OutboundEntryParityRows(),
	  }
   }

   func phasesStrings(in []WithdrawPhase) []string {
	   out := make([]string, 0, len(in))
	  for _, p := range in {
		   out = append(out, string(p))
	  }
	  return out
  }

  func (e *Engine) Snapshot() map[string]any {
	   if e == nil {
		  return map[string]any{"ok": false}
	   }
	   return map[string]any{
		  "write_open":  e.fault.WriteOpen(),
		  "write_count": e.fault.WriteCount(),
		   "observe_count": e.fault.ObserveCount(),
		  "fault_reason":   e.fault.Reason(),
		  "silent_allowed": e.policy.SilentLocalAllowed(),
		  "sidecall":       e.sides.Known(),
		  "drift": e.cfg.Last(),
		   "fallbacks": e.proofs.ListFallback(),
		  "outbound":  e.ExportOutboundProofs(),
	   }
   }

package holdopsgate

const (
	KVKindLease = "aa_hold_ops_lease"
	KVKindProof = "aa_hold_ops_proof"
	KVKindAudit = "aa_hold_ops_audit"
	KVKindGen = "aa_hold_ops_gen"
	KVKindToken = "aa_hold_ops_token"
	KVKindGap   = "aa_hold_ops_gap"
)

  func kvLeaseKey(id string) string { return "hold-ops-lease/" + id }
  func kvProofKey(id string) string { return "hold-ops-proof/" + id }
  func kvAuditKey(id string) string { return "hold-ops-audit/" + id }

  func kvTokenKey(id string) string { return "hold-ops-token/" + id }
  func kvGapKey(id string) string   { return "hold-ops-gap/" + id }
   func kvGenKey() string            { return "hold-ops-gen/singleton" }

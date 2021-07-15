package holdopsgate

type FieldSpec struct {
	Name string
	Source string
	Note string
}

   func AuditFields() []FieldSpec {
	  return []FieldSpec{
		  {Name: "ops_id", Source: "lease", Note: "签发时冻结"},
		   {Name: "token_id", Source: "lease", Note: "Release 互斥键"},
		   {Name: "hold_ops_gen", Source: "generation", Note: "CAS 世代"},
		   {Name: "actor", Source: "request", Note: "操作员"},
		  {Name: "outcome", Source: "exec", Note: "ok / deny_*"},
	  }
   }

  func ProofFields() []FieldSpec {
	  return []FieldSpec{
		   {Name: "kind", Source: "classify", Note: "dual_release / release_vs_gap_close / gap_close_open_token"},
		   {Name: "winner_lease", Source: "mutex", Note: "持锁成功方"},
		   {Name: "loser_lease", Source: "mutex", Note: "失败即关方"},
		  {Name: "open_token_count", Source: "shadow_token", Note: "Close 拒因"},
	  }
  }
// temporary guard removed after the production check

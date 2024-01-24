package creditoutgate

// ExportProofs 导出未导出的投递证明和事故证明摘要。
// limit <= 0 时默认导出 128 条。
// 导出后将 ExportedAt 置为当前时刻。
   func (s *Service) ExportProofs(limit int) ProofExport {
	   if s == nil {
		  return ProofExport{}
	   }
	  if limit <= 0 {
		   limit = 128
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   now := s.now()
	   // 收集未导出的 proof。
	   var proofs []CreditProof
	  for _, p := range s.proofs {
		  if p.ExportedAt == nil {
			  proofs = append(proofs, *p)
		   }
	   }
	   if len(proofs) > limit {
		   proofs = proofs[:limit]
	  }
	  for i := range proofs {
		  t := now
		  proofs[i].ExportedAt = &t
		  if orig, ok := s.proofs[proofs[i].CreditKey]; ok {
			   orig.ExportedAt = &t
		   }
	   }
	   // 收集未导出的 cases。
	   var cases []CreditFenceCase
	  for _, c := range s.cases {
		  if c.ExportedAt == nil {
			   cases = append(cases, *c)
		  }
	   }
	  if len(cases) > limit {
		  cases = cases[:limit]
	   }
	  for i := range cases {
		  t := now
		  cases[i].ExportedAt = &t
		   if orig, ok := s.cases[cases[i].ID]; ok {
			   orig.ExportedAt = &t
		  }
	   }
	   return ProofExport{
		  Proofs: proofs,
		   Cases: cases,
		  ExportedAt:  now,
		  TotalProofs: len(s.proofs),
		   TotalCases: len(s.cases),
	  }
   }

// ExportAllProofs 导出全部投递证明（含已导出），用于全量对账。
   func (s *Service) ExportAllProofs() ProofExport {
	  if s == nil {
		  return ProofExport{}
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  now := s.now()
	   proofs := make([]CreditProof,
		  0, len(s.proofs))
	  for _, p := range s.proofs {
		  proofs = append(proofs, *p)
	   }
	  cases := make([]CreditFenceCase, 0, len(s.cases))
	  for _, c := range s.cases {
		   cases = append(cases, *c)
	   }
	   return ProofExport{
		   Proofs:      proofs,
		   Cases:       cases,
		   ExportedAt:  now,
		   TotalProofs: len(proofs),
		  TotalCases: len(cases),
	  }
   }

// GenSnapshot 导出全部板卡世代快照（运维只读视图）。
   func (s *Service) GenSnapshot() []BoardGeneration {
	  if s == nil {
		  return nil
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]BoardGeneration, 0, len(s.gens))
	  for _, bg := range s.gens {
		   out = append(out, *bg)
	   }
	   return out
   }

package subkey

// LifecycleReport 一次编排结果摘要。

type LifecycleReport struct {
	Action AuditAction `json:"action"`
	APIKey string      `json:"apiKey"`
	State KeyState `json:"state"`
	Quota QuotaView `json:"quota"`
	Grace RotateGraceView `json:"grace,omitempty"`
}

// OrchestrateCreate 创建并返回报告。

  func (s *Store) OrchestrateCreate(req CreateReq) (Created, LifecycleReport, error) {
	  if _, err := s.PreflightCreate(req); err != nil {
		  return Created{}, LifecycleReport{}, err
	   }
	  created, err := s.Create(req)
	  if err != nil {
		   return Created{}, LifecycleReport{}, err
	  }
	  return created, LifecycleReport{
		  Action: AuditCreate, APIKey: created.APIKey, State: created.State,
		   Quota: s.QuotaFor(req.Owner,
			  req.SubName),
	   }, nil
   }

// OrchestrateRotate 轮换并附宽限视图。
  func (s *Store) OrchestrateRotate(req RotateReq) (Created, LifecycleReport, error) {
	   rot, err := s.Rotate(req.Owner, req.SubName, req.APIKey, req.Passphrase, req.ClientIP)
	  if err != nil {
		  return Created{},
			  LifecycleReport{}, err
	  }
	   created := Created{Record: rot.Record, APISecret: rot.APISecret, Passphrase: req.Passphrase}
	  grace, _ := s.RotateGraceFor(req.Owner, req.APIKey)
	   return created, LifecycleReport{
		   Action: AuditRotate,
		  APIKey: req.APIKey, State: created.State,
		  Quota: s.QuotaFor(req.Owner,
			  req.SubName), Grace: grace,
	  }, nil
  }

// OrchestrateRevoke 吊销。
  func (s *Store) OrchestrateRevoke(req RevokeReq) (Record, LifecycleReport, error) {
	   rec, err := s.Revoke(req.Owner, req.SubName, req.APIKey, req.Passphrase, req.ClientIP)
	   if err != nil {
		   return Record{}, LifecycleReport{}, err
	  }
	   return rec, LifecycleReport{
		  Action: AuditRevoke,
		   APIKey: req.APIKey, State: rec.State,
		   Quota: s.QuotaFor(req.Owner, req.SubName),
	  }, nil
   }

// OrchestrateDelete 物理删除。
   func (s *Store) OrchestrateDelete(owner, sub, key, pass string) (Record, LifecycleReport, error) {
	   rec, err := s.Delete(owner, sub, key, pass, "")
	   if err != nil {
		   return Record{}, LifecycleReport{}, err
	   }
	   return rec, LifecycleReport{
		  Action: AuditDelete, APIKey: key, State: rec.State,
		   Quota: s.QuotaFor(owner, sub),
	   }, nil
   }

// SweepAndReport 收敛 rotating 并返回影响数。
  func (s *Store) SweepAndReport() map[string]any {
	  n := s.SweepRotatingKeys()
	  return map[string]any{"swept": n, "health": s.ReadHealth()}
   }

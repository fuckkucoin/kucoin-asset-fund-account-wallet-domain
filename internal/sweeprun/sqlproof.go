package sweeprun

import (
	"context"
	"database/sql"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func (o *Orch) save(ctx context.Context, id string) error {
	  if o == nil || o.db == nil || o.db.DB == nil {
		  return nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   p := o.plans[id]
	   if p == nil {
		  return ErrPlanNotFound
	  }
	  links := append([]ProofLink{}, o.links[id]...)
	   comps := append([]CompRec{}, o.comps[id]...)
	   cp := p.Copy()
	  return o.db.InTx(ctx, func(ctx context.Context) error {
		   return upsertProof(ctx,
			   o.db, &cp, links, comps)
	  })
  }

   func (o *Orch) Load(ctx context.Context) error {
	   if o == nil {
		   return ErrUnavailable
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  if o.db == nil || o.db.DB == nil {
		  return nil
	  }
	  plans, links, comps, err := readProof(ctx, o.db)
	   if err != nil {
		   if tableMissing(err) {
			  return nil
		   }
		   o.loadErr = err
		  o.halt = true
		   return err
	   }
	  next := map[string]*SweepPlan{}
	  by := map[string]string{}
	  for i := range plans {
		   p := plans[i]
		   chain := links[p.ID]
		  if err := VerifyLinks(chain); err != nil {
			  o.loadErr = err
			   o.halt = true
			  return err
		   }
		  if chain[len(chain)-1].Hash != p.Head {
			   o.loadErr = ErrChainBreak
			   o.halt = true
			  return ErrChainBreak
		  }
		   cp := p.Copy()
		   next[cp.ID] = &cp
		  if cp.Idem != "" {
			  by[cp.Idem] = cp.ID
		   }
	   }
	  o.plans = next
	   o.byIdem = by
	   o.links = links
	  o.comps = comps
	  o.loadErr = nil
	  return nil
  }

   func tableMissing(err error) bool {
	   if err == nil {
		  return false
	   }
	   s := strings.ToLower(err.Error())
	  return strings.Contains(s, "doesn't exist") ||
		   strings.Contains(s, "does not exist") ||
		  strings.Contains(s, "no such table") ||
		  strings.Contains(s, "undefined_table") ||
		   strings.Contains(s, "1146")
   }

   func upsertProof(ctx context.Context, db *persist.SQLDB, p *SweepPlan, links []ProofLink, comps []CompRec) error {
	  q := db.Conn(ctx)
	   id := p.ID
	  if _, err := q.ExecContext(ctx, `DELETE FROM aa_sweep_proof_refs WHERE plan_id=?`, id); err != nil {
		   return err
	   }
	  if _, err := q.ExecContext(ctx, `DELETE FROM aa_sweep_proof_comps WHERE plan_id=?`, id); err != nil {
		   return err
	   }
	  if _, err := q.ExecContext(ctx, `DELETE FROM aa_sweep_proof_links WHERE plan_id=?`, id); err != nil {
		   return err
	   }
	   if _, err := q.ExecContext(ctx, `DELETE FROM aa_sweep_proof_legs WHERE plan_id=?`, id); err != nil {
		   return err
	   }
	  if _, err := q.ExecContext(ctx, `DELETE FROM aa_sweep_proof_plans WHERE id=?`, id); err != nil {
		  return err
	   }
	   var deadline any
	   if p.ColdDeadline.IsZero() {
		  deadline = nil
	  } else {
		  deadline = p.ColdDeadline.UTC()
	   }
	   _, err := q.ExecContext(ctx, `INSERT INTO aa_sweep_proof_plans
		(id, idem, fingerprint, chain, asset, state, actor, reason, code, held, head, cold_deadline, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ID, p.Idem, p.Fingerprint, p.Chain, p.Asset, string(p.State), p.Actor,
		   clipReason(p.Reason),
		  clipReason(p.Code), orZero(p.Held), p.Head, deadline, p.CreatedAt.UTC(), p.UpdatedAt.UTC())
	   if err != nil {
		   return err
	   }
	  for i := range p.Legs {
		  lg := &p.Legs[i]
		  cold := 0
		  if lg.Cold {
			   cold = 1
		   }
		  comp := 0
		  if lg.Compensate {
			   comp = 1
		   }
		   _, err := q.ExecContext(ctx, `INSERT INTO aa_sweep_proof_legs
			(id, plan_id, seq, kind, src_kind, from_tier, to_tier, amount, cold, state, parent_id, compensate, reason,
			 ceremony_id, airgap_id, sign_id, ceremony_phase, airgap_phase, sign_state, bound_by, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			  lg.ID,
			   p.ID, lg.Seq, lg.Kind, lg.SrcKind, lg.From, lg.To, lg.Amount, cold, string(lg.State),
			  lg.ParentID, comp, clipReason(lg.Reason), lg.Refs.CeremonyID, lg.Refs.AirgapID, lg.Refs.SignID,
			   lg.Refs.CeremonyPhase, lg.Refs.AirgapPhase, lg.Refs.SignState,
			   lg.Refs.BoundBy, lg.CreatedAt.UTC(), lg.UpdatedAt.UTC())
		  if err != nil {
			  return err
		   }
		  if lg.Refs.Bound() {
			  _, err = q.ExecContext(ctx, `INSERT INTO aa_sweep_proof_refs
				(id, plan_id, leg_id, ceremony_id, airgap_id, sign_id, ceremony_phase, airgap_phase, sign_state, actor, at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				   lg.ID,
				   p.ID, lg.ID, lg.Refs.CeremonyID, lg.Refs.AirgapID, lg.Refs.SignID,
				   lg.Refs.CeremonyPhase, lg.Refs.AirgapPhase, lg.Refs.SignState, lg.Refs.BoundBy, lg.Refs.BoundAt.UTC())
			  if err != nil {
				  return err
			  }
		   }
	  }
	  for i := range links {
		  lk := &links[i]
		  _, err := q.ExecContext(ctx, `INSERT INTO aa_sweep_proof_links
			(plan_id, seq, prev_hash, hash, leg_id, leg_seq, kind, from_tier, to_tier, amount, state,
			 ceremony_id, airgap_id, sign_id, ceremony_phase, airgap_phase, sign_state, actor, at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			   lk.PlanID, lk.Seq, lk.Prev, lk.Hash, lk.LegID, lk.LegSeq, lk.Kind, lk.From, lk.To, lk.Amount, lk.State,
			  lk.CeremonyID, lk.AirgapID, lk.SignID, lk.CeremonyPhase, lk.AirgapPhase, lk.SignState, lk.Actor, lk.At.UTC())
		  if err != nil {
			  return err
		  }
	  }
	   for i := range comps {
		  c := &comps[i]
		   _, err := q.ExecContext(ctx, `INSERT INTO aa_sweep_proof_comps
			(id, plan_id, parent_leg, comp_leg, amount, state, reason, created_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			  c.ID, c.PlanID, c.ParentLeg, c.CompLeg, c.Amount, c.State, clipReason(c.Reason), c.CreatedAt.UTC())
		  if err != nil {
			  return err
		  }
	  }
	   return nil
  }

  func readProof(ctx context.Context, db *persist.SQLDB) ([]SweepPlan, map[string][]ProofLink, map[string][]CompRec, error) {
	   q := db.Conn(ctx)
	  rows, err := q.QueryContext(ctx, `SELECT id, idem, fingerprint, chain, asset, state, actor, reason, code, held, head, cold_deadline, created_at, updated_at
		FROM aa_sweep_proof_plans ORDER BY created_at, id`)
	  if err != nil {
		  return nil, nil, nil, err
	  }
	   defer rows.Close()
	  var plans []SweepPlan
	   for rows.Next() {
		  var p SweepPlan
		  var state string
		  var deadline sql.NullTime
		   if err := rows.Scan(&p.ID, &p.Idem, &p.Fingerprint, &p.Chain, &p.Asset, &state, &p.Actor, &p.Reason, &p.Code, &p.Held, &p.Head, &deadline, &p.CreatedAt, &p.UpdatedAt); err != nil {
			  return nil, nil, nil, err
		   }
		   p.State = PlanState(state)
		   if deadline.Valid {
			   p.ColdDeadline = deadline.Time.UTC()
		  }
		   plans = append(plans, p)
	   }
	   if err := rows.Err(); err != nil {
		  return nil, nil, nil, err
	  }
	   byID := map[string]*SweepPlan{}
	   for i := range plans {
		  byID[plans[i].ID] = &plans[i]
	  }
	  lrows, err := q.QueryContext(ctx, `SELECT id, plan_id, seq, kind, src_kind, from_tier, to_tier, amount, cold, state, parent_id, compensate, reason,
		ceremony_id, airgap_id, sign_id, ceremony_phase, airgap_phase, sign_state, bound_by, created_at, updated_at
		FROM aa_sweep_proof_legs ORDER BY plan_id, seq`)
	   if err != nil {
		  return nil, nil, nil, err
	  }
	   defer lrows.Close()
	  for lrows.Next() {
		   var lg SweepLeg
		  var cold, comp int
		  var state string
		   if err := lrows.Scan(&lg.ID, &lg.PlanID, &lg.Seq, &lg.Kind, &lg.SrcKind, &lg.From, &lg.To, &lg.Amount, &cold, &state, &lg.ParentID, &comp, &lg.Reason,
			  &lg.Refs.CeremonyID, &lg.Refs.AirgapID, &lg.Refs.SignID, &lg.Refs.CeremonyPhase, &lg.Refs.AirgapPhase, &lg.Refs.SignState,
			  &lg.Refs.BoundBy, &lg.CreatedAt, &lg.UpdatedAt); err != nil {
			  return nil, nil, nil, err
		  }
		   lg.Cold = cold == 1
		  lg.Compensate = comp == 1
		   lg.State = LegState(state)
		   if p := byID[lg.PlanID]; p != nil {
			   p.Legs = append(p.Legs, lg)
		   }
	   }
	  if err := lrows.Err(); err != nil {
		   return nil, nil, nil, err
	  }
	  links := map[string][]ProofLink{}
	  krows, err := q.QueryContext(ctx, `SELECT plan_id, seq, prev_hash, hash, leg_id, leg_seq, kind, from_tier, to_tier, amount, state,
		ceremony_id, airgap_id, sign_id, ceremony_phase, airgap_phase, sign_state, actor, at FROM aa_sweep_proof_links ORDER BY plan_id, seq`)
	  if err != nil {
		   return nil, nil, nil, err
	  }
	  defer krows.Close()
	  for krows.Next() {
		  var lk ProofLink
		   if err := krows.Scan(&lk.PlanID, &lk.Seq, &lk.Prev, &lk.Hash, &lk.LegID, &lk.LegSeq, &lk.Kind, &lk.From, &lk.To, &lk.Amount, &lk.State,
			  &lk.CeremonyID, &lk.AirgapID, &lk.SignID, &lk.CeremonyPhase, &lk.AirgapPhase, &lk.SignState, &lk.Actor, &lk.At); err != nil {
			  return nil, nil, nil, err
		  }
		   links[lk.PlanID] = append(links[lk.PlanID], lk)
	  }
	  if err := krows.Err(); err != nil {
		   return nil, nil, nil, err
	   }
	   comps := map[string][]CompRec{}
	  crows, err := q.QueryContext(ctx, `SELECT id, plan_id, parent_leg, comp_leg, amount, state, reason, created_at FROM aa_sweep_proof_comps ORDER BY created_at, id`)
	   if err != nil {
		   return nil, nil, nil, err
	  }
	   defer crows.Close()
	  for crows.Next() {
		   var c CompRec
		   if err := crows.Scan(&c.ID, &c.PlanID, &c.ParentLeg, &c.CompLeg, &c.Amount, &c.State, &c.Reason, &c.CreatedAt); err != nil {
			   return nil, nil, nil, err
		  }
		  comps[c.PlanID] = append(comps[c.PlanID],
			   c)
	   }
	  if err := crows.Err(); err != nil {
		   return nil, nil, nil, err
	   }
	   return plans, links, comps, nil
   }

// ProofTables 迁移里的五张表，给运维目录。

   func ProofTables() []string {
	  return []string{
		   "aa_sweep_proof_plans",
		   "aa_sweep_proof_legs",
		  "aa_sweep_proof_links",
		   "aa_sweep_proof_comps",
		  "aa_sweep_proof_refs",
	   }
  }

  func (o *Orch) sqlReady() bool {
	   return o != nil && o.db != nil && o.db.DB != nil && o.loadErr == nil
  }

// TouchLoadErr 给健康检查看装载是否失败即关。

   func (o *Orch) LoadErr() string {
	  if o == nil {
		  return ""
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   if o.loadErr == nil {
		  return ""
	   }
	   return o.loadErr.Error()
  }

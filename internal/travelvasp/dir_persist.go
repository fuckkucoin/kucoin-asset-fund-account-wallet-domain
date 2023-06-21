package travelvasp

import (
	"context"
	"encoding/json"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	   if s != nil {
		  s.sql = db
	  }
	   return s
   }

  func (s *Service) sqlOn() bool {
	   return s != nil && s.sql != nil && s.sql.DB != nil
   }

// Hydrate 装回目录与出站案件。失败不得清空已有本端/对端种子。
  func (s *Service) Hydrate(ctx context.Context) error {
	   if !s.sqlOn() {
		  return nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	   store := sqlrepo.NewTravelOutbound(s.sql)
	   vasps, err := store.ListVASPs(ctx)
	   if err != nil {
		   return err
	  }
	   for _, row := range vasps {
		  _ = s.PutVASP(vaspFromRow(row))
	  }
	   cases, err := store.ListCases(ctx, "", "", 10000)
	   if err != nil {
		  return err
	  }
	   s.mu.Lock()
	   for _, row := range cases {
		   cs := caseFromRow(row)
		   if opened, err := s.openCaseParties(cs); err == nil {
			  cs = opened
		   }
		   s.cases[cs.ID] = &cs
		   if cs.MovementID != "" {
			   s.byMove[cs.MovementID] = cs.ID
		  }
	   }
	   s.mu.Unlock()
	  return nil
   }

  func (s *Service) persistVASP(v VASP) {
	   if !s.sqlOn() {
		   return
	   }
	   _ = sqlrepo.NewTravelOutbound(s.sql).PutVASP(context.Background(), sqlrepo.TravelVASP{
		  Code: v.Code, Name: v.Name, Country: v.Country, Endpoint: v.Endpoint,
		  DID: v.DID, LEI: v.LEI, Protocol: v.Protocol, Enabled: v.Enabled, Unhosted: v.Unhosted,
		   UpdatedAt: v.UpdatedAt,
	   })
   }

  func (s *Service) persistCase(c Case, actor, from, to, note string) {
	  if !s.sqlOn() {
		  return
	   }
	  stored, err := s.sealCaseParties(c)
	  if err != nil {
		  return
	  }
	  body, _ := json.Marshal(stored.Envelope)
	  _ = sqlrepo.NewTravelOutbound(s.sql).PutCase(context.Background(), sqlrepo.TravelCase{
		   ID:         c.ID,
		   MovementID: c.MovementID, UserID: c.UserID, Status: string(c.Status),
		  MsgID: c.MsgID, Chain: c.Chain, Asset: c.Asset, Amount: c.Amount, Dest: c.Dest,
		   PeerVASP: c.PeerVASP, Corridor: c.Corridor,
		   OriginatorName: stored.Originator.Name,
		  OriginatorAccount: stored.Originator.Account, OriginatorVASP: stored.Originator.VASP,
		   BeneficiaryName: stored.Beneficiary.Name, BeneficiaryAccount: stored.Beneficiary.Account, BeneficiaryVASP: stored.Beneficiary.VASP,
		   Checksum: stored.Checksum, EnvelopeJSON: string(body), Actor: stored.Actor, Note: stored.Note,
		   CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt,
	  })
	  if to != "" {
		  _ = sqlrepo.NewTravelOutbound(s.sql).PutCaseAudit(context.Background(), sqlrepo.TravelCaseAudit{
			  CaseID: c.ID, Actor: actor, From: from, To: to, Note: note, At: c.UpdatedAt,
		  })
	  }
  }

   func vaspFromRow(r sqlrepo.TravelVASP) VASP {
	  return VASP{
		  Code: r.Code, Name: r.Name, Country: r.Country, Endpoint: r.Endpoint,
		  DID: r.DID, LEI: r.LEI, Protocol: r.Protocol, Enabled: r.Enabled, Unhosted: r.Unhosted,
		   UpdatedAt: r.UpdatedAt,
	  }
  }

   func caseFromRow(r sqlrepo.TravelCase) Case {
	   var env Envelope
	  if r.EnvelopeJSON != "" {
		   _ = json.Unmarshal([]byte(r.EnvelopeJSON), &env)
	   }
	  return Case{
		   ID: r.ID, MovementID: r.MovementID, UserID: r.UserID, Status: CaseStatus(r.Status),
		   MsgID: r.MsgID, Chain: r.Chain, Asset: r.Asset, Amount: r.Amount, Dest: r.Dest,
		   PeerVASP: r.PeerVASP, Corridor: r.Corridor,
		  Originator: Party{Name: r.OriginatorName,
			  Account: r.OriginatorAccount, VASP: r.OriginatorVASP},
		  Beneficiary: Party{Name: r.BeneficiaryName, Account: r.BeneficiaryAccount, VASP: r.BeneficiaryVASP},
		   Checksum:    r.Checksum, Envelope: env, Actor: r.Actor, Note: r.Note,
		   CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	  }
   }

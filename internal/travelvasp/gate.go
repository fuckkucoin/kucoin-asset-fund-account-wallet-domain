package travelvasp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/validate"
)

type OutboundReq struct {
	UserID     string `json:"user_id"`
	MovementID string `json:"movement_id"`
	Actor string `json:"actor"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	Dest string `json:"dest"`
	PeerCode string `json:"peer_code"`
	PeerDID  string `json:"peer_did"`
	PeerLEI  string `json:"peer_lei"`
	PeerName string `json:"peer_name"`
	PeerCountry string `json:"peer_country"`
	PeerType    string `json:"peer_type"`
	Originator  Party  `json:"originator"`
	Beneficiary Party  `json:"beneficiary"`
	OrigCountry string `json:"orig_country,omitempty"`
	OrigAddress string `json:"orig_address,omitempty"`
	OrigNatID string `json:"orig_nat_id,omitempty"`
	OrigDOB   string `json:"orig_dob,omitempty"`
	OrigType string `json:"orig_type,omitempty"`
	BeneCountry string `json:"bene_country,omitempty"`
	BeneAddress string `json:"bene_address,omitempty"`
	BeneNatID   string `json:"bene_nat_id,omitempty"`
	BeneDOB     string `json:"bene_dob,omitempty"`
	BeneType    string `json:"bene_type,omitempty"`
}

type GateResult struct {
	Required       bool `json:"required"`
	AllowBroadcast bool `json:"allow_broadcast"`
	Corridor string `json:"corridor,omitempty"`
	Threshold string `json:"threshold,omitempty"`
	Case      Case   `json:"case,omitempty"`
	Peer      VASP   `json:"peer,omitempty"`
	Reason   string `json:"reason,omitempty"`
	DenyCode string `json:"deny_code,omitempty"`
}

   func (s *Service) BindThresholds(book *ThresholdBook) *Service {
	   if s != nil {
		   s.book = book
	   }
	  return s
  }

   func (s *Service) originVASP() VASP {
	   v, err := s.GetVASP(OriginCode())
	   if err != nil {
		  return VASP{Code: OriginCode(), Country: "XX", Protocol: ProtoLOCAL, Enabled: true}
	   }
	   return v
   }

   func (s *Service) corridorOf(origin, peer VASP) string {
	  oc, pc := strings.ToUpper(strings.TrimSpace(origin.Country)), strings.ToUpper(strings.TrimSpace(peer.Country))
	   if oc == "" {
		   oc = "XX"
	   }
	  if pc == "" {
		  pc = "XX"
	  }
	   return CorridorKey(oc, pc)
   }

   func fillParty(p Party, name, account, vasp string) Party {
	   if strings.TrimSpace(p.Name) == "" {
		  p.Name = name
	  }
	   if strings.TrimSpace(p.Account) == "" {
		  p.Account = account
	   }
	  if strings.TrimSpace(p.VASP) == "" {
		  p.VASP = vasp
	  }
	  return p
  }

  func partiesComplete(orig, bene Party) bool {
	  return partyOK(orig) == nil && partyOK(bene) == nil
   }

  func unknownCorridor(origin, peer VASP) bool {
	  if peer.Protocol == ProtoLOCAL || peer.Protocol == ProtoUNHOSTED {
		   return false
	   }
	  oc := strings.ToUpper(strings.TrimSpace(origin.Country))
	  pc := strings.ToUpper(strings.TrimSpace(peer.Country))
	  return (oc == "" || oc == "XX") && (pc == "" || pc == "XX")
   }

// GateOutbound 广播前组出站旅行规则信封并落案件。
// 未达门槛放行；达门槛须解析走廊与对手，缺一即关。
  func (s *Service) GateOutbound(ctx context.Context, req OutboundReq, book *ThresholdBook) (GateResult, error) {
	   start := time.Now()
	   defer func() {
		  metrics.SpanSeconds.WithLabelValues("travel.gate").Observe(time.Since(start).Seconds())
	   }()
	   got, err := s.gateOutbound(ctx, req, book)
	  if err != nil {
		  StampDeny(&got, err)
	  }
	   return got, err
   }

  func (s *Service) gateOutbound(ctx context.Context, req OutboundReq, book *ThresholdBook) (GateResult, error) {
	   if s == nil {
		  return GateResult{}, ErrUnavailable
	  }
	  _ = ctx
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return GateResult{}, ErrActor
	  }
	  as, err := validate.Asset(req.Asset)
	   if err != nil {
		   return GateResult{}, ErrAsset
	  }
	  ch, err := validate.Chain(req.Chain)
	  if err != nil {
		  return GateResult{}, ErrNetwork
	  }
	  if _, err := validate.Idem(req.MovementID); err != nil {
		   return GateResult{}, ErrIdem
	  }
	  actor := strings.TrimSpace(req.Actor)
	   if actor == "" {
		   actor = uid
	  }
	   q := ResolveQuery{
		  Code: req.PeerCode, DID: req.PeerDID, LEI: req.PeerLEI,
		   Name: req.PeerName, Country: req.PeerCountry, Type: req.PeerType,
	  }
	   hops := []string{}
	  if s.fallback != nil {
		  hops = s.fallback.Hops(req.PeerCode)
	  }
	  var peer VASP
	   if strings.TrimSpace(q.Code+q.DID+q.LEI+q.Name+q.Country+q.Type) == "" {
		   // 未指名对手：链上自托管地址按 UNHOSTED 走廊；达门槛再开报文，不得因空查询直接拒单。
		  peer, err = s.RequireEnabled(CatalogUnhosted)
		  if err != nil {
			   return GateResult{}, err
		  }
	  } else {
		   peer, err = s.ResolveHop(q, hops)
		  if err != nil {
			   return GateResult{}, err
		  }
	   }
	   if err := s.RequirePeerHealth(peer.Code, peer.Protocol); err != nil {
		   observeTravelGate("peer_circuit", true)
		  return GateResult{Required: true,
			   Peer: peer}, err
	   }
	   origin := s.originVASP()
	  corridor := s.corridorOf(origin,
		  peer)
	  if book == nil {
		  book = s.book
	  }
	   if book == nil {
		   book = NewThresholdBook("0", nil)
	   }
	   if s.sunrise != nil {
		   if row, ok := s.sunrise.Lookup(peer.Country); ok {
			   _, active := s.sunrise.Active(peer.Country, s.now())
			  if !active {
				   observeTravelGate("sunrise_pending", false)
				  return GateResult{Required: false, AllowBroadcast: true, Corridor: corridor, Peer: peer, Reason: "sunrise pending"}, nil
			  }
			   if strings.TrimSpace(row.MinAmount) != "" {
				   needSun, _, serr := NewThresholdBook(row.MinAmount, nil).RequiresTravel(corridor, as, req.Amount)
				  if serr != nil {
					  observeTravelGate("threshold", true)
					   return GateResult{Corridor: corridor, Threshold: row.MinAmount, Peer: peer}, serr
				  }
				   if !needSun {
					  observeTravelGate("sunrise_below", false)
					  return GateResult{Required: false, AllowBroadcast: true, Corridor: corridor, Threshold: row.MinAmount, Peer: peer}, nil
				  }
			   }
		   }
	  }
	   if pol, ok := s.lookupPolicy(corridor, as); ok {
		  if _, ok := PreferProtocol(pol, peer.Protocol); !ok {
			  observeTravelGate("protocol", true)
			   return GateResult{Required: true,
				  Corridor: corridor, Peer: peer}, ErrProtocol
		  }
	   } else if s.policy != nil {
		   if pol, _ := s.policy.Lookup("DEFAULT", as); pol.FailClosed {
			  observeTravelGate("corridor", true)
			  return GateResult{Corridor: corridor, Peer: peer}, ErrCorridor
		  }
	  }
	  minStr, found := book.Lookup(corridor, as)
	  if !found {
		   observeTravelGate("corridor", true)
		   return GateResult{Corridor: corridor, Peer: peer}, ErrCorridor
	   }
	   need, minStr, err := book.RequiresTravel(corridor, as, req.Amount)
	  if err != nil {
		   observeTravelGate("threshold", true)
		   return GateResult{Corridor: corridor, Threshold: minStr, Peer: peer}, err
	  }
	   if !need {
		   observeTravelGate("skip", false)
		   return GateResult{Required: false, AllowBroadcast: true, Corridor: corridor, Threshold: minStr, Peer: peer}, nil
	  }
	   if unknownCorridor(origin, peer) {
		   observeTravelGate("corridor", true)
		   return GateResult{Required: true, Corridor: corridor, Threshold: minStr, Peer: peer}, ErrCorridor
	  }
	   orig := fillParty(req.Originator, uid, uid, origin.Code)
	  bene := fillParty(req.Beneficiary, strings.TrimSpace(req.Dest), strings.TrimSpace(req.Dest), peer.Code)
	   if strings.TrimSpace(bene.Name) == "" {
		   bene.Name = peer.Name
	   }
	  if !partiesComplete(orig, bene) {
		  return GateResult{Required: true, Corridor: corridor, Threshold: minStr, Peer: peer}, ErrEnvelope
	   }
	   if peer.Protocol != ProtoLOCAL && peer.Protocol != ProtoUNHOSTED {
		  if err := ValidateIVMS(ivmsFromReq(req, orig, bene)); err != nil {
			   observeTravelGate("ivms", true)
			   return GateResult{Required: true, Corridor: corridor, Threshold: minStr, Peer: peer}, err
		  }
	  }
	  env,
		   err := ShapeOnChain(as, req.Amount, ch, orig, bene)
	  if err != nil {
		  return GateResult{Required: true, Corridor: corridor, Peer: peer}, err
	   }
	   msg, err := s.OpenOut(peer.Code, actor, strings.TrimSpace(req.MovementID)+"-trv", env)
	  if err != nil {
		  return GateResult{Required: true, Corridor: corridor, Peer: peer}, err
	   }
	   row := Case{
		   MovementID: strings.TrimSpace(req.MovementID), UserID: uid, Status: CaseRequired,
		   MsgID: msg.ID, Chain: ch, Asset: as, Amount: env.Amount, Dest: strings.TrimSpace(req.Dest),
		   PeerVASP: peer.Code, Corridor: corridor, Originator: orig, Beneficiary: bene,
		   Checksum: env.Checksum, Envelope: env, Actor: actor,
	  }
	  s.mu.Lock()
	   opened, err := s.openCaseLocked(row)
	   s.mu.Unlock()
	   if err != nil {
		   return GateResult{Required: true,
			  Corridor: corridor, Peer: peer}, err
	  }
	  opened, _ = s.progressOutbound(ctx, opened, peer, actor)
	  s.persistCase(opened,
		  actor, string(CaseRequired), string(opened.Status), opened.Note)
	   _, _ = s.LinkWithdraw(ctx, opened.ID, strings.TrimSpace(req.MovementID))
	  allow := opened.Status == CaseAcknowledged
	   reason := ""
	  if !allow {
		   reason = "travel-rule case " + string(opened.Status)
	   }
	  out := GateResult{
		   Required: true,
		   AllowBroadcast: allow, Corridor: corridor, Threshold: minStr,
		  Case: opened, Peer: peer, Reason: reason,
	  }
	   if allow {
		   observeTravelGate("ack", false)
	   } else {
		  observeTravelGate("block", true)
		   if opened.Status == CaseRejected {
			   out.DenyCode = DenyRejected
		  } else {
			   out.DenyCode = DenyPending
		  }
	   }
	  return out, nil
   }

  func (s *Service) lookupPolicy(corridor, asset string) (CorridorPolicy, bool) {
	   if s == nil || s.policy == nil {
		  return CorridorPolicy{}, false
	  }
	   return s.policy.Lookup(corridor, asset)
   }

  func observeTravelGate(result string, alert bool) {
	  if result == "" {
		   result = "unknown"
	   }
	  metrics.FundGates.WithLabelValues("travel", result).Inc()
	   if alert {
		   slo.FireMiddlewareAlertDetail("travel.gate.block", 1, 0, result)
	  }
  }

   func (s *Service) progressOutbound(ctx context.Context, cur Case, peer VASP, actor string) (Case, error) {
	  if cur.Status == CaseRequired {
		  got, err := s.AdvanceCase(cur.ID, actor, CaseSubmitted, "payload submitted")
		  if err != nil {
			  return cur, err
		  }
		   cur = got
		  if _, err := s.AdvanceOut(cur.MsgID, actor, OutQueued); err == nil {
			  _, _ = s.AdvanceOut(cur.MsgID, actor, OutSent)
		  }
	  }
	  if cur.Status != CaseSubmitted {
		   return cur, nil
	  }
	   switch peer.Protocol {
	   case ProtoLOCAL:
		  got, err := s.AdvanceCase(cur.ID, actor, CaseAcknowledged, "local desk ack")
		   if err == nil {
			   _, _ = s.AdvanceOut(cur.MsgID, actor, OutAcked)
			   return got, nil
		  }
	  case ProtoUNHOSTED:
		  if partiesComplete(cur.Originator, cur.Beneficiary) {
			  got, err := s.AdvanceCase(cur.ID, actor, CaseAcknowledged, "unhosted declaration")
			  if err == nil {
				  _, _ = s.AdvanceOut(cur.MsgID, actor, OutAcked)
				   return got, nil
			   }
		  }
		  got, err := s.AdvanceCase(cur.ID, actor, CaseManualReview, "unhosted incomplete")
		  if err == nil {
			   return got, nil
		   }
	   default:
		   rec, err := s.submitAdapter(ctx, peer, cur.Envelope)
		  if err != nil {
			   metrics.FundGates.WithLabelValues("travel", "adapter_fail").Inc()
			   slo.FireMiddlewareAlertDetail("travel.adapter.fail", 1, 0, peer.Protocol)
			  s.RecordPeer(peer.Code, false)
			  s.enqueueOutbox(cur, peer.Protocol, err.Error())
			   return cur, nil
		   }
		  if rec.Status == "acked" && strings.TrimSpace(rec.VendorID) != "" {
			  s.RecordPeer(peer.Code, true)
			  got, aerr := s.AdvanceCase(cur.ID, actor, CaseAcknowledged, "vendor receipt "+rec.VendorID)
			   if aerr == nil {
				  _, _ = s.AdvanceOut(cur.MsgID, actor, OutAcked)
				  return got, nil
			  }
		   }
		   s.RecordPeer(peer.Code, false)
		  s.enqueueOutbox(cur, peer.Protocol, rec.Detail)
	   }
	  return cur, nil
  }

// AllowBroadcast 广播闸：无案件视为未组包失败即关；仅 acknowledged 放行。

  func (s *Service) AllowBroadcast(movementID string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   got, err := s.CaseByMovement(movementID)
	  if err != nil {
		  return ErrBroadcast
	  }
	   if got.Status != CaseAcknowledged {
		  return ErrBroadcast
	   }
	  return nil
   }

// AllowBroadcastOptional 无案件时表示本笔未达门槛，放行；有案件则须已确认。
   func (s *Service) AllowBroadcastOptional(movementID string) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	   got, err := s.CaseByMovement(movementID)
	   if err != nil {
		   if err == ErrCase {
			  return nil
		   }
		   return err
	  }
	  if got.Status != CaseAcknowledged {
		   return ErrBroadcast
	   }
	  return nil
  }

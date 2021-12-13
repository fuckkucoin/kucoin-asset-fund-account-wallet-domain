package travelvasp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type CustomerGate struct {
	thresh *ThresholdBook
	vasp   *Service
	bind   *BindStore
	origin string
	peer   string
	strict bool
}

   func NewCustomerGate(thresh *ThresholdBook, vasp *Service, bind *BindStore, origin, peer string, strict bool) *CustomerGate {
	   if strings.TrimSpace(origin) == "" {
		   origin = OriginCode()
	  }
	   if strings.TrimSpace(peer) == "" {
		   peer = PeerCode()
	   }
	  return &CustomerGate{thresh: thresh, vasp: vasp, bind: bind, origin: origin, peer: peer, strict: strict}
  }

type RequirementsReq struct {
	Asset string
	Amount          string
	BeneficiaryVASP string
	OriginCountry   string
	PeerCountry     string
}

type RequirementsView struct {
	Required bool `json:"required"`
	Corridor string `json:"corridor"`
	Threshold   string `json:"threshold"`
	UsdNotional string `json:"usd_notional,omitempty"`
	Reason      string `json:"reason,omitempty"`
	OriginVASP      string `json:"origin_vasp"`
	BeneficiaryVASP string `json:"beneficiary_vasp"`
}

  func (g *CustomerGate) Requirements(req RequirementsReq) (RequirementsView, error) {
	  if g == nil {
		   return RequirementsView{}, ErrUnavailable
	   }
	   corridor := CorridorKey(req.OriginCountry, req.PeerCountry)
	   if strings.TrimSpace(req.BeneficiaryVASP) != "" {
		   corridor = CorridorKey(g.origin, strings.ToUpper(strings.TrimSpace(req.BeneficiaryVASP)))
	  }
	  need, minStr, err := g.thresh.RequiresTravel(corridor, req.Asset, req.Amount)
	  view := RequirementsView{
		   Corridor:  corridor,
		  Threshold: minStr,
		  OriginVASP: g.origin,
		   BeneficiaryVASP: firstNonEmpty(strings.ToUpper(strings.TrimSpace(req.BeneficiaryVASP)), g.peer),
		   Required:        need,
	  }
	  if err != nil {
		  view.Required = true
		  view.Reason = err.Error()
	   }
	   if g.thresh != nil && strings.TrimSpace(req.Asset) != "" && strings.TrimSpace(req.Amount) != "" {
		  if n, nerr := g.thresh.UsdNotional(req.Asset, req.Amount); nerr == nil {
			  view.UsdNotional = n
		   }
	   }
	   return view, nil
   }

type OpenOutboundReq struct {
	UserID    string
	ClientOid string
	Asset     string
	Amount    string
	Chain     string
	ToAddress string
	BeneficiaryName string
	BeneficiaryVASP string
	Actor           string
}

   func (g *CustomerGate) OpenOutbound(ctx context.Context, req OpenOutboundReq) (Message, error) {
	   if g == nil || g.vasp == nil {
		   return Message{}, ErrUnavailable
	  }
	   beneVASP := strings.ToUpper(strings.TrimSpace(req.BeneficiaryVASP))
	  if beneVASP == "" {
		  beneVASP = g.peer
	  }
	  orig := Party{
		  Name:    "origin:" + strings.TrimSpace(req.UserID),
		   Account: strings.TrimSpace(req.UserID),
		   VASP: g.origin,
	   }
	  bene := Party{
		  Name: strings.TrimSpace(req.BeneficiaryName),
		  Account: strings.TrimSpace(req.ToAddress),
		  VASP:    beneVASP,
	  }
	   if bene.Name == "" {
		  bene.Name = "beneficiary"
	  }
	  env, err := Shape(req.Asset, req.Amount, orig, bene)
	  if err != nil {
		  return Message{}, err
	  }
	  idem := strings.TrimSpace(req.ClientOid)
	  if idem == "" {
		   idem = "travel:" + strings.TrimSpace(req.UserID) + ":" + env.Checksum[:12]
	  }
	   actor := strings.TrimSpace(req.Actor)
	  if actor == "" {
		  actor = "user:" + strings.TrimSpace(req.UserID)
	  }
	   return g.vasp.OpenOut(beneVASP, actor, idem, env)
  }

  func (g *CustomerGate) SubmitOutbound(id, actor string) (Message, error) {
	   if g == nil || g.vasp == nil {
		  return Message{}, ErrUnavailable
	  }
	  if strings.TrimSpace(actor) == "" {
		   actor = "system:submit"
	   }
	   return g.vasp.AdvanceOut(id,
		  actor, OutQueued)
   }

   func (g *CustomerGate) LookupVASP(code string) (VASP, error) {
	   if g == nil || g.vasp == nil {
		   return VASP{}, ErrUnavailable
	   }
	   return g.vasp.GetVASP(code)
  }

   func (g *CustomerGate) GetOutbound(id string) (Message, error) {
	   if g == nil || g.vasp == nil {
		  return Message{}, ErrUnavailable
	  }
	  msg, err := g.vasp.Get(id)
	   if err != nil {
		  return Message{}, err
	  }
	   if msg.Dir != DirOut {
		   return Message{}, ErrDir
	   }
	  return msg, nil
  }

type WithdrawCheck struct {
	UserID    string
	Asset     string
	Amount    string
	Chain     string
	ToAddress string
	BeneficiaryVASP string
	TravelMessageID string
}

// ValidateWithdraw 出金写路径前置：判定是否须旅行报文并校验 messageId 与信封一致性。

  func (g *CustomerGate) ValidateWithdraw(req WithdrawCheck) error {
	   if g == nil {
		  return ErrUnavailable
	   }
	  corridor := CorridorKey(g.origin, firstNonEmpty(strings.ToUpper(strings.TrimSpace(req.BeneficiaryVASP)), g.peer))
	  need, _, err := g.thresh.RequiresTravel(corridor, req.Asset, req.Amount)
	  if err != nil {
		   need = true
	  }
	  if !need {
		   if strings.TrimSpace(req.TravelMessageID) != "" {
			  return nil
		  }
		   return nil
	  }
	   id := strings.TrimSpace(req.TravelMessageID)
	   if id == "" {
		  return errTravelRequired()
	   }
	  msg, err := g.GetOutbound(id)
	   if err != nil {
		  return err
	   }
	  if err := g.assertOutboundReady(msg); err != nil {
		  return err
	   }
	  if err := g.assertEnvelopeMatch(msg.Envelope, req); err != nil {
		   return err
	   }
	   return nil
  }

   func (g *CustomerGate) assertOutboundReady(msg Message) error {
	   switch msg.OutState {
	  case OutAcked, OutSent, OutQueued:
		  return nil
	   case OutDraft:
		   if g.strict {
			   return ErrState
		  }
		  return nil
	  default:
		  return ErrState
	  }
   }

  func (g *CustomerGate) assertEnvelopeMatch(env Envelope, req WithdrawCheck) error {
	   if strings.ToUpper(strings.TrimSpace(env.Asset)) != strings.ToUpper(strings.TrimSpace(req.Asset)) {
		   return ErrHash
	   }
	   amt, err := money.Parse(env.Amount)
	  if err != nil {
		  return ErrHash
	  }
	  want,
		  err := money.Parse(req.Amount)
	  if err != nil || amt.Cmp(want) != 0 {
		   return ErrHash
	   }
	   acct := strings.TrimSpace(env.Beneficiary.Account)
	  if acct != "" && !strings.EqualFold(acct, strings.TrimSpace(req.ToAddress)) {
		   return ErrHash
	   }
	   return nil
  }

   func (g *CustomerGate) BindWithdrawal(ctx context.Context, withdrawalID string, req WithdrawCheck) error {
	  if g.bind == nil {
		  return nil
	  }
	   id := strings.TrimSpace(req.TravelMessageID)
	  if id == "" {
		  return nil
	  }
	   return g.bind.Put(ctx, WithdrawBind{
		   WithdrawalID: withdrawalID,
		  TravelID:     id,
		   UserID: req.UserID,
		   Asset:  req.Asset,
		  Amount: req.Amount,
		   Chain:  req.Chain,
		   Dest:      req.ToAddress,
		  CreatedAt: time.Now().UTC().Format(time.RFC3339),
	  })
  }

  func errTravelRequired() error {
	   return ErrTravelNeed
  }

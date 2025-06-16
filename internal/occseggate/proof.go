package occseggate

import (
	"encoding/json"
	"time"
)

// ProofExport 对外导出视图。

type ProofExport struct {
	ID string `json:"id"`
	Kind      string `json:"kind"`
	TicketID  string `json:"ticket_id"`
	GrantIdem string `json:"grant_idem"`
	SegID string `json:"seg_id"`
	ExpectedGen uint64 `json:"expected_gen"`
	ActualGen uint64 `json:"actual_gen"`
	Reason    string `json:"reason"`
	State     string `json:"state"`
	Digest    string `json:"digest"`
	LedgerFrozen bool `json:"ledger_frozen"`
	AbortWouldOK bool   `json:"abort_would_ok"`
	GhostPair    bool   `json:"ghost_pair"`
	Phase        string `json:"phase_at_issue"`
	Actor string `json:"actor"`
	Note  string `json:"note"`
	CreatedAt string `json:"created_at"`
	AckedAt   string `json:"acked_at,omitempty"`
}

// ToExport 证明转导出。
  func ToExport(p *GhostOccupancyProof) ProofExport {
	  if p == nil {
		  return ProofExport{}
	   }
	  ex := ProofExport{
		  ID: p.ID, Kind: string(p.Kind), TicketID: p.TicketID, GrantIdem: p.GrantIdem,
		  SegID: p.SegID,
		   ExpectedGen: p.ExpectedGen, ActualGen: p.ActualGen,
		   Reason: string(p.Reason), State: string(p.State), Digest: p.Digest,
		  LedgerFrozen: p.LedgerFrozen, AbortWouldOK: p.AbortWouldOK, GhostPair: p.GhostPair(),
		  Phase: p.PhaseAtIssue, Actor: p.Actor, Note: p.Note,
		  CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano),
	  }
	   if p.AckedAt != nil {
		   ex.AckedAt = p.AckedAt.UTC().Format(time.RFC3339Nano)
	  }
	  return ex
  }

// MarshalProof 证明 JSON。

  func MarshalProof(p *GhostOccupancyProof) ([]byte, error) {
	   return json.Marshal(ToExport(p))
   }

// SegExport 占用段导出。

type SegExport struct {
	ID        string `json:"id"`
	TicketID  string `json:"ticket_id"`
	GrantIdem string `json:"grant_idem"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Gen   uint64 `json:"occ_seg_gen"`
	Epoch uint64 `json:"epoch"`
	State string `json:"state"`
	CreatedAt string `json:"created_at"`
}

// ToSegExport 占用段转导出。
   func ToSegExport(s *OccSegGen) SegExport {
	  if s == nil {
		  return SegExport{}
	  }
	   return SegExport{
		  ID: s.ID, TicketID: s.TicketID, GrantIdem: s.GrantIdem,
		  Asset: s.Asset, Amount: s.Amount, Gen: s.Gen, Epoch: s.Epoch,
		   State: string(s.State), CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339Nano),
	   }
  }

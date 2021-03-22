package closelatecredit

import (
	"encoding/json"
	"time"
)

// ProofExport 对外导出视图。

type ProofExport struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	ClaimedUser string `json:"claimed_user"`
	Book         string `json:"book"`
	Source       string `json:"source"`
	Asset        string `json:"asset"`
	Amount       string `json:"amount"`
	TxRef        string `json:"tx_ref"`
	ClaimedEpoch uint64 `json:"claimed_epoch"`
	ActualEpoch uint64 `json:"actual_epoch"`
	Reason      string `json:"reason"`
	State       string `json:"state"`
	Decision string `json:"decision"`
	Digest string `json:"digest"`
	HoldID string `json:"hold_id,omitempty"`
	WrongUser bool `json:"wrong_user"`
	Misroute bool `json:"misroute"`
	Actor     string `json:"actor"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	ClosedAt string `json:"closed_at,omitempty"`
}

// ToExport 证明转导出。
   func ToExport(p *CloseLateCreditProof) ProofExport {
	  if p == nil {
		  return ProofExport{}
	   }
	   ex := ProofExport{
		   ID:     p.ID,
		  UserID: p.UserID, ClaimedUser: p.ClaimedUser, Book: p.Book,
		   Source: string(p.Source), Asset: p.Asset, Amount: p.Amount, TxRef: p.TxRef,
		  ClaimedEpoch: p.ClaimedEpoch, ActualEpoch: p.ActualEpoch,
		  Reason: string(p.Reason), State: string(p.State), Decision: string(p.Decision),
		  Digest: p.Digest, HoldID: p.HoldID, WrongUser: p.WrongUser, Misroute: p.Misroute(),
		  Actor: p.Actor, Note: p.Note,
		   CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano),
	  }
	  if p.ClosedAt != nil {
		  ex.ClosedAt = p.ClosedAt.UTC().Format(time.RFC3339Nano)
	   }
	  return ex
   }

// MarshalProof 证明 JSON。

  func MarshalProof(p *CloseLateCreditProof) ([]byte, error) {
	   return json.Marshal(ToExport(p))
   }

// EpochExport 世代导出。
type EpochExport struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Book       string `json:"book"`
	CloseEpoch uint64 `json:"close_epoch"`
	State      string `json:"state"`
	ClosedEpoch uint64 `json:"closed_epoch"`
	LiveEpoch uint64 `json:"live_epoch"`
	Actor     string `json:"actor"`
	UpdatedAt string `json:"updated_at"`
}

// ToEpochExport 世代转导出。

   func ToEpochExport(e *CloseEpoch) EpochExport {
	   if e == nil {
		  return EpochExport{}
	  }
	  return EpochExport{
		  ID: e.ID, UserID: e.UserID, Book: e.Book, CloseEpoch: e.N,
		   State: string(e.State), ClosedEpoch: e.ClosedEpoch, LiveEpoch: e.LiveEpoch,
		   Actor: e.Actor, UpdatedAt: e.UpdatedAt.UTC().Format(time.RFC3339Nano),
	  }
   }

// HoldExport 争议仓导出。
type HoldExport struct {
	ID string `json:"id"`
	ProofID string `json:"proof_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Source      string `json:"source"`
	EpochAtPark uint64 `json:"epoch_at_park"`
	State string `json:"state"`
	Actor     string `json:"actor"`
	Note      string `json:"resolve_note,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// ToHoldExport 争议仓转导出。

   func ToHoldExport(h *DisputeHoldCase) HoldExport {
	   if h == nil {
		  return HoldExport{}
	   }
	  return HoldExport{
		   ID: h.ID, ProofID: h.ProofID, UserID: h.UserID, Asset: h.Asset, Amount: h.Amount,
		   Source: string(h.Source),
		   EpochAtPark: h.EpochAtPark, State: string(h.State),
		  Actor: h.Actor,
		  Note:  h.ResolveNote,
		   UpdatedAt: h.UpdatedAt.UTC().Format(time.RFC3339Nano),
	   }
   }

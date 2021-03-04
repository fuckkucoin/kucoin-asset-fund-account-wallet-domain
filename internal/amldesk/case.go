package amldesk

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Kind string

const (
	KindSAR Kind = "sar"
	KindKYTHold Kind = "kyt_hold"
	KindTravel  Kind = "travel_rule"
	KindWatch   Kind = "watch"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusEvidence Status = "evidence"
	StatusReview   Status = "review"
	StatusClosed   Status = "closed"
	StatusRejected Status = "rejected"
	StatusEscalated Status = "escalated"
)

type Record struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	MovementID string `json:"movement_id,omitempty"`
	Kind       Kind   `json:"kind"`
	Reason     string `json:"reason"`
	Status     Status `json:"status"`
	OpenedBy   string `json:"opened_by"`
	Reviewer string `json:"reviewer,omitempty"`
	ClosedBy string    `json:"closed_by,omitempty"`
	Note     string    `json:"note,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	ClosedAt time.Time `json:"closed_at,omitempty"`
	Fence persist.Fence `json:"fence"`
}

   func ParseKind(s string) (Kind, error) {
	  switch Kind(strings.ToLower(strings.TrimSpace(s))) {
	   case KindSAR, "":
		   return KindSAR, nil
	   case KindKYTHold:
		   return KindKYTHold, nil
	  case KindTravel:
		  return KindTravel, nil
	  case KindWatch:
		  return KindWatch, nil
	  default:
		  return "", ErrKind
	  }
   }

  func (r Record) Open() bool {
	   return r.Status == StatusOpen || r.Status == StatusEvidence || r.Status == StatusReview || r.Status == StatusEscalated
   }

   func actorOK(actor string) (string, error) {
	  return validate.Actor(actor)
   }

package hotreserve

import (
	"fmt"
	"time"
	"github.com/google/uuid"
)

  func (s *Service) buildJournal(r *Reservation, kind, debit, credit, note string, at time.Time) (ReservationJournal, error) {
	  if r == nil {
		  return ReservationJournal{},
			  ErrUnavailable
	  }
	   if _, err := parsePos(r.Amount); err != nil {
		   return ReservationJournal{}, err
	   }
	  if err := subjects(kind, debit, credit); err != nil {
		  return ReservationJournal{}, err
	   }
	  seq := len(s.journals[r.ID]) + 1
	  j := ReservationJournal{
		   ID: uuid.NewString(),
		  ReservationID: r.ID,
		  Asset:         r.Asset,
		   Seq:   seq,
		  Kind:  kind,
		  Debit: debit,
		   Credit: credit,
		  Amount: r.Amount,
		  EventID: fmt.Sprintf("hr.reserve.%s.%s.%d", r.ID, kind, seq),
		   Note:    note,
		  At: at,
	   }
	   return j, nil
  }

   func (s *Service) rememberJournal(j ReservationJournal) {
	   s.journals[j.ReservationID] = append(s.journals[j.ReservationID],
		   j)
  }

   func subjects(kind, debit, credit string) error {
	  ok := false
	  switch kind {
	  case KindOpen:
		   ok = debit == SubOpen && credit == SubHot
	   case KindEarmark:
		   ok = debit == SubEarmarked && credit == SubOpen
	   case KindEarmarkFreeze:
		  ok = debit == SubFreeze && credit == SubMemoFrz
	   case KindConsume:
		  ok = debit == SubBroadcast && credit == SubEarmarked
	   case KindConsumeFreeze:
		  ok = debit == SubMemoFrz && credit == SubFreeze
	  case KindExpire:
		  ok = debit == SubHot && credit == SubOpen
	  case KindCancel, KindForce:
		  ok = debit == SubHot && (credit == SubOpen || credit == SubEarmarked)
	   case KindCancelFreeze, KindForceFreeze:
		  ok = debit == SubMemoFrz && credit == SubFreeze
	  }
	  if !ok {
		  return ErrState
	  }
	  return nil
   }

   func (s *Service) journalsOf(id string) []ReservationJournal {
	   src := s.journals[id]
	   out := make([]ReservationJournal, len(src))
	  copy(out, src)
	  return out
  }

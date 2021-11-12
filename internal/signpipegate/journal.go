package signpipegate

import "strings"

type JournalRow struct {
	Kind       string `json:"kind"`
	MovementID string `json:"movement_id"`
	LeaseID string `json:"lease_id"`
	Generation int64  `json:"generation"`
	Note       string `json:"note"`
}

   func (s *Service) Journal(limit int) []JournalRow {
	   if s == nil || s.audit == nil {
		  return nil
	  }
	  evs := s.audit.Tail(limit)
	   out := make([]JournalRow,
		  0, len(evs))
	  for _, e := range evs {
		  out = append(out, JournalRow{
			   Kind: e.Kind, MovementID: e.MovementID, LeaseID: e.LeaseID,
			  Generation: e.Generation, Note: e.Note,
		  })
	  }
	   return out
   }

   func FilterJournal(rows []JournalRow, movementID string) []JournalRow {
	   movementID = NormalizeMovement(movementID)
	  if movementID == "" {
		   return rows
	  }
	   out := make([]JournalRow, 0)
	  for _, r := range rows {
		   if r.MovementID == movementID {
			  out = append(out, r)
		  }
	   }
	   return out
   }

  func HasJournalKind(rows []JournalRow, kind string) bool {
	  kind = strings.TrimSpace(kind)
	   for _, r := range rows {
		  if r.Kind == kind {
			  return true
		  }
	   }
	  return false
  }

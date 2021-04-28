package disputedesk

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

  func EvidenceHash(payload []byte) string {
	   sum := sha256.Sum256(payload)
	  return hex.EncodeToString(sum[:])
  }

   func hexOK(s string) bool {
	  s = strings.TrimSpace(s)
	   if len(s) != 64 {
		  return false
	   }
	  for _, r := range s {
		  if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			  return false
		  }
	  }
	  return true
  }

  func CheckEvidence(payload []byte, want string) error {
	  if !hexOK(want) {
		  return ErrHash
	  }
	  if !strings.EqualFold(EvidenceHash(payload), strings.TrimSpace(want)) {
		   return ErrHash
	   }
	   return nil
  }

   func (s *Service) Attach(id, actor, kind, note, sha string) (Evidence, error) {
	   if s == nil {
		  return Evidence{},
			   ErrUnavailable
	  }
	   actor, err := validate.Actor(actor)
	   if err != nil {
		  return Evidence{}, ErrActor
	  }
	  if !hexOK(sha) {
		  return Evidence{}, ErrHash
	   }
	   kind = strings.TrimSpace(kind)
	   if kind == "" || len(kind) > 32 {
		  return Evidence{}, ErrEvidence
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.cases[strings.TrimSpace(id)]
	  if !ok {
		  return Evidence{}, ErrCase
	  }
	  if cur.Status.Terminal() {
		  return Evidence{}, ErrState
	  }
	   for _, e := range s.ev[cur.ID] {
		   if strings.EqualFold(e.SHA256, sha) {
			  return e.Copy(), nil
		  }
	   }
	   now := s.now()
	   row := Evidence{
		   ID: uuid.NewString(), CaseID: cur.ID, SHA256: strings.ToLower(sha),
		   Kind: kind, Note: strings.TrimSpace(note), Actor: actor, At: now,
	   }
	   s.ev[cur.ID] = append(s.ev[cur.ID], row)
	  s.refreshRootLocked(cur.ID)
	  if cur.Status == StatusOpen {
		   cur.Status = StatusEvidence
	   }
	  cur.UpdatedAt = now
	  s.noteLocked(cur.ID, actor, "evidence", kind)
	  return row.Copy(), nil
  }

   func (s *Service) AttachPayload(id, actor, kind, note string, payload []byte) (Evidence, error) {
	   if len(payload) == 0 {
		   return Evidence{}, ErrEvidence
	  }
	   return s.Attach(id, actor, kind, note, EvidenceHash(payload))
   }

   func (s *Service) EvidenceOf(id string) []Evidence {
	  if s == nil {
		  return nil
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  rows := s.ev[strings.TrimSpace(id)]
	   out := make([]Evidence, len(rows))
	  copy(out, rows)
	   return out
  }

  func (s *Service) Verify(id, sha string, payload []byte) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   if err := CheckEvidence(payload, sha); err != nil {
		   return err
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, e := range s.ev[strings.TrimSpace(id)] {
		   if strings.EqualFold(e.SHA256, sha) {
			  return nil
		   }
	   }
	  return ErrEvidence
   }

package airgap

import (
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/secretbox"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func mapBox(err error) error {
	   if err == nil {
		   return nil
	   }
	   switch err {
	   case secretbox.ErrKey:
		   return ErrKey
	  case secretbox.ErrMagic, secretbox.ErrCipher:
		   return ErrCipher
	  case secretbox.ErrEmpty, secretbox.ErrKind, secretbox.ErrHash:
		   return ErrSealed
	   default:
		   return err
	   }
  }

// ExportSealed 把气隙信封再封进 HSA1。口令短于 8 失败即关。
   func (s *Service) ExportSealed(id, actor string, pass []byte) ([]byte, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return nil, ErrActor
	   }
	   var env Envelope
	   var err error
	  cur, gerr := s.Get(id)
	  if gerr != nil {
		   return nil, gerr
	  }
	   if cur.Phase == PhaseSealed {
		   env, err = s.Export(id, actor)
	   } else if cur.Phase == PhaseExported {
		   env, err = s.Envelope(id)
	   } else {
		  return nil, ErrPhase
	   }
	   if err != nil {
		  return nil, err
	  }
	  raw, err := json.Marshal(env)
	   if err != nil {
		   return nil, ErrEnvelope
	  }
	  wrap, err := secretbox.Wrap(secretbox.KindAirgap, raw)
	   if err != nil {
		   return nil, mapBox(err)
	  }
	  blob, err := secretbox.SealEnvelope(secretbox.KindAirgap, pass, wrap)
	   if err != nil {
		   return nil, mapBox(err)
	   }
	   s.mu.Lock()
	  s.sealed[strings.TrimSpace(id)] = append([]byte(nil), blob...)
	   s.mu.Unlock()
	   return blob, nil
   }

   func (s *Service) ImportSealed(pass, blob []byte, actor string) (Session, error) {
	   if s == nil {
		  return Session{}, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		  return Session{}, ErrActor
	   }
	   box, err := secretbox.OpenEnvelope(secretbox.KindAirgap, pass, blob)
	  if err != nil {
		   return Session{}, mapBox(err)
	   }
	  var env Envelope
	   if err := json.Unmarshal(box.Body, &env); err != nil {
		   return Session{}, ErrSealed
	   }
	   return s.Import(env, actor)
   }

   func (s *Service) Sealed(id string) ([]byte, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   b,
		  ok := s.sealed[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrSealed
	   }
	   return append([]byte(nil), b...), nil
   }

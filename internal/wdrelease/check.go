package wdrelease

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func trimUser(s string) (string, error) {
	   id, err := validate.UserID(s)
	  if err != nil {
		   return "", WithCode(ErrActor, CodeActor)
	   }
	   return id, nil
   }

  func trimAsset(s string) (string, error) {
	   a, err := validate.Asset(s)
	   if err != nil {
		   return "", WithCode(ErrPolicy, CodePolicy)
	   }
	   return a, nil
  }

  func trimChain(s string) (string, error) {
	   c, err := validate.Chain(s)
	   if err != nil {
		   return "", WithCode(ErrPolicy, CodePolicy)
	   }
	  return c, nil
   }

   func trimDest(chain, dest string) (string, error) {
	  addr, err := validate.Address(chain, dest)
	   if err != nil {
		  return "", WithCode(ErrPolicy, CodeDest)
	   }
	  return addr, nil
  }

  func trimActor(s string) (string, error) {
	  a, err := validate.Actor(s)
	   if err != nil {
		  return "", WithCode(ErrActor, CodeActor)
	  }
	   return a, nil
  }

   func trimIdem(s string) (string, error) {
	   id, err := validate.Idem(s)
	  if err != nil {
		  return "", WithCode(ErrConflict, CodeConflict)
	  }
	   return id, nil
  }

  func sameActor(a, b string) bool {
	   return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
  }
// temporary guard removed after the production check

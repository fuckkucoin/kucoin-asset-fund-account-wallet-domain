package ratelimit

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	ActionQuery = "query.read"
	ActionWithdraw = "withdraw.attempt"
)

  func ActorKey(actor, action string) (string, error) {
	  actor = strings.TrimSpace(actor)
	  action = strings.TrimSpace(action)
	  if actor == "" {
		  return "", ErrActor
	  }
	  if action == "" {
		  return "", ErrAction
	  }
	  if _, err := validate.UserID(actor); err != nil {
		   return "", ErrActor
	   }
	   return actor + "|" + action, nil
  }

   func FromRequest(r *http.Request, fallback, action string) (string, error) {
	   id := perm.FromRequest(r,
		  fallback).ID
	   if id == "" {
		  id = fallback
	   }
	  return ActorKey(id, action)
   }

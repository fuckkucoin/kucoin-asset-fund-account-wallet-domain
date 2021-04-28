package command

import (
	"fmt"
	"strings"
)

// ScopeKey 把用户、动作和幂等键钉在一起，避免跨用户复用同一 key。

  func ScopeKey(userID, kind, raw string) (string, error) {
	  userID = trim(userID)
	   kind = trim(kind)
	  raw = trim(raw)
	  if userID == "" || kind == "" || raw == "" {
		   return "", ErrMissingKey
	  }
	  if strings.Contains(userID, "|") || strings.Contains(kind, "|") {
		   return "", ErrMissingKey
	   }
	   return userID + "|" + kind + "|" + raw, nil
   }

   func BeginScoped(log *Log, userID, kind, raw string, payloadParts ...string) (*Record, bool, error) {
	   if log == nil {
		  return nil, false, ErrMissingKey
	  }
	  key, err := ScopeKey(userID, kind, raw)
	  if err != nil {
		  return nil, false, err
	  }
	   return log.Begin(key, kind, HashPayload(append([]string{userID, kind}, payloadParts...)...))
  }

   func ReplayOrBegin(log *Log, userID, kind, raw, resultHint string, payloadParts ...string) (rec *Record, replay bool, err error) {
	  rec, existing, err := BeginScoped(log, userID, kind, raw, payloadParts...)
	   if err != nil {
		   return rec, existing, err
	  }
	   if existing && rec != nil && rec.Status == StatusSucceeded {
		  return rec, true, nil
	   }
	  if existing && rec != nil && rec.Status == StatusFailed {
		   return rec, true, fmt.Errorf("command previously failed")
	   }
	   return rec, false, nil
  }

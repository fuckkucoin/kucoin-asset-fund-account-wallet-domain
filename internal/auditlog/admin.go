package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type AdminAction struct {
	Kind string `json:"kind"`
	Actor string `json:"actor"`
	Subject string `json:"subject"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Reason string `json:"reason,omitempty"`
	Fence string `json:"fence,omitempty"`
}

var ErrAdminActor = fmt.Errorf("admin audit actor invalid")

// RecordAdmin 记一条运维动作。system 拒；body 必须带 before/after 或 reason。

   func (s *Store) RecordAdmin(ctx context.Context, a AdminAction) (Entry, error) {
	  if s == nil {
		  return Entry{}, persistConflict()
	  }
	   a.Actor = strings.TrimSpace(a.Actor)
	   a.Kind = strings.TrimSpace(a.Kind)
	   if a.Actor == "" || strings.EqualFold(a.Actor, "system") {
		  return Entry{}, ErrAdminActor
	  }
	   if a.Kind == "" {
		   a.Kind = "admin"
	   }
	   if strings.TrimSpace(a.Before) == "" && strings.TrimSpace(a.After) == "" && strings.TrimSpace(a.Reason) == "" {
		   return Entry{}, persistConflict()
	  }
	  body, err := json.Marshal(a)
	   if err != nil {
		  return Entry{}, err
	  }
	  return s.Append(ctx, Entry{Kind: a.Kind, Actor: a.Actor, Subject: a.Subject, Body: string(body)})
  }

   func persistConflict() error {
	  return fmt.Errorf("admin audit rejected")
   }

   func (s *Store) ListAdmin(subject string, limit int) []Entry {
	   return s.List("admin", subject, limit)
   }

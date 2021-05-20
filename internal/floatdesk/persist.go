package floatdesk

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindFloatDump = "floatdesk_dump"

type dump struct {
	Targets []Target `json:"targets"`
	Tanks   []Tank   `json:"tanks"`
	Samples []Sample `json:"samples"`
	Halt    bool     `json:"halt"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrTarget
	   }
	  body, err := json.Marshal(dump{
		   Targets: s.cat.List(), Tanks: s.tanks.List(), Samples: s.Samples(), Halt: s.halt.Halted(),
	  })
	   if err != nil {
		   return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindFloatDump, key, body)
	  if err != nil {
		  return err
	   }
	  if existed {
		   return persist.Replace(ctx, kv, kindFloatDump, key, body)
	  }
	  return nil
  }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindFloatDump, strings.TrimSpace(key))
	  if err != nil {
		  if err == persist.ErrNotFound {
			  return nil, ErrNotFound
		   }
		   return nil, err
	   }
	  var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	  }
	   s := New()
	  s.cat = NewCatalog()
	   for _, t := range d.Targets {
		  if err := s.cat.Put(t); err != nil {
			   return nil, err
		  }
	   }
	   s.tanks = NewTanks()
	   for _, t := range d.Tanks {
		   if err := s.tanks.Put(t); err != nil {
			  return nil, err
		  }
	  }
	   for _, sm := range d.Samples {
		  if err := s.Record(sm); err != nil {
			   return nil, err
		  }
	   }
	  if d.Halt {
		  s.halt.HaltAll()
	  }
	   return s, nil
   }

package addrintel

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindIntelDump = "addrintel_dump"

type dump struct {
	Hits  []Hit             `json:"hits"`
	Tags  map[string]string `json:"tags"`
	Marks map[string]Kind   `json:"marks"`
	Sanctions []string `json:"sanctions"`
}

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrAddress
	   }
	   body, err := json.Marshal(dump{
		   Hits: s.book.Hits(), Tags: s.tags.by, Marks: s.class.mark, Sanctions: s.san.Keys(),
	  })
	   if err != nil {
		   return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindIntelDump, key, body)
	  if err != nil {
		  return err
	   }
	   if existed {
		   return persist.Replace(ctx, kv, kindIntelDump, key, body)
	  }
	   return nil
  }

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindIntelDump, strings.TrimSpace(key))
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
	   for _, h := range d.Hits {
		   if _, err := s.book.Observe(h.Chain, h.Address, h.UserID, h.At); err != nil {
			  return nil, err
		  }
	  }
	   for k, tag := range d.Tags {
		   parts := strings.SplitN(k, "|", 2)
		  if len(parts) != 2 {
			  continue
		   }
		  if err := s.tags.Tag(parts[0], parts[1], tag); err != nil {
			  return nil, err
		   }
	   }
	  for k, kind := range d.Marks {
		  parts := strings.SplitN(k, "|", 2)
		   if len(parts) != 2 {
			   continue
		   }
		  if err := s.class.Mark(parts[0], parts[1], kind); err != nil {
			  return nil, err
		   }
	  }
	  for _, k := range d.Sanctions {
		   parts := strings.SplitN(k, "|", 2)
		  if len(parts) != 2 {
			  continue
		   }
		  if err := s.san.Add(parts[0], parts[1]); err != nil {
			   return nil, err
		   }
	   }
	  return s, nil
   }

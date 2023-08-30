package webhook

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindWebhookDump = "webhook_dump"

type dump struct {
	Endpoints []Endpoint `json:"endpoints"`
	Dead   []Dead   `json:"dead"`
	Nonces []string `json:"nonces"`
}

  func (d *Dispatcher) Save(ctx context.Context, kv persist.KV, key string) error {
	  if d == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrEndpoint
	   }
	   d.mu.Lock()
	  nonces := make([]string, 0, len(d.used))
	   for n := range d.used {
		   nonces = append(nonces, n)
	   }
	   d.mu.Unlock()
	  body, err := json.Marshal(dump{
		  Endpoints: d.reg.listRaw(),
		  Dead:   d.dlq.List(),
		  Nonces: nonces,
	  })
	   if err != nil {
		   return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindWebhookDump, key, body)
	  if err != nil {
		  return err
	  }
	   if existed {
		  return persist.Replace(ctx, kv, kindWebhookDump, key, body)
	  }
	  return nil
   }

  func Load(ctx context.Context, kv persist.KV, key string, sink Sink) (*Dispatcher, error) {
	  if kv == nil {
		  return nil, ErrUnavailable
	   }
	   row,
		  err := kv.Get(ctx, kindWebhookDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrDead
		   }
		  return nil, err
	   }
	  var dump dump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		  return nil, err
	   }
	   d := New(sink)
	  for _, ep := range dump.Endpoints {
		  if err := d.reg.Put(ep); err != nil {
			  return nil, err
		  }
	   }
	  for _, dead := range dump.Dead {
		   d.dlq.Add(Endpoint{ID: dead.Endpoint},
			   dead.Env, dead.Attempts, errFrom(dead.LastErr))
	  }
	   d.mu.Lock()
	   for _, n := range dump.Nonces {
		  d.used[n] = struct{}{}
	   }
	  d.mu.Unlock()
	   return d, nil
  }

  func errFrom(s string) error {
	  if s == "" {
		   return nil
	   }
	   return &strErr{s: s}
   }

type strErr struct{ s string }

   func (e *strErr) Error() string { return e.s }

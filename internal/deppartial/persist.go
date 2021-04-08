package deppartial

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindSnapshot = "deppartial_snapshot"

type snapshot struct {
	BatchStates map[string]BatchResidueState `json:"batch_states"`
	BatchItems map[string]int     `json:"batch_items"`
	Residues   []ObserveResidue   `json:"residues,omitempty"`
	Leases     []ReopenLease      `json:"leases,omitempty"`
	Proofs     []CreditAlignProof `json:"proofs,omitempty"`
	Fences         []SeenFence `json:"fences,omitempty"`
	CreditedHashes []string    `json:"credited_hashes,omitempty"`
	GenCounter int64 `json:"gen_counter"`
}

// Save 持久化快照。
   func (svc *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	   if svc == nil || kv == nil {
		   return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrBatchNotFound
	   }
	   svc.mu.Lock()
	  snap := snapshot{
		   BatchStates: make(map[string]BatchResidueState),
		   BatchItems:  make(map[string]int),
		   GenCounter:  svc.store.CurrentGen(),
	  }
	   for _, id := range svc.store.AllBatchIDs() {
		  if s, ok := svc.store.GetBatchState(id); ok {
			   snap.BatchStates[id] = s
		   }
		   snap.BatchItems[id] = svc.store.GetBatchItems(id)
		   snap.Residues = append(snap.Residues, svc.store.ListResiduesByBatch(id)...)
		   snap.Proofs = append(snap.Proofs, svc.store.ListProofsByBatch(id)...)
		   snap.Fences = append(snap.Fences, svc.store.ListFencesByBatch(id)...)
	   }
	  for _, l := range svc.store.AllLeases() {
		  snap.Leases = append(snap.Leases,
			  l)
	   }
	  svc.mu.Unlock()
	  body, err := json.Marshal(snap)
	   if err != nil {
		  return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindSnapshot, key, body)
	   if err != nil {
		  return err
	   }
	   if existed {
		  return persist.Replace(ctx, kv, kindSnapshot, key, body)
	  }
	   return nil
  }

// Load 从持久化加载。
  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	  }
	  row, err := kv.Get(ctx, kindSnapshot, strings.TrimSpace(key))
	  if err != nil {
		  if err == persist.ErrNotFound {
			   return nil, ErrBatchNotFound
		   }
		   return nil, err
	   }
	  var snap snapshot
	   if err := json.Unmarshal(row.Body, &snap); err != nil {
		   return nil, err
	   }
	   svc := New()
	   for batchID, state := range snap.BatchStates {
		  svc.store.RegisterBatch(batchID, snap.BatchItems[batchID])
		   svc.store.SetBatchState(batchID,
			  state)
	  }
	   for i := range snap.Residues {
		  cp := snap.Residues[i]
		   svc.store.PutResidue(&cp)
	   }
	  for i := range snap.Leases {
		  cp := snap.Leases[i]
		   svc.store.PutLease(&cp)
	   }
	   for i := range snap.Proofs {
		   cp := snap.Proofs[i]
		  svc.store.PutProof(&cp)
	   }
	   for i := range snap.Fences {
		  cp := snap.Fences[i]
		  svc.store.PutFence(&cp)
	   }
	  return svc, nil
   }

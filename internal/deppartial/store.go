package deppartial

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 内存残余存储。环境替换为 SQL 实现（dep_observe_residues / dep_reopen_leases / dep_credit_proofs）。

type Store struct {
	mu       sync.Mutex
	residues map[string]*ObserveResidue   // id → residue
	leases   map[string]*ReopenLease      // id → lease
	proofs   map[string]*CreditAlignProof // id → proof
	fences   map[string]*SeenFence        // fenceKey → fence
	batchStates map[string]BatchResidueState
	batchItems map[string]int // batch_id → total_items
	byBatchResidue map[string][]string // batch_id → residue ids
	byBatchLease map[string]string   // batch_id → active lease id
	byBatchProof map[string][]string // batch_id → proof ids
	creditedHashes map[string]string // chain|tx_hash → residue_id (已贷记的)
	genCounter     int64
}

   func NewStore() *Store {
	   return &Store{
		   residues: make(map[string]*ObserveResidue),
		  leases: make(map[string]*ReopenLease),
		   proofs:      make(map[string]*CreditAlignProof),
		  fences:      make(map[string]*SeenFence),
		   batchStates: make(map[string]BatchResidueState),
		  batchItems:     make(map[string]int),
		  byBatchResidue: make(map[string][]string),
		   byBatchLease:   make(map[string]string),
		   byBatchProof:   make(map[string][]string),
		  creditedHashes: make(map[string]string),
		  genCounter:     0,
	   }
  }

  func fenceKey(chain, hash string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(hash))
   }

  func (st *Store) NextGen() int64 {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   st.genCounter++
	   return st.genCounter
  }

  func (st *Store) CurrentGen() int64 {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  return st.genCounter
   }

// ── Batch state ──

  func (st *Store) RegisterBatch(batchID string, totalItems int) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	  if _, ok := st.batchStates[batchID]; !ok {
		  st.batchStates[batchID] = BRSubmitted
	  }
	  st.batchItems[batchID] = totalItems
   }

  func (st *Store) GetBatchState(batchID string) (BatchResidueState, bool) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	   s, ok := st.batchStates[batchID]
	   return s, ok
   }

  func (st *Store) SetBatchState(batchID string, s BatchResidueState) {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   st.batchStates[batchID] = s
  }

   func (st *Store) GetBatchItems(batchID string) int {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   return st.batchItems[batchID]
  }

// ── Residue ──

   func (st *Store) PutResidue(r *ObserveResidue) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  st.residues[r.ID] = r
	  ids := st.byBatchResidue[r.BatchID]
	  found := false
	  for _, id := range ids {
		  if id == r.ID {
			   found = true
			  break
		  }
	   }
	   if !found {
		  st.byBatchResidue[r.BatchID] = append(ids, r.ID)
	   }
   }

  func (st *Store) GetResidue(id string) (*ObserveResidue, bool) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	  r, ok := st.residues[id]
	  if !ok {
		  return nil, false
	  }
	   cp := r.Copy()
	  return &cp, true
   }

   func (st *Store) UpdateResidue(r *ObserveResidue) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  st.residues[r.ID] = r
  }

  func (st *Store) ListResiduesByBatch(batchID string) []ObserveResidue {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  ids := st.byBatchResidue[batchID]
	   out := make([]ObserveResidue, 0, len(ids))
	  for _, id := range ids {
		   if r, ok := st.residues[id]; ok {
			  out = append(out, r.Copy())
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	   return out
   }

  func (st *Store) CountOpenResidues(batchID string) int {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	  ids := st.byBatchResidue[batchID]
	  count := 0
	  for _, id := range ids {
		  if r, ok := st.residues[id]; ok && (r.State == ResOpen || r.State == ResEscalated) {
			   count++
		   }
	  }
	   return count
   }

  func (st *Store) CountResiduesByState(batchID string) map[ResidueState]int {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   ids := st.byBatchResidue[batchID]
	   m := make(map[ResidueState]int)
	   for _, id := range ids {
		   if r, ok := st.residues[id]; ok {
			  m[r.State]++
		   }
	  }
	  return m
  }

// ── Credited hash tracking ──

   func (st *Store) MarkCredited(chain, txHash string) bool {
	   k := fenceKey(chain, txHash)
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   if _, ok := st.creditedHashes[k]; ok {
		  return false
	  }
	   st.creditedHashes[k] = ""
	   return true
   }

   func (st *Store) IsCredited(chain, txHash string) bool {
	   k := fenceKey(chain, txHash)
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   _, ok := st.creditedHashes[k]
	  return ok
  }

// ── Lease ──

   func (st *Store) PutLease(l *ReopenLease) {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  st.leases[l.ID] = l
	  st.byBatchLease[l.BatchID] = l.ID
   }

   func (st *Store) GetLease(id string) (*ReopenLease, bool) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	   l, ok := st.leases[id]
	  if !ok {
		  return nil, false
	   }
	   cp := l.Copy()
	  return &cp, true
  }

   func (st *Store) GetActiveLease(batchID string) (*ReopenLease, bool) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  id, ok := st.byBatchLease[batchID]
	   if !ok {
		   return nil, false
	  }
	  l, ok := st.leases[id]
	   if !ok {
		  return nil, false
	   }
	  if l.Status != LeaseHeld {
		  return nil, false
	   }
	  cp := l.Copy()
	  return &cp, true
  }

   func (st *Store) UpdateLease(l *ReopenLease) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  st.leases[l.ID] = l
	  if l.Status != LeaseHeld {
		  if cur, ok := st.byBatchLease[l.BatchID]; ok && cur == l.ID {
			  delete(st.byBatchLease, l.BatchID)
		   }
	   }
   }

  func (st *Store) AllLeases() []ReopenLease {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   out := make([]ReopenLease,
		   0, len(st.leases))
	  for _, l := range st.leases {
		  out = append(out, l.Copy())
	  }
	   return out
   }

// ── Proof ──

   func (st *Store) PutProof(p *CreditAlignProof) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  st.proofs[p.ID] = p
	   ids := st.byBatchProof[p.BatchID]
	   found := false
	   for _, id := range ids {
		   if id == p.ID {
			  found = true
			  break
		  }
	  }
	  if !found {
		  st.byBatchProof[p.BatchID] = append(ids, p.ID)
	  }
   }

  func (st *Store) GetProof(id string) (*CreditAlignProof, bool) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   p, ok := st.proofs[id]
	   if !ok {
		   return nil, false
	   }
	   cp := p.Copy()
	   return &cp, true
   }

   func (st *Store) ListProofsByBatch(batchID string) []CreditAlignProof {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   ids := st.byBatchProof[batchID]
	   out := make([]CreditAlignProof,
		  0, len(ids))
	   for _, id := range ids {
		  if p, ok := st.proofs[id]; ok {
			  out = append(out, p.Copy())
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	   return out
   }

  func (st *Store) AllProofsPassed(batchID string) bool {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	   ids := st.byBatchProof[batchID]
	  if len(ids) == 0 {
		   return false
	  }
	   for _, id := range ids {
		  if p, ok := st.proofs[id]; ok {
			   if p.Status != ProofPassed {
				   return false
			  }
		   }
	   }
	   totalItems := st.batchItems[batchID]
	   return len(ids) >= totalItems
  }

// ── Fence ──

   func (st *Store) PutFence(f *SeenFence) {
	   k := fenceKey(f.Chain, f.TxHash)
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  st.fences[k] = f
  }

   func (st *Store) GetFence(chain, txHash string) (*SeenFence, bool) {
	  k := fenceKey(chain, txHash)
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   f, ok := st.fences[k]
	   if !ok {
		   return nil, false
	  }
	   cp := f.Copy()
	   return &cp, true
  }

   func (st *Store) CloseFence(chain, txHash string) bool {
	   k := fenceKey(chain, txHash)
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   f, ok := st.fences[k]
	  if !ok {
		  return false
	  }
	  f.Closed = true
	  return true
   }

  func (st *Store) IsFenceClosed(chain, txHash string) bool {
	  k := fenceKey(chain, txHash)
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   f, ok := st.fences[k]
	   if !ok {
		   return false
	  }
	  return f.Closed
  }

  func (st *Store) ListFencesByBatch(batchID string) []SeenFence {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  var out []SeenFence
	   for _, f := range st.fences {
		  if f.BatchID == batchID {
			  out = append(out, f.Copy())
		   }
	   }
	   return out
   }

// ── View ──

   func (st *Store) BuildView(batchID string) BatchResidueView {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   state, ok := st.batchStates[batchID]
	   if !ok {
		  return BatchResidueView{BatchID: batchID}
	  }
	  rIDs := st.byBatchResidue[batchID]
	  openCount := 0
	   creditedCount := 0
	   for _, id := range rIDs {
		   if r, rok := st.residues[id]; rok {
			   switch r.State {
			  case ResOpen, ResEscalated:
				  openCount++
			   case ResCredited:
				  creditedCount++
			   }
		   }
	   }
	   hasLease := false
	   holder := ""
	   if lid, lok := st.byBatchLease[batchID]; lok {
		  if l, lrok := st.leases[lid]; lrok && l.Status == LeaseHeld {
			  hasLease = true
			   holder = l.Holder
		   }
	   }
	  pIDs := st.byBatchProof[batchID]
	   allAligned := len(pIDs) > 0 && len(pIDs) >= st.batchItems[batchID]
	  if allAligned {
		  for _, pid := range pIDs {
			   if p, pok := st.proofs[pid]; pok && p.Status != ProofPassed {
				   allAligned = false
				  break
			   }
		   }
	   }
	  canClose := openCount == 0 && allAligned && state != BRClosed
	  return BatchResidueView{
		   BatchID: batchID,
		  State:         state,
		  TotalItems:    st.batchItems[batchID],
		   ResidueCount:  len(rIDs),
		   CreditedCount: creditedCount,
		   OpenCount: openCount,
		   HasLease:    hasLease,
		   LeaseHolder: holder,
		   ProofCount:  len(pIDs),
		   AllAligned:  allAligned,
		   CanClose:    canClose,
	   }
  }

// ── 批次列表 ──

   func (st *Store) AllBatchIDs() []string {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   out := make([]string, 0, len(st.batchStates))
	  for id := range st.batchStates {
		  out = append(out, id)
	  }
	  sort.Strings(out)
	  return out
   }

  func (st *Store) BatchesInState(s BatchResidueState) []string {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   var out []string
	  for id, state := range st.batchStates {
		  if state == s {
			   out = append(out, id)
		   }
	   }
	   sort.Strings(out)
	   return out
  }

// ── 租约过期检查 ──

   func (st *Store) ExpiredLeases(now time.Time) []ReopenLease {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  var out []ReopenLease
	   for _, l := range st.leases {
		  if l.Status == LeaseHeld && now.After(l.ExpiresAt) {
			  out = append(out, l.Copy())
		  }
	   }
	  return out
   }

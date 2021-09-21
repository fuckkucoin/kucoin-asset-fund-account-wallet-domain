package occupbucketfence

import (
	"encoding/json"
	"sort"
	"strings"
)

// ProofView 导出视图。

type ProofView struct {
	ProofID string `json:"proof_id"`
	LotID   string `json:"lot_id"`
	FenceID  string `json:"bucket_fence_id"`
	State    string `json:"state"`
	Kind     string `json:"kind"`
	Inflight string `json:"inflight"`
	Available string `json:"available"`
	Void string `json:"void"`
	Sum string `json:"sum"`
	Total string `json:"total"`
	Delta      string `json:"delta"`
	ConflictOp string `json:"conflict_op"`
	WinnerOp string `json:"winner_op"`
	Reason   string `json:"reason"`
	Digest   string `json:"digest"`
}

// ViewOf 转换证明。
   func ViewOf(p *OccupancyBreakProof) *ProofView {
	   if p == nil {
		  return nil
	  }
	  return &ProofView{
		   ProofID: p.ProofID,
		  LotID:   p.LotID,
		   FenceID: p.FenceID,
		   State:     string(p.State),
		   Kind:      p.Kind,
		  Inflight:  p.Inflight,
		  Available: p.Available,
		  Void: p.Void,
		   Sum: p.Sum,
		  Total: p.Total,
		  Delta:      p.Delta,
		  ConflictOp: p.ConflictOp,
		  WinnerOp:   p.WinnerOp,
		   Reason:     p.Reason,
		  Digest:     p.Digest,
	   }
   }

// SortProofs 按 opened 稳定排序（用 ProofID）。

  func SortProofs(list []*OccupancyBreakProof) {
	   sort.SliceStable(list, func(i, j int) bool {
		   if list[i].OpenedAt.Equal(list[j].OpenedAt) {
			  return list[i].ProofID < list[j].ProofID
		   }
		   return list[i].OpenedAt.Before(list[j].OpenedAt)
	  })
  }

// MarshalProofs 稳定 JSON。

   func MarshalProofs(list []*OccupancyBreakProof) ([]byte, error) {
	   views := make([]*ProofView, 0, len(list))
	  for _, p := range list {
		  views = append(views, ViewOf(p))
	  }
	   return json.Marshal(views)
   }

// KindClass 归类证明种类（夹缝/指纹/巡检）。
   func KindClass(kind string) string {
	   switch strings.TrimSpace(kind) {
	  case "reclass_release_mutex", "double_exit", "fence_cas_conflict":
		   return "mutex"
	   case "fingerprint_mismatch":
		  return "fence"
	   case "conserve_broken", "hold_conserve", "resume_conserve", "post_write_conserve", "patrol_conserve":
		  return "conserve"
	   default:
		  return "other"
	   }
  }

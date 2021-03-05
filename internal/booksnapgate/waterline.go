package booksnapgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// WaterlineSet 一组科目水线。
type WaterlineSet struct {
	UserID       string
	Asset        string
	Observations []WaterlineObservation
}

// NewWaterlineSet 创建水线集合。

   func NewWaterlineSet(userID, asset string, obs []WaterlineObservation) WaterlineSet {
	  sorted := make([]WaterlineObservation,
		  len(obs))
	   copy(sorted, obs)
	  sort.SliceStable(sorted, func(i, j int) bool {
		  return sorted[i].Book < sorted[j].Book
	  })
	  return WaterlineSet{
		   UserID: strings.TrimSpace(userID),
		   Asset: strings.ToUpper(strings.TrimSpace(asset)),
		   Observations: sorted,
	   }
   }

// Checksum 水线集合指纹。
  func (ws WaterlineSet) Checksum() string {
	  h := sha256.New()
	   h.Write([]byte(ws.UserID))
	  h.Write([]byte{0})
	  h.Write([]byte(ws.Asset))
	  h.Write([]byte{0})
	  for _, obs := range ws.Observations {
		   h.Write([]byte(obs.Book))
		  h.Write([]byte{0})
		  h.Write([]byte(obs.Waterline))
		  h.Write([]byte{0})
		  h.Write([]byte(fmt.Sprintf("%d", obs.ObserveGen)))
		   h.Write([]byte{0})
	   }
	  return hex.EncodeToString(h.Sum(nil))
   }

// GetBook 按科目名取水线。

  func (ws WaterlineSet) GetBook(book string) (WaterlineObservation, bool) {
	   book = strings.ToLower(strings.TrimSpace(book))
	  for _, obs := range ws.Observations {
		   if strings.ToLower(obs.Book) == book {
			   return obs, true
		   }
	  }
	   return WaterlineObservation{}, false
   }

// Books 返回所有科目名。

  func (ws WaterlineSet) Books() []string {
	  out := make([]string, 0, len(ws.Observations))
	  for _, obs := range ws.Observations {
		   out = append(out, obs.Book)
	   }
	  return out
  }

// DiffWaterlines 两组水线，返回漂移科目列表。
  func DiffWaterlines(pinned, current WaterlineSet) []WaterlineDiff {
	   pinnedMap := make(map[string]WaterlineObservation, len(pinned.Observations))
	  for _, obs := range pinned.Observations {
		  pinnedMap[obs.Book] = obs
	   }
	   var diffs []WaterlineDiff
	   for _, cur := range current.Observations {
		  pin, ok := pinnedMap[cur.Book]
		   if !ok {
			   diffs = append(diffs, WaterlineDiff{
				   Book: cur.Book,
				   Kind: DiffKindNew,
				  NewVal: cur.Waterline,
				   NewGen: cur.ObserveGen,
			   })
			   continue
		   }
		   if cur.Waterline != pin.Waterline {
			  diffs = append(diffs, WaterlineDiff{
				  Book: cur.Book,
				  Kind: DiffKindWaterline,
				  OldVal: pin.Waterline,
				   NewVal: cur.Waterline,
				  OldGen: pin.ObserveGen,
				  NewGen: cur.ObserveGen,
			   })
		  } else if cur.ObserveGen != pin.ObserveGen {
			   diffs = append(diffs, WaterlineDiff{
				  Book: cur.Book,
				  Kind:   DiffKindGen,
				  OldVal: pin.Waterline,
				  NewVal: cur.Waterline,
				   OldGen: pin.ObserveGen,
				   NewGen: cur.ObserveGen,
			   })
		  } else if cur.Checksum != pin.Checksum {
			   diffs = append(diffs, WaterlineDiff{
				   Book: cur.Book,
				   Kind: DiffKindChecksum,
				  OldVal: pin.Checksum,
				  NewVal: cur.Checksum,
			  })
		  }
	   }
	  return diffs
  }

type DiffKind string

const (
	DiffKindWaterline DiffKind = "waterline"
	DiffKindGen       DiffKind = "gen"
	DiffKindChecksum  DiffKind = "checksum"
	DiffKindNew DiffKind = "new_book"
)

type WaterlineDiff struct {
	Book string `json:"book"`
	Kind DiffKind `json:"kind"`
	OldVal string `json:"old_val,omitempty"`
	NewVal string `json:"new_val,omitempty"`
	OldGen int64  `json:"old_gen,omitempty"`
	NewGen int64  `json:"new_gen,omitempty"`
}

// HasDrift 检查是否有任何漂移。
   func HasDrift(diffs []WaterlineDiff) bool {
	  return len(diffs) > 0
  }

// FilterByKind 按漂移类型过滤。
   func FilterByKind(diffs []WaterlineDiff, kind DiffKind) []WaterlineDiff {
	   var out []WaterlineDiff
	   for _, d := range diffs {
		  if d.Kind == kind {
			  out = append(out, d)
		   }
	  }
	  return out
   }

// SummarizeWaterlines 汇总水线集合信息。
  func SummarizeWaterlines(ws WaterlineSet) string {
	   var parts []string
	  for _, obs := range ws.Observations {
		   parts = append(parts, fmt.Sprintf("%s:gen=%d,wl=%s", obs.Book, obs.ObserveGen, obs.Waterline))
	   }
	   return strings.Join(parts, "; ")
   }

package por

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

   func hashPair(left, right []byte) []byte {
	   h := sha256.New()
	  _, _ = h.Write(left)
	  _, _ = h.Write(right)
	  return h.Sum(nil)
   }

type Tree struct {
	Leaves []Leaf     `json:"leaves"`
	Layers [][][]byte `json:"-"`
	Root   string     `json:"root"`
}

   func Build(leaves []Leaf) (Tree, error) {
	   if len(leaves) == 0 {
		  return Tree{}, ErrEmpty
	   }
	   sorted := append([]Leaf(nil), leaves...)
	   sort.SliceStable(sorted, func(i, j int) bool {
		   return sorted[i].Canonical() < sorted[j].Canonical()
	  })
	   level := make([][]byte, 0, len(sorted))
	  for _, l := range sorted {
		   if l.UserID == "" || l.Asset == "" {
			  return Tree{}, ErrLeaf
		   }
		  level = append(level, l.Hash())
	   }
	   layers := [][][]byte{level}
	   for len(level) > 1 {
		  if len(level)%2 == 1 {
			  level = append(level,
				  level[len(level)-1])
		   }
		  next := make([][]byte, 0, len(level)/2)
		   for i := 0; i < len(level); i += 2 {
			   next = append(next, hashPair(level[i], level[i+1]))
		   }
		  layers = append(layers, next)
		   level = next
	  }
	  return Tree{
		   Leaves: sorted, Layers: layers,
		  Root: hex.EncodeToString(level[0]),
	  }, nil
  }

   func (t Tree) RootBytes() []byte {
	  b, _ := hex.DecodeString(t.Root)
	   return b
  }

  func (t Tree) IndexOf(leaf Leaf) int {
	   want := leaf.Canonical()
	   for i, l := range t.Leaves {
		  if l.Canonical() == want {
			   return i
		   }
	   }
	  return -1
   }

package solvency

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

type Tree struct {
	Leaves []LiabilityLeaf
	Layers [][][]byte
	Root   string
}

  func hashPair(left, right []byte) []byte {
	  h := sha256.New()
	  _, _ = h.Write(left)
	   _, _ = h.Write(right)
	  return h.Sum(nil)
   }

  func sortLeaves(in []LiabilityLeaf) {
	   sort.SliceStable(in, func(i, j int) bool {
		  if in[i].UserID != in[j].UserID {
			   return in[i].UserID < in[j].UserID
		  }
		   if in[i].Book != in[j].Book {
			   return in[i].Book < in[j].Book
		  }
		   return in[i].Asset < in[j].Asset
	   })
   }

// EmptyRoot 无用户负债时的固定根。空切片不能冒充「未计算」。

   func EmptyRoot() string {
	  sum := sha256.Sum256([]byte("solvency-empty-v1"))
	  return hex.EncodeToString(sum[:])
   }

// BuildTree 由负债叶生成根。叶身份或金额为空则整棵树失败，不跳过坏叶。

  func BuildTree(leaves []LiabilityLeaf) (Tree, error) {
	   if len(leaves) == 0 {
		  return Tree{Root: EmptyRoot()}, nil
	  }
	   sorted := append([]LiabilityLeaf(nil), leaves...)
	   sortLeaves(sorted)
	   seen := map[string]struct{}{}
	  level := make([][]byte, 0, len(sorted))
	   for i := range sorted {
		  l := &sorted[i]
		   if l.UserID == "" || l.Asset == "" || l.Book == "" || l.LeafHash == "" {
			   return Tree{}, ErrLeaf
		  }
		   if !stringsHasPrefix(l.Canonical(), leafPrefix) {
			  return Tree{}, ErrNotSolvency
		   }
		  if _, ok := seen[l.Key()]; ok {
			  return Tree{}, ErrDuplicate
		  }
		  seen[l.Key()] = struct{}{}
		   sum := l.Hash()
		   if hex.EncodeToString(sum) != l.LeafHash {
			   return Tree{}, ErrLeaf
		  }
		   level = append(level, sum)
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
	   return Tree{Leaves: sorted,
		   Layers: layers, Root: hex.EncodeToString(level[0])}, nil
   }

   func stringsHasPrefix(s, prefix string) bool {
	   return len(s) >= len(prefix) && s[:len(prefix)] == prefix
   }

  func (t Tree) IndexOf(leaf LiabilityLeaf) int {
	   want := leaf.Key()
	   for i, l := range t.Leaves {
		  if l.Key() == want {
			   return i
		  }
	  }
	   return -1
   }

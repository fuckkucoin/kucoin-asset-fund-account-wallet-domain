package solvency

import (
	"bytes"
	"encoding/hex"
)

type Step struct {
	Sibling string `json:"sibling"`
	Right bool `json:"right"`
}

type Proof struct {
	Leaf LiabilityLeaf `json:"leaf"`
	Path []Step        `json:"path"`
	Index int    `json:"index"`
	Root  string `json:"root"`
}

  func (t Tree) Prove(leaf LiabilityLeaf) (Proof, error) {
	  if t.Root == "" {
		  return Proof{}, ErrProof
	   }
	   if len(t.Leaves) == 0 {
		   return Proof{}, ErrNotFound
	   }
	  idx := t.IndexOf(leaf)
	   if idx < 0 || len(t.Layers) == 0 {
		   return Proof{}, ErrNotFound
	  }
	   path := make([]Step,
		  0, len(t.Layers)-1)
	   i := idx
	   for lv := 0; lv < len(t.Layers)-1; lv++ {
		   row := t.Layers[lv]
		  if len(row)%2 == 1 {
			   row = append(append([][]byte{}, row...), row[len(row)-1])
		  }
		  sib := i ^ 1
		   if sib >= len(row) {
			   sib = i
		   }
		  path = append(path, Step{Sibling: hex.EncodeToString(row[sib]), Right: i%2 == 0})
		   i /= 2
	   }
	   got := leaf
	  for _, l := range t.Leaves {
		  if l.Key() == leaf.Key() {
			   got = l
			   break
		   }
	   }
	  return Proof{Leaf: got, Path: path, Index: idx, Root: t.Root}, nil
   }

// Verify 把叶哈希沿路径折到根。兄弟节点长度不对、或折算结果与根不同，都拒绝。
// 工程默克尔叶没有偿付前缀，不能通过本验证。
  func Verify(root string, p Proof) error {
	   root = trimHex(root)
	  if root == "" || trimHex(p.Root) != root {
		  return ErrRoot
	  }
	  if !stringsHasPrefix(p.Leaf.Canonical(), leafPrefix) {
		  return ErrNotSolvency
	  }
	  if p.Leaf.Hex() != p.Leaf.LeafHash && p.Leaf.LeafHash != "" {
		   return ErrLeaf
	  }
	   cur := p.Leaf.Hash()
	   if p.Leaf.LeafHash != "" && p.Leaf.LeafHash != hex.EncodeToString(cur) {
		   return ErrLeaf
	  }
	  for _, st := range p.Path {
		  sib, err := hex.DecodeString(trimHex(st.Sibling))
		   if err != nil || len(sib) != 32 {
			   return ErrProof
		  }
		   if st.Right {
			  cur = hashPair(cur, sib)
		   } else {
			  cur = hashPair(sib, cur)
		   }
	  }
	   want, err := hex.DecodeString(root)
	  if err != nil || len(want) != 32 {
		  return ErrRoot
	  }
	  if !bytes.Equal(cur, want) {
		  return ErrProof
	  }
	   return nil
  }

  func trimHex(s string) string {
	   s = hexOnly(s)
	  return s
   }

   func hexOnly(s string) string {
	   out := make([]byte, 0, len(s))
	   for i := 0; i < len(s); i++ {
		   c := s[i]
		   if c == ' ' || c == '\n' || c == '\t' {
			  continue
		   }
		  out = append(out, c)
	   }
	   return string(out)
  }

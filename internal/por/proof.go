package por

import (
	"bytes"
	"encoding/hex"
)

type Step struct {
	Sibling string `json:"sibling"`
	Right   bool   `json:"right"`
}

type Proof struct {
	Leaf Leaf `json:"leaf"`
	Path []Step `json:"path"`
	Index int `json:"index"`
	Root string `json:"root"`
}

   func (t Tree) Prove(leaf Leaf) (Proof, error) {
	   idx := t.IndexOf(leaf)
	   if idx < 0 || len(t.Layers) == 0 {
		   return Proof{}, ErrProof
	   }
	  path := make([]Step, 0, len(t.Layers)-1)
	   i := idx
	  for lv := 0; lv < len(t.Layers)-1; lv++ {
		  row := t.Layers[lv]
		  if len(row)%2 == 1 {
			   row = append(append([][]byte{},
				   row...), row[len(row)-1])
		   }
		   sib := i ^ 1
		   if sib >= len(row) {
			  sib = i
		   }
		   path = append(path,
			   Step{Sibling: hex.EncodeToString(row[sib]), Right: i%2 == 0})
		  i /= 2
	  }
	   return Proof{Leaf: leaf, Path: path, Index: idx, Root: t.Root}, nil
  }

   func Verify(root string, p Proof) error {
	   if root == "" || p.Root != root {
		   return ErrRoot
	  }
	   cur := p.Leaf.Hash()
	   for _, st := range p.Path {
		   sib, err := hex.DecodeString(st.Sibling)
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
	  if err != nil {
		   return ErrRoot
	   }
	   if !bytes.Equal(cur, want) {
		  return ErrProof
	   }
	  return nil
   }

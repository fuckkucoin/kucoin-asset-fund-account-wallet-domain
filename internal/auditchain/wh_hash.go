package auditchain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"time"
)

   func putStr(h interface{ Write([]byte) (int, error) }, s string) {
	  b := []byte(s)
	  var n [4]byte
	   binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	  _, _ = h.Write(n[:])
	  _, _ = h.Write(b)
  }

// HashRow 长度前缀拼接，避免字段被挪动后仍撞上同一串。

   func HashRow(r Row) string {
	   h := sha256.New()
	  putStr(h, r.PrevHash)
	  putStr(h, itoa(r.Seq))
	  putStr(h, r.SegmentID)
	  putStr(h, r.Source)
	   putStr(h, r.IdemKey)
	   putStr(h, r.Kind)
	  putStr(h, r.Domain)
	  putStr(h, r.Tenant)
	  putStr(h, r.UserID)
	   putStr(h, r.Asset)
	  putStr(h, r.Amount)
	  putStr(h, r.RefID)
	   putStr(h, r.Actor)
	  putStr(h, r.Body)
	  putStr(h, r.OccurredAt.UTC().Format(time.RFC3339Nano))
	  putStr(h, r.ID)
	   return hex.EncodeToString(h.Sum(nil))
   }

  func itoa(n int) string {
	  if n == 0 {
		  return "0"
	   }
	  neg := n < 0
	   if neg {
		   n = -n
	  }
	  var buf [16]byte
	  i := len(buf)
	   for n > 0 {
		  i--
		   buf[i] = byte('0' + n%10)
		   n /= 10
	  }
	   if neg {
		  i--
		   buf[i] = '-'
	  }
	  return string(buf[i:])
  }

  func pairHash(left, right string) string {
	  h := sha256.New()
	   _, _ = h.Write([]byte(left))
	   _, _ = h.Write([]byte{0})
	   _, _ = h.Write([]byte(right))
	   return hex.EncodeToString(h.Sum(nil))
  }

// MerkleRoot 奇数层复制最后一片叶子。空段用固定空根，避免和单叶子撞车。
  func MerkleRoot(leaves []string) string {
	  if len(leaves) == 0 {
		  return pairHash("empty", "empty")
	  }
	  level := append([]string(nil), leaves...)
	  for len(level) > 1 {
		   if len(level)%2 == 1 {
			   level = append(level,
				  level[len(level)-1])
		   }
		   next := make([]string, 0, len(level)/2)
		   for i := 0; i < len(level); i += 2 {
			  next = append(next, pairHash(level[i], level[i+1]))
		  }
		  level = next
	  }
	   return level[0]
   }

// Sib 证明路径上的兄弟。Left 表示兄弟在左边。
type Sib struct {
	Hash string `json:"hash"`
	Left bool `json:"left"`
}

// Proof 某条叶子到段根的路径。导出包必须能验过才算信任导出。
type Proof struct {
	Index    int    `json:"index"`
	Leaf     string `json:"leaf"`
	Siblings []Sib  `json:"siblings"`
}

// Prove 按下标取出路径。下标越界拒。

   func Prove(leaves []string, index int) (Proof, error) {
	   if index < 0 || index >= len(leaves) {
		   return Proof{}, ErrSegment
	   }
	   p := Proof{Index: index, Leaf: leaves[index]}
	   level := append([]string(nil), leaves...)
	  idx := index
	   for len(level) > 1 {
		   if len(level)%2 == 1 {
			   level = append(level, level[len(level)-1])
		  }
		   sib := idx ^ 1
		   p.Siblings = append(p.Siblings,
			   Sib{Hash: level[sib], Left: sib < idx})
		  next := make([]string, 0, len(level)/2)
		   for i := 0; i < len(level); i += 2 {
			  next = append(next,
				  pairHash(level[i], level[i+1]))
		   }
		   level = next
		   idx /= 2
	  }
	   return p, nil
   }

// VerifyProof 从叶子折到根。路径空只接受单叶子。
   func VerifyProof(root string, p Proof) bool {
	   if root == "" || p.Leaf == "" {
		   return false
	  }
	  cur := p.Leaf
	   for _, s := range p.Siblings {
		   if s.Hash == "" {
			  return false
		  }
		   if s.Left {
			  cur = pairHash(s.Hash, cur)
		  } else {
			   cur = pairHash(cur, s.Hash)
		   }
	  }
	  return cur == root
   }

// LeavesOf 段内哈希按 seq 升序。调用方应已排好。
   func LeavesOf(rows []Row) []string {
	  out := make([]string, len(rows))
	   for i, r := range rows {
		  out[i] = r.SelfHash
	   }
	   return out
   }

// CanonBody 把 map 收成稳定 JSON。空 map 记 {}。

   func CanonBody(v any) string {
	  if v == nil {
		  return "{}"
	   }
	  b, err := json.Marshal(v)
	   if err != nil || len(b) == 0 {
		   return "{}"
	   }
	  return string(b)
   }

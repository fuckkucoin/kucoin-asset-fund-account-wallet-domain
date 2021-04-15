package destallow

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Snapshot 白名单同步快照。

type Snapshot struct {
	UserID string `json:"user_id,omitempty"`
	Count int    `json:"count"`
	Hash  string `json:"hash"`
	Algo  string `json:"algo"`
	BuiltAt  time.Time `json:"built_at"`
	Match    *bool     `json:"match,omitempty"`
	PeerHash string    `json:"peer_hash,omitempty"`
}

  func snapLine(e Entry) string {
	   sec := int64(0)
	   if !e.AddedAt.IsZero() {
		  sec = e.AddedAt.UTC().Unix()
	  }
	  return strings.Join([]string{
		  strings.TrimSpace(e.UserID),
		  NormChain(e.Chain),
		   strings.TrimSpace(e.Address),
		   strings.TrimSpace(e.Label),
		  strconv.FormatInt(sec, 10),
	  }, "|")
   }

  func hashLines(lines []string) string {
	  sort.Strings(lines)
	   sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	   return hex.EncodeToString(sum[:])
   }

   func (b *Book) collectSnap(userID string) []Entry {
	  if b == nil {
		  return nil
	  }
	  uid := strings.TrimSpace(userID)
	   if uid != "" {
		  return b.List(uid)
	  }
	  if b.sqlOn() {
		   return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   out := make([]Entry, 0, len(b.rows))
	   for _, e := range b.rows {
		  out = append(out, e)
	  }
	   return out
   }

// SnapshotHash 计算用户或全台（进程内）白名单快照哈希。
  func (b *Book) SnapshotHash(userID string) Snapshot {
	  now := time.Now().UTC()
	   if b != nil && b.clock != nil {
		  now = b.clock()
	  }
	  items := b.collectSnap(userID)
	  lines := make([]string,
		  0, len(items))
	  for _, e := range items {
		   lines = append(lines,
			  snapLine(e))
	   }
	   return Snapshot{
		  UserID: strings.TrimSpace(userID), Count: len(items),
		   Hash: hashLines(lines), Algo: "sha256", BuiltAt: now,
	  }
  }

// CompareSnapshot 与对端哈希。空对端失败关闭。
   func (b *Book) CompareSnapshot(userID, peerHash string) (Snapshot, error) {
	  got := b.SnapshotHash(userID)
	  peerHash = strings.ToLower(strings.TrimSpace(peerHash))
	  if peerHash == "" || len(peerHash) != 64 {
		   return got, ErrStore
	  }
	   match := strings.EqualFold(got.Hash, peerHash)
	  got.Match = &match
	   got.PeerHash = peerHash
	  return got, nil
  }

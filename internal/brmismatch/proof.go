package brmismatch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ChainEventHash 计算链上事件的规范哈希。

  func ChainEventHash(txHash string, logIndex int) string {
	  h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(txHash)) + ":" + strconv.Itoa(logIndex)))
	  return hex.EncodeToString(h[:])
   }

// EntryHash 计算账本条目的规范哈希。

  func EntryHash(entryID, amount string) string {
	  h := sha256.Sum256([]byte(strings.TrimSpace(entryID) + ":" + strings.TrimSpace(amount)))
	   return hex.EncodeToString(h[:])
  }

// ComputeProof 由三个哈希派生双边守恒证明哈希。
// windowKey 形态示例："2026-04-28T19:00Z/1h"。

   func ComputeProof(chainEventHash, entryHash, windowKey string) (string, error) {
	  chainEventHash = strings.TrimSpace(chainEventHash)
	   entryHash = strings.TrimSpace(entryHash)
	  windowKey = strings.TrimSpace(windowKey)
	   if chainEventHash == "" || entryHash == "" || windowKey == "" {
		   return "", ErrProof
	   }
	  input := fmt.Sprintf("%s|%s|%s", chainEventHash, entryHash, windowKey)
	  h := sha256.Sum256([]byte(input))
	  return hex.EncodeToString(h[:]), nil
  }

// VerifyProof 校验三元组与已持久化的 proofHash 是否一致。

  func VerifyProof(chainEventHash, entryHash, windowKey, expected string) error {
	   got,
		   err := ComputeProof(chainEventHash, entryHash, windowKey)
	   if err != nil {
		   return err
	  }
	   if got != strings.TrimSpace(expected) {
		  return ErrProof
	   }
	   return nil
  }

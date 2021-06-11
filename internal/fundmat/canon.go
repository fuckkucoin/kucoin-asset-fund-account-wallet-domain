package fundmat

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// DigestRows 对一组物化行做稳定摘要。因果标识不参与摘要，避免只附加血缘号就升代次语义。

   func DigestRows(userID string, rows []MatRow) string {
	   cp := append([]MatRow(nil), rows...)
	   sort.Slice(cp, func(i, j int) bool {
		   if cp[i].Asset == cp[j].Asset {
			  return cp[i].Subject < cp[j].Subject
		  }
		  return cp[i].Asset < cp[j].Asset
	   })
	  h := sha256.New()
	  _, _ = h.Write([]byte(NormUser(userID)))
	   _, _ = h.Write([]byte{0})
	  for _, r := range cp {
		  _, _ = h.Write([]byte(r.Key()))
		  _, _ = h.Write([]byte{0})
		   for _, col := range []string{
			  r.Available, r.Frozen, r.InFlight,
			  r.InFlightDeposit,
			   r.InFlightWithdraw, r.InFlightTransfer,
			  r.Occupancy, r.OccupancyRisk, r.OccupancyReserve,
		  } {
			   _, _ = h.Write([]byte(col))
			  _, _ = h.Write([]byte{0})
		  }
	  }
	  return hex.EncodeToString(h.Sum(nil))
  }

// DigestToken 围栏令牌。令牌只绑定用户、流、纪元与要求序号，不包含投影正文。

   func DigestToken(userID string, stream Stream, epoch, minSeq int64, issued string) string {
	  h := sha256.New()
	   _, _ = h.Write([]byte(strings.Join([]string{
		   NormUser(userID), string(stream), issued,
	  }, "|")))
	   var buf [16]byte
	  put64(buf[:8], uint64(epoch))
	  put64(buf[8:], uint64(minSeq))
	  _, _ = h.Write(buf[:])
	  return hex.EncodeToString(h.Sum(nil))
   }

  func put64(b []byte, v uint64) {
	  for i := 7; i >= 0; i-- {
		   b[i] = byte(v)
		  v >>= 8
	  }
   }

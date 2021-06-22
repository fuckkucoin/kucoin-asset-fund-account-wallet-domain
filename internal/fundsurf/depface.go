package fundsurf

import (
	"strconv"
	"github.com/gin-gonic/gin"
)

// DepRead 资金页上的入金相位。

type DepRead struct {
	Phase      string `json:"phase"`
	PhaseLabel string `json:"phase_label"`
	Confirming bool   `json:"confirming"`
	Reversed   bool   `json:"reversed"`
	Explain     string `json:"explain,omitempty"`
	AvailBefore string `json:"avail_before,omitempty"`
	AvailAfter  string `json:"avail_after,omitempty"`
	ReverseEntry string `json:"reverse_entry,omitempty"`
	Amount string `json:"amount,omitempty"`
	Asset string `json:"asset,omitempty"`
	Chain  string `json:"chain,omitempty"`
	TxHash string `json:"tx_hash,omitempty"`
}

// DepReader 由入金服务挂上。资金页只读。
type DepReader func(user, chain, hash string, output int) (DepRead, bool)

// BindDepReader 挂只读相位。nil 表示尚未装配。

   func (b *Book) BindDepReader(fn DepReader) {
	   if b == nil {
		   return
	   }
	  b.depRead = fn
  }

   func registerDepFace(v1 *gin.RouterGroup, surf *Book) {
	  if v1 == nil {
		   return
	   }
	   v1.GET("/funding/deposit-face", func(c *gin.Context) {
		  if surf == nil || surf.depRead == nil {
			   c.JSON(503, gin.H{"error": "deposit face unavailable", "phase": "none", "confirming": false, "reversed": false})
			  return
		   }
		  output, _ := strconv.Atoi(c.DefaultQuery("output_index", "0"))
		   got, ok := surf.depRead(c.Query("user_id"), c.Query("chain"), c.Query("tx_hash"), output)
		   if !ok {
			  c.JSON(404, gin.H{"error": "deposit face not found", "confirming": false, "reversed": false})
			   return
		   }
		   c.JSON(200, got)
	  })
   }

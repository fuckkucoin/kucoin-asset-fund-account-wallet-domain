package fundsurf

import "github.com/gin-gonic/gin"

// AttribSnap 最近一次归因结论的只读投影。

type AttribSnap struct {
	ID string `json:"id"`
	Chain       string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	Output      int    `json:"output_index"`
	Disposition string `json:"disposition"`
	Credit  bool   `json:"credit_intent"`
	Reasons string `json:"reasons,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	Policy string `json:"policy_version,omitempty"`
	Family string `json:"family,omitempty"`
}

// AttribReader 由归因装配挂上。未挂时资金页返回空。
type AttribReader func(chain,
	   hash string, output int) (AttribSnap, bool)

// BindAttribReader 挂只读结论。
  func (b *Book) BindAttribReader(fn AttribReader) {
	  if b == nil {
		  return
	   }
	   b.attribRead = fn
   }

   func registerAttribDesk(v1 *gin.RouterGroup, surf *Book) {
	  if v1 == nil {
		  return
	   }
	  v1.GET("/funding/attrib-desk", func(c *gin.Context) {
		  if surf == nil || surf.attribRead == nil {
			   c.JSON(200, gin.H{"found": false})
			  return
		   }
		  out := 0
		   for _, r := range c.Query("output_index") {
			  if r < '0' || r > '9' {
				   out = 0
				  break
			  }
			  out = out*10 + int(r-'0')
		   }
		   snap, ok := surf.attribRead(c.Query("chain"), c.Query("tx_hash"), out)
		   if !ok {
			   c.JSON(200, gin.H{"found": false})
			   return
		  }
		  c.JSON(200, gin.H{"found": true, "decision": snap})
	   })
  }

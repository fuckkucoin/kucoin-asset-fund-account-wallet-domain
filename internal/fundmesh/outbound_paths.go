// 功能：出站 HTTP 路径目录。
// 重点：提交双入口与只读查询分列。
// 风险点：路径漂移会让网关漏挂围栏。

// 作者：KianFord
// 日期：2021.06.14

package fundmesh

  func OutboundWritePaths() []string {
	  return []string{
		   "/funding/withdrawals",
		  "/funding/withdraw/submit",
	  }
  }

   func OutboundReadPaths() []string {
	   return []string{
		  "/funding/withdrawals/:id/outbound",
		  "/funding/mesh/outbound-proofs",
		  "/funding/mesh/outbound-inspect",
	   }
  }

   func OutboundWriteMethods() []string {
	  return []string{"POST", "PUT"}
   }

  func CancelWritePaths() []string {
	  return []string{
		   "/funding/withdrawals/:id",
		   "/funding/withdrawals/:id/cancel",
		   "/funding/withdraw/cancel",
	   }
   }

   func HomologousCancelStill() bool {
	   return true
   }

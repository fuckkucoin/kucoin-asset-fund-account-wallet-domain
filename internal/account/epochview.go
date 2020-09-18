// 功能：关户世代只读视图：供 closelatecredit / acctlifegate 读取科目状态，不改写生命周期目录。
// 本文件不升降 close_epoch、不改 Status 常量、不改偏好 hop。
// Close 仍由 Service.Close 结清写 Closed；世代绑定由 acctlifegate.CloseWithEpoch 完成。

// 作者：Blaise
// 日期：2020.09.17

package account

import "context"

// BookStatus 只读科目相位。不存在记录时沿用 statusOf 默认（funding=active，其余=closed）。
  func (s *Service) BookStatus(userID string, book Book) Status {
	  if s == nil {
		  return StatusClosed
	  }
	  return s.statusOf(userID, book)
   }

// CanCreditBook 科目是否允许入活账（与 Status.CanCredit 一致，供迟贷闸只读）。
   func (s *Service) CanCreditBook(userID string, book Book) bool {
	   return s.BookStatus(userID, book).CanCredit()
   }

// ClosedBook 科目是否已关户。
   func (s *Service) ClosedBook(userID string, book Book) bool {
	  return s.BookStatus(userID,
		   book) == StatusClosed
   }

// LifecycleEvents 只读生命周期事件（不改 Event 结构）。
  func (s *Service) LifecycleEvents(userID string, limit int) []Event {
	   return s.Events(userID, limit)
   }

// ListBooks 只读科目快照别名。

   func (s *Service) ListBooks(ctx context.Context, userID string) (Snapshot, error) {
	  return s.List(ctx, userID)
   }

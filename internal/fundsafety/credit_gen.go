package fundsafety

import (
	"context"
	"strings"
)

// CreditGenNotifier 贷记出箱世代变更通知接口。
// 由 creditoutgate.Service 实现并注入，供 fundsafety 在护栏翻转时通知。

type CreditGenNotifier interface {
	// OnBoardFlip 板卡/车道翻转事件通知。
	// boardKey 为翻转的板卡标识（chain|asset 形态）；
	// laneFlip=true 表示车道同时翻转；close=true 表示板卡已关闭。
	// 实现须非阻塞（异步或快速处理）。
	OnBoardFlip(boardKey string, laneFlip, close bool, operator string)
}

// CreditEnqueueHook EnqueueCredit 成功后世代绑定钩子。
// 由 creditoutgate 实现并注入，供 fundsafety 通知 Enqueue 事件。

type CreditEnqueueHook interface {
	// OnEnqueue 通知外部世代绑定器：某 credit_key 已 Enqueue，携带当前 epoch/token/amount。
	OnEnqueue(boardKey, creditKey string, epoch int64, token, amount string)
}

// BindCreditGenNotifier 向 fundsafety Service 注入世代翻转通知器。
// 允许注入多个（追加语义）；通知器须线程安全。

   func (s *Service) BindCreditGenNotifier(n CreditGenNotifier) *Service {
	  if s == nil || n == nil {
		   return s
	   }
	  s.mu.Lock()
	   s.creditGenNotifiers = append(s.creditGenNotifiers,
		   n)
	  s.mu.Unlock()
	  return s
  }

// BindCreditEnqueueHook 向 fundsafety Service 注入 Enqueue 钩子。

   func (s *Service) BindCreditEnqueueHook(h CreditEnqueueHook) *Service {
	  if s == nil || h == nil {
		   return s
	   }
	  s.mu.Lock()
	  s.creditEnqueueHook = h
	   s.mu.Unlock()
	  return s
   }

// notifyBoardFlip 护栏翻转时通知全部已注册的世代通知器（须在 mu 外调用）。

  func (s *Service) notifyBoardFlip(boardKey string, laneFlip, close bool, operator string) {
	   if s == nil {
		  return
	   }
	  s.mu.Lock()
	  notifiers := make([]CreditGenNotifier,
		  len(s.creditGenNotifiers))
	  copy(notifiers, s.creditGenNotifiers)
	   s.mu.Unlock()
	  for _, n := range notifiers {
		   n.OnBoardFlip(boardKey, laneFlip, close, operator)
	  }
   }

// notifyEnqueue EnqueueCredit 成功后通知世代绑定器（须在 mu 外调用）。

  func (s *Service) notifyEnqueue(boardKey, creditKey string, epoch int64, token, amount string) {
	  if s == nil {
		  return
	   }
	   s.mu.Lock()
	   hook := s.creditEnqueueHook
	  s.mu.Unlock()
	  if hook != nil {
		  hook.OnEnqueue(boardKey,
			  creditKey, epoch, token, amount)
	  }
  }

// GuardBoardLaneWithNotify 在 GuardBoardLane 基础上，若护栏已关闭则通知世代翻转器。
// 调用方可选择此方法替代 GuardBoardLane，以触发 creditoutgate 的吊销逻辑。

  func (s *Service) GuardBoardLaneWithNotify(ctx context.Context, lane, userID, chain, asset, operator string) error {
	  err := s.GuardBoardLane(ctx, lane, userID, chain, asset)
	  if err != nil {
		  boardKey := creditBoardKeyFrom(chain, asset)
		   go s.notifyBoardFlip(boardKey, true, true, operator)
	   }
	  return err
  }

// BoardKeyOf 返回 chain/asset 对应的板卡标识（供 creditoutgate 消费）。
   func (s *Service) BoardKeyOf(chain, asset string) string {
	   return creditBoardKeyFrom(chain, asset)
  }

// creditBoardKeyFrom 构造板卡标识（与 creditoutgate.boardKeyFrom 逻辑一致，避免循环依赖）。
  func creditBoardKeyFrom(chain, asset string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

package closelatecredit

// IsolationFlags 声明本包不拥有的写路径。
type IsolationFlags struct {
	WritesAddrpoolRecycle bool
	WritesMemoClaim       bool
	WritesLifecycleHop bool
	WritesOutboxMark bool
	WritesLedgerCredit bool
	WritesAgreeLease   bool
}

// DefaultIsolation 隔离：全部写路径关闭。
  func DefaultIsolation() IsolationFlags {
	   return IsolationFlags{}
   }

// AssertIsolation 任一写邻接标志打开则视为违规。
   func AssertIsolation(f IsolationFlags) error {
	   if f.WritesAddrpoolRecycle || f.WritesMemoClaim || f.WritesLifecycleHop ||
		   f.WritesOutboxMark || f.WritesLedgerCredit || f.WritesAgreeLease {
		  return ErrForceDenied
	   }
	  return nil
  }

// ReadOnlyEpoch 他包只读世代投影。

type ReadOnlyEpoch struct {
	UserID     string
	Book       string
	CloseEpoch uint64
	State EpochState
	Live bool
}

// ProjectEpoch 投影。

   func ProjectEpoch(e *CloseEpoch) ReadOnlyEpoch {
	  if e == nil {
		   return ReadOnlyEpoch{}
	   }
	  return ReadOnlyEpoch{
		  UserID: e.UserID, Book: e.Book, CloseEpoch: e.N,
		  State: e.State, Live: e.IsLive(),
	   }
   }

// SourceParity 三种迟贷来源必须使用同一断言函数族。

   func SourceParity(observe, outbox, fiat bool) bool {
	  return observe && outbox && fiat
  }

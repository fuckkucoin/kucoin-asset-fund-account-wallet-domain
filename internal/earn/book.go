package earn

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

const kindPos = "aa_earn_pos"

type Book struct {
	mu   sync.Mutex
	byID map[string]*Position
	byUser map[string][]string
	byIdem map[string]string
	kv             persist.KV
	sql            *persist.SQLDB
	lastPersistErr string
	hydrateErr string
	hydratedAt time.Time
	hydratedN  int
}

   func NewBook() *Book {
	  return &Book{byID: map[string]*Position{}, byUser: map[string][]string{}, byIdem: map[string]string{}}
  }

  func (b *Book) BindKV(kv persist.KV) *Book {
	   if b != nil {
		  b.kv = kv
	   }
	  return b
   }

// BindSQL 挂 SQL 作为仓位权威源；OpenMem 不绑。
// Open 后须 Hydrate；Put 先写 SQL。
// 未绑定时仅内存/KV，多实例漂移。

   func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
	  }
	  return b
   }

   func (b *Book) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
   }

// Hydrate 启动从 SQL 装回全部仓位到内存表。
// 覆盖 KV；跨实例/重启后 Tick/List 可观测。
// Hydrate 失败不得假装无仓；错误写入 Health。

   func (b *Book) Hydrate(ctx context.Context) error {
	   if b == nil || !b.sqlOn() {
		   return nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   list, err := sqlrepo.NewEarn(b.sql).ListPositions(ctx, "")
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  if err != nil {
		  b.hydrateErr = err.Error()
		  return err
	  }
	   for _, row := range list {
		  b.rememberLocked(fromSQLPos(row))
	  }
	   b.hydratedN = len(list)
	  b.hydratedAt = time.Now().UTC()
	  b.hydrateErr = ""
	  return nil
   }

// Health 仓位 Hydrate/落库健康面（一致 wd-track）。

  func (b *Book) Health() map[string]any {
	   if b == nil {
		  return map[string]any{"sql": false}
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  out := map[string]any{
		  "sql": b.sqlOn(),
		  "hydrated_n":       b.hydratedN,
		   "hydrate_err":      b.hydrateErr,
		   "last_persist_err": b.lastPersistErr,
		  "mem_n":            len(b.byID),
	  }
	   if !b.hydratedAt.IsZero() {
		   out["hydrated_at"] = b.hydratedAt.UTC().Format(time.RFC3339Nano)
	   }
	   return out
  }

   func (b *Book) Put(p Position) error {
	   if b == nil {
		   return nil
	  }
	   b.mu.Lock()
	   var prev *Position
	   if old := b.byID[p.ID]; old != nil {
		   cp := *old
		   prev = &cp
	   }
	   prevUser := append([]string(nil), b.byUser[p.UserID]...)
	   prevIdem := ""
	   if p.Idempotency != "" {
		   prevIdem = b.byIdem[p.Idempotency]
	   }
	  cp := p
	  b.byID[p.ID] = &cp
	  if p.Idempotency != "" {
		  b.byIdem[p.Idempotency] = p.ID
	   }
	  found := false
	  for _, id := range b.byUser[p.UserID] {
		  if id == p.ID {
			   found = true
			   break
		  }
	  }
	  if !found {
		   b.byUser[p.UserID] = append(b.byUser[p.UserID], p.ID)
	  }
	   sqlDB := b.sql
	   kv := b.kv
	  b.mu.Unlock()
	  if sqlDB != nil && sqlDB.DB != nil && p.ID != "" {
		  err := sqlrepo.NewEarn(sqlDB).PutPosition(context.Background(), toSQLPos(p))
		  if err != nil {
			  b.mu.Lock()
			  b.lastPersistErr = err.Error()
			  if prev != nil {
				  b.byID[p.ID] = prev
			  } else {
				  delete(b.byID, p.ID)
			  }
			  b.byUser[p.UserID] = prevUser
			  if p.Idempotency != "" {
				  if prevIdem == "" {
					  delete(b.byIdem, p.Idempotency)
				  } else {
					  b.byIdem[p.Idempotency] = prevIdem
				  }
			  }
			  b.mu.Unlock()
			  return err
		  }
		  b.mu.Lock()
		  b.lastPersistErr = ""
		  b.mu.Unlock()
	  }
	  if kv != nil && p.ID != "" {
		   body, err := json.Marshal(p)
		  if err == nil {
			  exp := persist.Fence{}
			   if cur, err := kv.Get(context.Background(), kindPos, p.ID); err == nil {
				  exp = cur.Fence
			  }
			  if err := kv.Put(context.Background(),
				  persist.Row{Kind: kindPos, Key: p.ID, Body: body}, exp); err != nil {
				  return err
			  }
		   }
	  }
	  return nil
  }

  func (b *Book) Get(id string) (Position, bool) {
	   if b == nil {
		  return Position{}, false
	  }
	  id = strings.TrimSpace(id)
	   b.mu.Lock()
	   p, ok := b.byID[id]
	  if ok && p != nil {
		   cp := *p
		   b.mu.Unlock()
		   return cp, true
	   }
	   b.mu.Unlock()
	   if loaded, ok2 := b.loadSQL(id); ok2 {
		   b.mu.Lock()
		   b.rememberLocked(loaded)
		   b.mu.Unlock()
		   return loaded, true
	  }
	   loaded, ok2 := b.loadKV(id)
	  if !ok2 {
		  return Position{}, false
	  }
	   b.mu.Lock()
	  b.rememberLocked(loaded)
	  b.mu.Unlock()
	  return loaded, true
   }

  func (b *Book) loadSQL(id string) (Position, bool) {
	  if !b.sqlOn() || id == "" {
		  return Position{}, false
	  }
	   row, err := sqlrepo.NewEarn(b.sql).GetPosition(context.Background(), id)
	   if err != nil {
		   return Position{}, false
	  }
	   return fromSQLPos(row), true
   }

  func (b *Book) loadKV(id string) (Position, bool) {
	  if b.kv == nil || id == "" {
		   return Position{}, false
	  }
	   row, err := b.kv.Get(context.Background(), kindPos, id)
	   if err != nil {
		  return Position{}, false
	   }
	  var p Position
	   if json.Unmarshal(row.Body, &p) != nil || p.ID == "" {
		   return Position{}, false
	   }
	   p.Fence = row.Fence
	  return p, true
   }

   func (b *Book) rememberLocked(p Position) {
	   cp := p
	  b.byID[p.ID] = &cp
	   if p.Idempotency != "" {
		  b.byIdem[p.Idempotency] = p.ID
	  }
	   found := false
	   for _, id := range b.byUser[p.UserID] {
		  if id == p.ID {
			  found = true
			  break
		   }
	  }
	   if !found {
		   b.byUser[p.UserID] = append(b.byUser[p.UserID], p.ID)
	   }
   }

   func (b *Book) hydrateKV(userID string) {
	  if b == nil || b.kv == nil {
		   return
	   }
	   rows, err := b.kv.List(context.Background(), kindPos, 400)
	  if err != nil {
		   return
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  for _, r := range rows {
		  var p Position
		  if json.Unmarshal(r.Body, &p) != nil || p.ID == "" {
			   continue
		  }
		  if userID != "" && p.UserID != userID {
			  continue
		   }
		   p.Fence = r.Fence
		  b.rememberLocked(p)
	   }
  }

  func (b *Book) ByIdem(key string) (Position, bool) {
	   if b == nil {
		  return Position{}, false
	  }
	  key = strings.TrimSpace(key)
	   b.mu.Lock()
	   id, ok := b.byIdem[key]
	  if ok {
		   p := b.byID[id]
		   b.mu.Unlock()
		   if p == nil {
			   return Position{}, false
		  }
		  return *p, true
	   }
	  b.mu.Unlock()
	   if b.sqlOn() && key != "" {
		  if row, err := sqlrepo.NewEarn(b.sql).GetPositionByIdem(context.Background(), key); err == nil {
			   p := fromSQLPos(row)
			   b.mu.Lock()
			   b.rememberLocked(p)
			   b.mu.Unlock()
			  return p, true
		   }
	   }
	   b.hydrateKV("")
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  id, ok = b.byIdem[key]
	   if !ok {
		   return Position{}, false
	  }
	  p := b.byID[id]
	   if p == nil {
		  return Position{}, false
	   }
	  return *p, true
   }

   func (b *Book) List(userID string) []Position {
	  if b == nil {
		  return nil
	   }
	  userID = strings.TrimSpace(userID)
	  if b.sqlOn() {
		  if rows, err := sqlrepo.NewEarn(b.sql).ListPositions(context.Background(), userID); err == nil {
			   b.mu.Lock()
			  for _, r := range rows {
				  b.rememberLocked(fromSQLPos(r))
			   }
			   b.mu.Unlock()
		  }
	  } else {
		  b.hydrateKV(userID)
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   ids := b.byUser[userID]
	   out := make([]Position, 0, len(ids))
	  for _, id := range ids {
		  if p := b.byID[id]; p != nil {
			  out = append(out, *p)
		  }
	  }
	   return out
  }

// ListPaged SQL 分页/status 过滤列仓位；无 SQL 时回落内存过滤。
// 运营面与用户面共用；page/pageSize 走 sqlrepo 硬顶。
// 无 SQL 时 total 为过滤后条数，跨实例不完整。
  func (b *Book) ListPaged(ctx context.Context, userID, status string, page, pageSize int) (items []Position, total, pageOut, pageSizeOut, totalPage int, err error) {
	  if b == nil {
		  return nil, 0, 1, 50, 1, nil
	   }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  userID = strings.TrimSpace(userID)
	   status = strings.TrimSpace(strings.ToLower(status))
	  if b.sqlOn() {
		  pg, err := sqlrepo.NewEarn(b.sql).ListPositionsPaged(ctx, sqlrepo.PositionListFilter{
			   UserID: userID, Status: status, Page: page, PageSize: pageSize,
		  })
		  if err != nil {
			   return nil, 0, page, pageSize, 1, err
		  }
		   out := make([]Position, 0, len(pg.Items))
		   b.mu.Lock()
		   for _, r := range pg.Items {
			  p := fromSQLPos(r)
			   b.rememberLocked(p)
			  out = append(out, p)
		  }
		   b.mu.Unlock()
		   return out, pg.Total, pg.Page, pg.PageSize, pg.TotalPage, nil
	  }
	   all := b.List(userID)
	  filtered := make([]Position, 0, len(all))
	   for _, p := range all {
		   if status != "" && !strings.EqualFold(string(p.Status), status) {
			   continue
		   }
		   filtered = append(filtered, p)
	   }
	  if page <= 0 {
		  page = 1
	   }
	   if pageSize <= 0 {
		   pageSize = 50
	   }
	   if pageSize > 200 {
		  pageSize = 200
	   }
	   total = len(filtered)
	  totalPage = (total + pageSize - 1) / pageSize
	  if totalPage == 0 {
		  totalPage = 1
	  }
	  start := (page - 1) * pageSize
	  if start >= total {
		  return []Position{}, total, page, pageSize, totalPage, nil
	   }
	  end := start + pageSize
	  if end > total {
		   end = total
	  }
	  return filtered[start:end],
		  total, page, pageSize, totalPage, nil
   }

   func (b *Book) Open(userID, asset string) []Position {
	  all := b.List(userID)
	  out := make([]Position, 0)
	   for _, p := range all {
		   if p.Status != StatusOpen && p.Status != StatusLocked {
			   continue
		  }
		   if asset != "" && !strings.EqualFold(p.Asset, asset) {
			  continue
		   }
		   out = append(out, p)
	  }
	  return out
  }

   func (b *Book) Users() []string {
	  if b == nil {
		   return nil
	   }
	  // 有 SQL 时先装回全量，保证 Tick 覆盖跨实例仓位。
	  if b.sqlOn() {
		  _ = b.Hydrate(context.Background())
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]string, 0, len(b.byUser))
	  for uid := range b.byUser {
		   out = append(out, uid)
	   }
	  return out
   }

  func (b *Book) SumPrincipal(userID, productID string) (string, error) {
	  return sumPrincipal(b.List(userID),
		  productID)
  }

  func sumPrincipal(rows []Position, productID string) (string, error) {
	  var total = mustZero()
	   for _, p := range rows {
		  if productID != "" && p.ProductID != productID {
			   continue
		  }
		  if p.Status != StatusOpen && p.Status != StatusLocked {
			   continue
		  }
		  amt, err := p.PrincipalAmt()
		   if err != nil {
			   return "0", err
		  }
		  next, err := total.Add(amt)
		   if err != nil {
			  return "0", err
		  }
		   total = next
	   }
	  return total.String(), nil
   }

   func toSQLPos(p Position) sqlrepo.EarnPosition {
	   return sqlrepo.EarnPosition{
		   ID: p.ID, UserID: p.UserID, ProductID: p.ProductID, Asset: p.Asset,
		  Principal: p.Principal, Accrued: p.Accrued, Claimed: p.Claimed, Penalty: p.Penalty,
		   OpenedAt: p.OpenedAt,
		  UnlockAt: p.UnlockAt, LastAccrue: p.LastAccrue,
		   Status: string(p.Status), TransferID: p.TransferID, Idempotency: p.Idempotency,
		   AutoSubscribe: p.AutoSubscribe, UpdatedAt: time.Now().UTC(),
	  }
  }

  func fromSQLPos(row sqlrepo.EarnPosition) Position {
	  return Position{
		   ID: row.ID, UserID: row.UserID, ProductID: row.ProductID, Asset: row.Asset,
		   Principal: row.Principal,
		  Accrued:   row.Accrued, Claimed: row.Claimed, Penalty: row.Penalty,
		  OpenedAt: row.OpenedAt, UnlockAt: row.UnlockAt, LastAccrue: row.LastAccrue,
		  Status: Status(row.Status), TransferID: row.TransferID, Idempotency: row.Idempotency,
		  AutoSubscribe: row.AutoSubscribe,
	   }
   }

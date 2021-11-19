package stablebr

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	checks map[string]Check
	rows       map[string]*Journal
	byIdem     map[string]string
	ttl        time.Duration
	led        *ledger.Service
	clock      func() time.Time
	guard      PostGuard
	clearHooks []ClearAttestHook
	journalFence bool
	mintGen      map[string]int64
	ledErr error
}

  func New() *Service {
	  return &Service{
		   checks: map[string]Check{}, rows: map[string]*Journal{},
		  byIdem: map[string]string{}, ttl: 24 * time.Hour,
		  clock: func() time.Time { return time.Now().UTC() },
		  mintGen: map[string]int64{},
	  }
   }

   func (s *Service) BindLedger(led *ledger.Service) *Service {
	  if s != nil {
		   s.led = led
	  }
	   return s
  }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
  }

  func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil && fn != nil {
		   s.clock = fn
	   }
   }

   // InjectLedgerError 注入 ledger Create 失败，用于半截封堵复现。路径不得常驻。
  func (s *Service) InjectLedgerError(err error) {
	  if s != nil {
		   s.ledErr = err
	   }
   }

  func (s *Service) SetTTL(d time.Duration) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   if d <= 0 {
		  return ErrExpired
	  }
	   s.mu.Lock()
	  s.ttl = d
	  s.mu.Unlock()
	  return nil
   }

  func ParseKind(s string) (Kind, error) {
	  switch Kind(strings.ToLower(strings.TrimSpace(s))) {
	  case KindMint, KindRedeem:
		  return Kind(strings.ToLower(strings.TrimSpace(s))), nil
	  default:
		   return "", ErrKind
	  }
  }

   func ParseItem(s string) (Item, error) {
	   switch Item(strings.ToLower(strings.TrimSpace(s))) {
	  case ItemReserve, ItemIssuer, ItemBank, ItemBurn, ItemKYT:
		  return Item(strings.ToLower(strings.TrimSpace(s))), nil
	   default:
		  return "", ErrItem
	   }
  }

   func allowedAsset(s string) (string, error) {
	  as, err := validate.Asset(s)
	  if err != nil {
		  return "", ErrAsset
	   }
	  switch as {
	   case "USDT", "USDC":
		  return as, nil
	  default:
		  return "", ErrAsset
	  }
   }

  func required(kind Kind) []Item {
	  return RequiredItems(kind)
  }

// RequiredItems 返回铸/赎所需证明条目。不改 Conserve 金额公式。
   func RequiredItems(kind Kind) []Item {
	   if kind == KindMint {
		  return []Item{ItemReserve, ItemIssuer, ItemBank, ItemKYT}
	  }
	   return []Item{ItemReserve, ItemIssuer, ItemBurn, ItemKYT}
   }

   func genKey(kind Kind, asset string) string {
	   return string(kind) + "|" + asset
  }

  func (s *Service) GenerationOf(kind Kind, asset string) int64 {
	   if s == nil {
		   return 0
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.mintGen[genKey(kind, asset)]
   }

  func checkKey(kind Kind, asset string, item Item) string {
	   return string(kind) + "|" + asset + "|" + string(item)
  }

package quote_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestMapErrorTable(t *testing.T) {
	rows := []struct {
		err	error
		want	quote.Code
	}{
		{nil, quote.CodeOK},
		{quote.ErrQuoteEmpty, quote.CodeEmpty},
		{quote.ErrQuoteStale, quote.CodeStale},
		{quote.ErrQuoteIllegal, quote.CodeIllegal},
		{quote.ErrSourceMasked, quote.CodeMasked},
		{quote.ErrHouseOnConfirm, quote.CodeHouse},
		{quote.ErrClockRequired, quote.CodeClock},
		{quote.ErrPairRequired, quote.CodePair},
		{quote.ErrQuoteDiverge, quote.CodeDiverge},
		{quote.ErrQuoteTimeout, quote.CodeTimeout},
		{errors.New("other"), quote.CodeUnknown},
	}
	for _, row := range rows {
		if got := quote.MapError(row.err); got != row.want {
			t.Fatalf("%v %s", row.err, got)
		}
	}
	if quote.MapClass(quote.ClassOK) != quote.CodeOK || quote.MapClass(quote.ClassEmpty) != quote.CodeEmpty {
		t.Fatal("class")
	}
	if quote.MapClass(quote.ClassStale) != quote.CodeStale || quote.MapClass(quote.ClassIllegal) != quote.CodeIllegal {
		t.Fatal("class2")
	}
	if quote.MapClass(quote.Class(9)) != quote.CodeUnknown {
		t.Fatal("unk")
	}
}

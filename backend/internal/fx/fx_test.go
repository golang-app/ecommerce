package fx_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

func newSilentLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)
	return l
}

func TestRates_Defaults(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR,GBP", "EUR:0.92,GBP:0.79", newSilentLogger())

	is.Equal(r.Default(), "USD")
	is.Equal(r.Supported(), []string{"USD", "EUR", "GBP"})
	is.True(r.IsSupported("USD"))
	is.True(r.IsSupported("eur")) // case-insensitive
	is.True(!r.IsSupported("JPY"))
}

func TestRates_DefaultIsPrependedWhenMissingFromSupported(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "EUR,GBP", "EUR:0.92,GBP:0.79", newSilentLogger())

	// Default is always first in Supported even if the operator forgot it.
	is.Equal(r.Supported()[0], "USD")
	is.True(r.IsSupported("USD"))
}

func TestRates_ConvertRoundsToNearestMinorUnit(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR", "EUR:0.92", newSilentLogger())

	// 500 cents * 0.92 = 460 cents
	is.Equal(r.Convert(500, "EUR"), int64(460))
	// 199 cents * 0.92 = 183.08 -> 183 (round half-even / math.Round)
	is.Equal(r.Convert(199, "EUR"), int64(183))
	// 1 cent * 0.92 = 0.92 -> 1
	is.Equal(r.Convert(1, "EUR"), int64(1))
}

func TestRates_ConvertRoundTripWithRateOne(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR", "EUR:1.0", newSilentLogger())

	is.Equal(r.Convert(12345, "EUR"), int64(12345))
}

func TestRates_ConvertToDefaultReturnsInputUnchanged(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR", "EUR:0.5", newSilentLogger())

	is.Equal(r.Convert(987, "USD"), int64(987))
	// And the default is itself convertible (rate=1.0).
	is.Equal(r.Convert(987, "usd"), int64(987))
}

func TestRates_ConvertUnknownCurrencyIsPassthrough(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR", "EUR:0.5", newSilentLogger())

	// Crafted POSTs that slip past IsSupported still produce a sensible
	// number rather than zero.
	is.Equal(r.Convert(1000, "JPY"), int64(1000))
	is.Equal(r.Convert(1000, ""), int64(1000))
}

func TestRates_MissingRateDegradesToOne(t *testing.T) {
	is := is.New(t)

	// EUR is supported but absent from the rates CSV — should warn and
	// fall back to 1.0.
	r := fx.New("USD", "USD,EUR", "", newSilentLogger())

	is.Equal(r.Convert(1234, "EUR"), int64(1234))
}

func TestRates_MalformedEntriesAreSkipped(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR,GBP", "EUR:not-a-number,GBP:-1,JPY:130", newSilentLogger())

	// Both EUR (unparseable) and GBP (negative) fall back to 1.0; JPY is
	// not supported so its parsed rate never gets used.
	is.Equal(r.Convert(100, "EUR"), int64(100))
	is.Equal(r.Convert(100, "GBP"), int64(100))
	is.True(!r.IsSupported("JPY"))
}

func TestFormat_BuildsMoneyInTargetCurrency(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR", "EUR:0.92", newSilentLogger())

	m := fx.Format(r, 500, "EUR")
	is.Equal(m.Amount, int64(460))
	is.Equal(m.Currency, "EUR")
	is.Equal(m.Display(), "4.60")
}

func TestFormat_UnknownCurrencyFallsBackToDefault(t *testing.T) {
	is := is.New(t)

	r := fx.New("USD", "USD,EUR", "EUR:0.92", newSilentLogger())

	m := fx.Format(r, 500, "JPY")
	is.Equal(m.Currency, "USD")
	is.Equal(m.Amount, int64(500))
	is.Equal(m.Display(), "5.00")
}

func TestMoney_DisplayPadsCents(t *testing.T) {
	is := is.New(t)

	is.Equal(fx.Money{Amount: 100, Currency: "USD"}.Display(), "1.00")
	is.Equal(fx.Money{Amount: 105, Currency: "USD"}.Display(), "1.05")
	is.Equal(fx.Money{Amount: 0, Currency: "USD"}.Display(), "0.00")
	is.Equal(fx.Money{Amount: 99, Currency: "USD"}.Display(), "0.99")
	is.Equal(fx.Money{Amount: -150, Currency: "USD"}.Display(), "-1.50")
}

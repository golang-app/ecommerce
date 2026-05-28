// Package fx is the display-only multi-currency seam for the storefront.
//
// Rates is a static, operator-configured table of conversion multipliers
// FROM the storage currency (USD) TO each supported display currency. It
// is NOT a live FX feed: the multipliers are read once at startup from
// config (cmd/web/config.go FXRates) and never refreshed. To swap in a
// real provider, replace this type with one that fetches and caches
// rates from your favourite FX API — every caller (templates, the
// /currency handler) talks through the same Default / Supported /
// IsSupported / Convert / Format surface.
//
// Conversion is a render transformation only: orders are placed and
// persisted in USD minor units. Convert rounds to the nearest minor
// unit (cents) using math.Round, which matches what a customer expects
// when comparing "5.00 USD" to its EUR sticker.
package fx

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// Rates is the operator-configured conversion table. It is constructed
// once at boot from comma-separated env input and is immutable for the
// lifetime of the process. The zero value is unusable — call New.
type Rates struct {
	rates     map[string]float64
	def       string
	supported []string
}

// New parses the three config strings into a Rates value.
//
//   - defaultCcy is the storage currency. It is guaranteed to be in
//     Supported() (prepended when missing) and always has a rate of 1.0.
//   - supportedCSV is a comma-separated list of ISO codes ("USD,EUR,GBP").
//     Empty or unparseable entries are skipped with a warn.
//   - ratesCSV is a comma-separated list of CCY:rate pairs giving the
//     multiplier to convert FROM the default currency TO that currency
//     ("EUR:0.92,GBP:0.79"). Missing rates for supported currencies
//     degrade to 1.0 with a warn at startup.
//
// The returned Rates is safe for concurrent reads — callers may share a
// single instance across goroutines.
func New(defaultCcy, supportedCSV, ratesCSV string, logger logrus.FieldLogger) Rates {
	def := strings.ToUpper(strings.TrimSpace(defaultCcy))
	if def == "" {
		def = "USD"
	}

	supported := make([]string, 0)
	seen := make(map[string]bool)
	// Default currency is always listed first so the picker shows it on top.
	supported = append(supported, def)
	seen[def] = true
	for _, tok := range strings.Split(supportedCSV, ",") {
		c := strings.ToUpper(strings.TrimSpace(tok))
		if c == "" || seen[c] {
			continue
		}
		if len(c) != 3 {
			logger.WithField("currency", c).Warn("fx: skipping non-ISO-4217 currency in SUPPORTED_CURRENCIES")
			continue
		}
		supported = append(supported, c)
		seen[c] = true
	}

	rates := make(map[string]float64, len(supported))
	rates[def] = 1.0
	for _, tok := range strings.Split(ratesCSV, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		parts := strings.SplitN(tok, ":", 2)
		if len(parts) != 2 {
			logger.WithField("entry", tok).Warn("fx: skipping malformed FX_RATES entry (expected CCY:rate)")
			continue
		}
		ccy := strings.ToUpper(strings.TrimSpace(parts[0]))
		rate, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			logger.WithError(err).WithField("entry", tok).Warn("fx: skipping FX_RATES entry with unparseable rate")
			continue
		}
		if rate <= 0 {
			logger.WithField("entry", tok).Warn("fx: skipping FX_RATES entry with non-positive rate")
			continue
		}
		rates[ccy] = rate
	}

	// Any supported currency without an explicit rate falls back to 1.0
	// (i.e. shows the USD amount) and gets a loud warning so the operator
	// notices the missing entry.
	for _, c := range supported {
		if _, ok := rates[c]; !ok {
			logger.WithField("currency", c).Warn("fx: missing FX_RATES entry for supported currency; falling back to 1.0")
			rates[c] = 1.0
		}
	}

	return Rates{rates: rates, def: def, supported: supported}
}

// Default returns the storage / fallback currency. Every Price in the
// system is assumed to be denominated in it.
func (r Rates) Default() string { return r.def }

// Supported returns the offered ISO codes in declared order, with the
// default first. The returned slice is a copy — callers may mutate it
// without affecting the underlying table.
func (r Rates) Supported() []string {
	out := make([]string, len(r.supported))
	copy(out, r.supported)
	return out
}

// IsSupported reports whether ccy is one of the offered display
// currencies. It is the gate the /currency handler uses to reject a
// crafted POST that tries to set an unsupported value.
func (r Rates) IsSupported(ccy string) bool {
	ccy = strings.ToUpper(strings.TrimSpace(ccy))
	for _, c := range r.supported {
		if c == ccy {
			return true
		}
	}
	return false
}

// Convert returns amountMinorUSD converted to targetCcy in that
// currency's minor units. targetCcy == Default() (or any unknown ccy)
// returns the input unchanged so storefront renders fall back to USD
// transparently.
//
// Rounding is to the nearest minor unit via math.Round; the conversion
// is intentionally lossy (display only) and we accept a 1-cent rounding
// across the catalogue.
func (r Rates) Convert(amountMinorUSD int64, targetCcy string) int64 {
	targetCcy = strings.ToUpper(strings.TrimSpace(targetCcy))
	if targetCcy == "" || targetCcy == r.def {
		return amountMinorUSD
	}
	rate, ok := r.rates[targetCcy]
	if !ok {
		return amountMinorUSD
	}
	return int64(math.Round(float64(amountMinorUSD) * rate))
}

// Money is the formatted result of Format: the converted minor-unit
// amount and the currency code it is denominated in. Display is the
// human-facing "X.YY" string in major units; the Currency field is the
// ISO code the template appends after the amount.
type Money struct {
	Amount   int64
	Currency string
}

// Display formats the amount in major units as "X.YY" — the same format
// the existing Price.Display methods return so the storefront keeps a
// consistent look in any currency.
func (m Money) Display() string {
	a := m.Amount
	sign := ""
	if a < 0 {
		sign = "-"
		a = -a
	}
	return fmt.Sprintf("%s%d.%02d", sign, a/100, a%100)
}

// Format converts amountMinorUSD to targetCcy via r.Convert and wraps
// the result in a Money. It is the workhorse the `money` template
// FuncMap calls on every render.
func Format(r Rates, amountMinorUSD int64, targetCcy string) Money {
	targetCcy = strings.ToUpper(strings.TrimSpace(targetCcy))
	if targetCcy == "" || !r.IsSupported(targetCcy) {
		targetCcy = r.Default()
	}
	return Money{
		Amount:   r.Convert(amountMinorUSD, targetCcy),
		Currency: targetCcy,
	}
}

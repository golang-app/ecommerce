package sharedkernel_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/sharedkernel"
)

func TestCurrencyValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		c       sharedkernel.Currency
		wantErr bool
	}{
		{"valid EUR", "EUR", false},
		{"valid USD", "USD", false},
		{"valid PLN", "PLN", false},
		{"empty", "", true},
		{"too short", "EU", true},
		{"too long", "EURO", true},
		{"lowercase", "eur", true},
		{"mixed case", "Eur", true},
		{"digits", "123", true},
		{"symbol", "EU$", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.c.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, sharedkernel.ErrInvalidMoney) {
					t.Fatalf("expected ErrInvalidMoney, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewMoney(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		amount   int64
		currency sharedkernel.Currency
		wantErr  bool
	}{
		{"positive EUR", 1000, "EUR", false},
		{"zero", 0, "USD", false},
		{"negative", -500, "GBP", false},
		{"large", 9_999_999_999, "JPY", false},
		{"min int64", -9_223_372_036_854_775_808, "EUR", false},
		{"max int64", 9_223_372_036_854_775_807, "EUR", false},
		{"bad currency lowercase", 100, "eur", true},
		{"bad currency length", 100, "EU", true},
		{"bad currency empty", 100, "", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := sharedkernel.NewMoney(tc.amount, tc.currency)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Amount() != tc.amount {
				t.Fatalf("amount = %d, want %d", m.Amount(), tc.amount)
			}
			if m.Currency() != tc.currency {
				t.Fatalf("currency = %q, want %q", m.Currency(), tc.currency)
			}
		})
	}
}

func TestMustNewMoney(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		m := sharedkernel.MustNewMoney(500, "EUR")
		if m.Amount() != 500 || m.Currency() != "EUR" {
			t.Fatalf("unexpected money: %v", m)
		}
	})

	t.Run("panics on invalid", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic, got none")
			}
		}()
		_ = sharedkernel.MustNewMoney(500, "bad")
	})
}

func TestZero(t *testing.T) {
	t.Parallel()

	z := sharedkernel.Zero("EUR")
	if !z.IsZero() {
		t.Fatalf("Zero(EUR).IsZero() = false")
	}
	if z.Amount() != 0 {
		t.Fatalf("amount = %d, want 0", z.Amount())
	}
	if z.Currency() != "EUR" {
		t.Fatalf("currency = %q, want EUR", z.Currency())
	}
}

func TestIsZero(t *testing.T) {
	t.Parallel()

	if !sharedkernel.MustNewMoney(0, "USD").IsZero() {
		t.Fatalf("0 USD should be zero")
	}
	if sharedkernel.MustNewMoney(1, "USD").IsZero() {
		t.Fatalf("1 USD should not be zero")
	}
	if sharedkernel.MustNewMoney(-1, "USD").IsZero() {
		t.Fatalf("-1 USD should not be zero")
	}
}

func TestAdd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		a, b    sharedkernel.Money
		want    int64
		wantErr bool
	}{
		{
			"same currency positive",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.MustNewMoney(250, "EUR"),
			350, false,
		},
		{
			"same currency mixed signs",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.MustNewMoney(-30, "EUR"),
			70, false,
		},
		{
			"adding zero",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.Zero("EUR"),
			100, false,
		},
		{
			"large values",
			sharedkernel.MustNewMoney(1_000_000_000, "USD"),
			sharedkernel.MustNewMoney(2_000_000_000, "USD"),
			3_000_000_000, false,
		},
		{
			"cross currency rejected",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.MustNewMoney(100, "USD"),
			0, true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.a.Add(tc.b)
			if tc.wantErr {
				if !errors.Is(err, sharedkernel.ErrCurrencyMismatch) {
					t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Amount() != tc.want {
				t.Fatalf("amount = %d, want %d", got.Amount(), tc.want)
			}
			if got.Currency() != tc.a.Currency() {
				t.Fatalf("currency = %q, want %q", got.Currency(), tc.a.Currency())
			}
		})
	}
}

func TestSub(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		a, b    sharedkernel.Money
		want    int64
		wantErr bool
	}{
		{
			"same currency",
			sharedkernel.MustNewMoney(500, "EUR"),
			sharedkernel.MustNewMoney(200, "EUR"),
			300, false,
		},
		{
			"going negative",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.MustNewMoney(250, "EUR"),
			-150, false,
		},
		{
			"subtract zero",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.Zero("EUR"),
			100, false,
		},
		{
			"cross currency rejected",
			sharedkernel.MustNewMoney(100, "EUR"),
			sharedkernel.MustNewMoney(50, "USD"),
			0, true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.a.Sub(tc.b)
			if tc.wantErr {
				if !errors.Is(err, sharedkernel.ErrCurrencyMismatch) {
					t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Amount() != tc.want {
				t.Fatalf("amount = %d, want %d", got.Amount(), tc.want)
			}
		})
	}
}

func TestMul(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		m      sharedkernel.Money
		factor int64
		want   int64
	}{
		{"by zero", sharedkernel.MustNewMoney(100, "EUR"), 0, 0},
		{"by one", sharedkernel.MustNewMoney(100, "EUR"), 1, 100},
		{"by three", sharedkernel.MustNewMoney(100, "EUR"), 3, 300},
		{"by negative", sharedkernel.MustNewMoney(100, "EUR"), -2, -200},
		{"negative amount", sharedkernel.MustNewMoney(-50, "EUR"), 4, -200},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.m.Mul(tc.factor)
			if got.Amount() != tc.want {
				t.Fatalf("amount = %d, want %d", got.Amount(), tc.want)
			}
			if got.Currency() != tc.m.Currency() {
				t.Fatalf("currency = %q, want %q", got.Currency(), tc.m.Currency())
			}
		})
	}
}

func TestMulFloat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		m      sharedkernel.Money
		factor float64
		want   int64
	}{
		{"tax 8.875% on 12345 cents", sharedkernel.MustNewMoney(12345, "USD"), 0.08875, 1096},
		{"half off", sharedkernel.MustNewMoney(2000, "EUR"), 0.5, 1000},
		{"rounds up", sharedkernel.MustNewMoney(100, "EUR"), 0.005, 1}, // 0.5 rounds up via math.Round
		{"rounds down", sharedkernel.MustNewMoney(100, "EUR"), 0.004, 0},
		{"rounds .5 away from zero (positive)", sharedkernel.MustNewMoney(1, "EUR"), 0.5, 1},
		{"rounds .5 away from zero (negative)", sharedkernel.MustNewMoney(-1, "EUR"), 0.5, -1},
		{"zero amount", sharedkernel.Zero("EUR"), 0.5, 0},
		{"zero factor", sharedkernel.MustNewMoney(1234, "EUR"), 0, 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.m.MulFloat(tc.factor)
			if got.Amount() != tc.want {
				t.Fatalf("amount = %d, want %d", got.Amount(), tc.want)
			}
			if got.Currency() != tc.m.Currency() {
				t.Fatalf("currency mismatch")
			}
		})
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		a, b    sharedkernel.Money
		want    int
		wantErr bool
	}{
		{"less", sharedkernel.MustNewMoney(100, "EUR"), sharedkernel.MustNewMoney(200, "EUR"), -1, false},
		{"equal", sharedkernel.MustNewMoney(200, "EUR"), sharedkernel.MustNewMoney(200, "EUR"), 0, false},
		{"greater", sharedkernel.MustNewMoney(300, "EUR"), sharedkernel.MustNewMoney(200, "EUR"), 1, false},
		{"negative less than positive", sharedkernel.MustNewMoney(-1, "EUR"), sharedkernel.MustNewMoney(0, "EUR"), -1, false},
		{"cross currency rejected", sharedkernel.MustNewMoney(100, "EUR"), sharedkernel.MustNewMoney(100, "USD"), 0, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.a.Compare(tc.b)
			if tc.wantErr {
				if !errors.Is(err, sharedkernel.ErrCurrencyMismatch) {
					t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNegate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{"positive", 100, -100},
		{"negative", -100, 100},
		{"zero", 0, 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := sharedkernel.MustNewMoney(tc.in, "EUR")
			got := m.Negate()
			if got.Amount() != tc.want {
				t.Fatalf("amount = %d, want %d", got.Amount(), tc.want)
			}
			if got.Currency() != "EUR" {
				t.Fatalf("currency lost: %q", got.Currency())
			}
		})
	}
}

func TestDisplay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		amount int64
		want   string
	}{
		{"zero", 0, "0.00"},
		{"one cent", 1, "0.01"},
		{"one dollar", 100, "1.00"},
		{"with cents", 1234, "12.34"},
		{"big", 1_234_567, "12345.67"},
		{"negative", -150, "-1.50"},
		{"negative single cent", -1, "-0.01"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sharedkernel.MustNewMoney(tc.amount, "EUR").Display()
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		amount   int64
		currency sharedkernel.Currency
		want     string
	}{
		{"positive", 1234, "EUR", "12.34 EUR"},
		{"negative", -150, "USD", "-1.50 USD"},
		{"zero", 0, "GBP", "0.00 GBP"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sharedkernel.MustNewMoney(tc.amount, tc.currency).String()
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCurrencyString(t *testing.T) {
	t.Parallel()

	if got := sharedkernel.Currency("EUR").String(); got != "EUR" {
		t.Fatalf("got %q, want EUR", got)
	}
}

func TestImmutability(t *testing.T) {
	t.Parallel()

	// Operations must never mutate the receiver — Money is a value object.
	m := sharedkernel.MustNewMoney(100, "EUR")
	_, _ = m.Add(sharedkernel.MustNewMoney(50, "EUR"))
	_, _ = m.Sub(sharedkernel.MustNewMoney(10, "EUR"))
	_ = m.Mul(3)
	_ = m.MulFloat(0.5)
	_ = m.Negate()
	if m.Amount() != 100 {
		t.Fatalf("original amount mutated: %d", m.Amount())
	}
	if m.Currency() != "EUR" {
		t.Fatalf("original currency mutated: %q", m.Currency())
	}
}

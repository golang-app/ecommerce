package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/matryer/is"
)

type mockRepo struct {
	stats []query.CustomerOrderStat
	err   error
}

func (m mockRepo) Find(ctx context.Context, id string) (query.OrderView, error) {
	return query.OrderView{}, nil
}
func (m mockRepo) ListByCustomer(ctx context.Context, customerID string) ([]query.OrderSummary, error) {
	return nil, nil
}
func (m mockRepo) ListAll(ctx context.Context) ([]query.OrderSummary, error) {
	return nil, nil
}
func (m mockRepo) ListExpiredPending(ctx context.Context, olderThan time.Time) ([]string, error) {
	return nil, nil
}
func (m mockRepo) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}
func (m mockRepo) TodaysSales(ctx context.Context) (map[string]query.DailySalesRow, error) {
	return nil, nil
}
func (m mockRepo) ListCustomerOrderStats(ctx context.Context) ([]query.CustomerOrderStat, error) {
	return m.stats, m.err
}

func TestService_ListCustomerOrderStats(t *testing.T) {
	is := is.New(t)
	now := time.Now()
	expected := []query.CustomerOrderStat{
		query.NewCustomerOrderStat("cust_1", "John Doe", 3, 15000, "USD", now),
	}

	repo := mockRepo{stats: expected}
	service := query.NewService(repo)

	stats, err := service.ListCustomerOrderStats(context.Background())
	is.NoErr(err)
	is.Equal(len(stats), 1)
	is.Equal(stats[0].CustomerID(), "cust_1")
	is.Equal(stats[0].LatestShipName(), "John Doe")
	is.Equal(stats[0].OrderCount(), 3)
	is.Equal(stats[0].TotalSpent(), int64(15000))
	is.Equal(stats[0].TotalDisplay(), "150.00")
	is.Equal(stats[0].Currency(), "USD")
	is.Equal(stats[0].LastOrderAt(), now)
}

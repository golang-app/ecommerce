package shipment

import (
	"context"
	"database/sql"
)

type postgres struct {
	db *sql.DB
}

func newPostgres(db *sql.DB) postgres {
	return postgres{db: db}
}

func (repo postgres) List(ctx context.Context, customerID string) ([]Address, error) {
	addresses := []Address{}

	query := `SELECT customer_id, street, city, state, postal_code, country FROM addresses WHERE customer_id = $1`
	rows, err := repo.db.QueryContext(ctx, query, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var addr Address
		if err := rows.Scan(&addr.customerID, &addr.street, &addr.city, &addr.state, &addr.postalCode, &addr.country); err != nil {
			return nil, err
		}
		addresses = append(addresses, addr)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return addresses, nil
}

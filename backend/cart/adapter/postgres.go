package adapter

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

type postgres struct {
	db *sql.DB
}

type cartItem struct {
	id        string
	productID string
	name      string
	qty       int
	price     int
	currency  string
}

func NewPostgres(db *sql.DB) postgres {
	return postgres{db: db}
}

func (p postgres) Get(ctx context.Context, user domain.User) (*domain.Cart, error) {
	q := `SELECT user_id FROM cart_cart WHERE user_id = $1`
	row := p.db.QueryRowContext(ctx, q, user.ID())

	var userID string
	err := row.Scan(&userID)

	if err == sql.ErrNoRows {
		cart := domain.NewCart(user)
		return cart, nil
	}

	if err != nil {
		return nil, fmt.Errorf("could not read cart from the DB: %w", err)
	}

	items, err := p.readItems(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("cart item err: %w", err)
	}

	c := domain.NewCart(domain.NewUser(userID))
	for _, i := range items {
		cur, err := domain.NewCurrency(i.currency)
		if err != nil {
			return nil, fmt.Errorf("invalid currency on cart item: %w", err)
		}
		p := domain.NewProduct(i.productID, i.name, int64(i.price), cur)
		err = c.Add(p, i.qty)
		if err != nil {
			return nil, fmt.Errorf("could not add item to the cart: %w", err)
		}
	}

	return c, nil
}

func (p postgres) readItems(ctx context.Context, cartID string) ([]cartItem, error) {
	q := `SELECT id, product_id, product_name, qty, price, currency FROM cart_cart_item WHERE cart_id = $1`
	rows, err := p.db.QueryContext(ctx, q, cartID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []cartItem

	for rows.Next() {
		ci := cartItem{}
		err := rows.Scan(&ci.id, &ci.productID, &ci.name, &ci.qty, &ci.price, &ci.currency)
		if err != nil {
			return nil, fmt.Errorf("cart item scan err: %w", err)
		}

		items = append(items, ci)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("cart item rows err: %w", err)
	}

	return items, nil
}

// Clear deletes every line item for this user's cart but leaves the cart
// row itself. Called by the checkout flow after a successful order.
func (p postgres) Clear(ctx context.Context, user domain.User) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM cart_cart_item WHERE cart_id = $1`, user.ID())
	if err != nil {
		return fmt.Errorf("could not clear cart items: %w", err)
	}
	return nil
}

func (p postgres) Persist(ctx context.Context, cart *domain.Cart) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	q := `INSERT INTO cart_cart (user_id) VALUES ($1) ON CONFLICT (user_id) DO UPDATE SET user_id = $1`
	_, err = tx.ExecContext(ctx, q, cart.User().ID())
	if err != nil {
		return fmt.Errorf("could not insert cart: %w", err)
	}

	q = `DELETE FROM cart_cart_item WHERE cart_id = $1`
	_, err = tx.ExecContext(ctx, q, cart.User().ID())
	if err != nil {
		return fmt.Errorf("could not delete cart items: %w", err)
	}

	for _, i := range cart.Items() {
		cartItemID := fmt.Sprintf("%s-%s", cart.User().ID(), i.Product().ID())

		q = `INSERT INTO cart_cart_item (id,        cart_id, 	      product_id,       product_name,       qty,          price, currency) VALUES ($1, $2, $3, $4, $5, $6, $7)`
		_, err = tx.ExecContext(ctx, q, cartItemID, cart.User().ID(), i.Product().ID(), i.Product().Name(), i.Quantity(), i.Product().Price().Amount(), string(i.Product().Price().Currency()))
		if err != nil {
			return fmt.Errorf("could not insert cart item: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

	return nil
}

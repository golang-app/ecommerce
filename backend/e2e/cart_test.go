//go:build integration

package e2e

import (
	"io"
	"strings"
	"testing"

	"github.com/antchfx/jsonquery"
)

func Test_CartAddProduct(t *testing.T) {
	appCtx := newAppContext(t)
	defer appCtx.shutdown()

	pId := appCtx.addProduct("cookies!", "product 1 description", 10.0, "USD")
	cartID := randomID()

	resp, err := appCtx.sendApi("POST", "/api/v1/cart/"+cartID, []byte(`{"product_id": "`+pId+`", "qty": 1}`))
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 204, got %d, body: %s", resp.StatusCode, string(body))
	}

	resp2, err := appCtx.sendApi("GET", "/api/v1/cart/"+cartID, nil)
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp2.Body.Close()

	body, err := io.ReadAll(resp2.Body)

	if err != nil {
		t.Fatalf("could not read response body: %s", err)
	}

	doc, err := jsonquery.Parse(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("could not parse response: %s", err)
	}

	items := jsonquery.FindOne(doc, "data/items")
	val := items.Value().([]interface{})

	if len(val) != 1 {
		t.Fatalf("expected 1 item, got %d", len(val))
	}
}

func Test_CartAddProductNonExistingProduct(t *testing.T) {
	appCtx := newAppContext(t)
	defer appCtx.shutdown()

	cartID := randomID()

	resp, err := appCtx.sendApi("POST", "/api/v1/cart/"+cartID, []byte(`{"product_id": "not-exists", "qty": 1}`))
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected status code 404, got %d", resp.StatusCode)
	}
}

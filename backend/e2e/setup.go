//go:build integration

package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"testing"

	"github.com/ardanlabs/conf"
	"github.com/bkielbasa/go-ecommerce/backend/cart"
	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/sirupsen/logrus"
)

type config struct {
	Postgres postgresConfig
}

type postgresConfig struct {
	User     string `conf:"default:postgres"`
	Password string `conf:"default:postgres"`
	Port     int    `conf:"default:5432"`
	Host     string `conf:"default:localhost"`
	Db       string `conf:"default:ecommerce"`
}

func (pc postgresConfig) connectionString() string {
	var conn string

	if pc.Password != "" {
		conn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", pc.Host, pc.Port, pc.User, pc.Password, pc.Db)
	} else {
		conn = fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", pc.Host, pc.Port, pc.User, pc.Db)
	}

	return conn
}

type appContext struct {
	shutdown func()

	c    *http.Client
	port int

	prodStorage productStorage

	t *testing.T
}

func newAppContext(t *testing.T) appContext {
	cfg := config{}

	err := conf.Parse([]string{}, "", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			t.Fatal(conf.Usage("", &cfg))
		}
		t.Fatal(err)
	}
	ctx, cancel := internal.Context()
	port := freePort()

	app := application.New(ctx, port)

	connString := cfg.Postgres.connectionString()
	db, err := sql.Open("postgres", connString)
	if err != nil {
		t.Fatalf("cannot open connection to the DB: %s", err)
	}

	app.AddDependency(dependency.NewSQL(db))
	pcBD, cartService := productcatalog.New(db)

	app.AddBoundedContext(pcBD)
	app.AddBoundedContext(cart.New(db, logrus.New(), cartService))

	go func() {
		_ = app.Run()
	}()

	t.Logf("server started on port %d", port)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	return appContext{
		shutdown: cancel,
		t:        t,
		c: &http.Client{
			Jar: jar,
		},
		port: port,

		prodStorage: cartService,
	}
}

type productStorage interface {
	Add(ctx context.Context, id, name, desc string, price float64, currency string) error
}

func (appCtx *appContext) addProduct(name, description string, price float64, currency string) string {
	id := randomID()

	err := appCtx.prodStorage.Add(context.Background(), id, name, description, price, currency)
	if err != nil {
		appCtx.t.Fatalf("cannot add product to the storage: %s", err)
	}

	return id
}

func randomID() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func (appCtx *appContext) sendApi(method, path string, body []byte) (*http.Response, error) {
	r := fmt.Sprintf("Sending request: %s http://localhost:%d%s", method, appCtx.port, path)
	appCtx.t.Log(r)

	req, err := http.NewRequest(method, fmt.Sprintf("http://localhost:%d%s", appCtx.port, path), bytes.NewBuffer(body))
	if err != nil {
		appCtx.t.Fatalf("cannot create request: %s", err)
	}

	return appCtx.c.Do(req)
}

var (
	startMX   sync.Mutex
	startPort = 3000
)

func freePort() int {
	startMX.Lock()
	defer startMX.Unlock()

	for i := startPort; ; i++ {
		startPort++

		l, err := net.Listen("tcp", fmt.Sprintf(":%d", i))
		if err == nil {
			_ = l.Close()
			return i
		}
	}
}

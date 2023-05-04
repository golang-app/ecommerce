//go:build integration

package e2e

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func Test_Auth_Create_New_Account(t *testing.T) {
	appCtx := newAppContext(t)
	defer appCtx.shutdown()

	// create new account
	email := randomEmail()
	t.Logf("email: %s", email)
	resp, err := appCtx.sendApi("POST", "/api/v1/auth/register", []byte(fmt.Sprintf(`{"username": "%s", "password": "password"}`, email)))
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 204, got %d, body: %s", resp.StatusCode, string(body))
	}

	// we don't have confirming the email so we skip this step for now

	// login
	resp, err = appCtx.sendApi("POST", "/api/v1/auth/login", []byte(fmt.Sprintf(`{"username": "%s", "password": "password"}`, email)))
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 200, got %d, body: %s", resp.StatusCode, string(body))
	}

	// get user info
	resp, err = appCtx.sendApi("GET", "/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 200, got %d, body: %s", resp.StatusCode, string(body))
	}
}

func Test_Auth_Logout(t *testing.T) {
	appCtx := newAppContext(t)
	defer appCtx.shutdown()

	// create new account
	email := randomEmail()
	t.Log(email)
	resp, err := appCtx.sendApi("POST", "/api/v1/auth/register", []byte(fmt.Sprintf(`{"username": "%s", "password": "password"}`, email)))
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 204, got %d, body: %s", resp.StatusCode, string(body))
	}

	// we don't have confirming the email so we skip this step for now

	// login
	resp, err = appCtx.sendApi("POST", "/api/v1/auth/login", []byte(fmt.Sprintf(`{"username": "%s", "password": "password"}`, email)))
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 204, got %d, body: %s", resp.StatusCode, string(body))
	}

	// logout
	resp, err = appCtx.sendApi("DELETE", "/api/v1/auth/logout", nil)
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	// get user info
	resp, err = appCtx.sendApi("GET", "/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 401, got %d, body: %s", resp.StatusCode, string(body))
	}
}

func Test_WithoutLoginWeShouldntAccessMeEndpoint(t *testing.T) {
	appCtx := newAppContext(t)
	defer appCtx.shutdown()

	resp, err := appCtx.sendApi("GET", "/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("could not send request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status code 401, got %d, body: %s", resp.StatusCode, string(body))
	}
}

func randomEmail() string {
	return "test-" + randomID() + "@example.com"
}

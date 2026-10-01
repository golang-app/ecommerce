package imagestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestS3Save_UploadsFileAndReturnsCDNURL(t *testing.T) {
	var (
		receivedMethod      string
		receivedPath        string
		receivedContentType string
		receivedAuth        string
		receivedPayload     []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")
		receivedAuth = r.Header.Get("Authorization")
		receivedPayload, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewS3(S3Config{
		Bucket:         "ecommerce-assets",
		Region:         "us-east-1",
		Endpoint:       server.URL,
		AccessKey:      "AKIAIOSFODNN7EXAMPLE",
		SecretKey:      "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		CDNURL:         "https://cdn.store.com",
		ForcePathStyle: true,
		HTTPClient:     server.Client(),
	})

	url, err := store.Save(context.Background(), "photo.png", "image/png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("unexpected Save error: %v", err)
	}

	sum := sha256.Sum256(pngBytes)
	hash := hex.EncodeToString(sum[:])
	expectedKey := hash[:2] + "/" + hash + ".png"
	expectedURL := "https://cdn.store.com/" + expectedKey

	if url != expectedURL {
		t.Errorf("got url %q, want %q", url, expectedURL)
	}
	if receivedMethod != http.MethodPut {
		t.Errorf("got method %q, want PUT", receivedMethod)
	}
	if receivedPath != "/ecommerce-assets/"+expectedKey {
		t.Errorf("got path %q, want %q", receivedPath, "/ecommerce-assets/"+expectedKey)
	}
	if receivedContentType != "image/png" {
		t.Errorf("got content-type %q, want image/png", receivedContentType)
	}
	if !strings.HasPrefix(receivedAuth, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/") {
		t.Errorf("got auth %q, expected AWS4-HMAC-SHA256 credential prefix", receivedAuth)
	}
	if !bytes.Equal(receivedPayload, pngBytes) {
		t.Errorf("payload did not match uploaded bytes")
	}
}

func TestS3Save_DefaultURLWithoutCDN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewS3(S3Config{
		Bucket:         "assets",
		Region:         "eu-west-1",
		Endpoint:       server.URL,
		AccessKey:      "testkey",
		SecretKey:      "testsecret",
		ForcePathStyle: true,
		HTTPClient:     server.Client(),
	})

	url, err := store.Save(context.Background(), "icon.webp", "image/webp", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("unexpected Save error: %v", err)
	}

	sum := sha256.Sum256(pngBytes)
	hash := hex.EncodeToString(sum[:])
	expectedKey := hash[:2] + "/" + hash + ".webp"

	// With ForcePathStyle and Endpoint
	expectedURL := server.URL + "/assets/" + expectedKey
	if url != expectedURL {
		t.Errorf("got url %q, want %q", url, expectedURL)
	}
}

type mockRoundTripper struct {
	target *url.URL
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = m.target.Scheme
	clone.URL.Host = m.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestS3Save_StandardAWSURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targetURL, _ := url.Parse(server.URL)
	store := NewS3(S3Config{
		Bucket:    "mybucket",
		Region:    "us-west-2",
		AccessKey: "key",
		SecretKey: "secret",
		HTTPClient: &http.Client{
			Transport: &mockRoundTripper{target: targetURL},
		},
	})

	urlStr, err := store.Save(context.Background(), "photo.jpg", "image/jpeg", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("unexpected Save error: %v", err)
	}

	sum := sha256.Sum256(pngBytes)
	hash := hex.EncodeToString(sum[:])
	expectedKey := hash[:2] + "/" + hash + ".jpg"
	expectedURL := "https://mybucket.s3.us-west-2.amazonaws.com/" + expectedKey

	if urlStr != expectedURL {
		t.Errorf("got url %q, want %q", urlStr, expectedURL)
	}
}

func TestS3Save_RejectsUnsupportedType(t *testing.T) {
	store := NewS3(S3Config{Bucket: "b", Region: "us-east-1"})

	_, err := store.Save(context.Background(), "script.sh", "application/x-sh", bytes.NewReader([]byte("echo hi")))
	if !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("expected ErrUnsupportedType, got %v", err)
	}
}

func TestS3Save_RejectsOversizedInput(t *testing.T) {
	store := NewS3(S3Config{Bucket: "b", Region: "us-east-1"})

	big := bytes.NewReader(make([]byte, MaxImageSize+10))
	_, err := store.Save(context.Background(), "big.jpg", "image/jpeg", big)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("expected ErrTooLarge, got %v", err)
	}
}

func TestS3Save_ServerReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "<Error><Code>AccessDenied</Code></Error>", http.StatusForbidden)
	}))
	defer server.Close()

	store := NewS3(S3Config{
		Bucket:         "protected",
		Region:         "us-east-1",
		Endpoint:       server.URL,
		AccessKey:      "key",
		SecretKey:      "secret",
		ForcePathStyle: true,
		HTTPClient:     server.Client(),
	})

	_, err := store.Save(context.Background(), "test.png", "image/png", bytes.NewReader(pngBytes))
	if err == nil {
		t.Fatal("expected error when S3 returns 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to mention 403, got %v", err)
	}
}

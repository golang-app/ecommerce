package imagestore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// S3Config holds connection and bucket settings for S3-compatible cloud object storage
// (e.g. AWS S3, Cloudflare R2, Google Cloud Storage, or MinIO).
type S3Config struct {
	Bucket         string
	Region         string
	Endpoint       string // Custom endpoint, e.g. "https://<account>.r2.cloudflarestorage.com" or "http://minio:9000"
	AccessKey      string
	SecretKey      string
	CDNURL         string // Public CDN base URL (e.g. "https://cdn.example.com")
	ForcePathStyle bool   // When true, uses path-style URLs: http://endpoint/bucket/key
	HTTPClient     *http.Client
}

// S3Store is a Store that uploads product images to an S3-compatible object storage
// service with content-addressing and optional public CDN URL generation.
type S3Store struct {
	bucket         string
	region         string
	endpoint       string
	accessKey      string
	secretKey      string
	cdnURL         string
	forcePathStyle bool
	httpClient     *http.Client
	now            func() time.Time
}

var _ Store = (*S3Store)(nil)

// NewS3 constructs an S3Store.
func NewS3(cfg S3Config) *S3Store {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &S3Store{
		bucket:         cfg.Bucket,
		region:         region,
		endpoint:       strings.TrimRight(cfg.Endpoint, "/"),
		accessKey:      cfg.AccessKey,
		secretKey:      cfg.SecretKey,
		cdnURL:         strings.TrimRight(cfg.CDNURL, "/"),
		forcePathStyle: cfg.ForcePathStyle,
		httpClient:     client,
		now:            time.Now,
	}
}

// Save uploads an image to S3-compatible object storage using AWS Signature v4.
func (s *S3Store) Save(ctx context.Context, filename, contentType string, r io.Reader) (string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "image/jpg" {
		ct = "image/jpeg"
	}
	ext, ok := extByContentType[ct]
	if !ok {
		fallback := strings.ToLower(filepath.Ext(filename))
		switch fallback {
		case ".jpg", ".jpeg":
			ext = ".jpg"
			ct = "image/jpeg"
		case ".png":
			ext = ".png"
			ct = "image/png"
		case ".webp":
			ext = ".webp"
			ct = "image/webp"
		case ".gif":
			ext = ".gif"
			ct = "image/gif"
		default:
			return "", ErrUnsupportedType
		}
	}

	limited := io.LimitReader(r, MaxImageSize+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if int64(len(buf)) > MaxImageSize {
		return "", ErrTooLarge
	}

	sum := sha256.Sum256(buf)
	hash := hex.EncodeToString(sum[:])
	key := hash[:2] + "/" + hash + ext

	uploadURL, hostHeader, err := s.buildUploadURL(key)
	if err != nil {
		return "", fmt.Errorf("build upload url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("create s3 put request: %w", err)
	}

	req.Header.Set("Content-Type", ct)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(buf)))
	if hostHeader != "" {
		req.Host = hostHeader
	}

	payloadHash := hex.EncodeToString(sum[:])
	req.Header.Set("x-amz-content-sha256", payloadHash)

	now := s.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)

	if s.accessKey != "" && s.secretKey != "" {
		s.signV4(req, payloadHash, amzDate, dateStamp)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload to s3: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("s3 upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return s.buildPublicURL(key), nil
}

func (s *S3Store) buildUploadURL(key string) (string, string, error) {
	if s.endpoint != "" {
		u, err := url.Parse(s.endpoint)
		if err != nil {
			return "", "", err
		}
		host := u.Hostname()
		if s.forcePathStyle || net.ParseIP(host) != nil || host == "localhost" {
			return fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key), u.Host, nil
		}
		// Virtual hosted style
		uploadHost := fmt.Sprintf("%s.%s", s.bucket, u.Host)
		return fmt.Sprintf("%s://%s/%s", u.Scheme, uploadHost, key), uploadHost, nil
	}

	host := fmt.Sprintf("%s.s3.%s.amazonaws.com", s.bucket, s.region)
	return fmt.Sprintf("https://%s/%s", host, key), host, nil
}

func (s *S3Store) buildPublicURL(key string) string {
	if s.cdnURL != "" {
		return s.cdnURL + "/" + key
	}
	if s.endpoint != "" {
		u, err := url.Parse(s.endpoint)
		if err == nil {
			host := u.Hostname()
			if s.forcePathStyle || net.ParseIP(host) != nil || host == "localhost" {
				return s.endpoint + "/" + s.bucket + "/" + key
			}
			return fmt.Sprintf("%s://%s.%s/%s", u.Scheme, s.bucket, u.Host, key)
		}
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
}

func (s *S3Store) signV4(req *http.Request, payloadHash, amzDate, dateStamp string) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.Header.Get("Content-Type"),
		host,
		payloadHash,
		amzDate,
	)
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		canonicalURI,
		"", // query string
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	reqHash := sha256Hex([]byte(canonicalRequest))

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		reqHash,
	}, "\n")

	signingKey := getSignatureKey(s.secretKey, dateStamp, s.region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authHeader)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	return hmacSHA256(kService, []byte("aws4_request"))
}

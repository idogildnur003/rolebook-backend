// Package avatarstore wraps S3 presigned-URL generation for player avatars.
//
// Two modes:
//
//   - Configured: all four AWS_* env vars are set. PresignPut/PresignGet return
//     working presigned URLs for the configured bucket.
//   - Unconfigured: any of the four AWS_* env vars is empty (typical for local
//     dev). IsConfigured() returns false; the upload handler short-circuits to
//     503/UPLOAD_NOT_CONFIGURED, and Player loads pass avatarUri through
//     unchanged. See cmd/server/routes.go for wiring.
//
// Storage convention: avatarUri stored on Player records is an S3 object key
// (no scheme), e.g. "players/abc123/avatar/def.png". On read, handlers call
// PresignGet to convert the key into a short-lived signed URL before
// responding. Legacy values that already look like URLs (have a scheme) are
// passed through.
package avatarstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/elad/rolebook-backend/config"
	"github.com/google/uuid"
)

// PutPresignTTL is the lifetime of a presigned PUT URL.
const PutPresignTTL = 5 * time.Minute

// GetPresignTTL is the lifetime of a presigned GET URL on Player reads.
const GetPresignTTL = 1 * time.Minute

// CatalogImagePresignTTL is the lifetime of presigned GET URLs for catalog
// images. Long enough that clients cache the image across sessions; under the
// SigV4 7-day maximum.
const CatalogImagePresignTTL = 6 * 24 * time.Hour

// catalogCacheRefreshAhead re-signs a cached catalog URL once it is within this
// window of expiry, so callers never receive an about-to-expire URL.
const catalogCacheRefreshAhead = 12 * time.Hour

// MaxAvatarBytes is the upper bound on a single avatar upload (10 MiB).
// Enforced by the frontend before requesting an upload URL. The backend
// rejects oversized requests at /uploads/url when ContentLength is provided.
const MaxAvatarBytes int64 = 10 * 1024 * 1024

// AllowedContentTypes are the only image MIME types accepted for avatars.
var AllowedContentTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

// ErrInvalidContentType is returned when the requested contentType is not allowed.
var ErrInvalidContentType = errors.New("invalid content type")

// ErrNotConfigured is returned when avatar storage is not configured.
var ErrNotConfigured = errors.New("avatar storage not configured")

// ErrNotFound indicates the requested S3 object does not exist.
var ErrNotFound = errors.New("object not found")

// PresignedPut is the result of a successful PresignPut call.
type PresignedPut struct {
	URL       string
	Key       string
	ExpiresAt time.Time
}

// cachedURL holds a memoized presigned GET URL and its expiry time.
type cachedURL struct {
	url       string
	expiresAt time.Time
}

// presignClient is the small slice of the AWS SDK presign API we use.
// Defining it as an interface lets tests stub the SDK without mocking S3.
type presignClient interface {
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*presignedRequest, error)
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*presignedRequest, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// presignedRequest mirrors v4.PresignedHTTPRequest (URL only — that's all we expose).
type presignedRequest struct {
	URL string
}

// Store generates presigned URLs for player avatar objects.
type Store struct {
	baseUrl      string
	region       string
	bucket       string
	accessKey    string
	secretKey    string
	usePathStyle bool

	once   sync.Once
	client presignClient
	initEr error

	cacheMu  sync.RWMutex
	urlCache map[string]cachedURL
}

// New constructs a Store from raw env values. Empty values are allowed; the
// store will report IsConfigured() == false and refuse presign calls.
func New(cfg config.Config) *Store {
	return &Store{
		baseUrl:      cfg.AWSS3Endpoint,
		region:       cfg.AWSRegion,
		bucket:       cfg.AWSS3Bucket,
		accessKey:    cfg.AWSAccessKeyID,
		secretKey:    cfg.AWSSecretAccessKey,
		usePathStyle: cfg.AWSS3UsePathStyle,
	}
}

// IsConfigured reports whether all four credential pieces are non-empty.
func (s *Store) IsConfigured() bool {
	return s.baseUrl != "" && s.region != "" && s.bucket != "" && s.accessKey != "" && s.secretKey != ""
}

// SetClient replaces the underlying presign client. Tests use this to swap
// the real S3 SDK for a stub.
func (s *Store) SetClient(c presignClient) {
	s.once.Do(func() {})
	s.client = c
	s.initEr = nil
}

func (s *Store) ensureClient(ctx context.Context) (presignClient, error) {
	if !s.IsConfigured() {
		return nil, ErrNotConfigured
	}
	s.once.Do(func() {
		cfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithBaseEndpoint(s.baseUrl),
			awsconfig.WithRegion(s.region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s.accessKey, s.secretKey, "")),
		)
		if err != nil {
			s.initEr = fmt.Errorf("avatarstore: aws config: %w", err)
			return
		}
		s3client := s3.NewFromConfig(cfg, func(o *s3.Options) {
			// Path-style ("<endpoint>/<bucket>/<key>") is required by
			// S3-compatible stores like MinIO. Real AWS uses virtual-hosted
			// style ("<bucket>.<endpoint>") via DNS, so this stays opt-in.
			if s.usePathStyle {
				o.UsePathStyle = true
			}
		})
		s.client = realPresign{ps: s3.NewPresignClient(s3client), client: s3client}
	})
	if s.initEr != nil {
		return nil, s.initEr
	}
	return s.client, nil
}

// PresignPutForKey returns a presigned PUT URL whose object key lives under
// the given prefix. The prefix should be a slash-delimited path with NO
// trailing slash and NO scheme (e.g. "campaigns/abc/maps", "players/p1/avatar").
//
// Returns ErrInvalidContentType if contentType is outside AllowedContentTypes.
// Returns ErrNotConfigured if the store has no AWS credentials.
func (s *Store) PresignPutForKey(ctx context.Context, prefix, contentType string) (*PresignedPut, error) {
	ext, ok := AllowedContentTypes[contentType]
	if !ok {
		return nil, ErrInvalidContentType
	}
	client, err := s.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s/%s.%s", prefix, uuid.NewString(), ext)
	req, err := client.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(o *s3.PresignOptions) {
		o.Expires = PutPresignTTL
	})
	if err != nil {
		return nil, fmt.Errorf("avatarstore: presign put: %w", err)
	}
	return &PresignedPut{
		URL:       req.URL,
		Key:       key,
		ExpiresAt: time.Now().Add(PutPresignTTL).UTC(),
	}, nil
}

// PresignPut returns a presigned PUT URL for a new avatar object belonging to
// playerID. The contentType is signed into the URL; the client must use the
// same value when uploading.
func (s *Store) PresignPut(ctx context.Context, playerID, contentType string) (*PresignedPut, error) {
	return s.PresignPutForKey(ctx, fmt.Sprintf("players/%s/avatar", playerID), contentType)
}

// PresignGet returns a short-lived signed URL for reading the given key.
// Callers should pre-filter with LooksLikeKey.
func (s *Store) PresignGet(ctx context.Context, key string) (string, error) {
	client, err := s.ensureClient(ctx)
	if err != nil {
		return "", err
	}
	req, err := client.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = GetPresignTTL
	})
	if err != nil {
		return "", fmt.Errorf("avatarstore: presign get: %w", err)
	}
	return req.URL, nil
}

// Verify checks that the object referenced by key exists in S3.
// No-op (returns nil) when the store is unconfigured or the value is not a key
// (e.g., empty string or legacy fully-qualified URL).
// Returns ErrNotFound for missing objects; wraps other S3 errors.
func (s *Store) Verify(ctx context.Context, key string) error {
	if !LooksLikeKey(key) || !s.IsConfigured() {
		return nil
	}
	client, err := s.ensureClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nf *s3types.NotFound
		if errors.As(err, &nf) {
			return ErrNotFound
		}
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return ErrNotFound
		}
		return fmt.Errorf("avatarstore: head object: %w", err)
	}
	return nil
}

// Delete removes the S3 object at key. No-op when the store is unconfigured or
// the value is not a key (e.g., empty string or legacy fully-qualified URL).
// Callers should treat returned errors as best-effort signals: a failed delete
// should be logged but not fail the user-facing request.
func (s *Store) Delete(ctx context.Context, key string) error {
	if !LooksLikeKey(key) || !s.IsConfigured() {
		return nil
	}
	client, err := s.ensureClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("avatarstore: delete object: %w", err)
	}
	return nil
}

// LooksLikeKey reports whether s should be treated as an S3 object key (true)
// versus a fully-qualified URL (false). Anything with a "<scheme>:" prefix
// (http, https, file, content, data, idb, blob, …) is a URL.
func LooksLikeKey(s string) bool {
	if s == "" {
		return false
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		// schemes are alpha-only; reject if all chars before ':' qualify
		for _, c := range s[:i] {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '+' || c == '-' || c == '.') {
				return true
			}
		}
		return false
	}
	return true
}

// realPresign adapts an *s3.PresignClient to our presignClient interface.
type realPresign struct {
	ps     *s3.PresignClient
	client *s3.Client
}

func (r realPresign) PresignPutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*presignedRequest, error) {
	out, err := r.ps.PresignPutObject(ctx, in, optFns...)
	if err != nil {
		return nil, err
	}
	return &presignedRequest{URL: out.URL}, nil
}

func (r realPresign) PresignGetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*presignedRequest, error) {
	out, err := r.ps.PresignGetObject(ctx, in, optFns...)
	if err != nil {
		return nil, err
	}
	return &presignedRequest{URL: out.URL}, nil
}

func (r realPresign) HeadObject(ctx context.Context, in *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return r.client.HeadObject(ctx, in, optFns...)
}

func (r realPresign) DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return r.client.DeleteObject(ctx, in, optFns...)
}

// ResolveAvatarURI returns either a presigned GET URL (if avatarUri looks like
// a key and the store is configured) or the original value.
// On signing failure it returns the original value — losing access to the
// avatar is preferable to dropping the entire Player response.
func (s *Store) ResolveAvatarURI(ctx context.Context, avatarUri string) string {
	if !LooksLikeKey(avatarUri) || !s.IsConfigured() {
		return avatarUri
	}
	url, err := s.PresignGet(ctx, avatarUri)
	if err != nil {
		return avatarUri
	}
	return url
}

// ResolveImageURI is identical to ResolveAvatarURI; it's named generically so
// handlers for non-player resources don't read as "resolve avatar URI for a
// location thumbnail." Same pass-through rules: bare keys → presigned GET URL
// (when the store is configured), fully-qualified URLs → unchanged.
func (s *Store) ResolveImageURI(ctx context.Context, uri string) string {
	return s.ResolveAvatarURI(ctx, uri)
}

// presignGetWithTTL signs a GET URL for key with an explicit expiry.
func (s *Store) presignGetWithTTL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	client, err := s.ensureClient(ctx)
	if err != nil {
		return "", err
	}
	req, err := client.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("avatarstore: presign get: %w", err)
	}
	return req.URL, nil
}

// ResolveImageURICached behaves like ResolveImageURI but memoizes the presigned
// GET URL per object key with a long TTL, so repeated catalog reads return a
// stable URL (letting clients cache the image) and each key is signed at most
// once per refresh window. Falls back to the original value on any failure or
// when unconfigured / not a key.
func (s *Store) ResolveImageURICached(ctx context.Context, uri string) string {
	if !LooksLikeKey(uri) || !s.IsConfigured() {
		return uri
	}
	now := time.Now()

	s.cacheMu.RLock()
	entry, ok := s.urlCache[uri]
	s.cacheMu.RUnlock()
	if ok && now.Before(entry.expiresAt.Add(-catalogCacheRefreshAhead)) {
		return entry.url
	}

	url, err := s.presignGetWithTTL(ctx, uri, CatalogImagePresignTTL)
	if err != nil {
		return uri
	}

	s.cacheMu.Lock()
	if s.urlCache == nil {
		s.urlCache = make(map[string]cachedURL)
	}
	s.urlCache[uri] = cachedURL{url: url, expiresAt: now.Add(CatalogImagePresignTTL)}
	s.cacheMu.Unlock()
	return url
}

// Command set-bucket-cors applies the CORS policy required by the avatar
// upload flow to the configured S3 bucket. Reads credentials from the same
// env vars the server uses (AWS_S3_ENDPOINT, AWS_REGION, AWS_S3_BUCKET,
// AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY); .env in the working directory
// is loaded automatically.
//
// Usage:
//
//	go run ./cmd/set-bucket-cors
//	go run ./cmd/set-bucket-cors -origins "http://localhost:8081,https://app.example.com"
//
// Default origins cover Expo's web dev server (8081) and the Playwright
// rig (8090). Re-running is safe: PutBucketCors replaces the policy
// wholesale, so it's also the path to update origins later.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/joho/godotenv"
)

func main() {
	originsFlag := flag.String("origins", "http://localhost:8081,http://localhost:8090",
		"Comma-separated list of allowed browser origins")
	flag.Parse()

	_ = godotenv.Load()

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = os.Getenv("AWS_S3_ENDPOINT") // legacy fallback
	}
	pathStyle := isTrue(os.Getenv("AWS_S3_FORCE_PATH_STYLE"))
	region := mustEnv("AWS_REGION")
	bucket := mustEnv("AWS_S3_BUCKET")
	accessKey := mustEnv("AWS_ACCESS_KEY_ID")
	secretKey := mustEnv("AWS_SECRET_ACCESS_KEY")

	origins := splitCSV(*originsFlag)
	if len(origins) == 0 {
		log.Fatal("-origins must list at least one origin")
	}

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}

	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint) })
	}
	if pathStyle {
		clientOpts = append(clientOpts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	client := s3.NewFromConfig(cfg, clientOpts...)

	_, err = client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(bucket),
		CORSConfiguration: &types.CORSConfiguration{
			CORSRules: []types.CORSRule{{
				AllowedOrigins: origins,
				AllowedMethods: []string{"PUT", "GET"},
				AllowedHeaders: []string{"Content-Type"},
				ExposeHeaders:  []string{"ETag"},
				MaxAgeSeconds:  aws.Int32(3600),
			}},
		},
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotImplemented" {
			log.Fatalf("This S3 endpoint does not implement the bucket CORS API (got NotImplemented).\n"+
				"MinIO and some S3-compatible stores manage CORS at the server level, not per-bucket.\n"+
				"For MinIO, set CORS on the server instead and restart it, e.g.:\n"+
				"  MINIO_API_CORS_ALLOW_ORIGIN=\"%s\"   (or \"*\" for local dev)",
				strings.Join(origins, ","))
		}
		log.Fatalf("PutBucketCors: %v", err)
	}

	fmt.Printf("CORS applied to %s for origins: %s\n", bucket, strings.Join(origins, ", "))
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable: %s", key)
	}
	return v
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

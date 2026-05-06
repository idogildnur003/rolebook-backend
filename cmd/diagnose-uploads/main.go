// Command diagnose-uploads exercises the avatar upload flow end-to-end
// against the configured bucket and prints what each step returned. Useful
// when "the upload is failing" and you want to know which layer is at fault.
//
// Steps:
//   1. GetBucketCors — what's currently set on the bucket
//   2. PresignPut    — sign a throwaway URL the same way /uploads/url does
//   3. Preflight     — send an OPTIONS request from a fake browser origin
//                      and dump the CORS response headers
//   4. PUT           — upload a tiny payload and print the status
//
// Reads the same env vars the server uses (and .env in cwd).
//
// Usage:
//
//	go run ./cmd/diagnose-uploads
//	ORIGIN=https://app.example.com go run ./cmd/diagnose-uploads
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	endpoint := os.Getenv("AWS_S3_ENDPOINT")
	region := mustEnv("AWS_REGION")
	bucket := mustEnv("AWS_S3_BUCKET")
	accessKey := mustEnv("AWS_ACCESS_KEY_ID")
	secretKey := mustEnv("AWS_SECRET_ACCESS_KEY")

	origin := "http://localhost:8081"
	if v := os.Getenv("ORIGIN"); v != "" {
		origin = v
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
	client := s3.NewFromConfig(cfg, clientOpts...)

	fmt.Printf("bucket=%s region=%s endpoint=%q origin=%s\n\n", bucket, region, endpoint, origin)

	// 1. Current CORS
	fmt.Println("─── 1. GetBucketCors ───")
	cors, err := client.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: aws.String(bucket)})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		fmt.Println("  → If this says 'NoSuchCORSConfiguration', re-run cmd/set-bucket-cors first.")
	} else {
		for i, r := range cors.CORSRules {
			fmt.Printf("  rule[%d]\n", i)
			fmt.Printf("    AllowedOrigins: %v\n", r.AllowedOrigins)
			fmt.Printf("    AllowedMethods: %v\n", r.AllowedMethods)
			fmt.Printf("    AllowedHeaders: %v\n", r.AllowedHeaders)
			fmt.Printf("    ExposeHeaders : %v\n", r.ExposeHeaders)
			fmt.Printf("    MaxAgeSeconds : %v\n", aws.ToInt32(r.MaxAgeSeconds))
		}
	}
	fmt.Println()

	// 2. Presign
	fmt.Println("─── 2. Presign PUT ───")
	presigner := s3.NewPresignClient(client)
	testKey := fmt.Sprintf("diagnostics/test-%d.png", time.Now().Unix())
	put, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(testKey),
		ContentType: aws.String("image/png"),
	}, func(o *s3.PresignOptions) { o.Expires = 5 * time.Minute })
	if err != nil {
		log.Fatalf("PresignPutObject: %v", err)
	}
	fmt.Printf("  key: %s\n  url: %s\n\n", testKey, put.URL)

	// 3. Preflight as a browser would
	fmt.Println("─── 3. Preflight OPTIONS ───")
	preflight, _ := http.NewRequest(http.MethodOptions, put.URL, nil)
	preflight.Header.Set("Origin", origin)
	preflight.Header.Set("Access-Control-Request-Method", "PUT")
	preflight.Header.Set("Access-Control-Request-Headers", "content-type")
	preResp, err := http.DefaultClient.Do(preflight)
	if err != nil {
		log.Fatalf("preflight: %v", err)
	}
	fmt.Printf("  status: %s\n", preResp.Status)
	for _, h := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Max-Age",
	} {
		fmt.Printf("  %-30s %s\n", h+":", preResp.Header.Get(h))
	}
	preBody, _ := io.ReadAll(preResp.Body)
	preResp.Body.Close()
	if preResp.StatusCode >= 300 && len(preBody) > 0 {
		fmt.Printf("  body: %s\n", truncate(string(preBody), 400))
	}
	fmt.Println()

	// 4. Actual PUT
	fmt.Println("─── 4. PUT ───")
	body := bytes.Repeat([]byte{0xff}, 64)
	putReq, _ := http.NewRequest(http.MethodPut, put.URL, bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "image/png")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		log.Fatalf("PUT: %v", err)
	}
	defer putResp.Body.Close()
	rb, _ := io.ReadAll(putResp.Body)
	fmt.Printf("  status: %s\n", putResp.Status)
	if putResp.StatusCode >= 300 {
		fmt.Printf("  body: %s\n", truncate(string(rb), 600))
	}
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing required environment variable: %s", k)
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

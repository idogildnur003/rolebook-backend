package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/model"
)

// Presigning is offline signing, so this exercises the real avatarstore with
// no network, S3 or Mongo.
func TestResolvePlayerAvatar_PresignsStoredKey(t *testing.T) {
	avatars := avatarstore.New(config.Config{
		AWSS3Endpoint:      "https://s3.example",
		AWSRegion:          "us-east-1",
		AWSS3Bucket:        "bucket",
		AWSAccessKeyID:     "access",
		AWSSecretAccessKey: "secret",
	})
	p := &model.Player{AvatarURI: "players/p1/avatar/a.png"}

	resolvePlayerAvatar(context.Background(), avatars, p)

	if !strings.HasPrefix(p.AvatarURI, "https://") || !strings.Contains(p.AvatarURI, "players/p1/avatar/a.png") {
		t.Fatalf("stored key was not presigned, client cannot render it: %q", p.AvatarURI)
	}
	if !strings.Contains(p.AvatarURI, "X-Amz-Signature=") {
		t.Fatalf("presigned URL carries no signature: %q", p.AvatarURI)
	}
}

func TestResolvePlayerAvatar_PassThrough(t *testing.T) {
	unconfigured := avatarstore.New(config.Config{})
	p := &model.Player{AvatarURI: "players/p1/avatar/a.png"}
	resolvePlayerAvatar(context.Background(), unconfigured, p)
	if p.AvatarURI != "players/p1/avatar/a.png" {
		t.Fatalf("unconfigured store should pass through, got %q", p.AvatarURI)
	}

	resolvePlayerAvatar(context.Background(), unconfigured, nil) // must not panic
}

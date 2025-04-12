package tests

import (
	"context"
	"os"
	"testing"

	"go.orx.me/apps/hyper-sync/internal/social"
)

func TestNewBlueskyClientFromEnv(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

	if client == nil {
		t.Fatalf("Client is nil")
	}

	if client.Client == nil {
		t.Fatalf("XRPC client is nil")
	}

	if client.Client.Auth == nil {
		t.Fatalf("Auth is nil")
	}

	if client.Client.Auth.Did == "" {
		t.Fatalf("DID is empty")
	}
}

func TestPost(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

	// Test posting - only run this if specifically enabled with TEST_POST=1
	if os.Getenv("TEST_POST") == "1" {
		err = client.Post(context.Background(), "Test post from HyperSync unit test")
		if err != nil {
			t.Fatalf("Error posting to Bluesky: %v", err)
		}
	} else {
		t.Log("Skipping actual post test. Set TEST_POST=1 to enable.")
	}
}

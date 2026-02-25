package githubapi

import (
	"context"
	"net/http"
	"testing"

	"cpgo"
)

func TestNewClientFromToken(t *testing.T) {
	t.Run("returns client when token is set", func(t *testing.T) {
		client, err := NewClientFromToken(&http.Client{}, "token")
		if err != nil {
			t.Fatalf("new client from token: %v", err)
		}

		if client == nil {
			t.Fatalf("expected client")
		}
	})

	t.Run("returns error when token is empty", func(t *testing.T) {
		_, err := NewClientFromToken(&http.Client{}, "")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestNewClientFromAppValidation(t *testing.T) {
	t.Run("returns error for invalid app id", func(t *testing.T) {
		_, err := NewClientFromApp(context.Background(), AppClientRequest{
			AppID: 0,
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("returns error for missing private key", func(t *testing.T) {
		_, err := NewClientFromApp(context.Background(), AppClientRequest{
			AppID: 123,
			Repository: cpgo.RepositoryRef{
				Owner: "acme",
				Name:  "payments",
			},
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
)

func SeedUser(ctx context.Context, store UserStore, username, password string) error {
	_, err := store.GetByUsername(ctx, username)
	if err == nil {
		slog.Info("user already exists, skipping seed", "username", username)
		return nil
	}
	// A transient lookup error must not be mistaken for "user missing" —
	// creating anyway could insert a duplicate.
	if !errors.Is(err, ErrUserNotFound) {
		return fmt.Errorf("check existing user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	err = store.Create(ctx, &User{
		Username:     username,
		PasswordHash: string(hash),
	})
	if err != nil {
		return err
	}

	slog.Info("seeded initial user", "username", username)
	return nil
}

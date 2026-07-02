package auth

import (
	"context"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
)

func SeedUser(ctx context.Context, store UserStore, username, password string) error {
	_, err := store.GetByUsername(ctx, username)
	if err == nil {
		slog.Info("user already exists, skipping seed", "username", username)
		return nil
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

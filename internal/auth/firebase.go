package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
)

type firebaseVerifier struct {
	client *firebaseauth.Client
}

// NewFirebaseVerifier verifies Firebase ID tokens against Google's public keys.
func NewFirebaseVerifier(ctx context.Context, projectID string) (TokenVerifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("auth: firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: firebase auth client: %w", err)
	}
	return firebaseVerifier{client: client}, nil
}

func (v firebaseVerifier) Verify(ctx context.Context, idToken string) (Claims, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return Claims{}, fmt.Errorf("auth: verify id token: %w", err)
	}
	email, _ := token.Claims["email"].(string)
	return Claims{UID: token.UID, Email: email}, nil
}

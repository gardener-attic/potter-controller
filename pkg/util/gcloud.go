package util

import (
	"context"

	"github.com/pkg/errors"
	"golang.org/x/oauth2/google"
)

// GetGCloudAccessToken returns an access token from a google service account json string. Returns the access token or error.
func GetGCloudAccessToken(gcloudServiceAccountJSON string) (string, error) {
	jwtConfig, err := google.JWTConfigFromJSON([]byte(gcloudServiceAccountJSON), "https://www.googleapis.com/auth/devstorage.read_only")
	if err != nil {
		return "", errors.Wrap(err, "Couldn't create Google Service Account object")
	}
	tokenSource := jwtConfig.TokenSource(context.TODO())
	token, err := tokenSource.Token()
	if err != nil {
		return "", errors.Wrap(err, "Couldn't fetch token from token source")
	}
	return token.AccessToken, nil
}

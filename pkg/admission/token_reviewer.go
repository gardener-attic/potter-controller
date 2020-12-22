package admission

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/coreos/go-oidc"
	"github.com/go-logr/logr"
)

const (
	audienceMutatingWebhook = "mutating-webhook"
	bearerPrefix            = "BEARER "
)

func newTokenReviewer(handler http.Handler, issuer string, log logr.Logger) *tokenReviewer {
	return &tokenReviewer{
		handler: handler,
		issuer:  issuer,
		log:     log,
	}
}

// Implements http.Handler. Serves a request by first reviewing the bearer token
// and then delegating to the given http handler.
type tokenReviewer struct {
	handler http.Handler
	issuer  string
	log     logr.Logger
}

func (r *tokenReviewer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	token, err := r.getBearerToken(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	err = r.validateToken(token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	r.log.V(util.LogLevelDebug).Info("webhook token review: token successfully validated")

	r.handler.ServeHTTP(w, req)
}

func (r *tokenReviewer) getBearerToken(req *http.Request) (string, error) {
	authHeaders := req.Header["Authorization"]
	if len(authHeaders) == 0 {
		r.log.V(util.LogLevelWarning).Info("webhook token review: authorization header not set")
		return "", errors.New("authorization header not set")
	}

	for _, authHeader := range authHeaders {
		upperCaseAuthHeader := strings.ToUpper(authHeader)
		if strings.HasPrefix(upperCaseAuthHeader, bearerPrefix) {
			r.log.Info("webhook token review: bearer token found")
			return authHeader[len(bearerPrefix):], nil
		}
	}

	r.log.V(util.LogLevelWarning).Info("webhook token review: no bearer token found")
	return "", errors.New("no bearer token found")
}

func (r *tokenReviewer) validateToken(token string) error {
	ctx := context.Background()

	provider, err := oidc.NewProvider(ctx, r.issuer)
	if err != nil {
		r.log.Error(err, "webhook token review: oidc provider initialization failed")
		return errors.New("oidc provider initialization failed. " + err.Error())
	}

	var verifier = provider.Verifier(&oidc.Config{
		ClientID:             audienceMutatingWebhook,
		SupportedSigningAlgs: []string{oidc.RS256, oidc.RS384, oidc.RS512},
		SkipClientIDCheck:    false,
		SkipExpiryCheck:      false,
		SkipIssuerCheck:      false,
		Now:                  nil,
	})

	// Parse and verify ID Token payload.
	_, err = verifier.Verify(ctx, token)
	if err != nil {
		r.log.Error(err, "webhook token review: invalid bearer token")
		return errors.New("invalid bearer token: " + err.Error())
	}

	return nil
}

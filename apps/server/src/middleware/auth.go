package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/bananocoin/boompow/apps/server/src/database"
	"github.com/bananocoin/boompow/apps/server/src/models"
	"github.com/bananocoin/boompow/apps/server/src/repository"
	"github.com/bananocoin/boompow/libs/utils"
	"github.com/bananocoin/boompow/libs/utils/auth"
	"github.com/bananocoin/boompow/libs/utils/net"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

// We distinguish the type of authentication so we can restrict service tokens to only be used for work requests
type UserContextValue struct {
	User     *models.User
	AuthType string
}

var userCtxKey = &contextKey{"user"}

type contextKey struct {
	name string
}

func formatGraphqlError(ctx context.Context, msg string) string {
	marshalled, err := json.Marshal(graphql.ErrorResponse(ctx, "Invalid token"))
	if err != nil {
		return "\"errors\": [{\"message\": \"Unknown\"}]"
	}
	return string(marshalled)
}

func AuthMiddleware(userRepo *repository.UserService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// There are two types of tokens
			// The first is a JWT token that is used to authenticate users
			// The second is an "application" token that is used to authenticate services (no expiry)
			header := r.Header.Get("Authorization")

			// Allow unauthenticated users in
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}

			var ctx context.Context

			// Determine token type
			if strings.HasPrefix(header, "resetpassword:") {
				token := header[len("resetpassword:"):]
				email, err := auth.ParseToken(token)
				if err != nil {
					http.Error(w, formatGraphqlError(r.Context(), "Invalid Token"), http.StatusForbidden)
					return
				}
				// Get from redis
				_, err = database.GetRedisDB().GetResetPasswordToken(email)
				if err != nil {
					http.Error(w, formatGraphqlError(r.Context(), "Invalid Token"), http.StatusForbidden)
					return
				}
				// create user and check if user exists in db
				user, err := userRepo.GetUser(nil, &email)
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}
				// put it in context
				ctx = context.WithValue(r.Context(), userCtxKey, &UserContextValue{User: user, AuthType: "token"})
			} else if strings.HasPrefix(header, "service:") {
				// Service token
				if !slices.Contains(utils.GetServiceTokens(), header) {
					klog.Errorf("INVALID TOKEN ATTEMPT 1 %s:%s", header, net.GetIPAddress(r))
					http.Error(w, formatGraphqlError(r.Context(), "Invalid Token"), http.StatusForbidden)
					return
				}
				userID, err := database.GetRedisDB().GetServiceTokenUser(header)
				if err != nil {
					klog.Errorf("INVALID TOKEN ATTEMPT %s:%s", header, net.GetIPAddress(r))
					http.Error(w, formatGraphqlError(r.Context(), "Invalid Token"), http.StatusForbidden)
					return
				}
				userUUID, err := uuid.Parse(userID)
				if err != nil {
					http.Error(w, formatGraphqlError(r.Context(), "Invalid Token"), http.StatusForbidden)
					return
				}
				// create user and check if user exists in db
				user, err := userRepo.GetUser(&userUUID, nil)
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}
				// put it in context
				ctx = context.WithValue(r.Context(), userCtxKey, &UserContextValue{User: user, AuthType: "token"})
			} else {
				tokenStr := header
				email, err := auth.ParseToken(tokenStr)
				if err != nil {
					http.Error(w, formatGraphqlError(r.Context(), "Invalid Token"), http.StatusForbidden)
					return
				}
				// create user and check if user exists in db
				user, err := userRepo.GetUser(nil, &email)
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}
				// put it in context
				ctx = context.WithValue(r.Context(), userCtxKey, &UserContextValue{User: user, AuthType: "jwt"})

			}

			// and call the next with our new context
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// forContext finds the user from the context. REQUIRES Middleware to have run.
func forContext(ctx context.Context) *UserContextValue {
	raw, _ := ctx.Value(userCtxKey).(*UserContextValue)
	return raw
}

// AuthorizedUser returns user from context if they are logged in
func AuthorizedUser(ctx context.Context) *UserContextValue {
	contextValue := forContext(ctx)
	if contextValue == nil || contextValue.User == nil || contextValue.AuthType != "jwt" {
		return nil
	}
	return contextValue
}

// AuthorizedProvider returns user from context if they are an authorized provider type
func AuthorizedProvider(ctx context.Context) *UserContextValue {
	contextValue := forContext(ctx)
	if contextValue == nil || contextValue.User == nil || contextValue.AuthType != "jwt" || !contextValue.User.EmailVerified || contextValue.User.Type != models.PROVIDER {
		return nil
	}
	return contextValue
}

// AuthorizedRequester returns user from context if they are an authorized requester
func AuthorizedRequester(ctx context.Context) *UserContextValue {
	contextValue := forContext(ctx)
	if contextValue == nil || contextValue.User == nil || contextValue.AuthType != "jwt" || !contextValue.User.EmailVerified || !contextValue.User.CanRequestWork || contextValue.User.Type != models.REQUESTER {
		return nil
	}
	return contextValue
}

// AuthorizedServiceToken returns user from context if they are an authorized service token
func AuthorizedServiceToken(ctx context.Context) *UserContextValue {
	contextValue := forContext(ctx)
	if contextValue == nil || contextValue.User == nil || contextValue.AuthType != "token" || !contextValue.User.EmailVerified || !contextValue.User.CanRequestWork || contextValue.User.Type != models.REQUESTER {
		return nil
	}
	return contextValue
}

// AuthorizedChangePassword getsuser from context if they are authorized to change their password
func AuthorizedChangePassword(ctx context.Context) *UserContextValue {
	contextValue := forContext(ctx)
	if contextValue == nil || contextValue.User == nil || contextValue.AuthType != "token" {
		return nil
	}
	return contextValue
}

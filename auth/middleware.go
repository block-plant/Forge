package auth

import (
	"strings"

	"github.com/ayushkunwarsingh/forge/server"
)

// Middleware returns an HTTP middleware that verifies JWT tokens on requests.
// If the token is valid, it sets "auth_uid", "auth_email", and "auth_claims"
// on the request context. If the token is missing or invalid, the request
// continues without auth context (public endpoints still work).
func Middleware(jwtMgr *JWTManager) server.HandlerFunc {
	return func(ctx *server.Context) {
		token := extractBearerToken(ctx)
		if token == "" {
			ctx.Next()
			return
		}

		claims, err := jwtMgr.VerifyToken(token)
		if err != nil {
			// Token is present but invalid — still continue (let handlers decide)
			ctx.Set("auth_error", err.Error())
			ctx.Next()
			return
		}

		// Set auth context
		ctx.Set("auth_uid", claims.Sub)
		ctx.Set("auth_email", claims.Email)
		ctx.Set("auth_claims", claims)
		ctx.Set("auth_admin", claims.Admin)

		ctx.Next()
	}
}

// RequireAuth returns a middleware that rejects unauthenticated requests.
// Must be placed AFTER Middleware in the chain.
func RequireAuth() server.HandlerFunc {
	return func(ctx *server.Context) {
		uid := ctx.GetString("auth_uid")
		if uid == "" {
			authErr, _ := ctx.Get("auth_error")
			msg := "Authentication required"
			if authErr != nil {
				msg = "Invalid token: " + authErr.(string)
			}
			ctx.AbortWithError(401, msg)
			return
		}
		ctx.Next()
	}
}

// RequireAdmin returns a middleware that rejects non-admin requests.
// Must be placed AFTER Middleware and RequireAuth in the chain.
func RequireAdmin() server.HandlerFunc {
	return func(ctx *server.Context) {
		admin, ok := ctx.Get("auth_admin")
		if !ok || admin != true {
			ctx.AbortWithError(403, "Admin access required")
			return
		}
		ctx.Next()
	}
}

// GetAuthUID extracts the authenticated user's UID from the request context.
// Returns empty string if not authenticated.
func GetAuthUID(ctx *server.Context) string {
	return ctx.GetString("auth_uid")
}

// GetAuthClaims extracts the JWT claims from the request context.
// Returns nil if not authenticated.
func GetAuthClaims(ctx *server.Context) *JWTClaims {
	val, ok := ctx.Get("auth_claims")
	if !ok {
		return nil
	}
	claims, ok := val.(*JWTClaims)
	if !ok {
		return nil
	}
	return claims
}

// extractBearerToken extracts the JWT from the Authorization header.
// Expected format: "Bearer <token>"
func extractBearerToken(ctx *server.Context) string {
	auth := ctx.Header("authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

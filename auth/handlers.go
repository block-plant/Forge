package auth

import (
	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/server"
)

// RegisterRoutes registers all auth HTTP endpoints on the router.
func RegisterRoutes(router *server.Router, svc *Service) {
	auth := router.Group("/auth")

	// Public endpoints
	auth.POST("/signup", handleSignup(svc))
	auth.POST("/signin", handleSignin(svc))
	auth.POST("/refresh", handleRefresh(svc))
	auth.POST("/verify-email", handleVerifyEmail(svc))
	auth.POST("/forgot-password", handleForgotPassword(svc))
	auth.POST("/reset-password", handleResetPassword(svc))

	// JWKS public key endpoint
	auth.GET("/.well-known/jwks.json", handleJWKS(svc))

	// Authenticated endpoints
	auth.GET("/me", RequireAuth(), handleGetMe(svc))
	auth.PUT("/me", RequireAuth(), handleUpdateMe(svc))
	auth.POST("/signout", RequireAuth(), handleSignout(svc))
	auth.POST("/change-password", RequireAuth(), handleChangePassword(svc))

	// Admin endpoints
	admin := auth.Group("/admin", RequireAuth(), RequireAdmin())
	admin.GET("/users", handleAdminListUsers(svc))
	admin.PUT("/users/:uid", handleAdminUpdateUser(svc))
	admin.DELETE("/users/:uid", handleAdminDeleteUser(svc))
}

// ---- Public Handlers ----

// handleSignup handles POST /auth/signup
func handleSignup(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if req.Email == "" || req.Password == "" {
			ctx.Error(400, "Email and password are required")
			return
		}

		user, err := svc.Signup(req.Email, req.Password, req.DisplayName)
		if err != nil {
			switch err {
			case ErrUserAlreadyExists:
				ctx.Error(409, "A user with this email already exists")
			case ErrWeakPassword:
				ctx.Error(400, err.Error())
			default:
				ctx.Error(400, err.Error())
			}
			return
		}

		// Create session tokens
		tokens, err := svc.CreateSession(user, ctx.Header("user-agent"), ctx.RemoteAddr())
		if err != nil {
			ctx.Error(500, "Failed to create session")
			return
		}

		ctx.JSON(201, map[string]interface{}{
			"user":   user.ToPublic(),
			"tokens": tokens,
		})
	}
}

// handleSignin handles POST /auth/signin
func handleSignin(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if req.Email == "" || req.Password == "" {
			ctx.Error(400, "Email and password are required")
			return
		}

		user, err := svc.Signin(req.Email, req.Password)
		if err != nil {
			switch err {
			case ErrUserNotFound, ErrInvalidPassword:
				ctx.Error(401, "Invalid email or password")
			case ErrUserDisabled:
				ctx.Error(403, "Account is disabled")
			default:
				ctx.Error(401, "Authentication failed")
			}
			return
		}

		tokens, err := svc.CreateSession(user, ctx.Header("user-agent"), ctx.RemoteAddr())
		if err != nil {
			ctx.Error(500, "Failed to create session")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"user":   user.ToPublic(),
			"tokens": tokens,
		})
	}
}

// handleRefresh handles POST /auth/refresh
func handleRefresh(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if req.RefreshToken == "" {
			ctx.Error(400, "Refresh token is required")
			return
		}

		tokens, err := svc.RefreshSession(req.RefreshToken, ctx.Header("user-agent"), ctx.RemoteAddr())
		if err != nil {
			ctx.Error(401, "Invalid or expired refresh token")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"tokens": tokens,
		})
	}
}

// handleJWKS handles GET /auth/.well-known/jwks.json
func handleJWKS(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.SetResponseHeader("Cache-Control", "public, max-age=3600")
		ctx.JSON(200, svc.jwt.PublicKeyJWKS())
	}
}

// ---- Authenticated Handlers ----

// handleGetMe handles GET /auth/me
func handleGetMe(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		uid := GetAuthUID(ctx)
		user := svc.GetUser(uid)
		if user == nil {
			ctx.Error(404, "User not found")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"user": user.ToPublic(),
		})
	}
}

// handleUpdateMe handles PUT /auth/me
func handleUpdateMe(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		uid := GetAuthUID(ctx)

		var updates map[string]interface{}
		if err := ctx.BindJSON(&updates); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		// Only allow updating safe fields
		safeUpdates := make(map[string]interface{})
		if v, ok := updates["display_name"]; ok {
			safeUpdates["display_name"] = v
		}
		if v, ok := updates["photo_url"]; ok {
			safeUpdates["photo_url"] = v
		}

		user, err := svc.UpdateUser(uid, safeUpdates)
		if err != nil {
			ctx.Error(500, "Failed to update profile")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"user": user.ToPublic(),
		})
	}
}

// handleSignout handles POST /auth/signout
func handleSignout(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if req.RefreshToken != "" {
			svc.Signout(req.RefreshToken)
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "Signed out successfully",
		})
	}
}

// handleChangePassword handles POST /auth/change-password
func handleChangePassword(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		uid := GetAuthUID(ctx)

		var req struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if req.OldPassword == "" || req.NewPassword == "" {
			ctx.Error(400, "Old password and new password are required")
			return
		}

		if err := svc.ChangePassword(uid, req.OldPassword, req.NewPassword); err != nil {
			switch err {
			case ErrInvalidPassword:
				ctx.Error(401, "Current password is incorrect")
			case ErrWeakPassword:
				ctx.Error(400, err.Error())
			default:
				ctx.Error(500, "Failed to change password")
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "Password changed successfully",
		})
	}
}

// ---- Admin Handlers ----

// handleAdminListUsers handles GET /auth/admin/users
func handleAdminListUsers(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		users := svc.ListUsers()
		ctx.JSON(200, map[string]interface{}{
			"users": users,
			"total": len(users),
		})
	}
}

// handleAdminUpdateUser handles PUT /auth/admin/users/:uid
func handleAdminUpdateUser(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		uid := ctx.Param("uid")

		var updates map[string]interface{}
		if err := ctx.BindJSON(&updates); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		user, err := svc.UpdateUserAdmin(uid, updates)
		if err != nil {
			switch err {
			case ErrUserNotFound:
				ctx.Error(404, "User not found")
			default:
				ctx.Error(500, "Failed to update user")
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"user": user.ToPublic(),
		})
	}
}

// handleAdminDeleteUser handles DELETE /auth/admin/users/:uid
func handleAdminDeleteUser(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		uid := ctx.Param("uid")

		// Prevent self-deletion
		callerUID := GetAuthUID(ctx)
		if uid == callerUID {
			ctx.Error(400, "Cannot delete your own account via admin API")
			return
		}

		if err := svc.DeleteUser(uid); err != nil {
			switch err {
			case ErrUserNotFound:
				ctx.Error(404, "User not found")
			default:
				ctx.Error(500, "Failed to delete user")
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "User deleted successfully",
		})
	}
}

// handleVerifyEmail handles POST /auth/verify-email
func handleVerifyEmail(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if err := svc.VerifyOTP(req.Email, req.Code, "signup"); err != nil {
			ctx.Error(401, err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "Email verified successfully",
		})
	}
}

// handleForgotPassword handles POST /auth/forgot-password
func handleForgotPassword(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			Email string `json:"email"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if err := svc.RequestPasswordReset(req.Email); err != nil {
			// Log the actual error for debugging
			svc.log.Error("Failed to send password reset email", logger.Fields{
				"email": req.Email,
				"error": err.Error(),
			})

			// Don't leak user existence
			ctx.JSON(200, map[string]interface{}{
				"message": "If this email exists, a reset code has been sent.",
			})
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "Reset code sent to email",
		})
	}
}

// handleResetPassword handles POST /auth/reset-password
func handleResetPassword(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var req struct {
			Email       string `json:"email"`
			Code        string `json:"code"`
			NewPassword string `json:"new_password"`
		}
		if err := ctx.BindJSON(&req); err != nil {
			ctx.Error(400, "Invalid request body")
			return
		}

		if err := svc.ResetPassword(req.Email, req.Code, req.NewPassword); err != nil {
			ctx.Error(401, err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "Password reset successfully",
		})
	}
}

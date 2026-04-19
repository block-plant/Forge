package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ayushkunwarsingh/forge/server"
	"github.com/ayushkunwarsingh/forge/utils"
)

// OAuthConfig holds OAuth2 provider configuration.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
}

// GoogleOAuthConfig returns the OAuth2 config for Google.
func GoogleOAuthConfig(clientID, clientSecret, baseURL string) *OAuthConfig {
	return &OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  baseURL + "/auth/oauth/google/callback",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
		Scopes:       []string{"openid", "email", "profile"},
	}
}

// GitHubOAuthConfig returns the OAuth2 config for GitHub.
func GitHubOAuthConfig(clientID, clientSecret, baseURL string) *OAuthConfig {
	return &OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  baseURL + "/auth/oauth/github/callback",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		Scopes:       []string{"user:email"},
	}
}

// RegisterOAuthRoutes registers OAuth2 routes for configured providers.
func RegisterOAuthRoutes(router *server.Router, svc *Service, baseURL string) {
	cfg := svc.cfg

	if cfg.Auth.GoogleClientID != "" && cfg.Auth.GoogleClientSecret != "" {
		google := GoogleOAuthConfig(cfg.Auth.GoogleClientID, cfg.Auth.GoogleClientSecret, baseURL)
		router.GET("/auth/oauth/google", handleOAuthRedirect(google))
		router.GET("/auth/oauth/google/callback", handleOAuthCallback(svc, google, "google"))
	}

	if cfg.Auth.GitHubClientID != "" && cfg.Auth.GitHubClientSecret != "" {
		github := GitHubOAuthConfig(cfg.Auth.GitHubClientID, cfg.Auth.GitHubClientSecret, baseURL)
		router.GET("/auth/oauth/github", handleOAuthRedirect(github))
		router.GET("/auth/oauth/github/callback", handleOAuthCallback(svc, github, "github"))
	}
}

// handleOAuthRedirect redirects the user to the OAuth provider's authorization page.
func handleOAuthRedirect(cfg *OAuthConfig) server.HandlerFunc {
	return func(ctx *server.Context) {
		// Generate state parameter for CSRF protection
		state, err := utils.RandomHex(16)
		if err != nil {
			ctx.Error(500, "Failed to generate state")
			return
		}

		// Build authorization URL
		params := url.Values{}
		params.Set("client_id", cfg.ClientID)
		params.Set("redirect_uri", cfg.RedirectURL)
		params.Set("response_type", "code")
		params.Set("scope", strings.Join(cfg.Scopes, " "))
		params.Set("state", state)

		authURL := cfg.AuthURL + "?" + params.Encode()

		// In a full implementation, the state would be stored in a temporary session
		// For now, we include it in the redirect (client-side validation)
		ctx.Redirect(302, authURL)
	}
}

// handleOAuthCallback processes the OAuth provider's callback with the authorization code.
func handleOAuthCallback(svc *Service, cfg *OAuthConfig, provider string) server.HandlerFunc {
	return func(ctx *server.Context) {
		code := ctx.QueryParam("code")
		if code == "" {
			errMsg := ctx.QueryParam("error")
			if errMsg == "" {
				errMsg = "No authorization code received"
			}
			ctx.Error(400, "OAuth error: "+errMsg)
			return
		}

		// Exchange code for access token
		accessToken, err := exchangeCodeForToken(cfg, code)
		if err != nil {
			ctx.Error(500, "Failed to exchange authorization code: "+err.Error())
			return
		}

		// Get user info from provider
		email, name, picture, err := getUserInfo(cfg, accessToken, provider)
		if err != nil {
			ctx.Error(500, "Failed to get user info: "+err.Error())
			return
		}

		if email == "" {
			ctx.Error(400, "Could not obtain email from OAuth provider")
			return
		}

		// Find or create user
		user, created, err := svc.FindOrCreateOAuthUser(email, name, picture, provider)
		if err != nil {
			ctx.Error(500, "Failed to create user account")
			return
		}

		// Create session
		tokens, err := svc.CreateSession(user, ctx.Header("user-agent"), ctx.RemoteAddr())
		if err != nil {
			ctx.Error(500, "Failed to create session")
			return
		}

		// Return JSON with tokens (client-side SPA can handle this)
		status := 200
		if created {
			status = 201
		}

		ctx.JSON(status, map[string]interface{}{
			"user":    user.ToPublic(),
			"tokens":  tokens,
			"created": created,
		})
	}
}

// exchangeCodeForToken exchanges an OAuth authorization code for an access token.
func exchangeCodeForToken(cfg *OAuthConfig, code string) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", cfg.RedirectURL)
	data.Set("client_id", cfg.ClientID)
	data.Set("client_secret", cfg.ClientSecret)

	req, err := http.NewRequest("POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		// GitHub sometimes returns form-encoded responses
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return "", fmt.Errorf("failed to parse token response: %w", err)
		}
		return values.Get("access_token"), nil
	}

	token, ok := result["access_token"].(string)
	if !ok {
		if errMsg, ok := result["error"].(string); ok {
			return "", fmt.Errorf("token error: %s", errMsg)
		}
		return "", fmt.Errorf("no access_token in response")
	}

	return token, nil
}

// getUserInfo fetches user profile information from the OAuth provider.
func getUserInfo(cfg *OAuthConfig, accessToken, provider string) (email, name, picture string, err error) {
	req, err := http.NewRequest("GET", cfg.UserInfoURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	// GitHub requires a special User-Agent
	if provider == "github" {
		req.Header.Set("User-Agent", "Forge-Auth-Service")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read user info: %w", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", "", "", fmt.Errorf("failed to parse user info: %w", err)
	}

	switch provider {
	case "google":
		email, _ = info["email"].(string)
		name, _ = info["name"].(string)
		picture, _ = info["picture"].(string)
	case "github":
		email, _ = info["email"].(string)
		name, _ = info["name"].(string)
		if name == "" {
			name, _ = info["login"].(string)
		}
		picture, _ = info["avatar_url"].(string)

		// GitHub may not include email — need to fetch separately
		if email == "" {
			email, err = getGitHubEmail(accessToken)
			if err != nil {
				return "", "", "", err
			}
		}
	}

	return email, name, picture, nil
}

// getGitHubEmail fetches the user's primary email from GitHub's emails API.
func getGitHubEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Forge-Auth-Service")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var emails []map[string]interface{}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}

	// Find primary verified email
	for _, e := range emails {
		primary, _ := e["primary"].(bool)
		verified, _ := e["verified"].(bool)
		email, _ := e["email"].(string)
		if primary && verified && email != "" {
			return email, nil
		}
	}

	// Fallback: any verified email
	for _, e := range emails {
		verified, _ := e["verified"].(bool)
		email, _ := e["email"].(string)
		if verified && email != "" {
			return email, nil
		}
	}

	return "", fmt.Errorf("no verified email found on GitHub account")
}

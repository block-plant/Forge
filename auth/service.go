package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/utils"
)

// Service errors
var (
	ErrUserNotFound      = errors.New("auth: user not found")
	ErrUserAlreadyExists = errors.New("auth: user with this email already exists")
	ErrInvalidPassword   = errors.New("auth: invalid password")
	ErrUserDisabled      = errors.New("auth: user account is disabled")
	ErrWeakPassword      = errors.New("auth: password must be at least 6 characters")
	ErrInvalidOTP        = errors.New("auth: invalid or expired verification code")
)

// OTPRecord represents a temporary verification code.
type OTPRecord struct {
	Code      string
	ExpiresAt time.Time
	Type      string // "signup", "reset"
}

// User represents a Forge user account.
type User struct {
	UID           string                 `json:"uid"`
	Email         string                 `json:"email"`
	PasswordHash  string                 `json:"password_hash,omitempty"`
	DisplayName   string                 `json:"display_name,omitempty"`
	PhotoURL      string                 `json:"photo_url,omitempty"`
	EmailVerified bool                   `json:"email_verified"`
	Disabled      bool                   `json:"disabled"`
	Admin         bool                   `json:"admin"`
	Provider      string                 `json:"provider"` // "password", "google", "github"
	CustomClaims  map[string]interface{} `json:"custom_claims,omitempty"`
	CreatedAt     int64                  `json:"created_at"`
	UpdatedAt     int64                  `json:"updated_at"`
	LastLoginAt   int64                  `json:"last_login_at,omitempty"`
}

// PublicUser is the user data safe to send to clients (no password hash).
type PublicUser struct {
	UID           string                 `json:"uid"`
	Email         string                 `json:"email"`
	DisplayName   string                 `json:"display_name,omitempty"`
	PhotoURL      string                 `json:"photo_url,omitempty"`
	EmailVerified bool                   `json:"email_verified"`
	Disabled      bool                   `json:"disabled"`
	Provider      string                 `json:"provider"`
	CustomClaims  map[string]interface{} `json:"custom_claims,omitempty"`
	CreatedAt     int64                  `json:"created_at"`
	UpdatedAt     int64                  `json:"updated_at"`
	LastLoginAt   int64                  `json:"last_login_at,omitempty"`
}

// ToPublic converts a User to a PublicUser (strips sensitive fields).
func (u *User) ToPublic() *PublicUser {
	return &PublicUser{
		UID:           u.UID,
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		PhotoURL:      u.PhotoURL,
		EmailVerified: u.EmailVerified,
		Disabled:      u.Disabled,
		Provider:      u.Provider,
		CustomClaims:  u.CustomClaims,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		LastLoginAt:   u.LastLoginAt,
	}
}

// Service is the main authentication service.
type Service struct {
	mu       sync.RWMutex
	users    map[string]*User   // uid → User
	byEmail  map[string]string  // email → uid
	cfg      *config.Config
	log      *logger.Logger
	jwt      *JWTManager
	sessions *SessionManager
	tokens   *TokenStore
	dataDir    string
	bcryptCost int
	otpStore   map[string]*OTPRecord // email → OTPRecord
}

// NewService creates and initializes the auth service.
func NewService(cfg *config.Config, log *logger.Logger) (*Service, error) {
	dataDir := cfg.ResolveDataPath("auth")

	// Parse token expiry durations
	tokenExpiry, err := time.ParseDuration(cfg.Auth.TokenExpiry)
	if err != nil {
		tokenExpiry = 1 * time.Hour
	}

	refreshExpiry, err := time.ParseDuration(cfg.Auth.RefreshExpiry)
	if err != nil {
		refreshExpiry = 30 * 24 * time.Hour // 30 days
	}

	// Initialize JWT manager
	privateKeyPath := filepath.Join(dataDir, "private.pem")
	publicKeyPath := filepath.Join(dataDir, "public.pem")

	jwtManager, err := NewJWTManager(privateKeyPath, publicKeyPath, "forge", tokenExpiry, cfg.Auth.KeySize)
	if err != nil {
		return nil, fmt.Errorf("auth: failed to initialize JWT manager: %w", err)
	}

	// Initialize token store
	tokenDir := filepath.Join(dataDir, "tokens")
	tokenStore, err := NewTokenStore(tokenDir)
	if err != nil {
		return nil, fmt.Errorf("auth: failed to initialize token store: %w", err)
	}

	// Create session manager
	sessionMgr := NewSessionManager(jwtManager, tokenStore, refreshExpiry)

	svc := &Service{
		users:      make(map[string]*User),
		byEmail:    make(map[string]string),
		cfg:        cfg,
		log:        log.WithField("service", "auth"),
		jwt:        jwtManager,
		sessions:   sessionMgr,
		tokens:     tokenStore,
		dataDir:    dataDir,
		bcryptCost: cfg.Auth.BcryptCost,
		otpStore:   make(map[string]*OTPRecord),
	}

	// Load existing users from disk
	if err := svc.loadUsers(); err != nil {
		log.Warn("Could not load existing users", logger.Fields{"error": err.Error()})
	}

	log.Info("Auth service initialized", logger.Fields{
		"users_loaded": len(svc.users),
		"key_size":     cfg.Auth.KeySize,
		"token_expiry": cfg.Auth.TokenExpiry,
	})

	return svc, nil
}

// JWTManager returns the JWT manager (for middleware and other services).
func (s *Service) JWTManager() *JWTManager {
	return s.jwt
}

// ---- User Management ----

// Signup creates a new user with email and password.
func (s *Service) Signup(email, password, displayName string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	// Validate email
	if err := utils.ValidateEmail("email", email); err != nil {
		return nil, err
	}

	// Validate password strength
	if len(password) < 6 {
		return nil, ErrWeakPassword
	}

	s.mu.Lock()

	// Check for duplicate email
	if _, exists := s.byEmail[email]; exists {
		s.mu.Unlock()
		return nil, ErrUserAlreadyExists
	}

	// Hash password
	hash, err := HashPassword(password, s.bcryptCost)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("auth: failed to hash password: %w", err)
	}

	now := time.Now().Unix()
	uid := utils.MustGenerateUUID()

	user := &User{
		UID:          uid,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Provider:     "password",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.users[uid] = user
	s.byEmail[email] = uid
	s.mu.Unlock()

	// Persist to disk
	if err := s.saveUser(user); err != nil {
		s.log.Error("Failed to persist user", logger.Fields{"uid": uid, "error": err.Error()})
	}

	s.log.Info("User created", logger.Fields{"uid": uid, "email": email})

	// Generate and send OTP if email is enabled
	if s.cfg.Email.Enabled {
		s.SendVerificationOTP(email)
	}

	return user, nil
}

// SendVerificationOTP generates a 6-digit code and sends it via email.
func (s *Service) SendVerificationOTP(email string) error {
	code := fmt.Sprintf("%06d", utils.GenerateRandomInt(100000, 999999))
	
	s.mu.Lock()
	s.otpStore[email] = &OTPRecord{
		Code:      code,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Type:      "signup",
	}
	s.mu.Unlock()

	s.log.Info("Verification OTP generated", logger.Fields{"email": email, "code": code})

	if s.cfg.Email.Enabled {
		subject := "Verify your Forge account"
		body := fmt.Sprintf("Your verification code is: %s\n\nThis code will expire in 15 minutes.", code)
		
		err := utils.SendEmail(
			s.cfg.Email.Host,
			s.cfg.Email.Port,
			s.cfg.Email.User,
			s.cfg.Email.Password,
			s.cfg.Email.From,
			email,
			subject,
			body,
		)
		if err != nil {
			s.log.Error("Failed to send verification email", logger.Fields{"email": email, "error": err.Error()})
			return err
		}
	}
	
	return nil
}

// VerifyOTP checks if a code is valid and updates user status.
func (s *Service) VerifyOTP(email, code, otpType string) error {
	s.mu.Lock()
	record, exists := s.otpStore[email]
	if !exists || record.Code != code || record.Type != otpType || time.Now().After(record.ExpiresAt) {
		s.mu.Unlock()
		return ErrInvalidOTP
	}
	delete(s.otpStore, email)
	s.mu.Unlock()

	if otpType == "signup" {
		user := s.GetUserByEmail(email)
		if user == nil {
			return ErrUserNotFound
		}
		
		s.mu.Lock()
		user.EmailVerified = true
		user.UpdatedAt = time.Now().Unix()
		s.mu.Unlock()
		
		return s.saveUser(user)
	}

	return nil
}

// RequestPasswordReset sends an OTP for password recovery.
func (s *Service) RequestPasswordReset(email string) error {
	user := s.GetUserByEmail(email)
	if user == nil {
		return ErrUserNotFound
	}

	code := fmt.Sprintf("%06d", utils.GenerateRandomInt(100000, 999999))
	
	s.mu.Lock()
	s.otpStore[email] = &OTPRecord{
		Code:      code,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Type:      "reset",
	}
	s.mu.Unlock()

	s.log.Info("Reset OTP generated", logger.Fields{"email": email, "code": code})

	if s.cfg.Email.Enabled {
		subject := "Reset your Forge password"
		body := fmt.Sprintf("Your password reset code is: %s\n\nIf you did not request this, please ignore this email.", code)
		
		return utils.SendEmail(
			s.cfg.Email.Host,
			s.cfg.Email.Port,
			s.cfg.Email.User,
			s.cfg.Email.Password,
			s.cfg.Email.From,
			email,
			subject,
			body,
		)
	}
	
	return nil
}

// ResetPassword updates a password using a reset OTP.
func (s *Service) ResetPassword(email, code, newPassword string) error {
	if len(newPassword) < 6 {
		return ErrWeakPassword
	}

	if err := s.VerifyOTP(email, code, "reset"); err != nil {
		return err
	}

	user := s.GetUserByEmail(email)
	if user == nil {
		return ErrUserNotFound
	}

	hash, err := HashPassword(newPassword, s.bcryptCost)
	if err != nil {
		return err
	}

	s.mu.Lock()
	user.PasswordHash = hash
	user.UpdatedAt = time.Now().Unix()
	s.mu.Unlock()

	return s.saveUser(user)
}

// Signin authenticates a user with email and password.
func (s *Service) Signin(email, password string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	s.mu.RLock()
	uid, exists := s.byEmail[email]
	if !exists {
		s.mu.RUnlock()
		return nil, ErrUserNotFound
	}
	user := s.users[uid]
	s.mu.RUnlock()

	if user.Disabled {
		return nil, ErrUserDisabled
	}

	// Verify password
	if err := CheckPassword(password, user.PasswordHash); err != nil {
		return nil, ErrInvalidPassword
	}

	// Update last login
	s.mu.Lock()
	user.LastLoginAt = time.Now().Unix()
	s.mu.Unlock()
	s.saveUser(user)

	return user, nil
}

// GetUser retrieves a user by UID.
func (s *Service) GetUser(uid string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[uid]
}

// GetUserByEmail retrieves a user by email.
func (s *Service) GetUserByEmail(email string) *User {
	email = strings.TrimSpace(strings.ToLower(email))
	s.mu.RLock()
	defer s.mu.RUnlock()
	uid, ok := s.byEmail[email]
	if !ok {
		return nil
	}
	return s.users[uid]
}

// UpdateUser updates a user's profile fields.
func (s *Service) UpdateUser(uid string, updates map[string]interface{}) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[uid]
	if !ok {
		return nil, ErrUserNotFound
	}

	if name, ok := updates["display_name"].(string); ok {
		user.DisplayName = name
	}
	if photo, ok := updates["photo_url"].(string); ok {
		user.PhotoURL = photo
	}
	if verified, ok := updates["email_verified"].(bool); ok {
		user.EmailVerified = verified
	}

	user.UpdatedAt = time.Now().Unix()

	if err := s.saveUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdateUserAdmin performs admin-level user updates (disable, set admin, custom claims).
func (s *Service) UpdateUserAdmin(uid string, updates map[string]interface{}) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[uid]
	if !ok {
		return nil, ErrUserNotFound
	}

	if name, ok := updates["display_name"].(string); ok {
		user.DisplayName = name
	}
	if photo, ok := updates["photo_url"].(string); ok {
		user.PhotoURL = photo
	}
	if verified, ok := updates["email_verified"].(bool); ok {
		user.EmailVerified = verified
	}
	if disabled, ok := updates["disabled"].(bool); ok {
		user.Disabled = disabled
	}
	if admin, ok := updates["admin"].(bool); ok {
		user.Admin = admin
	}
	if claims, ok := updates["custom_claims"].(map[string]interface{}); ok {
		user.CustomClaims = claims
	}

	user.UpdatedAt = time.Now().Unix()

	if err := s.saveUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteUser removes a user and revokes all their tokens.
func (s *Service) DeleteUser(uid string) error {
	s.mu.Lock()

	user, ok := s.users[uid]
	if !ok {
		s.mu.Unlock()
		return ErrUserNotFound
	}

	delete(s.byEmail, user.Email)
	delete(s.users, uid)
	s.mu.Unlock()

	// Revoke all refresh tokens
	s.tokens.RevokeAllForUser(uid)

	// Remove from disk
	s.removeUserFile(uid)

	s.log.Info("User deleted", logger.Fields{"uid": uid, "email": user.Email})

	return nil
}

// ListUsers returns all users (admin API).
func (s *Service) ListUsers() []*PublicUser {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*PublicUser, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u.ToPublic())
	}
	return users
}

// ChangePassword changes a user's password.
func (s *Service) ChangePassword(uid, oldPassword, newPassword string) error {
	if len(newPassword) < 6 {
		return ErrWeakPassword
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[uid]
	if !ok {
		return ErrUserNotFound
	}

	// Verify old password
	if err := CheckPassword(oldPassword, user.PasswordHash); err != nil {
		return ErrInvalidPassword
	}

	// Hash new password
	hash, err := HashPassword(newPassword, s.bcryptCost)
	if err != nil {
		return fmt.Errorf("auth: failed to hash new password: %w", err)
	}

	user.PasswordHash = hash
	user.UpdatedAt = time.Now().Unix()

	return s.saveUser(user)
}

// CreateSession creates a token pair for a user (used after signup/signin).
func (s *Service) CreateSession(user *User, userAgent, ip string) (*TokenPair, error) {
	return s.sessions.CreateSession(user, userAgent, ip)
}

// RefreshSession exchanges a refresh token for new tokens.
func (s *Service) RefreshSession(refreshToken, userAgent, ip string) (*TokenPair, error) {
	return s.sessions.RefreshSession(refreshToken, s.GetUser, userAgent, ip)
}

// Signout revokes a refresh token.
func (s *Service) Signout(refreshToken string) error {
	return s.sessions.RevokeSession(refreshToken)
}

// ---- OAuth User Creation ----

// FindOrCreateOAuthUser finds an existing user by email or creates a new one from OAuth data.
func (s *Service) FindOrCreateOAuthUser(email, displayName, photoURL, provider string) (*User, bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if user exists
	if uid, exists := s.byEmail[email]; exists {
		user := s.users[uid]
		user.LastLoginAt = time.Now().Unix()
		s.saveUser(user)
		return user, false, nil // existing user
	}

	// Create new user
	now := time.Now().Unix()
	uid := utils.MustGenerateUUID()

	user := &User{
		UID:           uid,
		Email:         email,
		DisplayName:   displayName,
		PhotoURL:      photoURL,
		EmailVerified: true, // OAuth emails are verified
		Provider:      provider,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastLoginAt:   now,
	}

	s.users[uid] = user
	s.byEmail[email] = uid

	if err := s.saveUser(user); err != nil {
		s.log.Error("Failed to persist OAuth user", logger.Fields{"uid": uid, "error": err.Error()})
	}

	s.log.Info("OAuth user created", logger.Fields{"uid": uid, "email": email, "provider": provider})

	return user, true, nil
}

// ---- Persistence ----

// saveUser writes a user record to disk.
func (s *Service) saveUser(user *User) error {
	data, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(s.dataDir, "users", user.UID+".json")
	os.MkdirAll(filepath.Dir(filename), 0755)
	return os.WriteFile(filename, data, 0600)
}

// removeUserFile deletes a user's file from disk.
func (s *Service) removeUserFile(uid string) error {
	filename := filepath.Join(s.dataDir, "users", uid+".json")
	return os.Remove(filename)
}

// loadUsers reads all user files from disk into memory.
func (s *Service) loadUsers() error {
	usersDir := filepath.Join(s.dataDir, "users")

	entries, err := os.ReadDir(usersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No users yet
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(usersDir, entry.Name()))
		if err != nil {
			continue
		}

		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			continue
		}

		s.users[user.UID] = &user
		s.byEmail[user.Email] = user.UID
	}

	return nil
}

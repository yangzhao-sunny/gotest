package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo            *Repo
	jwtSecret       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewService(repo *Repo, secret string, accessTTLMin, refreshTTLDays int) *Service {
	return &Service{
		repo:            repo,
		jwtSecret:       secret,
		accessTokenTTL:  time.Duration(accessTTLMin) * time.Minute,
		refreshTokenTTL: time.Duration(refreshTTLDays) * 24 * time.Hour,
	}
}

func (s *Service) Register(ctx context.Context, email, password, displayName string) (*User, error) {
	existing, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("email_taken: %s already registered", email)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("bcrypt: %w", err)
	}
	return s.repo.CreateUser(ctx, email, string(hash), displayName)
}

func (s *Service) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	u, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("invalid_credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid_credentials")
	}
	return s.issueTokenPair(ctx, u.ID)
}

func (s *Service) Refresh(ctx context.Context, rawToken string) (string, error) {
	rt, err := s.repo.FindRefreshToken(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if rt == nil || rt.RevokedAt != nil || rt.ExpiresAt.Before(time.Now()) {
		return "", fmt.Errorf("invalid_refresh_token")
	}
	access, err := s.issueAccessToken(rt.UserID)
	if err != nil {
		return "", err
	}
	return access, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.repo.RevokeRefreshToken(ctx, rawToken)
}

func (s *Service) issueTokenPair(ctx context.Context, userID string) (*TokenPair, error) {
	access, err := s.issueAccessToken(userID)
	if err != nil {
		return nil, err
	}
	rawRefresh := uuid.NewString()
	if _, err := s.repo.CreateRefreshToken(ctx, userID, rawRefresh, time.Now().Add(s.refreshTokenTTL)); err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: rawRefresh}, nil
}

func (s *Service) issueAccessToken(userID string) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(s.accessTokenTTL).Unix(),
		"jti": uuid.NewString(),
	})
	return tok.SignedString([]byte(s.jwtSecret))
}

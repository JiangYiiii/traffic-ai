// Package jwt 封装 JWT access/refresh token 签发与校验。
// @ai_doc JWT双令牌机制: accessToken 短期(2h) + refreshToken 长期(7d)，refresh 使用后轮换
package jwt

import (
	"errors"
	"time"

	jwtgo "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwtgo.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewManager(secret string, accessTTLSec, refreshTTLSec int) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  time.Duration(accessTTLSec) * time.Second,
		refreshTTL: time.Duration(refreshTTLSec) * time.Second,
	}
}

func (m *Manager) GeneratePair(userID int64, role string) (*TokenPair, error) {
	now := time.Now()
	accessToken, err := m.generate(userID, role, "access", now, m.accessTTL)
	if err != nil {
		return nil, err
	}
	refreshToken, err := m.generate(userID, role, "refresh", now, m.refreshTTL)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(m.accessTTL.Seconds()),
	}, nil
}

func (m *Manager) generate(userID int64, role, subject string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwtgo.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwtgo.NewNumericDate(now),
			ExpiresAt: jwtgo.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) ParseAccess(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr, "access")
}

func (m *Manager) ParseRefresh(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr, "refresh")
}

func (m *Manager) parse(tokenStr, expectedSubject string) (*Claims, error) {
	token, err := jwtgo.ParseWithClaims(tokenStr, &Claims{}, func(t *jwtgo.Token) (interface{}, error) {
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Subject != expectedSubject {
		return nil, errors.New("token subject mismatch")
	}
	return claims, nil
}

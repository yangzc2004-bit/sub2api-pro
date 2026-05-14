package service

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const kimiAuthorizeDeviceURL = "https://www.kimi.com/code/authorize_device"

type KimiOAuthService struct {
	proxyRepo   ProxyRepository
	oauthClient KimiOAuthClient
	sessions    map[string]*KimiOAuthSession
	mu          sync.RWMutex
}

type KimiOAuthSession struct {
	DeviceCode string
	UserCode   string
	AuthURL    string
	ProxyURL   string
	ExpiresAt  time.Time
	Interval   int64
}

type KimiAuthURLResult struct {
	AuthURL    string `json:"auth_url"`
	SessionID  string `json:"session_id"`
	UserCode   string `json:"user_code"`
	DeviceCode string `json:"device_code,omitempty"`
	ExpiresIn  int64  `json:"expires_in"`
	Interval   int64  `json:"interval"`
}

type KimiTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	Scope        string `json:"scope,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
}

func NewKimiOAuthService(proxyRepo ProxyRepository, oauthClient KimiOAuthClient) *KimiOAuthService {
	return &KimiOAuthService{
		proxyRepo:   proxyRepo,
		oauthClient: oauthClient,
		sessions:    make(map[string]*KimiOAuthSession),
	}
}

func (s *KimiOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*KimiAuthURLResult, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	device, err := s.oauthClient.StartDeviceAuthorization(ctx, proxyURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(device.DeviceCode) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "KIMI_OAUTH_MISSING_DEVICE_CODE", "kimi device authorization response is missing device_code")
	}

	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}
	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}
	interval := device.Interval
	if interval <= 0 {
		interval = 5
	}
	authURL := strings.TrimSpace(device.VerificationURIComplete)
	if authURL == "" {
		authURL = buildKimiAuthorizeURL(device.UserCode)
	}

	session := &KimiOAuthSession{
		DeviceCode: device.DeviceCode,
		UserCode:   device.UserCode,
		AuthURL:    authURL,
		ProxyURL:   proxyURL,
		ExpiresAt:  time.Now().Add(time.Duration(expiresIn) * time.Second),
		Interval:   interval,
	}
	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	return &KimiAuthURLResult{
		AuthURL:    authURL,
		SessionID:  sessionID,
		UserCode:   device.UserCode,
		DeviceCode: device.DeviceCode,
		ExpiresIn:  expiresIn,
		Interval:   interval,
	}, nil
}

func (s *KimiOAuthService) ExchangeDeviceCode(ctx context.Context, sessionID string, proxyID *int64) (*KimiTokenInfo, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	if time.Now().After(session.ExpiresAt) {
		s.deleteSession(sessionID)
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_EXPIRED", "device authorization session expired")
	}

	proxyURL := session.ProxyURL
	if proxyID != nil {
		var err error
		proxyURL, err = s.proxyURL(ctx, proxyID)
		if err != nil {
			return nil, err
		}
	}

	tokenResp, err := s.oauthClient.PollToken(ctx, session.DeviceCode, proxyURL)
	if err != nil {
		return nil, err
	}
	s.deleteSession(sessionID)
	return kimiTokenInfoFromResponse(tokenResp), nil
}

func (s *KimiOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*KimiTokenInfo, error) {
	tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
	}
	return kimiTokenInfoFromResponse(tokenResp), nil
}

func (s *KimiOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KimiTokenInfo, error) {
	if account.Platform != PlatformKimi || account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_ACCOUNT", "account is not a Kimi OAuth account")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		accessToken := strings.TrimSpace(account.GetCredential("access_token"))
		if accessToken == "" {
			return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
		}
		info := &KimiTokenInfo{
			AccessToken: accessToken,
			TokenType:   account.GetCredential("token_type"),
			Scope:       account.GetCredential("scope"),
			ClientID:    account.GetCredential("client_id"),
		}
		if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil {
			info.ExpiresAt = expiresAt.Unix()
			info.ExpiresIn = int64(time.Until(*expiresAt).Seconds())
		}
		return info, nil
	}

	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	return s.RefreshToken(ctx, refreshToken, proxyURL)
}

func (s *KimiOAuthService) BuildAccountCredentials(tokenInfo *KimiTokenInfo) map[string]any {
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   expiresAt,
	}
	if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if strings.TrimSpace(tokenInfo.TokenType) != "" {
		creds["token_type"] = tokenInfo.TokenType
	}
	if strings.TrimSpace(tokenInfo.Scope) != "" {
		creds["scope"] = tokenInfo.Scope
	}
	if strings.TrimSpace(tokenInfo.ClientID) != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	return creds
}

func (s *KimiOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "KIMI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func (s *KimiOAuthService) deleteSession(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

func kimiTokenInfoFromResponse(resp *KimiTokenResponse) *KimiTokenInfo {
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return &KimiTokenInfo{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresIn:    expiresIn,
		ExpiresAt:    time.Now().Unix() + expiresIn,
		Scope:        resp.Scope,
		ClientID:     "kimi-code",
	}
}

func buildKimiAuthorizeURL(userCode string) string {
	userCode = strings.TrimSpace(userCode)
	if userCode == "" {
		return kimiAuthorizeDeviceURL
	}
	return kimiAuthorizeDeviceURL + "?user_code=" + userCode
}

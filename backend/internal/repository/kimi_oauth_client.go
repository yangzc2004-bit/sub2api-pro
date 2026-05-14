package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

const (
	kimiDeviceAuthorizationURL = "https://auth.kimi.com/api/oauth/device_authorization"
	kimiTokenURL               = "https://auth.kimi.com/api/oauth/token"
	kimiOAuthClientID          = "17e5f671-d194-4dfb-9706-5516cb48c098"
)

var kimiOAuthDeviceID = generateKimiOAuthDeviceID()

type kimiOAuthClient struct{}

func NewKimiOAuthClient() service.KimiOAuthClient {
	return &kimiOAuthClient{}
}

func (c *kimiOAuthClient) StartDeviceAuthorization(ctx context.Context, proxyURL string) (*service.KimiDeviceAuthorizationResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("client_id", kimiOAuthClientID)

	var out service.KimiDeviceAuthorizationResponse
	req := client.R().
		SetContext(ctx).
		SetFormDataFromValues(formData).
		SetSuccessResult(&out)
	setKimiOAuthHeaders(req)

	resp, err := req.Post(kimiDeviceAuthorizationURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_DEVICE_AUTH_FAILED", "device authorization failed: status %d, body: %s", resp.StatusCode, resp.String())
	}
	return &out, nil
}

func (c *kimiOAuthClient) PollToken(ctx context.Context, deviceCode string, proxyURL string) (*service.KimiTokenResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	formData.Set("client_id", kimiOAuthClientID)
	formData.Set("device_code", deviceCode)

	var out service.KimiTokenResponse
	req := client.R().
		SetContext(ctx).
		SetFormDataFromValues(formData).
		SetSuccessResult(&out)
	setKimiOAuthHeaders(req)

	resp, err := req.Post(kimiTokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		body := strings.TrimSpace(resp.String())
		code := "KIMI_OAUTH_TOKEN_EXCHANGE_FAILED"
		status := http.StatusBadGateway
		if strings.Contains(body, "authorization_pending") || strings.Contains(body, "slow_down") {
			code = "KIMI_OAUTH_AUTHORIZATION_PENDING"
			status = http.StatusAccepted
		}
		return nil, infraerrors.Newf(status, code, "token exchange failed: status %d, body: %s", resp.StatusCode, body)
	}
	return &out, nil
}

func (c *kimiOAuthClient) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*service.KimiTokenResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("client_id", kimiOAuthClientID)
	formData.Set("refresh_token", refreshToken)

	var out service.KimiTokenResponse
	req := client.R().
		SetContext(ctx).
		SetFormDataFromValues(formData).
		SetSuccessResult(&out)
	setKimiOAuthHeaders(req)

	resp, err := req.Post(kimiTokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status %d, body: %s", resp.StatusCode, resp.String())
	}
	return &out, nil
}

func createKimiReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
	})
}

func setKimiOAuthHeaders(r *req.Request) {
	deviceName, err := os.Hostname()
	if err != nil || strings.TrimSpace(deviceName) == "" {
		deviceName = "sub2api"
	}
	deviceModel := runtime.GOOS + " " + runtime.GOARCH

	r.SetHeader("User-Agent", kimi.CLIUserAgent).
		SetHeader("X-Msh-Platform", "kimi_cli").
		SetHeader("X-Msh-Version", kimi.CLIClientVersion).
		SetHeader("X-Msh-Device-Name", deviceName).
		SetHeader("X-Msh-Device-Model", deviceModel).
		SetHeader("X-Msh-Os-Version", runtime.GOOS).
		SetHeader("X-Msh-Device-Id", kimiOAuthDeviceID)
}

func generateKimiOAuthDeviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}

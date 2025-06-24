package social

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type ThreadsClient struct {
	ClientID     string
	ClientSecret string
	AccessToken  string
}

// TokenResponse 表示 Threads API token 响应
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"` // 秒数，直到令牌过期
}

func NewThreadsClient(clientID, clientSecret, accessToken string) (*ThreadsClient, error) {
	return &ThreadsClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AccessToken:  accessToken,
	}, nil
}

// ExchangeForLongLivedToken 将短期访问令牌交换为长期访问令牌
// 长期令牌有效期为60天，可以刷新
func (c *ThreadsClient) ExchangeForLongLivedToken(shortLivedToken string) (*TokenResponse, error) {
	if c.ClientSecret == "" {
		return nil, fmt.Errorf("client secret is required for token exchange")
	}

	// 构建请求URL
	baseURL := "https://graph.threads.net/access_token"
	params := url.Values{}
	params.Add("grant_type", "th_exchange_token")
	params.Add("client_secret", c.ClientSecret)
	params.Add("access_token", shortLivedToken)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 发送GET请求
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// 更新客户端的访问令牌
	c.AccessToken = tokenResp.AccessToken

	return &tokenResp, nil
}

// RefreshLongLivedToken 刷新长期访问令牌
// 长期令牌必须至少24小时旧但尚未过期才能刷新
// 刷新后的令牌有效期为60天
func (c *ThreadsClient) RefreshLongLivedToken() (*TokenResponse, error) {
	if c.AccessToken == "" {
		return nil, fmt.Errorf("access token is required for token refresh")
	}

	// 构建请求URL
	baseURL := "https://graph.threads.net/refresh_access_token"
	params := url.Values{}
	params.Add("grant_type", "th_refresh_token")
	params.Add("access_token", c.AccessToken)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 发送GET请求
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// 更新客户端的访问令牌
	c.AccessToken = tokenResp.AccessToken

	return &tokenResp, nil
}

// GetTokenExpirationTime 根据 ExpiresIn 计算令牌过期时间
func (tr *TokenResponse) GetTokenExpirationTime() time.Time {
	return time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
}

// IsTokenExpiringSoon 检查令牌是否在指定时间内过期
// 建议在令牌过期前7天开始尝试刷新
func (tr *TokenResponse) IsTokenExpiringSoon(threshold time.Duration) bool {
	expirationTime := tr.GetTokenExpirationTime()
	return time.Until(expirationTime) <= threshold
}

// ShouldRefreshToken 检查是否应该刷新令牌
// 长期令牌必须至少24小时旧才能刷新，建议在过期前7天刷新
func (tr *TokenResponse) ShouldRefreshToken() bool {
	// 检查是否在7天内过期
	return tr.IsTokenExpiringSoon(7 * 24 * time.Hour)
}

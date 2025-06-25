package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// TokenInfo 包含 token 和过期时间信息
type TokenInfo struct {
	AccessToken string
	ExpiresAt   *time.Time
}

// ConfigDao 定义配置数据访问接口，只处理 access token
type ConfigDao interface {
	GetAccessToken(ctx context.Context, platform string) (string, error)
	GetTokenInfo(ctx context.Context, platform string) (*TokenInfo, error)
	SaveAccessToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error
}

// ThreadsConfig represents Threads configuration
type ThreadsConfig struct {
	ClientID     string     `json:"client_id"`
	ClientSecret string     `json:"client_secret"`
	AccessToken  string     `json:"access_token"`
	TokenType    string     `json:"token_type,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

type ThreadsClient struct {
	ClientID     string
	ClientSecret string
	AccessToken  string
	configDao    ConfigDao
}

// SetConfigDao sets the config dao for the client (useful for testing)
func (c *ThreadsClient) SetConfigDao(dao ConfigDao) {
	c.configDao = dao
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

// NewThreadsClientWithDao creates a new ThreadsClient with dao support
// clientID 和 clientSecret 通过参数传入，只有 accessToken 从 dao 获取
func NewThreadsClientWithDao(clientID, clientSecret string, configDao ConfigDao) (*ThreadsClient, error) {
	client := &ThreadsClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		configDao:    configDao,
	}

	// Load access token from dao
	err := client.LoadAccessTokenFromDao(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load access token from dao: %w", err)
	}

	return client, nil
}

// LoadAccessTokenFromDao loads access token from database
func (c *ThreadsClient) LoadAccessTokenFromDao(ctx context.Context) error {
	if c.configDao == nil {
		return fmt.Errorf("config dao is not set")
	}

	accessToken, err := c.configDao.GetAccessToken(ctx, "threads")
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	if accessToken == "" {
		return fmt.Errorf("access token not found in database")
	}

	c.AccessToken = accessToken
	return nil
}

// EnsureValidToken 确保 token 有效，如果快过期则自动刷新
func (c *ThreadsClient) EnsureValidToken(ctx context.Context) error {
	if c.configDao == nil {
		return fmt.Errorf("config dao is not set")
	}

	// 获取 token 信息（包含过期时间）
	tokenInfo, err := c.configDao.GetTokenInfo(ctx, "threads")
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if tokenInfo == nil || tokenInfo.AccessToken == "" {
		return fmt.Errorf("access token not found in database")
	}

	// 更新客户端的 access token
	c.AccessToken = tokenInfo.AccessToken

	// 如果没有过期时间信息，无法判断是否需要刷新，直接使用现有 token
	if tokenInfo.ExpiresAt == nil {
		return nil
	}

	// 检查 token 是否在 7 天内过期
	const refreshThreshold = 7 * 24 * time.Hour
	timeUntilExpiry := time.Until(*tokenInfo.ExpiresAt)

	if timeUntilExpiry <= refreshThreshold {
		// Token 即将过期，尝试刷新
		fmt.Printf("Token expires in %v, attempting to refresh...\n", timeUntilExpiry)

		tokenResp, err := c.RefreshLongLivedToken()
		if err != nil {
			// 刷新失败，但如果 token 还没完全过期，仍可使用
			if timeUntilExpiry > 0 {
				fmt.Printf("Token refresh failed but token is still valid for %v: %v\n", timeUntilExpiry, err)
				return nil
			}
			return fmt.Errorf("token expired and refresh failed: %w", err)
		}

		// 保存新的 token 到数据库
		err = c.SaveTokenToDao(ctx, tokenResp)
		if err != nil {
			return fmt.Errorf("failed to save refreshed token: %w", err)
		}

		fmt.Printf("Token successfully refreshed, new expiry: %v\n", tokenResp.GetTokenExpirationTime())
	}

	return nil
}

// SaveTokenToDao saves the updated access token to database
func (c *ThreadsClient) SaveTokenToDao(ctx context.Context, tokenResp *TokenResponse) error {
	if c.configDao == nil {
		return fmt.Errorf("config dao is not set")
	}

	var expiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		expTime := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		expiresAt = &expTime
	}

	err := c.configDao.SaveAccessToken(ctx, "threads", tokenResp.AccessToken, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to save access token to dao: %w", err)
	}

	// Update client's access token
	c.AccessToken = tokenResp.AccessToken

	return nil
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

// PostRequest represents a post creation request
type PostRequest struct {
	MediaType      string   `json:"media_type"`                 // TEXT, IMAGE, VIDEO, CAROUSEL
	Text           string   `json:"text,omitempty"`             // Post text content
	ImageURL       string   `json:"image_url,omitempty"`        // For images
	VideoURL       string   `json:"video_url,omitempty"`        // For videos
	LinkAttachment string   `json:"link_attachment,omitempty"`  // For text posts only
	IsCarouselItem bool     `json:"is_carousel_item,omitempty"` // For carousel items
	Children       []string `json:"children,omitempty"`         // For carousel containers
}

// MediaContainerResponse represents the response when creating a media container
type MediaContainerResponse struct {
	ID string `json:"id"`
}

// PublishResponse represents the response when publishing a post
type PublishResponse struct {
	ID string `json:"id"`
}

// CreateMediaContainer creates a media container for a post
// This is step 1 of the posting process
func (c *ThreadsClient) CreateMediaContainer(ctx context.Context, userID string, req *PostRequest) (*MediaContainerResponse, error) {
	// 确保 token 有效（自动刷新如果需要）
	if c.configDao != nil {
		if err := c.EnsureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("failed to ensure valid token: %w", err)
		}
	}

	if c.AccessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}

	// 构建请求URL
	baseURL := fmt.Sprintf("https://graph.threads.net/v1.0/%s/threads", userID)

	// 构建请求参数
	params := url.Values{}
	params.Add("access_token", c.AccessToken)
	params.Add("media_type", req.MediaType)

	if req.Text != "" {
		params.Add("text", req.Text)
	}

	if req.ImageURL != "" {
		params.Add("image_url", req.ImageURL)
	}

	if req.VideoURL != "" {
		params.Add("video_url", req.VideoURL)
	}

	if req.LinkAttachment != "" {
		params.Add("link_attachment", req.LinkAttachment)
	}

	if req.IsCarouselItem {
		params.Add("is_carousel_item", "true")
	}

	if len(req.Children) > 0 {
		// For carousel containers
		children := ""
		for i, child := range req.Children {
			if i > 0 {
				children += ","
			}
			children += child
		}
		params.Add("children", children)
	}

	// 发送POST请求
	resp, err := http.PostForm(baseURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create media container: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create media container failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var containerResp MediaContainerResponse
	if err := json.Unmarshal(body, &containerResp); err != nil {
		return nil, fmt.Errorf("failed to parse container response: %w", err)
	}

	return &containerResp, nil
}

// PublishMediaContainer publishes a media container
// This is step 2 of the posting process
func (c *ThreadsClient) PublishMediaContainer(ctx context.Context, userID, containerID string) (*PublishResponse, error) {
	// 确保 token 有效（自动刷新如果需要）
	if c.configDao != nil {
		if err := c.EnsureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("failed to ensure valid token: %w", err)
		}
	}

	if c.AccessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}

	// 构建请求URL
	baseURL := fmt.Sprintf("https://graph.threads.net/v1.0/%s/threads_publish", userID)

	// 构建请求参数
	params := url.Values{}
	params.Add("access_token", c.AccessToken)
	params.Add("creation_id", containerID)

	// 发送POST请求
	resp, err := http.PostForm(baseURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to publish media container: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("publish media container failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var publishResp PublishResponse
	if err := json.Unmarshal(body, &publishResp); err != nil {
		return nil, fmt.Errorf("failed to parse publish response: %w", err)
	}

	return &publishResp, nil
}

// PostText creates and publishes a text-only post
func (c *ThreadsClient) PostText(ctx context.Context, userID, text string, linkAttachment ...string) (*PublishResponse, error) {
	req := &PostRequest{
		MediaType: "TEXT",
		Text:      text,
	}

	if len(linkAttachment) > 0 && linkAttachment[0] != "" {
		req.LinkAttachment = linkAttachment[0]
	}

	// Step 1: Create media container
	container, err := c.CreateMediaContainer(ctx, userID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create text post container: %w", err)
	}

	// Step 2: Publish the container (recommended to wait 30 seconds, but we'll proceed immediately for now)
	return c.PublishMediaContainer(ctx, userID, container.ID)
}

// PostImage creates and publishes an image post
func (c *ThreadsClient) PostImage(ctx context.Context, userID, imageURL, text string) (*PublishResponse, error) {
	req := &PostRequest{
		MediaType: "IMAGE",
		ImageURL:  imageURL,
		Text:      text,
	}

	// Step 1: Create media container
	container, err := c.CreateMediaContainer(ctx, userID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create image post container: %w", err)
	}

	// Step 2: Publish the container
	return c.PublishMediaContainer(ctx, userID, container.ID)
}

// PostVideo creates and publishes a video post
func (c *ThreadsClient) PostVideo(ctx context.Context, userID, videoURL, text string) (*PublishResponse, error) {
	req := &PostRequest{
		MediaType: "VIDEO",
		VideoURL:  videoURL,
		Text:      text,
	}

	// Step 1: Create media container
	container, err := c.CreateMediaContainer(ctx, userID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create video post container: %w", err)
	}

	// Step 2: Publish the container
	return c.PublishMediaContainer(ctx, userID, container.ID)
}

// CarouselItem represents an item in a carousel
type CarouselItem struct {
	MediaType string `json:"media_type"` // IMAGE or VIDEO
	ImageURL  string `json:"image_url,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
}

// PostCarousel creates and publishes a carousel post
func (c *ThreadsClient) PostCarousel(ctx context.Context, userID string, items []CarouselItem, text string) (*PublishResponse, error) {
	if len(items) < 2 || len(items) > 20 {
		return nil, fmt.Errorf("carousel must have between 2 and 20 items, got %d", len(items))
	}

	var itemContainerIDs []string

	// Step 1: Create item containers for each carousel item
	for _, item := range items {
		req := &PostRequest{
			MediaType:      item.MediaType,
			ImageURL:       item.ImageURL,
			VideoURL:       item.VideoURL,
			IsCarouselItem: true,
		}

		container, err := c.CreateMediaContainer(ctx, userID, req)
		if err != nil {
			return nil, fmt.Errorf("failed to create carousel item container: %w", err)
		}

		itemContainerIDs = append(itemContainerIDs, container.ID)
	}

	// Step 2: Create carousel container
	carouselReq := &PostRequest{
		MediaType: "CAROUSEL",
		Text:      text,
		Children:  itemContainerIDs,
	}

	carouselContainer, err := c.CreateMediaContainer(ctx, userID, carouselReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create carousel container: %w", err)
	}

	// Step 3: Publish the carousel
	return c.PublishMediaContainer(ctx, userID, carouselContainer.ID)
}

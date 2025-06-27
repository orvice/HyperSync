package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"butterfly.orx.me/core/log"
)

// ThreadsConfig represents Threads configuration

type ThreadsClient struct {
	name         string
	ClientID     string
	ClientSecret string
	AccessToken  string
	UserID       int64
	tokenManager TokenManager
}

// SetTokenManager sets the token manager for the client (useful for testing)
func (c *ThreadsClient) SetTokenManager(manager TokenManager) {
	c.tokenManager = manager
}

// TokenResponse 表示 Threads API token 响应
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"` // 秒数，直到令牌过期
}

// NewThreadsClientWithDao creates a new ThreadsClient with dao support
// 当 dao 里不存在 access token 时，将传入的 accessToken 写入 dao
// 当 dao 里有 access token 时，使用 dao 里的，方便第一次初始化
func NewThreadsClientWithDao(name string,
	clientID, clientSecret, accessToken string, userID int64, tokenManager TokenManager) (*ThreadsClient, error) {

	logger := log.FromContext(context.Background())

	logger.Info("NewThreadsClientWithDao",
		"name", name, "clientID", clientID, "userID", userID)

	client := &ThreadsClient{
		name:         name,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		UserID:       userID,
		tokenManager: tokenManager,
	}

	ctx := context.Background()

	// 尝试从 dao 加载 access token
	existingToken, err := tokenManager.GetAccessToken(ctx, client.name)
	if err != nil {
		logger.Error("failed to get access token from dao", "error", err)
		return nil, fmt.Errorf("failed to get access token from dao: %w", err)
	}

	if existingToken == "" {
		// dao 中没有 access token，使用传入的 accessToken 并保存到 dao
		if accessToken == "" {
			return nil, fmt.Errorf("no access token found in dao and no access token provided")
		}

		// 保存到 dao（不设置过期时间，因为这是初始化时的 token）
		err = tokenManager.SaveAccessToken(ctx, client.name, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to save access token to dao: %w", err)
		}

		logger.Info("saved access token to dao,success", "name", client.name)

		client.AccessToken = accessToken
	} else {
		// dao 中有 access token，使用 dao 中的
		client.AccessToken = existingToken
	}

	return client, nil
}

// EnsureValidToken 确保 token 有效，如果快过期则自动刷新
func (c *ThreadsClient) EnsureValidToken(ctx context.Context) error {
	logger := log.FromContext(ctx).With("method", "ThreadsClient.EnsureValidToken")

	if c.tokenManager == nil {
		logger.Error("token manager is not set")
		return fmt.Errorf("token manager is not set")
	}

	logger.Debug("checking token validity", "client", c.name)

	// 获取 token 信息（包含过期时间）
	tokenInfo, err := c.tokenManager.GetTokenInfo(ctx, c.name)
	if err != nil {
		logger.Error("failed to get token info from dao", "error", err)
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if tokenInfo == nil || tokenInfo.AccessToken == "" {
		logger.Error("access token not found in database")
		return fmt.Errorf("access token not found in database")
	}

	// 更新客户端的 access token
	c.AccessToken = tokenInfo.AccessToken
	logger.Debug("loaded access token from dao", "client", c.name)

	// 如果没有过期时间信息，强制刷新token以获取正确的过期时间
	if tokenInfo.ExpiresAt == nil {
		logger.Info("no expiration time found for token, forcing refresh to obtain expiry information", "client", c.name)

		tokenResp, err := c.RefreshLongLivedToken()
		if err != nil {
			logger.Error("forced token refresh failed", "client", c.name, "error", err)
			return fmt.Errorf("forced token refresh failed: %w", err)
		}

		// 保存新的 token 到数据库
		err = c.SaveTokenToDao(ctx, tokenResp)
		if err != nil {
			logger.Error("failed to save force-refreshed token", "client", c.name, "error", err)
			return fmt.Errorf("failed to save force-refreshed token: %w", err)
		}

		logger.Info("token successfully force-refreshed with expiry information",
			"client", c.name,
			"new_expiry", tokenResp.GetTokenExpirationTime().Format(time.RFC3339))

		return nil
	}

	// 检查 token 是否在 7 天内过期
	const refreshThreshold = 7 * 24 * time.Hour
	timeUntilExpiry := time.Until(*tokenInfo.ExpiresAt)

	if timeUntilExpiry <= refreshThreshold {
		// Token 即将过期，尝试刷新
		logger.Info("token expires soon, attempting to refresh",
			"client", c.name,
			"time_until_expiry", timeUntilExpiry,
			"expires_at", tokenInfo.ExpiresAt.Format(time.RFC3339))

		tokenResp, err := c.RefreshLongLivedToken()
		if err != nil {
			// 刷新失败，但如果 token 还没完全过期，仍可使用
			if timeUntilExpiry > 0 {
				logger.Warn("token refresh failed but token is still valid",
					"client", c.name,
					"time_until_expiry", timeUntilExpiry,
					"error", err)
				return nil
			}
			logger.Error("token expired and refresh failed", "client", c.name, "error", err)
			return fmt.Errorf("token expired and refresh failed: %w", err)
		}

		// 保存新的 token 到数据库
		err = c.SaveTokenToDao(ctx, tokenResp)
		if err != nil {
			logger.Error("failed to save refreshed token", "client", c.name, "error", err)
			return fmt.Errorf("failed to save refreshed token: %w", err)
		}

		logger.Info("token successfully refreshed",
			"client", c.name,
			"new_expiry", tokenResp.GetTokenExpirationTime().Format(time.RFC3339))
	} else {
		logger.Debug("token is still valid",
			"client", c.name,
			"time_until_expiry", timeUntilExpiry,
			"expires_at", tokenInfo.ExpiresAt.Format(time.RFC3339))
	}

	return nil
}

// SaveTokenToDao saves the updated access token to database
func (c *ThreadsClient) SaveTokenToDao(ctx context.Context, tokenResp *TokenResponse) error {
	logger := log.FromContext(ctx)

	if c.tokenManager == nil {
		logger.Error("token manager is not set")
		return fmt.Errorf("token manager is not set")
	}

	var expiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		expTime := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		expiresAt = &expTime
	}

	logger.Debug("saving access token to dao",
		"client", c.name,
		"expires_at", func() string {
			if expiresAt != nil {
				return expiresAt.Format(time.RFC3339)
			}
			return "never"
		}())

	err := c.tokenManager.SaveAccessToken(ctx, c.name, tokenResp.AccessToken, expiresAt)
	if err != nil {
		logger.Error("failed to save access token to dao", "client", c.name, "error", err)
		return fmt.Errorf("failed to save access token to dao: %w", err)
	}

	// Update client's access token
	c.AccessToken = tokenResp.AccessToken

	logger.Info("access token saved successfully", "client", c.name)

	return nil
}

// ExchangeForLongLivedToken 将短期访问令牌交换为长期访问令牌
// 长期令牌有效期为60天，可以刷新
func (c *ThreadsClient) ExchangeForLongLivedToken(shortLivedToken string) (*TokenResponse, error) {
	logger := log.FromContext(context.Background())

	if c.ClientSecret == "" {
		logger.Error("client secret is required for token exchange", "client", c.name)
		return nil, fmt.Errorf("client secret is required for token exchange")
	}

	logger.Info("exchanging short-lived token for long-lived token", "client", c.name)

	// 构建请求URL
	baseURL := "https://graph.threads.net/v1.0/access_token"
	params := url.Values{}
	params.Add("grant_type", "th_exchange_token")
	params.Add("client_secret", c.ClientSecret)
	params.Add("access_token", shortLivedToken)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 发送GET请求
	resp, err := http.Get(requestURL)
	if err != nil {
		logger.Error("failed to send token exchange request", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read token exchange response body", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Error("token exchange failed",
			"client", c.name,
			"status_code", resp.StatusCode,
			"response", string(body))
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		logger.Error("failed to parse token exchange response", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// 更新客户端的访问令牌
	c.AccessToken = tokenResp.AccessToken

	logger.Info("successfully exchanged token",
		"client", c.name,
		"expires_in_seconds", tokenResp.ExpiresIn)

	return &tokenResp, nil
}

// RefreshLongLivedToken 刷新长期访问令牌
// 长期令牌必须至少24小时旧但尚未过期才能刷新
// 刷新后的令牌有效期为60天
func (c *ThreadsClient) RefreshLongLivedToken() (*TokenResponse, error) {
	logger := log.FromContext(context.Background())

	if c.AccessToken == "" {
		logger.Error("access token is required for token refresh", "client", c.name)
		return nil, fmt.Errorf("access token is required for token refresh")
	}

	logger.Info("refreshing long-lived token", "client", c.name)

	// 构建请求URL
	baseURL := "https://graph.threads.net/v1.0/refresh_access_token"
	params := url.Values{}
	params.Add("grant_type", "th_refresh_token")
	params.Add("access_token", c.AccessToken)

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 发送GET请求
	resp, err := http.Get(requestURL)
	if err != nil {
		logger.Error("failed to send token refresh request", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read token refresh response body", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Error("token refresh failed",
			"client", c.name,
			"status_code", resp.StatusCode,
			"response", string(body))
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		logger.Error("failed to parse token refresh response", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// 更新客户端的访问令牌
	c.AccessToken = tokenResp.AccessToken

	logger.Info("successfully refreshed token",
		"client", c.name,
		"expires_in_seconds", tokenResp.ExpiresIn)

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
	logger := log.FromContext(ctx)

	// 确保 token 有效（自动刷新如果需要）
	if c.tokenManager != nil {
		if err := c.EnsureValidToken(ctx); err != nil {
			logger.Error("failed to ensure valid token", "client", c.name, "error", err)
			return nil, fmt.Errorf("failed to ensure valid token: %w", err)
		}
	}

	if c.AccessToken == "" {
		logger.Error("access token is required", "client", c.name)
		return nil, fmt.Errorf("access token is required")
	}

	logger.Info("creating media container",
		"client", c.name,
		"user_id", userID,
		"media_type", req.MediaType,
		"has_text", req.Text != "",
		"has_image", req.ImageURL != "",
		"has_video", req.VideoURL != "",
		"is_carousel_item", req.IsCarouselItem)

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
		logger.Debug("carousel container with children",
			"client", c.name,
			"children_count", len(req.Children))
	}

	// 发送POST请求
	resp, err := http.PostForm(baseURL, params)
	if err != nil {
		logger.Error("failed to send create media container request", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to create media container: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read create media container response", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Error("create media container failed",
			"client", c.name,
			"status_code", resp.StatusCode,
			"response", string(body))
		return nil, fmt.Errorf("create media container failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var containerResp MediaContainerResponse
	if err := json.Unmarshal(body, &containerResp); err != nil {
		logger.Error("failed to parse media container response", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to parse container response: %w", err)
	}

	logger.Info("media container created successfully",
		"client", c.name,
		"container_id", containerResp.ID)

	return &containerResp, nil
}

// PublishMediaContainer publishes a media container
// This is step 2 of the posting process
func (c *ThreadsClient) PublishMediaContainer(ctx context.Context, userID, containerID string) (*PublishResponse, error) {
	logger := log.FromContext(ctx)

	// 确保 token 有效（自动刷新如果需要）
	if c.tokenManager != nil {
		if err := c.EnsureValidToken(ctx); err != nil {
			logger.Error("failed to ensure valid token", "client", c.name, "error", err)
			return nil, fmt.Errorf("failed to ensure valid token: %w", err)
		}
	}

	if c.AccessToken == "" {
		logger.Error("access token is required", "client", c.name)
		return nil, fmt.Errorf("access token is required")
	}

	logger.Info("publishing media container",
		"client", c.name,
		"user_id", userID,
		"container_id", containerID)

	// 构建请求URL
	baseURL := fmt.Sprintf("https://graph.threads.net/v1.0/%s/threads_publish", userID)

	// 构建请求参数
	params := url.Values{}
	params.Add("access_token", c.AccessToken)
	params.Add("creation_id", containerID)

	// 发送POST请求
	resp, err := http.PostForm(baseURL, params)
	if err != nil {
		logger.Error("failed to send publish media container request", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to publish media container: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read publish response", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Error("publish media container failed",
			"client", c.name,
			"status_code", resp.StatusCode,
			"response", string(body))
		return nil, fmt.Errorf("publish media container failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析JSON响应
	var publishResp PublishResponse
	if err := json.Unmarshal(body, &publishResp); err != nil {
		logger.Error("failed to parse publish response", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to parse publish response: %w", err)
	}

	logger.Info("media container published successfully",
		"client", c.name,
		"post_id", publishResp.ID)

	return &publishResp, nil
}

// PostText creates and publishes a text-only post
func (c *ThreadsClient) PostText(ctx context.Context, userID, text string, linkAttachment ...string) (*PublishResponse, error) {
	logger := log.FromContext(ctx)

	logger.Debug("starting text post",
		"client", c.name,
		"user_id", userID,
		"text_length", len(text),
		"has_link", len(linkAttachment) > 0 && linkAttachment[0] != "")

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
		logger.Error("failed to create text post container", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to create text post container: %w", err)
	}

	// Step 2: Publish the container (recommended to wait 30 seconds, but we'll proceed immediately for now)
	return c.PublishMediaContainer(ctx, userID, container.ID)
}

// PostImage creates and publishes an image post
func (c *ThreadsClient) PostImage(ctx context.Context, userID, imageURL, text string) (*PublishResponse, error) {
	logger := log.FromContext(ctx)

	logger.Debug("starting image post",
		"client", c.name,
		"user_id", userID,
		"image_url", imageURL,
		"text_length", len(text))

	req := &PostRequest{
		MediaType: "IMAGE",
		ImageURL:  imageURL,
		Text:      text,
	}

	// Step 1: Create media container
	container, err := c.CreateMediaContainer(ctx, userID, req)
	if err != nil {
		logger.Error("failed to create image post container", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to create image post container: %w", err)
	}

	// Step 2: Publish the container
	return c.PublishMediaContainer(ctx, userID, container.ID)
}

// PostVideo creates and publishes a video post
func (c *ThreadsClient) PostVideo(ctx context.Context, userID, videoURL, text string) (*PublishResponse, error) {
	logger := log.FromContext(ctx)

	logger.Debug("starting video post",
		"client", c.name,
		"user_id", userID,
		"video_url", videoURL,
		"text_length", len(text))

	req := &PostRequest{
		MediaType: "VIDEO",
		VideoURL:  videoURL,
		Text:      text,
	}

	// Step 1: Create media container
	container, err := c.CreateMediaContainer(ctx, userID, req)
	if err != nil {
		logger.Error("failed to create video post container", "client", c.name, "error", err)
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
	logger := log.FromContext(ctx)

	if len(items) < 2 || len(items) > 20 {
		logger.Error("invalid carousel item count",
			"client", c.name,
			"item_count", len(items),
			"min_required", 2,
			"max_allowed", 20)
		return nil, fmt.Errorf("carousel must have between 2 and 20 items, got %d", len(items))
	}

	logger.Debug("starting carousel post",
		"client", c.name,
		"user_id", userID,
		"item_count", len(items),
		"text_length", len(text))

	var itemContainerIDs []string

	// Step 1: Create item containers for each carousel item
	for i, item := range items {
		logger.Debug("creating carousel item container",
			"client", c.name,
			"item_index", i,
			"media_type", item.MediaType)

		req := &PostRequest{
			MediaType:      item.MediaType,
			ImageURL:       item.ImageURL,
			VideoURL:       item.VideoURL,
			IsCarouselItem: true,
		}

		container, err := c.CreateMediaContainer(ctx, userID, req)
		if err != nil {
			logger.Error("failed to create carousel item container",
				"client", c.name,
				"item_index", i,
				"error", err)
			return nil, fmt.Errorf("failed to create carousel item container: %w", err)
		}

		itemContainerIDs = append(itemContainerIDs, container.ID)
	}

	logger.Debug("all carousel item containers created, creating main carousel container",
		"client", c.name,
		"item_container_count", len(itemContainerIDs))

	// Step 2: Create carousel container
	carouselReq := &PostRequest{
		MediaType: "CAROUSEL",
		Text:      text,
		Children:  itemContainerIDs,
	}

	carouselContainer, err := c.CreateMediaContainer(ctx, userID, carouselReq)
	if err != nil {
		logger.Error("failed to create carousel container", "client", c.name, "error", err)
		return nil, fmt.Errorf("failed to create carousel container: %w", err)
	}

	// Step 3: Publish the carousel
	return c.PublishMediaContainer(ctx, userID, carouselContainer.ID)
}

// =============================================================================
// SocialClient Interface Implementation
// =============================================================================

// Name returns the name of this social client (implements SocialClient interface)
func (c *ThreadsClient) Name() string {
	return c.name
}

// Post implements the SocialClient interface for posting content
func (c *ThreadsClient) Post(ctx context.Context, post *Post) (interface{}, error) {
	logger := log.FromContext(ctx)
	userID := strconv.FormatInt(c.UserID, 10)

	// Validate visibility for Threads
	if post.Visibility != "" {
		if err := ValidateVisibility("threads", post.Visibility); err != nil {
			return nil, fmt.Errorf("invalid visibility for Threads: %w", err)
		}
	}

	// Determine post type based on media content
	mediaCount := len(post.Media)

	logger.Info("posting to threads",
		"client", c.name,
		"user_id", userID,
		"media_count", mediaCount,
		"content_length", len(post.Content))

	switch {
	case mediaCount == 0:
		// Text-only post
		logger.Debug("posting text-only content", "client", c.name)
		result, err := c.PostText(ctx, userID, post.Content)
		if err != nil {
			logger.Error("failed to post text content", "client", c.name, "error", err)
			return nil, err
		}
		logger.Info("text post successful", "client", c.name, "post_id", result.ID)
		return result, nil

	case mediaCount == 1:
		// Single media post
		media := post.Media[0]
		mediaURL := media.GetURL()

		if mediaURL == "" {
			logger.Error("media URL is required for Threads posting", "client", c.name)
			return nil, fmt.Errorf("media URL is required for Threads posting")
		}

		logger.Debug("posting single media content",
			"client", c.name,
			"media_url", mediaURL)

		// For now, assume it's an image. TODO: Add media type detection based on URL or content type
		result, err := c.PostImage(ctx, userID, mediaURL, post.Content)
		if err != nil {
			logger.Error("failed to post image content", "client", c.name, "error", err)
			return nil, err
		}
		logger.Info("image post successful", "client", c.name, "post_id", result.ID)
		return result, nil

	case mediaCount > 1:
		// Carousel post
		if mediaCount > 20 {
			logger.Error("too many media items for carousel",
				"client", c.name,
				"media_count", mediaCount,
				"max_allowed", 20)
			return nil, fmt.Errorf("too many media items for carousel: %d (max 20)", mediaCount)
		}

		logger.Debug("posting carousel content",
			"client", c.name,
			"media_count", mediaCount)

		var carouselItems []CarouselItem
		for i, media := range post.Media {
			mediaURL := media.GetURL()
			if mediaURL == "" {
				logger.Error("media URL is required for carousel items",
					"client", c.name,
					"item_index", i)
				return nil, fmt.Errorf("media URL is required for carousel items")
			}

			// For now, assume all are images. TODO: Add proper media type detection
			carouselItems = append(carouselItems, CarouselItem{
				MediaType: "IMAGE",
				ImageURL:  mediaURL,
			})
		}

		result, err := c.PostCarousel(ctx, userID, carouselItems, post.Content)
		if err != nil {
			logger.Error("failed to post carousel content", "client", c.name, "error", err)
			return nil, err
		}
		logger.Info("carousel post successful", "client", c.name, "post_id", result.ID)
		return result, nil

	default:
		logger.Error("invalid media count", "client", c.name, "media_count", mediaCount)
		return nil, fmt.Errorf("invalid media count: %d", mediaCount)
	}
}

// ListPosts implements the SocialClient interface for retrieving posts
func (c *ThreadsClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	logger := log.FromContext(ctx)

	// TODO: Implement when Threads API provides user posts endpoint
	// Currently, Threads API doesn't have a public endpoint to list user's own posts
	// This would require the user's posts endpoint once it becomes available

	logger.Warn("ListPosts is not yet implemented for Threads",
		"client", c.name,
		"reason", "API endpoint not available")

	return nil, fmt.Errorf("ListPosts is not yet implemented for Threads - API endpoint not available")
}

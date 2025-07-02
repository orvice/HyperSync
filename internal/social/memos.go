package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"butterfly.orx.me/core/log"
)

// Memos API endpoints constants
const (
	MemosAPIVersion   = "/api/v1"
	MemosEndpointList = MemosAPIVersion + "/memos"
	MemosEndpointGet  = MemosAPIVersion + "/memos/%s"    // 需要传入memo ID
	MemosEndpointPost = MemosAPIVersion + "/memos"       // 创建memo
	MemosEndpointPut  = MemosAPIVersion + "/memos/%s"    // 更新memo，需要传入memo ID
	MemosEndpointDel  = MemosAPIVersion + "/memos/%s"    // 删除memo，需要传入memo ID
	MemosEndpointUser = MemosAPIVersion + "/users/me"    // 获取当前用户信息
	MemosEndpointAuth = MemosAPIVersion + "/auth/signin" // 认证登录
)

type Memos struct {
	name     string
	Endpoint string
	Token    string
}

func NewMemos(endpoint, token, name string) *Memos {
	endpoint = strings.TrimSuffix(endpoint, "/")
	return &Memos{
		Endpoint: endpoint,
		Token:    token,
		name:     name,
	}
}

// ListMemosRequest 定义获取备忘录列表的请求参数
type ListMemosRequest struct {
	PageSize      int    `json:"pageSize,omitempty"`      // 页面大小
	PageToken     string `json:"pageToken,omitempty"`     // 分页令牌
	Filter        string `json:"filter,omitempty"`        // 过滤条件
	Creator       string `json:"creator,omitempty"`       // 创建者
	Visibility    string `json:"visibility,omitempty"`    // 可见性：PUBLIC, PROTECTED, PRIVATE
	OrderBy       string `json:"orderBy,omitempty"`       // 排序方式：display_time desc, display_time asc
	Tag           string `json:"tag,omitempty"`           // 标签过滤
	ContentSearch string `json:"contentSearch,omitempty"` // 内容搜索
}

// Resource 定义资源结构
type Resource struct {
	Name         string `json:"name"`
	CreateTime   string `json:"createTime"`
	Filename     string `json:"filename"`
	Content      string `json:"content"`
	ExternalLink string `json:"externalLink"`
	Type         string `json:"type"`
	Size         string `json:"size"`
	Memo         string `json:"memo"`
}

// Memo 定义备忘录结构
type Memo struct {
	Name        string     `json:"name"`
	UID         string     `json:"uid"`
	RowStatus   string     `json:"rowStatus"`
	Creator     string     `json:"creator"`
	CreateTime  time.Time  `json:"createTime"`
	UpdateTime  time.Time  `json:"updateTime"`
	DisplayTime time.Time  `json:"displayTime"`
	Content     string     `json:"content"`
	Visibility  string     `json:"visibility"`
	Pinned      bool       `json:"pinned"`
	Resources   []Resource `json:"resources,omitempty"`
	Relations   []string   `json:"relations,omitempty"`
	Reactions   []string   `json:"reactions,omitempty"`
}

// ListMemosResponse 定义备忘录列表响应
type ListMemosResponse struct {
	Memos         []Memo `json:"memos"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// makeRequest 封装HTTP请求，为调用Memos API做统一处理
func (m *Memos) makeRequest(ctx context.Context, method, path string, params url.Values, body interface{}) ([]byte, error) {
	logger := log.FromContext(ctx)

	// 构建完整URL
	apiURL := fmt.Sprintf("%s%s", m.Endpoint, path)
	if len(params) > 0 {
		apiURL += "?" + params.Encode()
	}

	logger.Debug("making request to Memos API",
		"client", m.name,
		"method", method,
		"path", path,
		"url", apiURL)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			logger.Error("failed to marshal request body",
				"client", m.name,
				"method", method,
				"path", path,
				"error", err)
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
		logger.Debug("request body prepared",
			"client", m.name,
			"body_size", len(jsonData))
	}

	// 创建HTTP请求，使用传入的 context
	httpReq, err := http.NewRequestWithContext(ctx, method, apiURL, reqBody)
	if err != nil {
		logger.Error("failed to create request",
			"client", m.name,
			"method", method,
			"path", path,
			"error", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	if m.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.Token)
		logger.Debug("authorization header added", "client", m.name)
	}

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	logger.Debug("sending HTTP request", "client", m.name)
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Error("failed to send request",
			"client", m.name,
			"method", method,
			"path", path,
			"error", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body",
			"client", m.name,
			"method", method,
			"path", path,
			"status_code", resp.StatusCode,
			"error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Debug("received response",
		"client", m.name,
		"method", method,
		"path", path,
		"status_code", resp.StatusCode,
		"response_size", len(responseBody))

	// 检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.Error("API request failed",
			"client", m.name,
			"method", method,
			"path", path,
			"status_code", resp.StatusCode,
			"response_body", string(responseBody))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	logger.Info("API request completed successfully",
		"client", m.name,
		"method", method,
		"path", path,
		"status_code", resp.StatusCode)

	return responseBody, nil
}

// makeRequestAndParse 封装HTTP请求并解析响应到指定结构体
func (m *Memos) makeRequestAndParse(ctx context.Context, method, path string, params url.Values, requestBody interface{}, responseBody interface{}) error {
	logger := log.FromContext(ctx)

	logger.Debug("making request with response parsing",
		"client", m.name,
		"method", method,
		"path", path)

	respData, err := m.makeRequest(ctx, method, path, params, requestBody)
	if err != nil {
		logger.Error("request failed",
			"client", m.name,
			"method", method,
			"path", path,
			"error", err)
		return err
	}

	if responseBody != nil {
		logger.Debug("parsing response JSON",
			"client", m.name,
			"response_size", len(respData))

		if err := json.Unmarshal(respData, responseBody); err != nil {
			logger.Error("failed to parse response JSON",
				"client", m.name,
				"method", method,
				"path", path,
				"response_size", len(respData),
				"error", err)
			return fmt.Errorf("failed to parse response: %w", err)
		}

		logger.Debug("response parsed successfully",
			"client", m.name)
	}

	return nil
}

// ListMemos 获取备忘录列表
func (m *Memos) ListMemos(ctx context.Context, req *ListMemosRequest) (*ListMemosResponse, error) {
	// 构建查询参数
	params := url.Values{}
	if req != nil {
		if req.PageSize > 0 {
			params.Set("pageSize", strconv.Itoa(req.PageSize))
		}
		if req.PageToken != "" {
			params.Set("pageToken", req.PageToken)
		}
		if req.Filter != "" {
			params.Set("filter", req.Filter)
		}
		if req.Creator != "" {
			params.Set("creator", req.Creator)
		}
		if req.Visibility != "" {
			params.Set("visibility", req.Visibility)
		}
		if req.OrderBy != "" {
			params.Set("orderBy", req.OrderBy)
		} else {
			// 设置默认排序为 display_time desc
			params.Set("orderBy", "display_time desc")
		}
		if req.Tag != "" {
			params.Set("tag", req.Tag)
		}
		if req.ContentSearch != "" {
			params.Set("contentSearch", req.ContentSearch)
		}
	} else {
		// 如果 req 为 nil，设置默认排序
		params.Set("orderBy", "display_time desc")
	}

	// 使用封装的请求方法
	var response ListMemosResponse
	err := m.makeRequestAndParse(ctx, "GET", MemosEndpointList, params, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (m *Memos) Name() string {
	return m.name
}

// Post implements SocialClient interface - posts content to Memos
func (m *Memos) Post(ctx context.Context, post *Post) (interface{}, error) {
	// Check if visibility level is supported for Memos
	if post.Visibility.IsValid() {
		if !IsVisibilityLevelSupported(PlatformMemos.String(), post.Visibility) {
			// Skip posting if visibility level is not supported
			return nil, nil
		}
		// Convert enum to Memos-specific visibility value
		platformVisibility := GetPlatformVisibilityString(PlatformMemos.String(), post.Visibility)
		// TODO: Use platformVisibility when implementing actual posting logic
		_ = platformVisibility
	}

	// TODO: Implement Memos posting logic
	return nil, fmt.Errorf("Memos Post method not implemented yet")
}

// ListPosts implements SocialClient interface - converts Memos to social Posts
func (m *Memos) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	req := &ListMemosRequest{
		PageSize: limit,
		OrderBy:  "display_time desc",
	}

	resp, err := m.ListMemos(ctx, req)
	if err != nil {
		return nil, err
	}

	var posts []*Post
	for _, memo := range resp.Memos {
		var medias = make([]Media, 0)
		if memo.Resources != nil {
			for _, resource := range memo.Resources {
				// 根据资源类型创建不同的 Media 对象
				if resource.ExternalLink != "" {
					// 如果有外部链接，使用外部链接创建 Media
					media := NewMediaFromURL(resource.ExternalLink)
					media.Description = resource.Filename
					medias = append(medias, *media)
				} else if resource.Content != "" {
					// 如果有内容数据，使用内容创建 Media
					// 注意：这里假设 Content 是 base64 编码的数据或直接的字节数据
					// 实际情况可能需要根据 Memos API 的具体实现进行调整
					media := NewMedia([]byte(resource.Content))
					media.Description = resource.Filename
					medias = append(medias, *media)
				} else if resource.Name != "" {
					// 如果有资源名称，构建资源 URL
					// 假设资源可以通过 Memos 的 API 端点访问
					resourceURL := fmt.Sprintf("%s/file/%s/%s", m.Endpoint, resource.Name, resource.Filename)
					media := NewMediaFromURL(resourceURL)
					media.Description = resource.Filename
					medias = append(medias, *media)
				}
			}
		}

		// Convert string visibility to enum
		visibility, err := ParsePlatformVisibility(PlatformMemos.String(), memo.Visibility)
		if err != nil {
			// Use default visibility if parsing fails
			visibility = VisibilityLevelPublic
		}

		post := &Post{
			ID:             memo.Name,
			Content:        memo.Content,
			Visibility:     visibility,
			Media:          medias,
			SourcePlatform: m.name,
			OriginalID:     memo.UID,
			CreatedAt:      memo.CreateTime,
		}
		posts = append(posts, post)
	}

	return posts, nil
}

// GetMemo 获取单个备忘录
func (m *Memos) GetMemo(ctx context.Context, memoID string) (*Memo, error) {
	endpoint := fmt.Sprintf(MemosEndpointGet, memoID)

	var memo Memo
	err := m.makeRequestAndParse(ctx, "GET", endpoint, nil, nil, &memo)
	if err != nil {
		return nil, err
	}

	return &memo, nil
}

// CreateMemoRequest 定义创建备忘录的请求结构
type CreateMemoRequest struct {
	Content    string `json:"content"`
	Visibility string `json:"visibility,omitempty"`
}

// CreateMemo 创建新的备忘录
func (m *Memos) CreateMemo(ctx context.Context, req *CreateMemoRequest) (*Memo, error) {
	var memo Memo
	err := m.makeRequestAndParse(ctx, "POST", MemosEndpointPost, nil, req, &memo)
	if err != nil {
		return nil, err
	}

	return &memo, nil
}

// UpdateMemoRequest 定义更新备忘录的请求结构
type UpdateMemoRequest struct {
	Content    string `json:"content,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// UpdateMemo 更新备忘录
func (m *Memos) UpdateMemo(ctx context.Context, memoID string, req *UpdateMemoRequest) (*Memo, error) {
	endpoint := fmt.Sprintf(MemosEndpointPut, memoID)

	var memo Memo
	err := m.makeRequestAndParse(ctx, "PUT", endpoint, nil, req, &memo)
	if err != nil {
		return nil, err
	}

	return &memo, nil
}

// DeleteMemo 删除备忘录
func (m *Memos) DeleteMemo(ctx context.Context, memoID string) error {
	endpoint := fmt.Sprintf(MemosEndpointDel, memoID)

	// 删除操作不需要解析响应体
	_, err := m.makeRequest(ctx, "DELETE", endpoint, nil, nil)
	return err
}

// UserInfo 定义用户信息结构
type UserInfo struct {
	Name        string `json:"name"`
	ID          int    `json:"id"`
	Role        string `json:"role"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Nickname    string `json:"nickname"`
	AvatarURL   string `json:"avatarUrl"`
	Description string `json:"description"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
}

// GetCurrentUser 获取当前用户信息
func (m *Memos) GetCurrentUser(ctx context.Context) (*UserInfo, error) {
	var user UserInfo
	err := m.makeRequestAndParse(ctx, "GET", MemosEndpointUser, nil, nil, &user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

package social

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Memos struct {
	Endpoint string
	Token    string
}

func NewMemos(endpoint, token string) *Memos {
	endpoint = strings.TrimSuffix(endpoint, "/")
	return &Memos{
		Endpoint: endpoint,
		Token:    token,
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

// ListMemos 获取备忘录列表
func (m *Memos) ListMemos(req *ListMemosRequest) (*ListMemosResponse, error) {
	// 构建请求URL
	apiURL := fmt.Sprintf("%s/api/v1/memos", m.Endpoint)

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
		}
		if req.Tag != "" {
			params.Set("tag", req.Tag)
		}
		if req.ContentSearch != "" {
			params.Set("contentSearch", req.ContentSearch)
		}
	}

	if len(params) > 0 {
		apiURL += "?" + params.Encode()
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置认证头
	if m.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.Token)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response ListMemosResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

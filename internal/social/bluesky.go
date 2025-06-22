package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"butterfly.orx.me/core/log"
	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
)

// BlueskyClient
type BlueskyClient struct {
	name   string
	Client *xrpc.Client
}

// NewBlueskyClient 创建一个新的Bluesky客户端
func NewBlueskyClient(host, handle, password string, name string) (*BlueskyClient, error) {
	// 创建xrpc客户端
	client := &xrpc.Client{
		Host: host,
	}

	// 登录Bluesky获取认证信息
	ctx := context.Background()
	auth, err := atproto.ServerCreateSession(ctx, client, &atproto.ServerCreateSession_Input{
		Identifier: handle,
		Password:   password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Bluesky session: %w", err)
	}

	// 设置认证信息
	client.Auth = &xrpc.AuthInfo{
		AccessJwt:  auth.AccessJwt,
		RefreshJwt: auth.RefreshJwt,
		Handle:     auth.Handle,
		Did:        auth.Did,
	}

	return &BlueskyClient{
		name:   name,
		Client: client,
	}, nil
}

func (c *BlueskyClient) Name() string {
	return c.name
}

// NewBlueskyClientFromEnv 从环境变量创建一个新的Bluesky客户端
func NewBlueskyClientFromEnv() (*BlueskyClient, error) {
	host := os.Getenv("BLUESKY_HOST")
	if host == "" {
		host = "https://bsky.social" // 默认API服务器
	}

	handle := os.Getenv("BLUESKY_HANDLE")
	if handle == "" {
		return nil, fmt.Errorf("BLUESKY_HANDLE environment variable is not set")
	}

	password := os.Getenv("BLUESKY_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("BLUESKY_PASSWORD environment variable is not set")
	}

	return NewBlueskyClient(host, handle, password, "bluesky")
}

// Post 发布一条Bluesky帖子
func (b *BlueskyClient) Post(ctx context.Context, post *Post) (interface{}, error) {
	logger := log.FromContext(ctx)
	// 使用HTTP客户端直接发出请求
	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.createRecord", b.Client.Host)

	// 生成一个唯一的rkey，使用当前时间戳
	rkey := fmt.Sprintf("%d", time.Now().UnixNano())

	// 创建基本的记录结构
	record := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      post.Content,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	// 处理媒体附件
	if len(post.Media) > 0 {
		// 上传每个媒体文件并添加到 embed
		var images []interface{}
		for _, media := range post.Media {
			// 获取媒体数据，可能需要从 URL 获取
			mediaData, err := media.GetData()
			if err != nil {
				logger.Error("failed to get media data",
					"error", err)
				return nil, fmt.Errorf("failed to get media data: %w", err)
			}

			// 上传图像到 Bluesky
			resp, err := atproto.RepoUploadBlob(ctx, b.Client, bytes.NewReader(mediaData))
			if err != nil {
				logger.Error("failed to upload media",
					"error", err)
				return nil, fmt.Errorf("failed to upload media: %w", err)
			}

			// 添加图像到集合 - image字段应该直接是blob对象
			images = append(images, map[string]interface{}{
				"alt":   media.Description, // 使用媒体描述作为alt文本
				"image": resp.Blob,         // 直接使用返回的blob对象
			})
		}

		// 设置embed结构
		record["embed"] = map[string]interface{}{
			"$type":  "app.bsky.embed.images",
			"images": images,
		}
	}

	// 创建请求体
	reqBody := map[string]interface{}{
		"repo":       b.Client.Auth.Did,
		"collection": "app.bsky.feed.post",
		"rkey":       rkey,
		"record":     record,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("failed to marshal request body",
			"error", err)
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 添加请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.Client.Auth.AccessJwt)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to send request",
			"error", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("failed with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("failed with status %d: %v", resp.StatusCode, errResp)
	}

	// 解析响应以获取URI
	var respData struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body",
			"error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(s, &respData); err != nil {
		logger.Error("failed to decode response",
			"error", err)
		return rkey, nil // 即使解析响应失败，我们也能返回自己生成的rkey
	}

	logger.Info("success to post to bluesky",
		"response", string(s),
		"post_id", respData.URI)

	return respData, nil
}

// DeletePost 删除一条Bluesky帖子
func (b *BlueskyClient) DeletePost(ctx context.Context, rkey string) error {
	// 使用HTTP客户端直接发出请求
	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.deleteRecord", b.Client.Host)

	// 创建请求体
	reqBody := map[string]interface{}{
		"repo":       b.Client.Auth.Did,
		"collection": "app.bsky.feed.post",
		"rkey":       rkey,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// 添加请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.Client.Auth.AccessJwt)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("failed with status %d", resp.StatusCode)
		}
		return fmt.Errorf("failed with status %d: %v", resp.StatusCode, errResp)
	}

	return nil
}

// toJSONMap 将结构体转换为map[string]interface{}
func toJSONMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return m, nil
}

// ListPosts 获取当前用户的最新帖子
func (b *BlueskyClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	// 设置默认限制
	if limit <= 0 {
		limit = 20
	}

	// 获取用户 DID
	did := b.Client.Auth.Did
	if did == "" {
		return nil, fmt.Errorf("user DID not found in client auth")
	}

	// 获取用户的帖子
	feed, err := b.getAuthorFeed(ctx, did, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get author feed: %w", err)
	}

	// 转换为 Post 类型
	posts := make([]*Post, 0, len(feed.Feed))
	for _, item := range feed.Feed {
		// 检查 post 记录是否存在
		if item.Post == nil || item.Post.Record == nil {
			continue
		}

		// 提取记录 ID
		parts := strings.Split(item.Post.URI, "/")
		rkey := ""
		if len(parts) > 0 {
			rkey = parts[len(parts)-1]
		}

		// 创建 post 对象
		post := &Post{
			ID:         rkey,
			Content:    item.Post.Record.Text,
			Visibility: "public", // Bluesky 目前没有可见性设置，默认为公开
		}

		// 处理媒体附件
		if item.Post.Embed != nil && item.Post.Embed.Images != nil && len(item.Post.Embed.Images) > 0 {
			// 我们没有原始媒体数据，只是记录媒体存在
			post.Media = []Media{}
		}

		posts = append(posts, post)
	}

	return posts, nil
}

// getAuthorFeed 获取作者的帖子流
func (b *BlueskyClient) getAuthorFeed(ctx context.Context, did string, limit int) (*struct {
	Feed []struct {
		Post *struct {
			URI    string `json:"uri"`
			CID    string `json:"cid"`
			Record *struct {
				Text string `json:"text"`
			} `json:"record"`
			Embed *struct {
				Images []interface{} `json:"images"`
			} `json:"embed,omitempty"`
		} `json:"post"`
	} `json:"feed"`
}, error) {
	// 使用 HTTP 客户端直接发出请求
	url := fmt.Sprintf("%s/xrpc/app.bsky.feed.getAuthorFeed", b.Client.Host)

	// 创建请求参数
	params := map[string]interface{}{
		"actor": did,
		"limit": limit,
	}

	// 构建查询字符串
	var queryParts []string
	for k, v := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=%v", k, v))
	}
	fullURL := fmt.Sprintf("%s?%s", url, strings.Join(queryParts, "&"))

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 添加请求头
	req.Header.Set("Authorization", "Bearer "+b.Client.Auth.AccessJwt)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("failed with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("failed with status %d: %v", resp.StatusCode, errResp)
	}

	// 解析响应
	var feed struct {
		Feed []struct {
			Post *struct {
				URI    string `json:"uri"`
				CID    string `json:"cid"`
				Record *struct {
					Text string `json:"text"`
				} `json:"record"`
				Embed *struct {
					Images []interface{} `json:"images"`
				} `json:"embed,omitempty"`
			} `json:"post"`
		} `json:"feed"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &feed, nil
}

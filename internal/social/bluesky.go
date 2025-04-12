package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
)

// BlueskyClient
type BlueskyClient struct {
	Client *xrpc.Client
}

// NewBlueskyClient 创建一个新的Bluesky客户端
func NewBlueskyClient(host, handle, password string) (*BlueskyClient, error) {
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
		Client: client,
	}, nil
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

	return NewBlueskyClient(host, handle, password)
}

// Post 发布一条Bluesky帖子
func (b *BlueskyClient) Post(ctx context.Context, text string) error {
	// 使用HTTP客户端直接发出请求
	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.createRecord", b.Client.Host)

	// 创建请求体
	reqBody := map[string]interface{}{
		"repo":       b.Client.Auth.Did,
		"collection": "app.bsky.feed.post",
		"record": map[string]interface{}{
			"$type":     "app.bsky.feed.post",
			"text":      text,
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		},
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

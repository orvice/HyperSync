package social

import (
	"context"
	"fmt"
	"os"
	"strings"

	"butterfly.orx.me/core/log"
	"github.com/davhofer/botsky/pkg/botsky"
)

// BlueskyClient 使用 botsky 库的 Bluesky 客户端
type BlueskyClient struct {
	name   string
	client *botsky.Client
}

// NewBlueskyClient 创建一个新的Bluesky客户端
func NewBlueskyClient(host, handle, password string, name string) (*BlueskyClient, error) {
	ctx := context.Background()

	// 使用 botsky 库创建客户端
	client, err := botsky.NewClient(ctx, handle, password)
	if err != nil {
		return nil, fmt.Errorf("failed to create botsky client: %w", err)
	}

	// 进行认证
	err = client.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with Bluesky: %w", err)
	}

	return &BlueskyClient{
		name:   name,
		client: client,
	}, nil
}

func (c *BlueskyClient) Name() string {
	return c.name
}

// NewBlueskyClientFromEnv 从环境变量创建一个新的Bluesky客户端
func NewBlueskyClientFromEnv() (*BlueskyClient, error) {
	handle := os.Getenv("BLUESKY_HANDLE")
	if handle == "" {
		return nil, fmt.Errorf("BLUESKY_HANDLE environment variable is not set")
	}

	password := os.Getenv("BLUESKY_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("BLUESKY_PASSWORD environment variable is not set")
	}

	// botsky 使用 app password 而不是普通密码
	// 如果用户提供的是 app password，直接使用；否则假设是 app password
	return NewBlueskyClient("", handle, password, "bluesky")
}

// Post 发布一条Bluesky帖子
func (b *BlueskyClient) Post(ctx context.Context, post *Post) (interface{}, error) {
	logger := log.FromContext(ctx)

	// 检查客户端是否已初始化
	if b.client == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	logger.Info("creating bluesky post",
		"content_length", len(post.Content),
		"media_count", len(post.Media))

	// 使用 botsky 的 PostBuilder 创建帖子
	pb := botsky.NewPostBuilder(post.Content)

	// 处理媒体附件
	if len(post.Media) > 0 {
		logger.Info("processing media attachments", "count", len(post.Media))

		var images []botsky.ImageSource
		for i, media := range post.Media {
			// 获取媒体数据
			mediaData, err := media.GetData()
			if err != nil {
				logger.Error("failed to get media data",
					"index", i,
					"error", err)
				return nil, fmt.Errorf("failed to get media data for attachment %d: %w", i, err)
			}

			// 创建临时文件来存储媒体数据，因为 botsky 需要文件路径或URL
			tmpFile, err := os.CreateTemp("", fmt.Sprintf("hypersync_media_%d_*.jpg", i))
			if err != nil {
				logger.Error("failed to create temp file",
					"index", i,
					"error", err)
				return nil, fmt.Errorf("failed to create temp file for media %d: %w", i, err)
			}

			// 写入媒体数据
			_, err = tmpFile.Write(mediaData)
			tmpFile.Close()
			if err != nil {
				os.Remove(tmpFile.Name())
				logger.Error("failed to write media to temp file",
					"index", i,
					"error", err)
				return nil, fmt.Errorf("failed to write media data for attachment %d: %w", i, err)
			}

			// 创建 ImageSource
			imageSource := botsky.ImageSource{
				Uri: tmpFile.Name(),
				Alt: media.Description,
			}
			images = append(images, imageSource)

			logger.Info("prepared media for upload",
				"index", i,
				"temp_file", tmpFile.Name(),
				"alt_text", media.Description)

			// 确保在函数结束时清理临时文件
			defer func(filename string) {
				if err := os.Remove(filename); err != nil {
					logger.Warn("failed to remove temp file", "file", filename, "error", err)
				}
			}(tmpFile.Name())
		}

		// 添加图像到帖子
		pb = pb.AddImages(images)
	}

	// 发布帖子
	logger.Info("posting to bluesky via botsky")
	cid, uri, err := b.client.Post(ctx, pb)
	if err != nil {
		logger.Error("failed to post via botsky", "error", err)
		return nil, fmt.Errorf("failed to post to Bluesky: %w", err)
	}

	logger.Info("successfully posted to bluesky",
		"cid", cid,
		"uri", uri)

	// 从 URI 中提取 rkey (at://did:plc:xxx/app.bsky.feed.post/rkey)
	parts := strings.Split(uri, "/")
	var rkey string
	if len(parts) > 0 {
		rkey = parts[len(parts)-1]
	}

	// 返回与原始接口兼容的响应
	return map[string]interface{}{
		"uri":  uri,
		"cid":  cid,
		"rkey": rkey,
	}, nil
}

// DeletePost 删除一条Bluesky帖子
func (b *BlueskyClient) DeletePost(ctx context.Context, rkey string) error {
	logger := log.FromContext(ctx)

	if b.client == nil {
		return fmt.Errorf("client not initialized")
	}

	logger.Info("deleting bluesky post", "rkey", rkey)

	// 构造完整的 URI - at://did/app.bsky.feed.post/rkey
	// 我们需要客户端的 DID 来构造完整的 URI
	if b.client.Did == "" {
		return fmt.Errorf("client DID not available")
	}

	postUri := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", b.client.Did, rkey)
	logger.Info("constructed post URI for deletion", "uri", postUri)

	// 使用 botsky 的删除方法
	err := b.client.RepoDeletePost(ctx, postUri)
	if err != nil {
		logger.Error("failed to delete post via botsky", "error", err, "uri", postUri)
		return fmt.Errorf("failed to delete post: %w", err)
	}

	logger.Info("successfully deleted bluesky post", "rkey", rkey, "uri", postUri)
	return nil
}

// ListPosts 获取当前用户的最新帖子
func (b *BlueskyClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	logger := log.FromContext(ctx)

	if b.client == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	// 设置默认限制
	if limit <= 0 {
		limit = 20
	}

	logger.Info("listing bluesky posts", "limit", limit, "did", b.client.Did)

	// 使用 GetPosts 方法获取当前用户的帖子
	// 如果遇到服务器错误，我们提供优雅的处理
	richPosts, err := b.client.GetPosts(ctx, b.client.Did, limit)
	if err != nil {
		logger.Error("failed to get posts via botsky", "error", err)

		// 检查是否是服务器错误 (502, 503 等)
		if strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "Internal Server Error") {
			logger.Warn("bluesky server error detected, returning empty list for graceful degradation")
			// 对于服务器错误，返回空列表而不是失败，这样不会阻止其他功能
			return []*Post{}, nil
		}

		return nil, fmt.Errorf("failed to list posts: %w", err)
	}

	logger.Info("retrieved posts from botsky", "count", len(richPosts))

	return b.convertRichPostsToInternalPosts(ctx, richPosts), nil
}

// convertRichPostsToInternalPosts 将 RichPost 转换为内部 Post 结构（后备方法）
func (b *BlueskyClient) convertRichPostsToInternalPosts(ctx context.Context, richPosts []*botsky.RichPost) []*Post {
	logger := log.FromContext(ctx)

	posts := make([]*Post, 0, len(richPosts))
	for i, richPost := range richPosts {
		// 从 URI 中提取 rkey (at://did:plc:xxx/app.bsky.feed.post/rkey)
		parts := strings.Split(richPost.Uri, "/")
		var rkey string
		if len(parts) > 0 {
			rkey = parts[len(parts)-1]
		}

		// 转换为我们的 Post 结构
		post := &Post{
			ID:             rkey,
			Content:        richPost.Text,
			SourcePlatform: "bluesky",
		}

		// 处理媒体附件（如果有的话）
		if richPost.Embed != nil {
			logger.Debug("post has embeds", "index", i, "rkey", rkey)
		}

		posts = append(posts, post)

		logger.Debug("converted rich post to our format",
			"index", i,
			"rkey", rkey,
			"content_length", len(post.Content),
			"uri", richPost.Uri)
	}

	logger.Info("successfully converted rich posts", "final_count", len(posts))
	return posts
}

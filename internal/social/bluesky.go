package social

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
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

// 定义 Bluesky 的文件大小限制（976KB）
const BlueskyMaxFileSize = 976 * 1024 // 976KB in bytes

// resizeImageIfNeeded 如果图片超过Bluesky限制则调整大小
func resizeImageIfNeeded(data []byte, maxSize int) ([]byte, error) {
	if len(data) <= maxSize {
		return data, nil // 文件已经在限制内
	}

	// 检测图片格式
	contentType := detectImageFormat(data)
	if contentType == "" {
		return nil, fmt.Errorf("unsupported image format")
	}

	// 解码图片
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// 计算缩放比例（基于文件大小比例）
	ratio := float64(maxSize) / float64(len(data))
	if ratio >= 1.0 {
		return data, nil // 不需要缩放
	}

	// 根据比例计算新的尺寸（稍微保守一点，使用 0.8 的系数）
	scaleFactor := ratio * 0.8
	bounds := img.Bounds()
	newWidth := int(float64(bounds.Dx()) * scaleFactor)
	newHeight := int(float64(bounds.Dy()) * scaleFactor)

	// 确保最小尺寸
	if newWidth < 100 {
		newWidth = 100
	}
	if newHeight < 100 {
		newHeight = 100
	}

	// 创建新的缩放图片
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// 简单的最近邻缩放算法
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// 映射到原图坐标
			srcX := int(float64(x) * float64(bounds.Dx()) / float64(newWidth))
			srcY := int(float64(y) * float64(bounds.Dy()) / float64(newHeight))

			// 确保坐标在范围内
			if srcX >= bounds.Dx() {
				srcX = bounds.Dx() - 1
			}
			if srcY >= bounds.Dy() {
				srcY = bounds.Dy() - 1
			}

			resized.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	// 编码新图片
	var buf bytes.Buffer
	var quality int = 85 // JPEG 质量

	switch contentType {
	case "image/jpeg":
		err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality})
	case "image/png":
		err = png.Encode(&buf, resized)
	default:
		// 默认使用 JPEG
		err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode resized image: %w", err)
	}

	result := buf.Bytes()

	// 如果仍然太大，降低JPEG质量重试
	if len(result) > maxSize && contentType == "image/jpeg" {
		for quality > 20 && len(result) > maxSize {
			quality -= 10
			buf.Reset()
			err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality})
			if err != nil {
				return nil, fmt.Errorf("failed to encode resized image with quality %d: %w", quality, err)
			}
			result = buf.Bytes()
		}
	}

	return result, nil
}

// detectImageFormat 检测图片格式
func detectImageFormat(data []byte) string {
	if len(data) < 8 {
		return ""
	}

	// JPEG
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return "image/jpeg"
	}

	// PNG
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "image/png"
	}

	return ""
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

	// Validate visibility for Bluesky using enum
	if post.Visibility != "" {
		_, err := ValidateAndNormalizeVisibilityLevel("bluesky", post.Visibility)
		if err != nil {
			return nil, fmt.Errorf("invalid visibility for Bluesky: %w", err)
		}
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

			logger.Info("processing media attachment",
				"index", i,
				"original_size", len(mediaData),
				"max_allowed_size", BlueskyMaxFileSize)

			// 检查并调整图片大小如果超过Bluesky限制
			processedData, err := resizeImageIfNeeded(mediaData, BlueskyMaxFileSize)
			if err != nil {
				logger.Error("failed to resize image",
					"index", i,
					"error", err)
				return nil, fmt.Errorf("failed to resize image for attachment %d: %w", i, err)
			}

			// 记录处理结果
			if len(processedData) != len(mediaData) {
				logger.Info("image resized to fit Bluesky limits",
					"index", i,
					"original_size", len(mediaData),
					"new_size", len(processedData),
					"reduction_percent", (1.0-float64(len(processedData))/float64(len(mediaData)))*100)
			} else {
				logger.Info("image size within limits, no resizing needed",
					"index", i,
					"size", len(mediaData))
			}

			// 创建临时文件来存储媒体数据，因为 botsky 需要文件路径或URL
			tmpFile, err := os.CreateTemp("", fmt.Sprintf("hypersync_media_%d_*.jpg", i))
			if err != nil {
				logger.Error("failed to create temp file",
					"index", i,
					"error", err)
				return nil, fmt.Errorf("failed to create temp file for media %d: %w", i, err)
			}

			// 写入处理后的媒体数据
			_, err = tmpFile.Write(processedData)
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

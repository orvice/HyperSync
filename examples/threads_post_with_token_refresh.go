package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/social"
)

func main() {
	// 1. 初始化 MongoDB 连接
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer client.Disconnect(context.Background())

	// 2. 创建 DAO 和适配器
	socialDao := dao.NewMongoDAO(client)
	tokenDao := dao.NewThreadsConfigAdapter(socialDao)

	// 3. 从环境变量获取 Threads 配置
	clientID := os.Getenv("THREADS_CLIENT_ID")
	clientSecret := os.Getenv("THREADS_CLIENT_SECRET")
	userID := os.Getenv("THREADS_USER_ID")

	if clientID == "" || clientSecret == "" || userID == "" {
		log.Fatal("Please set THREADS_CLIENT_ID, THREADS_CLIENT_SECRET, and THREADS_USER_ID environment variables")
	}

	// 4. 创建 Threads 客户端（会自动从数据库加载 access token）
	// 如果数据库中没有 token，可以从环境变量提供初始 token
	initialAccessToken := os.Getenv("THREADS_ACCESS_TOKEN") // 可选，用于首次初始化

	threadsClient, err := social.NewThreadsClientWithDao("threads", clientID, clientSecret, initialAccessToken, tokenDao)
	if err != nil {
		log.Fatal("Failed to create Threads client:", err)
	}

	fmt.Println("🎯 Threads客户端初始化成功！")

	// 5. 发布不同类型的帖子（每次发帖前会自动检查和刷新 token）

	// 发布文本帖子
	fmt.Println("\n📝 发布文本帖子...")
	textPost, err := threadsClient.PostText(context.Background(), userID, "Hello from HyperSync! 🚀 #automation")
	if err != nil {
		log.Printf("❌ 文本帖子发布失败: %v", err)
	} else {
		fmt.Printf("✅ 文本帖子发布成功! ID: %s\n", textPost.ID)
	}

	// 发布带链接的文本帖子
	fmt.Println("\n🔗 发布带链接的文本帖子...")
	linkPost, err := threadsClient.PostText(context.Background(), userID, "Check out this awesome project!", "https://github.com/orvice/HyperSync")
	if err != nil {
		log.Printf("❌ 链接帖子发布失败: %v", err)
	} else {
		fmt.Printf("✅ 链接帖子发布成功! ID: %s\n", linkPost.ID)
	}

	// 发布图片帖子
	fmt.Println("\n🖼️ 发布图片帖子...")
	imagePost, err := threadsClient.PostImage(context.Background(), userID, "https://picsum.photos/800/600", "Beautiful random image from Picsum!")
	if err != nil {
		log.Printf("❌ 图片帖子发布失败: %v", err)
	} else {
		fmt.Printf("✅ 图片帖子发布成功! ID: %s\n", imagePost.ID)
	}

	// 发布轮播帖子
	fmt.Println("\n🎠 发布轮播帖子...")
	carouselItems := []social.CarouselItem{
		{MediaType: "IMAGE", ImageURL: "https://picsum.photos/800/600?random=1"},
		{MediaType: "IMAGE", ImageURL: "https://picsum.photos/800/600?random=2"},
		{MediaType: "IMAGE", ImageURL: "https://picsum.photos/800/600?random=3"},
	}
	carouselPost, err := threadsClient.PostCarousel(context.Background(), userID, carouselItems, "A collection of beautiful images! 📸✨")
	if err != nil {
		log.Printf("❌ 轮播帖子发布失败: %v", err)
	} else {
		fmt.Printf("✅ 轮播帖子发布成功! ID: %s\n", carouselPost.ID)
	}

	// 6. 演示手动 token 检查
	fmt.Println("\n🔄 手动检查 token 有效性...")
	err = threadsClient.EnsureValidToken(context.Background())
	if err != nil {
		log.Printf("❌ Token 验证失败: %v", err)
	} else {
		fmt.Println("✅ Token 验证成功!")
	}

	// 7. 演示获取 token 信息
	fmt.Println("\n📊 获取 token 信息...")
	tokenInfo, err := tokenDao.GetTokenInfo(context.Background(), "threads")
	if err != nil {
		log.Printf("❌ 获取 token 信息失败: %v", err)
	} else if tokenInfo != nil {
		fmt.Printf("✅ Token 信息:\n")
		fmt.Printf("   Access Token: %s...%s\n", tokenInfo.AccessToken[:10], tokenInfo.AccessToken[len(tokenInfo.AccessToken)-10:])
		if tokenInfo.ExpiresAt != nil {
			timeUntilExpiry := time.Until(*tokenInfo.ExpiresAt)
			fmt.Printf("   过期时间: %s\n", tokenInfo.ExpiresAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("   剩余时间: %s\n", timeUntilExpiry.String())

			// 检查是否需要刷新
			if timeUntilExpiry <= 7*24*time.Hour {
				fmt.Printf("   🟡 建议刷新: Token 将在 7 天内过期\n")
			} else {
				fmt.Printf("   🟢 状态良好: Token 还有足够的有效时间\n")
			}
		} else {
			fmt.Printf("   ⚠️  无过期时间信息\n")
		}
	}

	fmt.Println("\n🎉 所有操作完成!")
}

// 使用说明:
// 1. 设置环境变量:
//    export THREADS_CLIENT_ID="your_client_id"
//    export THREADS_CLIENT_SECRET="your_client_secret"
//    export THREADS_USER_ID="your_user_id"
//    export THREADS_ACCESS_TOKEN="your_initial_token"  # 首次初始化时需要
//    export MONGO_URI="mongodb://localhost:27017"  # 可选
//
// 2. 首次运行时，程序会自动将 THREADS_ACCESS_TOKEN 保存到数据库
//    后续运行时会自动使用数据库中的 token（可能已自动刷新）
//
// 3. 运行程序:
//    go run examples/threads_post_with_token_refresh.go
//
// 特性:
// ✅ 自动 token 验证和刷新
// ✅ 支持所有类型的帖子发布
// ✅ 完整的错误处理
// ✅ Token 状态监控
// ✅ 生产级别的安全实践

package http

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// setupServices initializes all services
func setupServices(db *mongo.Client) {
	// Create DAO
	mongoDAO := dao.NewMongoDAO(db, "hyper_sync")

	// Initialize services with the DAO
	InitServices(mongoDAO)
}

func Router(r *gin.Engine) {
	// Get the MongoDB client from the app context
	// This assumes the app framework provides a way to get the MongoDB client
	// You may need to adjust this based on your actual app structure
	db := getMongoDB()

	// Setup services
	setupServices(db)

	// Routes
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// API routes
	api := r.Group("/api")
	{
		// Posts routes
		posts := api.Group("/posts")
		{
			posts.GET("", listPosts)
			posts.GET("/:id", getPost)
			posts.POST("", createPost)
			posts.DELETE("/:id", deletePost)
		}

		// Sync routes
		sync := api.Group("/sync")
		{
			sync.POST("/trigger", triggerSync)
			sync.GET("/status", getSyncStatus)
			sync.GET("/history", getSyncHistory)
			sync.POST("/retry", retryFailedSyncs)
		}

		// Scheduler routes
		scheduler := api.Group("/scheduler")
		{
			scheduler.GET("/status", getSchedulerStatus)
			scheduler.POST("/start", startScheduler)
			scheduler.POST("/stop", stopScheduler)
			scheduler.POST("/task", scheduleTask)
			scheduler.GET("/queue", getTaskQueue)
			scheduler.GET("/cron", getCronJobs)
			scheduler.PUT("/config", updateSchedulerConfig)
			scheduler.DELETE("/queue", clearTaskQueue)
			scheduler.POST("/retry", retryFailedTasks)
		}

		// Webhook routes
		webhook := api.Group("/webhook")
		{
			webhook.POST("/memos", handleMemosWebhook)
			webhook.POST("/generic", handleGenericWebhook)
			webhook.GET("/config", getWebhookConfig)
			webhook.PUT("/config", updateWebhookConfig)
			webhook.POST("/test", testWebhook)
			webhook.GET("/stats", getWebhookStats)
		}

		// Config routes
		config := api.Group("/config")
		{
			config.GET("", getConfig)
			config.PUT("", updateConfig)
		}

		// Health check
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"service": "hyper-sync",
				"time":    time.Now().UTC(),
			})
		})
	}
}

// getMongoDB gets the MongoDB client from the app context
// This is a placeholder - replace with your actual implementation
func getMongoDB() *mongo.Client {
	// This is just a placeholder function
	// In your real implementation, you would get the MongoDB client
	// from your app's configuration or context
	return nil
}

// Post handler functions
func listPosts(c *gin.Context) {
	postService := GetPostService()
	if postService == nil {
		c.JSON(500, gin.H{"error": "Post service not initialized"})
		return
	}

	// Parse query parameters
	platform := c.Query("platform")
	limitStr := c.DefaultQuery("limit", "20")
	pageStr := c.DefaultQuery("page", "1")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	// Get posts
	posts, err := postService.ListPosts(c.Request.Context(), platform, limit, page)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to list posts: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"posts": posts,
		"page":  page,
		"limit": limit,
		"count": len(posts),
	})
}

func getPost(c *gin.Context) {
	postService := GetPostService()
	if postService == nil {
		c.JSON(500, gin.H{"error": "Post service not initialized"})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "Post ID is required"})
		return
	}

	post, err := postService.GetPost(c.Request.Context(), id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get post: " + err.Error()})
		return
	}

	if post == nil {
		c.JSON(404, gin.H{"error": "Post not found"})
		return
	}

	c.JSON(200, post)
}

func createPost(c *gin.Context) {
	postService := GetPostService()
	if postService == nil {
		c.JSON(500, gin.H{"error": "Post service not initialized"})
		return
	}

	var post struct {
		Content        string   `json:"content" binding:"required"`
		Visibility     string   `json:"visibility"`
		SourcePlatform string   `json:"source_platform"`
		OriginalID     string   `json:"original_id"`
		Platforms      []string `json:"platforms"`
	}

	if err := c.ShouldBindJSON(&post); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	// Create post object
	newPost := &social.Post{
		Content:        post.Content,
		Visibility:     post.Visibility,
		SourcePlatform: post.SourcePlatform,
		OriginalID:     post.OriginalID,
	}

	// Create post
	id, err := postService.CreatePost(c.Request.Context(), newPost, post.Platforms)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create post: " + err.Error()})
		return
	}

	c.JSON(201, gin.H{
		"id":      id,
		"message": "Post created successfully",
	})
}

func deletePost(c *gin.Context) {
	postService := GetPostService()
	if postService == nil {
		c.JSON(500, gin.H{"error": "Post service not initialized"})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "Post ID is required"})
		return
	}

	err := postService.DeletePost(c.Request.Context(), id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete post: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Post deleted successfully"})
}

// Scheduler handler functions
func getSchedulerStatus(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.GetSchedulerStatus(c)
}

func startScheduler(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.StartScheduler(c)
}

func stopScheduler(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.StopScheduler(c)
}

func scheduleTask(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.ScheduleTask(c)
}

func getTaskQueue(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.GetTaskQueue(c)
}

func getCronJobs(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.GetCronJobs(c)
}

func updateSchedulerConfig(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.UpdateSchedulerConfig(c)
}

func clearTaskQueue(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.ClearTaskQueue(c)
}

func retryFailedTasks(c *gin.Context) {
	schedulerService := GetSchedulerService()
	if schedulerService == nil {
		c.JSON(500, gin.H{"error": "Scheduler service not initialized"})
		return
	}

	schedulerHandlers := NewSchedulerHandlers(schedulerService)
	schedulerHandlers.RetryFailedTasks(c)
}

// Webhook handler functions
func handleMemosWebhook(c *gin.Context) {
	webhookService := GetWebhookService()
	if webhookService == nil {
		c.JSON(500, gin.H{"error": "Webhook service not initialized"})
		return
	}

	webhookHandlers := NewWebhookHandlers(webhookService)
	webhookHandlers.HandleMemosWebhook(c)
}

func handleGenericWebhook(c *gin.Context) {
	webhookService := GetWebhookService()
	if webhookService == nil {
		c.JSON(500, gin.H{"error": "Webhook service not initialized"})
		return
	}

	webhookHandlers := NewWebhookHandlers(webhookService)
	webhookHandlers.HandleGenericWebhook(c)
}

func getWebhookConfig(c *gin.Context) {
	webhookService := GetWebhookService()
	if webhookService == nil {
		c.JSON(500, gin.H{"error": "Webhook service not initialized"})
		return
	}

	webhookHandlers := NewWebhookHandlers(webhookService)
	webhookHandlers.GetWebhookConfig(c)
}

func updateWebhookConfig(c *gin.Context) {
	webhookService := GetWebhookService()
	if webhookService == nil {
		c.JSON(500, gin.H{"error": "Webhook service not initialized"})
		return
	}

	webhookHandlers := NewWebhookHandlers(webhookService)
	webhookHandlers.UpdateWebhookConfig(c)
}

func testWebhook(c *gin.Context) {
	webhookService := GetWebhookService()
	if webhookService == nil {
		c.JSON(500, gin.H{"error": "Webhook service not initialized"})
		return
	}

	webhookHandlers := NewWebhookHandlers(webhookService)
	webhookHandlers.TestWebhook(c)
}

func getWebhookStats(c *gin.Context) {
	webhookService := GetWebhookService()
	if webhookService == nil {
		c.JSON(500, gin.H{"error": "Webhook service not initialized"})
		return
	}

	webhookHandlers := NewWebhookHandlers(webhookService)
	webhookHandlers.GetWebhookStats(c)
}

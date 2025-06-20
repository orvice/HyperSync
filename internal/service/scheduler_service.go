package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"butterfly.orx.me/core/log"
	"go.orx.me/apps/hyper-sync/internal/dao"
)

// SchedulerService handles automatic synchronization scheduling
type SchedulerService struct {
	syncService *SyncService
	dao         *dao.MongoDAO
	config      *SchedulerConfig

	// Internal state
	isRunning bool
	stopChan  chan struct{}
	taskQueue *TaskQueue
	cronJobs  map[string]*CronJob
	mutex     sync.RWMutex

	// Statistics
	stats *SchedulerStats
}

// SchedulerConfig contains configuration for the scheduler
type SchedulerConfig struct {
	// Auto sync configuration
	AutoSyncEnabled    bool          // Whether auto sync is enabled
	DefaultInterval    time.Duration // Default sync interval
	MaxConcurrentTasks int           // Maximum concurrent sync tasks

	// Retry configuration
	MaxRetries int           // Maximum retries for failed tasks
	RetryDelay time.Duration // Delay between retries

	// Queue configuration
	QueueSize   int           // Maximum queue size
	TaskTimeout time.Duration // Maximum time for a single task

	// Schedule patterns
	SchedulePatterns []SchedulePattern // Custom schedule patterns
}

// SchedulePattern defines a custom sync schedule
type SchedulePattern struct {
	Name      string       // Pattern name
	CronExpr  string       // Cron expression (e.g., "0 */15 * * * *")
	Enabled   bool         // Whether this pattern is enabled
	Platforms []string     // Target platforms for this pattern
	Filters   *SyncFilters // Optional filters for this pattern
}

// SyncFilters defines filtering criteria for scheduled syncs
type SyncFilters struct {
	MemosCreator string        // Filter by memos creator
	SkipPrivate  bool          // Skip private memos
	SkipOlder    time.Duration // Skip memos older than this
	MaxMemos     int           // Maximum memos per sync
}

// TaskQueue represents a queue of sync tasks
type TaskQueue struct {
	tasks      chan *SyncTask
	workers    []*TaskWorker
	maxWorkers int
	mutex      sync.RWMutex
}

// SyncTask represents a single sync task
type SyncTask struct {
	ID          string       // Unique task ID
	Type        TaskType     // Type of sync task
	CreatedAt   time.Time    // When task was created
	ScheduledAt time.Time    // When task should be executed
	Retries     int          // Number of retries attempted
	MaxRetries  int          // Maximum retries allowed
	Platforms   []string     // Target platforms
	Filters     *SyncFilters // Sync filters
	Priority    TaskPriority // Task priority
	Status      TaskStatus   // Current task status

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// TaskType represents different types of sync tasks
type TaskType string

const (
	TaskTypeAutoSync   TaskType = "auto_sync"   // Automatic scheduled sync
	TaskTypeManualSync TaskType = "manual_sync" // Manual triggered sync
	TaskTypeRetrySync  TaskType = "retry_sync"  // Retry failed sync
)

// TaskPriority represents task priority levels
type TaskPriority int

const (
	PriorityLow    TaskPriority = 0
	PriorityNormal TaskPriority = 1
	PriorityHigh   TaskPriority = 2
	PriorityUrgent TaskPriority = 3
)

// TaskStatus represents current status of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskWorker represents a worker that processes sync tasks
type TaskWorker struct {
	id          int
	queue       *TaskQueue
	syncService *SyncService
	isRunning   bool
	stopChan    chan struct{}
}

// CronJob represents a scheduled cron job
type CronJob struct {
	Pattern    *SchedulePattern
	NextRun    time.Time
	LastRun    time.Time
	IsRunning  bool
	RunCount   int64
	ErrorCount int64
}

// SchedulerStats contains scheduler statistics
type SchedulerStats struct {
	TotalTasksProcessed int64         // Total tasks processed
	TasksInQueue        int           // Current tasks in queue
	ActiveWorkers       int           // Number of active workers
	LastSyncTime        time.Time     // Last successful sync time
	AverageTaskTime     time.Duration // Average task processing time
	SuccessRate         float64       // Success rate percentage

	// Error statistics
	RecentErrors []string // Recent error messages
	ErrorRate    float64  // Error rate percentage

	mutex sync.RWMutex
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(syncService *SyncService, dao *dao.MongoDAO, config *SchedulerConfig) *SchedulerService {
	if config == nil {
		config = &SchedulerConfig{
			AutoSyncEnabled:    false,
			DefaultInterval:    15 * time.Minute,
			MaxConcurrentTasks: 3,
			MaxRetries:         3,
			RetryDelay:         5 * time.Minute,
			QueueSize:          100,
			TaskTimeout:        10 * time.Minute,
		}
	}

	scheduler := &SchedulerService{
		syncService: syncService,
		dao:         dao,
		config:      config,
		isRunning:   false,
		stopChan:    make(chan struct{}),
		cronJobs:    make(map[string]*CronJob),
		stats:       &SchedulerStats{},
	}

	// Initialize task queue
	scheduler.taskQueue = &TaskQueue{
		tasks:      make(chan *SyncTask, config.QueueSize),
		maxWorkers: config.MaxConcurrentTasks,
	}

	return scheduler
}

// Start starts the scheduler service
func (s *SchedulerService) Start(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isRunning {
		return fmt.Errorf("scheduler is already running")
	}

	logger := log.FromContext(ctx)
	logger.Info("Starting scheduler service",
		"auto_sync_enabled", s.config.AutoSyncEnabled,
		"default_interval", s.config.DefaultInterval,
		"max_workers", s.config.MaxConcurrentTasks,
	)

	// Start task workers
	s.startWorkers(ctx)

	// Start cron job scheduler if auto sync is enabled
	if s.config.AutoSyncEnabled {
		s.setupDefaultCronJobs()
		go s.cronSchedulerLoop(ctx)
	}

	// Start statistics updater
	go s.statisticsLoop(ctx)

	s.isRunning = true
	logger.Info("Scheduler service started successfully")

	return nil
}

// Stop stops the scheduler service
func (s *SchedulerService) Stop(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isRunning {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Stopping scheduler service")

	// Signal stop
	close(s.stopChan)

	// Stop workers
	s.stopWorkers(ctx)

	s.isRunning = false
	logger.Info("Scheduler service stopped")

	return nil
}

// ScheduleTask adds a new task to the queue
func (s *SchedulerService) ScheduleTask(ctx context.Context, taskType TaskType, priority TaskPriority, platforms []string, filters *SyncFilters) (string, error) {
	if !s.isRunning {
		return "", fmt.Errorf("scheduler is not running")
	}

	taskID := generateTaskID()
	taskCtx, cancel := context.WithTimeout(ctx, s.config.TaskTimeout)

	task := &SyncTask{
		ID:          taskID,
		Type:        taskType,
		CreatedAt:   time.Now(),
		ScheduledAt: time.Now(),
		Retries:     0,
		MaxRetries:  s.config.MaxRetries,
		Platforms:   platforms,
		Filters:     filters,
		Priority:    priority,
		Status:      TaskStatusPending,
		ctx:         taskCtx,
		cancel:      cancel,
	}

	select {
	case s.taskQueue.tasks <- task:
		logger := log.FromContext(ctx)
		logger.Info("Task scheduled", "task_id", taskID, "type", taskType, "priority", priority)
		return taskID, nil
	default:
		cancel()
		return "", fmt.Errorf("task queue is full")
	}
}

// GetSchedulerStatus returns current scheduler status
func (s *SchedulerService) GetSchedulerStatus(ctx context.Context) map[string]interface{} {
	s.stats.mutex.RLock()
	defer s.stats.mutex.RUnlock()

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	cronJobsStatus := make(map[string]interface{})
	for name, job := range s.cronJobs {
		cronJobsStatus[name] = map[string]interface{}{
			"next_run":    job.NextRun,
			"last_run":    job.LastRun,
			"is_running":  job.IsRunning,
			"run_count":   job.RunCount,
			"error_count": job.ErrorCount,
		}
	}

	return map[string]interface{}{
		"is_running":            s.isRunning,
		"auto_sync_enabled":     s.config.AutoSyncEnabled,
		"default_interval":      s.config.DefaultInterval.String(),
		"max_concurrent_tasks":  s.config.MaxConcurrentTasks,
		"tasks_in_queue":        s.stats.TasksInQueue,
		"active_workers":        s.stats.ActiveWorkers,
		"total_tasks_processed": s.stats.TotalTasksProcessed,
		"last_sync_time":        s.stats.LastSyncTime,
		"average_task_time":     s.stats.AverageTaskTime.String(),
		"success_rate":          s.stats.SuccessRate,
		"error_rate":            s.stats.ErrorRate,
		"cron_jobs":             cronJobsStatus,
	}
}

// startWorkers starts the task workers
func (s *SchedulerService) startWorkers(ctx context.Context) {
	s.taskQueue.workers = make([]*TaskWorker, s.config.MaxConcurrentTasks)

	for i := 0; i < s.config.MaxConcurrentTasks; i++ {
		worker := &TaskWorker{
			id:          i,
			queue:       s.taskQueue,
			syncService: s.syncService,
			isRunning:   true,
			stopChan:    make(chan struct{}),
		}

		s.taskQueue.workers[i] = worker
		go worker.run(ctx, s)
	}
}

// stopWorkers stops all task workers
func (s *SchedulerService) stopWorkers(ctx context.Context) {
	for _, worker := range s.taskQueue.workers {
		if worker != nil {
			close(worker.stopChan)
		}
	}
}

// setupDefaultCronJobs sets up default cron jobs for auto sync
func (s *SchedulerService) setupDefaultCronJobs() {
	// Default auto sync job
	defaultPattern := &SchedulePattern{
		Name:      "default_auto_sync",
		CronExpr:  cronFromInterval(s.config.DefaultInterval),
		Enabled:   true,
		Platforms: []string{"mastodon", "bluesky"},
		Filters: &SyncFilters{
			SkipPrivate: true,
			SkipOlder:   24 * time.Hour,
			MaxMemos:    50,
		},
	}

	s.cronJobs["default_auto_sync"] = &CronJob{
		Pattern: defaultPattern,
		NextRun: time.Now().Add(s.config.DefaultInterval),
	}

	// Add custom patterns if configured
	for _, pattern := range s.config.SchedulePatterns {
		if pattern.Enabled {
			s.cronJobs[pattern.Name] = &CronJob{
				Pattern: &pattern,
				NextRun: calculateNextRun(pattern.CronExpr),
			}
		}
	}
}

// cronSchedulerLoop runs the cron job scheduler
func (s *SchedulerService) cronSchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRunCronJobs(ctx)
		}
	}
}

// checkAndRunCronJobs checks if any cron jobs should run and executes them
func (s *SchedulerService) checkAndRunCronJobs(ctx context.Context) {
	now := time.Now()

	for name, job := range s.cronJobs {
		if job.IsRunning || now.Before(job.NextRun) {
			continue
		}

		// Run the cron job
		go s.executeCronJob(ctx, name, job)
	}
}

// executeCronJob executes a specific cron job
func (s *SchedulerService) executeCronJob(ctx context.Context, name string, job *CronJob) {
	logger := log.FromContext(ctx)

	job.IsRunning = true
	job.LastRun = time.Now()
	job.RunCount++

	logger.Info("Executing cron job", "job_name", name, "pattern", job.Pattern.CronExpr)

	// Schedule sync task
	taskID, err := s.ScheduleTask(ctx, TaskTypeAutoSync, PriorityNormal, job.Pattern.Platforms, job.Pattern.Filters)
	if err != nil {
		logger.Error("Failed to schedule cron job task", "job_name", name, "error", err)
		job.ErrorCount++
	} else {
		logger.Info("Cron job task scheduled", "job_name", name, "task_id", taskID)
	}

	// Calculate next run time
	job.NextRun = calculateNextRun(job.Pattern.CronExpr)
	job.IsRunning = false
}

// statisticsLoop updates scheduler statistics
func (s *SchedulerService) statisticsLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateStatistics()
		}
	}
}

// updateStatistics updates scheduler statistics
func (s *SchedulerService) updateStatistics() {
	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	// Update queue size
	s.stats.TasksInQueue = len(s.taskQueue.tasks)

	// Update active workers count
	activeCount := 0
	for _, worker := range s.taskQueue.workers {
		if worker != nil && worker.isRunning {
			activeCount++
		}
	}
	s.stats.ActiveWorkers = activeCount
}

// run executes the worker loop
func (w *TaskWorker) run(ctx context.Context, scheduler *SchedulerService) {
	logger := log.FromContext(ctx)
	logger.Info("Task worker started", "worker_id", w.id)

	for {
		select {
		case <-w.stopChan:
			logger.Info("Task worker stopped", "worker_id", w.id)
			return
		case <-ctx.Done():
			return
		case task := <-w.queue.tasks:
			w.processTask(ctx, task, scheduler)
		}
	}
}

// processTask processes a single sync task
func (w *TaskWorker) processTask(ctx context.Context, task *SyncTask, scheduler *SchedulerService) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	defer task.cancel() // Always cancel task context when done

	logger.Info("Processing task",
		"worker_id", w.id,
		"task_id", task.ID,
		"task_type", task.Type,
		"priority", task.Priority,
	)

	task.Status = TaskStatusRunning

	// Execute sync operation
	result, err := w.syncService.SyncFromMemos(task.ctx)

	duration := time.Since(startTime)

	// Update statistics
	scheduler.stats.mutex.Lock()
	scheduler.stats.TotalTasksProcessed++

	if err != nil {
		task.Status = TaskStatusFailed
		scheduler.stats.RecentErrors = append(scheduler.stats.RecentErrors, err.Error())
		if len(scheduler.stats.RecentErrors) > 10 {
			scheduler.stats.RecentErrors = scheduler.stats.RecentErrors[1:]
		}

		logger.Error("Task failed",
			"worker_id", w.id,
			"task_id", task.ID,
			"error", err,
			"duration", duration,
		)

		// Schedule retry if applicable
		if task.Retries < task.MaxRetries {
			w.scheduleRetry(ctx, task, scheduler)
		}
	} else {
		task.Status = TaskStatusCompleted
		scheduler.stats.LastSyncTime = time.Now()

		logger.Info("Task completed successfully",
			"worker_id", w.id,
			"task_id", task.ID,
			"duration", duration,
			"memos_synced", result.MemosSynced,
			"memos_skipped", result.MemosSkipped,
		)
	}

	// Update average task time
	if scheduler.stats.TotalTasksProcessed > 0 {
		scheduler.stats.AverageTaskTime = (scheduler.stats.AverageTaskTime*time.Duration(scheduler.stats.TotalTasksProcessed-1) + duration) / time.Duration(scheduler.stats.TotalTasksProcessed)
	} else {
		scheduler.stats.AverageTaskTime = duration
	}

	scheduler.stats.mutex.Unlock()
}

// scheduleRetry schedules a retry for a failed task
func (w *TaskWorker) scheduleRetry(ctx context.Context, task *SyncTask, scheduler *SchedulerService) {
	logger := log.FromContext(ctx)
	retryTask := &SyncTask{
		ID:          generateTaskID(),
		Type:        TaskTypeRetrySync,
		CreatedAt:   time.Now(),
		ScheduledAt: time.Now().Add(scheduler.config.RetryDelay),
		Retries:     task.Retries + 1,
		MaxRetries:  task.MaxRetries,
		Platforms:   task.Platforms,
		Filters:     task.Filters,
		Priority:    PriorityHigh, // Higher priority for retries
		Status:      TaskStatusPending,
	}

	// Add delay before retry
	go func() {
		time.Sleep(scheduler.config.RetryDelay)

		retryCtx, cancel := context.WithTimeout(context.Background(), scheduler.config.TaskTimeout)
		retryTask.ctx = retryCtx
		retryTask.cancel = cancel

		select {
		case scheduler.taskQueue.tasks <- retryTask:
			logger.Info("Retry task scheduled", "original_task_id", task.ID, "retry_task_id", retryTask.ID, "retry_count", retryTask.Retries)
		default:
			cancel()
			logger.Error("Failed to schedule retry task - queue full", "task_id", task.ID)
		}
	}()
}

// Helper functions

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

// cronFromInterval converts a time interval to a simple cron expression
func cronFromInterval(interval time.Duration) string {
	minutes := int(interval.Minutes())
	if minutes < 60 {
		return fmt.Sprintf("0 */%d * * * *", minutes)
	}
	hours := minutes / 60
	return fmt.Sprintf("0 0 */%d * * *", hours)
}

// calculateNextRun calculates the next run time for a cron expression
// This is a simplified implementation - in production, use a proper cron library
func calculateNextRun(cronExpr string) time.Time {
	// Simplified: assume it's an interval-based expression
	// In production, use a proper cron parsing library like github.com/robfig/cron
	return time.Now().Add(15 * time.Minute) // Default fallback
}

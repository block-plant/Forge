package functions

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/logger"
)

// Scheduler manages cron-based function scheduling.
// It parses cron expressions and runs functions at specified intervals.
type Scheduler struct {
	deployer *Deployer
	runtime  *Runtime
	log      *logger.Logger

	// jobs holds active scheduled jobs.
	jobs map[string]*ScheduledJob
	mu   sync.Mutex

	// stop signals the scheduler to shut down.
	stop chan struct{}
}

// ScheduledJob represents a scheduled function execution.
type ScheduledJob struct {
	// FunctionName is the function to execute.
	FunctionName string `json:"function_name"`
	// Schedule is the cron expression.
	Schedule string `json:"schedule"`
	// NextRun is when this job will next execute.
	NextRun time.Time `json:"next_run"`
	// LastRun is when this job last executed.
	LastRun time.Time `json:"last_run,omitempty"`
	// LastResult is the outcome of the last execution.
	LastResult string `json:"last_result,omitempty"`
	// RunCount tracks total executions.
	RunCount int `json:"run_count"`
}

// CronSchedule represents a parsed cron expression.
// Supports: minute hour day-of-month month day-of-week
type CronSchedule struct {
	Minutes    []int // 0-59
	Hours      []int // 0-23
	DaysOfMonth []int // 1-31
	Months     []int // 1-12
	DaysOfWeek []int // 0-6 (0=Sunday)
}

// NewScheduler creates a new function scheduler.
func NewScheduler(deployer *Deployer, runtime *Runtime, log *logger.Logger) *Scheduler {
	return &Scheduler{
		deployer: deployer,
		runtime:  runtime,
		log:      log,
		jobs:     make(map[string]*ScheduledJob),
		stop:     make(chan struct{}),
	}
}

// Start begins the scheduling loop, checking for due jobs every minute.
func (s *Scheduler) Start() {
	// Register scheduled functions
	s.refreshJobs()

	go s.runLoop()

	s.log.Info("Function scheduler started", logger.Fields{
		"jobs": len(s.jobs),
	})
}

// Stop shuts down the scheduler.
func (s *Scheduler) Stop() {
	close(s.stop)
}

// Jobs returns all scheduled jobs.
func (s *Scheduler) Jobs() []*ScheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]*ScheduledJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		list = append(list, job)
	}
	return list
}

// runLoop checks for due jobs every 30 seconds.
func (s *Scheduler) runLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Also refresh job list every 5 minutes
	refreshTicker := time.NewTicker(5 * time.Minute)
	defer refreshTicker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.checkAndRunDueJobs()
		case <-refreshTicker.C:
			s.refreshJobs()
		}
	}
}

// refreshJobs rebuilds the job list from deployed functions.
func (s *Scheduler) refreshJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	functions := s.deployer.List()
	for _, fn := range functions {
		if fn.Status != "active" {
			continue
		}
		for _, trigger := range fn.Triggers {
			if trigger.Type != "schedule" || trigger.Schedule == "" {
				continue
			}

			key := fn.Name + ":" + trigger.Schedule
			if _, exists := s.jobs[key]; exists {
				continue // Already registered
			}

			nextRun, err := nextCronTime(trigger.Schedule, time.Now())
			if err != nil {
				s.log.Warn("Invalid cron schedule", logger.Fields{
					"function": fn.Name,
					"schedule": trigger.Schedule,
					"error":    err.Error(),
				})
				continue
			}

			s.jobs[key] = &ScheduledJob{
				FunctionName: fn.Name,
				Schedule:     trigger.Schedule,
				NextRun:      nextRun,
			}
		}
	}
}

// checkAndRunDueJobs executes any jobs whose next run time has passed.
func (s *Scheduler) checkAndRunDueJobs() {
	s.mu.Lock()
	now := time.Now()
	var dueJobs []*ScheduledJob

	for _, job := range s.jobs {
		if now.After(job.NextRun) || now.Equal(job.NextRun) {
			dueJobs = append(dueJobs, job)
		}
	}
	s.mu.Unlock()

	for _, job := range dueJobs {
		go s.executeJob(job)
	}
}

// executeJob runs a scheduled job and updates its state.
func (s *Scheduler) executeJob(job *ScheduledJob) {
	fn, ok := s.deployer.Get(job.FunctionName)
	if !ok {
		return
	}

	req := &ExecRequest{
		FunctionName: fn.Name,
		Trigger:      "schedule",
		Payload: map[string]interface{}{
			"schedule":  job.Schedule,
			"run_count": job.RunCount + 1,
		},
	}

	result := s.runtime.Execute(fn, req)

	s.mu.Lock()
	job.LastRun = time.Now()
	job.RunCount++
	if result.Success {
		job.LastResult = "success"
	} else {
		job.LastResult = "error: " + result.Error
	}

	// Calculate next run time
	nextRun, err := nextCronTime(job.Schedule, time.Now())
	if err == nil {
		job.NextRun = nextRun
	} else {
		job.NextRun = time.Now().Add(1 * time.Hour) // Fallback
	}
	s.mu.Unlock()
}

// ── Cron Expression Parser ──

// nextCronTime calculates the next execution time from a cron expression.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week
// Also supports shortcuts: @hourly, @daily, @weekly, @monthly
func nextCronTime(expr string, from time.Time) (time.Time, error) {
	// Handle shortcuts
	switch expr {
	case "@hourly":
		return from.Truncate(time.Hour).Add(time.Hour), nil
	case "@daily", "@midnight":
		next := from.AddDate(0, 0, 1)
		return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, from.Location()), nil
	case "@weekly":
		daysUntilSunday := (7 - int(from.Weekday())) % 7
		if daysUntilSunday == 0 {
			daysUntilSunday = 7
		}
		next := from.AddDate(0, 0, daysUntilSunday)
		return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, from.Location()), nil
	case "@monthly":
		next := from.AddDate(0, 1, 0)
		return time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, from.Location()), nil
	}

	// Parse standard cron expression
	cron, err := parseCron(expr)
	if err != nil {
		return time.Time{}, err
	}

	// Brute-force search for next match (within 1 year)
	candidate := from.Add(time.Minute).Truncate(time.Minute)
	limit := from.AddDate(1, 0, 0)

	for candidate.Before(limit) {
		if cronMatches(cron, candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching time found within 1 year")
}

// parseCron parses a standard 5-field cron expression.
func parseCron(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}

	hours, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}

	days, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron day: %w", err)
	}

	months, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}

	weekdays, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron weekday: %w", err)
	}

	return &CronSchedule{
		Minutes:     minutes,
		Hours:       hours,
		DaysOfMonth: days,
		Months:      months,
		DaysOfWeek:  weekdays,
	}, nil
}

// parseCronField parses a single cron field (e.g., "*/5", "1,3,5", "1-10", "*").
func parseCronField(field string, min, max int) ([]int, error) {
	if field == "*" {
		values := make([]int, 0, max-min+1)
		for i := min; i <= max; i++ {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle */step
	if strings.HasPrefix(field, "*/") {
		step := 0
		for _, c := range field[2:] {
			if c < '0' || c > '9' {
				return nil, fmt.Errorf("invalid step: %s", field)
			}
			step = step*10 + int(c-'0')
		}
		if step <= 0 {
			return nil, fmt.Errorf("step must be positive: %s", field)
		}
		var values []int
		for i := min; i <= max; i += step {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle comma-separated values
	var values []int
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		// Handle range (e.g., "1-5")
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start := parseIntSimple(bounds[0])
			end := parseIntSimple(bounds[1])

			if start < min || end > max || start > end {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			for i := start; i <= end; i++ {
				values = append(values, i)
			}
		} else {
			v := parseIntSimple(part)
			if v < min || v > max {
				return nil, fmt.Errorf("value out of range: %s", part)
			}
			values = append(values, v)
		}
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("empty field: %s", field)
	}
	return values, nil
}

// cronMatches checks if a time matches a cron schedule.
func cronMatches(cron *CronSchedule, t time.Time) bool {
	return intSliceContains(cron.Minutes, t.Minute()) &&
		intSliceContains(cron.Hours, t.Hour()) &&
		intSliceContains(cron.DaysOfMonth, t.Day()) &&
		intSliceContains(cron.Months, int(t.Month())) &&
		intSliceContains(cron.DaysOfWeek, int(t.Weekday()))
}

// intSliceContains checks if a slice contains a value.
func intSliceContains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// parseIntSimple parses a simple decimal integer string.
func parseIntSimple(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

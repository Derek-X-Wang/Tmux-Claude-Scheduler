package claude

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/derekxwang/tcs/internal/config"
)

// UsageEntry represents a Claude usage entry from JSONL data
type UsageEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Model        string    `json:"model"`
	MessageID    string    `json:"message_id"`
	RequestID    string    `json:"request_id"`
}

// ClaudeDataReader reads Claude usage data from ~/.claude directory
type ClaudeDataReader struct {
	claudeDir string
}

// NewClaudeDataReader creates a new Claude data reader
func NewClaudeDataReader() *ClaudeDataReader {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: could not get user home directory: %v", err)
		return &ClaudeDataReader{claudeDir: ""}
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	return &ClaudeDataReader{claudeDir: claudeDir}
}

// ReadUsageEntries reads and parses Claude usage entries from JSONL files
func (r *ClaudeDataReader) ReadUsageEntries(hoursBack int) ([]UsageEntry, error) {
	// Debug: log that we're trying to read entries
	debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if debugFile != nil {
		fmt.Fprintf(debugFile, "DEBUG ReadUsageEntries: hoursBack=%d, claudeDir=%s\n", hoursBack, r.claudeDir)
		debugFile.Close()
	}

	// Validate hoursBack parameter
	if hoursBack < 0 {
		return nil, fmt.Errorf("hoursBack must be non-negative, got %d", hoursBack)
	}
	if hoursBack > 8760 { // More than a year seems unreasonable
		return nil, fmt.Errorf("hoursBack too large, maximum is 8760 hours (1 year), got %d", hoursBack)
	}

	if r.claudeDir == "" {
		debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if debugFile != nil {
			fmt.Fprintf(debugFile, "DEBUG ReadUsageEntries: claude directory is empty\n")
			debugFile.Close()
		}
		return nil, fmt.Errorf("claude directory not available")
	}

	// Check if directory exists
	if _, err := os.Stat(r.claudeDir); os.IsNotExist(err) {
		debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if debugFile != nil {
			fmt.Fprintf(debugFile, "DEBUG ReadUsageEntries: directory does not exist: %s\n", r.claudeDir)
			debugFile.Close()
		}
		return nil, fmt.Errorf("claude data directory does not exist: %s", r.claudeDir)
	}

	// Find JSONL files
	jsonlFiles, err := r.findJSONLFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to find JSONL files: %w", err)
	}

	if len(jsonlFiles) == 0 {
		log.Printf("No JSONL files found in %s", r.claudeDir)
		return []UsageEntry{}, nil
	}

	// Calculate cutoff time
	var cutoffTime *time.Time
	if hoursBack > 0 {
		cutoff := time.Now().Add(-time.Duration(hoursBack) * time.Hour)
		cutoffTime = &cutoff
	}

	// Process files
	var allEntries []UsageEntry
	processedIDs := make(map[string]bool)

	for _, file := range jsonlFiles {
		entries, err := r.processJSONLFile(file, cutoffTime, processedIDs)
		if err != nil {
			log.Printf("Warning: failed to process file %s: %v", file, err)
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	// Sort by timestamp
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Timestamp.Before(allEntries[j].Timestamp)
	})

	log.Printf("Loaded %d Claude usage entries from %d files", len(allEntries), len(jsonlFiles))
	return allEntries, nil
}

// GetCurrentWindowEntries gets entries for the current dynamic 5-hour session
func (r *ClaudeDataReader) GetCurrentWindowEntries(resetHour int) ([]UsageEntry, time.Time, time.Time, error) {
	// Get all entries from last 72 hours to find current session (spans across multiple days)
	// This ensures we capture the 7:00 AM session from yesterday morning
	entries, err := r.ReadUsageEntries(72)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}

	if len(entries) == 0 {
		// No entries - create a potential window starting now
		now := time.Now()
		return []UsageEntry{}, now, now.Add(5 * time.Hour), nil
	}

	// Find the current active session using dynamic detection
	windowStart, windowEnd := r.findCurrentSession(entries)

	// Filter entries for current session - include ALL entries within the session window
	// regardless of gaps in activity
	var windowEntries []UsageEntry
	for _, entry := range entries {
		if (entry.Timestamp.Equal(windowStart) || entry.Timestamp.After(windowStart)) &&
			entry.Timestamp.Before(windowEnd) {
			windowEntries = append(windowEntries, entry)
		}
	}

	return windowEntries, windowStart, windowEnd, nil
}

// roundToHour rounds timestamp to the nearest full hour in UTC (like Claude Monitor)
func (r *ClaudeDataReader) roundToHour(timestamp time.Time) time.Time {
	if timestamp.IsZero() {
		return timestamp
	}

	// Convert to UTC if not already
	if timestamp.Location() != time.UTC {
		timestamp = timestamp.UTC()
	}

	// Round to the nearest hour
	return timestamp.Truncate(time.Hour)
}

// findCurrentSession finds the current active 5-hour session from actual Claude usage data
// This implements Claude's dynamic session logic matching Claude Monitor exactly
// The key insight: Claude limits start from the FIRST MESSAGE of the day, not current time
func (r *ClaudeDataReader) findCurrentSession(entries []UsageEntry) (time.Time, time.Time) {
	if len(entries) == 0 {
		now := time.Now().UTC()
		sessionStart := r.roundToHour(now)
		return sessionStart, sessionStart.Add(5 * time.Hour)
	}

	now := time.Now().UTC()

	// Sort entries by timestamp (should already be sorted, but ensure it)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	// Debug: log some information about the entries and analyze session boundaries
	if len(entries) > 0 {
		// Write debug info to a file we can check
		debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if debugFile != nil {
			fmt.Fprintf(debugFile, "DEBUG findCurrentSession: now=%s, first_entry=%s (%s), last_entry=%s (%s), total_entries=%d\n",
				now.Format("2006-01-02 15:04"),
				entries[0].Timestamp.Format("2006-01-02 15:04"), entries[0].Timestamp.Format("15:04"),
				entries[len(entries)-1].Timestamp.Format("2006-01-02 15:04"), entries[len(entries)-1].Timestamp.Format("15:04"),
				len(entries))

			// Log time boundaries for different days to help understand the data
			dayBoundaries := make(map[string][]time.Time)
			for _, entry := range entries {
				day := entry.Timestamp.Format("2006-01-02")
				dayBoundaries[day] = append(dayBoundaries[day], entry.Timestamp)
			}

			for day, timestamps := range dayBoundaries {
				if len(timestamps) > 0 {
					sort.Slice(timestamps, func(i, j int) bool {
						return timestamps[i].Before(timestamps[j])
					})
					fmt.Fprintf(debugFile, "DEBUG day %s: first=%s, last=%s, count=%d\n",
						day, timestamps[0].Format("15:04"), timestamps[len(timestamps)-1].Format("15:04"), len(timestamps))
				}
			}

			debugFile.Close()
		}
	}

	// Claude's key behavior: Find the CLOSEST session to current time
	// This matches how Claude Monitor tracks the active 5-hour window

	// Build all possible 5-hour sessions from the data
	type session struct {
		start        time.Time
		end          time.Time
		messageCount int
		entries      []UsageEntry
	}

	sessions := make(map[string]*session)

	// Group entries into 5-hour blocks based on first message in each block
	for _, entry := range entries {
		entryTime := entry.Timestamp.UTC()
		roundedStart := r.roundToHour(entryTime)
		sessionKey := roundedStart.Format("2006-01-02 15:04")

		s := sessions[sessionKey]
		if s == nil {
			s = &session{
				start:        roundedStart,
				end:          roundedStart.Add(5 * time.Hour),
				messageCount: 0,
				entries:      []UsageEntry{},
			}
			sessions[sessionKey] = s
		}

		// Add entry if it falls within this session's 5-hour window
		if (entryTime.Equal(s.start) || entryTime.After(s.start)) && entryTime.Before(s.end) {
			s.entries = append(s.entries, entry)
			s.messageCount++
		}
	}

	// Find the best session:
	// 1. First priority: Session that contains current time
	// 2. Second priority: Most recent session that hasn't expired too long ago
	// 3. Third priority: Next upcoming session if we're between sessions

	var activeSession *session
	var closestSession *session
	var minTimeDiff time.Duration

	for _, s := range sessions {
		// Check if current time is within this session
		if (now.Equal(s.start) || now.After(s.start)) && now.Before(s.end) {
			activeSession = s
			debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if debugFile != nil {
				fmt.Fprintf(debugFile, "DEBUG found active session containing current time: %s - %s (%d messages)\n",
					s.start.Format("15:04"), s.end.Format("15:04"), s.messageCount)
				debugFile.Close()
			}
			break
		}

		// Calculate how close this session is to current time
		var timeDiff time.Duration
		if now.Before(s.start) {
			// Future session
			timeDiff = s.start.Sub(now)
		} else if now.After(s.end) {
			// Past session
			timeDiff = now.Sub(s.end)
		} else {
			// Current time is within session
			timeDiff = 0
		}

		// Track closest session with significant activity (>50 messages)
		if s.messageCount > 50 && (closestSession == nil || timeDiff < minTimeDiff) {
			closestSession = s
			minTimeDiff = timeDiff
		}
	}

	// Use active session if found
	if activeSession != nil {
		return activeSession.start, activeSession.end
	}

	// Use closest session if it's within reasonable time (last 12 hours)
	if closestSession != nil && minTimeDiff < 12*time.Hour {
		debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if debugFile != nil {
			fmt.Fprintf(debugFile, "DEBUG using closest session (%.1f hours away): %s - %s (%d messages)\n",
				minTimeDiff.Hours(), closestSession.start.Format("15:04"), closestSession.end.Format("15:04"), closestSession.messageCount)
			debugFile.Close()
		}
		return closestSession.start, closestSession.end
	}

	// If no good session found, create a new one starting now
	newStart := r.roundToHour(now)
	debugFile, _ := os.OpenFile("/tmp/tcs-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if debugFile != nil {
		fmt.Fprintf(debugFile, "DEBUG creating new session starting now: %s - %s\n",
			newStart.Format("15:04"), newStart.Add(5*time.Hour).Format("15:04"))
		debugFile.Close()
	}
	return newStart, newStart.Add(5 * time.Hour)
}

// findJSONLFiles finds all JSONL files in the Claude directory
func (r *ClaudeDataReader) findJSONLFiles() ([]string, error) {
	var jsonlFiles []string

	err := filepath.Walk(r.claudeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".jsonl") {
			jsonlFiles = append(jsonlFiles, path)
		}

		return nil
	})

	return jsonlFiles, err
}

// processJSONLFile processes a single JSONL file
func (r *ClaudeDataReader) processJSONLFile(filePath string, cutoffTime *time.Time, processedIDs map[string]bool) ([]UsageEntry, error) {
	// Check file size before processing
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info for %s: %w", filePath, err)
	}

	// Get max file size from config (default 50MB)
	cfg := config.Get()
	maxFileSize := int64(52428800) // 50MB default
	if cfg != nil && cfg.Claude.MaxFileSize > 0 {
		maxFileSize = cfg.Claude.MaxFileSize
	}

	if fileInfo.Size() > maxFileSize {
		log.Printf("Warning: Skipping file %s (size: %d bytes) as it exceeds maximum size limit (%d bytes)",
			filePath, fileInfo.Size(), maxFileSize)
		return nil, nil // Return empty results, not an error
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []UsageEntry
	decoder := json.NewDecoder(file)

	// Get limits from config with defaults
	maxEntriesPerFile := 100000
	maxEntriesInMemory := 50000
	if cfg != nil {
		if cfg.Claude.MaxEntriesPerFile > 0 {
			maxEntriesPerFile = cfg.Claude.MaxEntriesPerFile
		}
		if cfg.Claude.MaxEntriesInMemory > 0 {
			maxEntriesInMemory = cfg.Claude.MaxEntriesInMemory
		}
	}

	// Add protection against excessive memory usage
	entriesProcessed := 0

	for decoder.More() {
		// Check if we've processed too many entries
		if entriesProcessed >= maxEntriesPerFile {
			log.Printf("Warning: Reached maximum entries limit (%d) for file %s, stopping processing",
				maxEntriesPerFile, filePath)
			break
		}

		var rawEntry map[string]interface{}
		if err := decoder.Decode(&rawEntry); err != nil {
			log.Printf("Warning: failed to decode JSON line in %s: %v", filePath, err)
			continue
		}
		entriesProcessed++

		entry, err := r.parseUsageEntry(rawEntry)
		if err != nil {
			continue // Skip invalid entries
		}

		// Check cutoff time
		if cutoffTime != nil && entry.Timestamp.Before(*cutoffTime) {
			continue
		}

		// Check for duplicates
		entryID := fmt.Sprintf("%s:%s", entry.MessageID, entry.RequestID)
		if processedIDs[entryID] {
			continue
		}
		processedIDs[entryID] = true

		entries = append(entries, *entry)

		// Additional check: limit valid entries stored in memory
		if len(entries) >= maxEntriesInMemory {
			log.Printf("Warning: Reached maximum valid entries limit (%d) for file %s",
				maxEntriesInMemory, filePath)
			break
		}
	}

	return entries, nil
}

// parseUsageEntry parses a raw JSONL entry into a UsageEntry
func (r *ClaudeDataReader) parseUsageEntry(rawEntry map[string]interface{}) (*UsageEntry, error) {
	// Parse timestamp
	timestampStr, ok := rawEntry["timestamp"].(string)
	if !ok {
		return nil, fmt.Errorf("no timestamp field")
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		// Try alternative formats
		timestamp, err = time.Parse("2006-01-02T15:04:05.000Z", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
	}

	// Extract usage data from the message
	var inputTokens, outputTokens int
	var model string

	// Check for usage data in message
	if message, ok := rawEntry["message"].(map[string]interface{}); ok {
		if usage, ok := message["usage"].(map[string]interface{}); ok {
			if input, ok := usage["input_tokens"].(float64); ok {
				inputTokens = int(input)
			}
			if output, ok := usage["output_tokens"].(float64); ok {
				outputTokens = int(output)
			}
		}
		if modelStr, ok := message["model"].(string); ok {
			model = modelStr
		}
	}

	// Extract IDs
	messageID := r.extractStringField(rawEntry, "message_id")
	if messageID == "" {
		if message, ok := rawEntry["message"].(map[string]interface{}); ok {
			if id, ok := message["id"].(string); ok {
				messageID = id
			}
		}
	}

	requestID := r.extractStringField(rawEntry, "request_id")
	if requestID == "" {
		requestID = r.extractStringField(rawEntry, "requestId")
	}

	// Only include entries with actual token usage (i.e., actual Claude interactions)
	if inputTokens == 0 && outputTokens == 0 {
		return nil, fmt.Errorf("no token usage data")
	}

	return &UsageEntry{
		Timestamp:    timestamp,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Model:        model,
		MessageID:    messageID,
		RequestID:    requestID,
	}, nil
}

// extractStringField safely extracts a string field from a map
func (r *ClaudeDataReader) extractStringField(data map[string]interface{}, field string) string {
	if value, ok := data[field].(string); ok {
		return value
	}
	return ""
}

// CountMessagesInWindow counts the number of messages in the current dynamic session
func (r *ClaudeDataReader) CountMessagesInWindow(resetHour int) (int, time.Time, time.Time, error) {
	entries, windowStart, windowEnd, err := r.GetCurrentWindowEntries(resetHour)
	if err != nil {
		return 0, time.Time{}, time.Time{}, err
	}

	return len(entries), windowStart, windowEnd, nil
}

// GetDynamicLimits calculates dynamic usage limits based on historical patterns (like Claude Monitor)
func (r *ClaudeDataReader) GetDynamicLimits() (messageLimit int, tokenLimit int, costLimit float64, err error) {
	// Get all entries from the last 30 days to analyze usage patterns
	entries, err := r.ReadUsageEntries(24 * 30) // 30 days
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to read historical entries: %w", err)
	}

	if len(entries) < 10 {
		// Not enough data, use conservative defaults
		return 500, 50000, 25.0, nil
	}

	// Group entries into sessions and calculate session totals
	sessions := r.groupEntriesIntoSessions(entries)

	if len(sessions) < 3 {
		// Not enough sessions, use conservative defaults
		return 500, 50000, 25.0, nil
	}

	// Calculate P90 (90th percentile) limits like Claude Monitor does
	messageLimit = r.calculateP90MessageLimit(sessions)
	tokenLimit = r.calculateP90TokenLimit(sessions)
	costLimit = r.calculateP90CostLimit(sessions)

	// Apply minimum reasonable limits
	if messageLimit < 100 {
		messageLimit = 100
	}
	if tokenLimit < 10000 {
		tokenLimit = 10000
	}
	if costLimit < 5.0 {
		costLimit = 5.0
	}

	return messageLimit, tokenLimit, costLimit, nil
}

// SessionSummary represents aggregated data for a single 5-hour session
type SessionSummary struct {
	StartTime     time.Time
	EndTime       time.Time
	MessageCount  int
	TotalTokens   int
	EstimatedCost float64
}

// groupEntriesIntoSessions groups usage entries into 5-hour sessions using hour-based boundaries
func (r *ClaudeDataReader) groupEntriesIntoSessions(entries []UsageEntry) []SessionSummary {
	if len(entries) == 0 {
		return nil
	}

	var sessions []SessionSummary
	var currentSession *SessionSummary

	// Sort entries by timestamp
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	for _, entry := range entries {
		entryTime := entry.Timestamp.UTC()

		// Round entry timestamp to nearest hour (matching Claude Monitor logic)
		roundedStart := r.roundToHour(entryTime)

		// Check if we need a new session
		if currentSession == nil {
			// Start first session
			currentSession = &SessionSummary{
				StartTime:     roundedStart,
				EndTime:       roundedStart.Add(5 * time.Hour),
				MessageCount:  0,
				TotalTokens:   0,
				EstimatedCost: 0,
			}
		} else {
			// Check if entry falls outside current session's time window
			if entryTime.After(currentSession.EndTime) {
				// Finalize previous session
				sessions = append(sessions, *currentSession)

				// Start new session from rounded hour
				currentSession = &SessionSummary{
					StartTime:     roundedStart,
					EndTime:       roundedStart.Add(5 * time.Hour),
					MessageCount:  0,
					TotalTokens:   0,
					EstimatedCost: 0,
				}
			}
		}

		// Add entry to current session
		currentSession.MessageCount++
		currentSession.TotalTokens += entry.InputTokens + entry.OutputTokens

		// Estimate cost (rough approximation - $3 per million tokens for Sonnet)
		tokens := entry.InputTokens + entry.OutputTokens
		currentSession.EstimatedCost += float64(tokens) * 3.0 / 1000000.0
	}

	// Don't forget the last session
	if currentSession != nil {
		sessions = append(sessions, *currentSession)
	}

	return sessions
}

// calculateP90MessageLimit calculates the 90th percentile message limit
func (r *ClaudeDataReader) calculateP90MessageLimit(sessions []SessionSummary) int {
	if len(sessions) == 0 {
		return 500
	}

	// Extract message counts
	var counts []int
	for _, session := range sessions {
		if session.MessageCount > 0 { // Only include sessions with actual usage
			counts = append(counts, session.MessageCount)
		}
	}

	if len(counts) == 0 {
		return 500
	}

	// Sort and find P90
	sort.Ints(counts)
	p90Index := int(float64(len(counts)) * 0.9)
	if p90Index >= len(counts) {
		p90Index = len(counts) - 1
	}

	// Add some buffer (Claude Monitor seems to add ~20% buffer)
	p90Value := counts[p90Index]
	return int(float64(p90Value) * 1.2)
}

// calculateP90TokenLimit calculates the 90th percentile token limit
func (r *ClaudeDataReader) calculateP90TokenLimit(sessions []SessionSummary) int {
	if len(sessions) == 0 {
		return 50000
	}

	var counts []int
	for _, session := range sessions {
		if session.TotalTokens > 0 {
			counts = append(counts, session.TotalTokens)
		}
	}

	if len(counts) == 0 {
		return 50000
	}

	sort.Ints(counts)
	p90Index := int(float64(len(counts)) * 0.9)
	if p90Index >= len(counts) {
		p90Index = len(counts) - 1
	}

	p90Value := counts[p90Index]
	return int(float64(p90Value) * 1.2)
}

// calculateP90CostLimit calculates the 90th percentile cost limit
func (r *ClaudeDataReader) calculateP90CostLimit(sessions []SessionSummary) float64 {
	if len(sessions) == 0 {
		return 25.0
	}

	var costs []float64
	for _, session := range sessions {
		if session.EstimatedCost > 0 {
			costs = append(costs, session.EstimatedCost)
		}
	}

	if len(costs) == 0 {
		return 25.0
	}

	sort.Float64s(costs)
	p90Index := int(float64(len(costs)) * 0.9)
	if p90Index >= len(costs) {
		p90Index = len(costs) - 1
	}

	p90Value := costs[p90Index]
	return p90Value * 1.2
}

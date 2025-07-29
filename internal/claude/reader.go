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
	// Validate hoursBack parameter
	if hoursBack < 0 {
		return nil, fmt.Errorf("hoursBack must be non-negative, got %d", hoursBack)
	}
	if hoursBack > 8760 { // More than a year seems unreasonable
		return nil, fmt.Errorf("hoursBack too large, maximum is 8760 hours (1 year), got %d", hoursBack)
	}

	if r.claudeDir == "" {
		return nil, fmt.Errorf("claude directory not available")
	}

	// Check if directory exists
	if _, err := os.Stat(r.claudeDir); os.IsNotExist(err) {
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

// GetCurrentWindowEntries gets entries for the current 5-hour window based on reset time
func (r *ClaudeDataReader) GetCurrentWindowEntries(resetHour int) ([]UsageEntry, time.Time, time.Time, error) {
	// Get all entries from last 24 hours to ensure we capture the current window
	entries, err := r.ReadUsageEntries(24)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}

	// Calculate current window based on reset time
	windowStart, windowEnd := r.calculateCurrentWindow(resetHour)

	// Filter entries for current window
	var windowEntries []UsageEntry
	for _, entry := range entries {
		if (entry.Timestamp.Equal(windowStart) || entry.Timestamp.After(windowStart)) &&
			entry.Timestamp.Before(windowEnd) {
			windowEntries = append(windowEntries, entry)
		}
	}

	return windowEntries, windowStart, windowEnd, nil
}

// calculateCurrentWindow calculates the current 5-hour window based on reset hour
func (r *ClaudeDataReader) calculateCurrentWindow(resetHour int) (time.Time, time.Time) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), resetHour, 0, 0, 0, now.Location())

	// Define the 5-hour windows based on reset time
	windows := []time.Time{
		today.Add(-10 * time.Hour), // 5 hours before reset
		today.Add(-5 * time.Hour),  // 5 hours before reset
		today,                      // Reset time
		today.Add(5 * time.Hour),   // 5 hours after reset
		today.Add(10 * time.Hour),  // 10 hours after reset
	}

	// If we're past the last window of today, consider tomorrow's windows
	if now.After(windows[len(windows)-1]) {
		tomorrow := today.Add(24 * time.Hour)
		windows = append(windows, tomorrow, tomorrow.Add(5*time.Hour))
	}

	// Find current window
	for i := 0; i < len(windows)-1; i++ {
		if (now.Equal(windows[i]) || now.After(windows[i])) && now.Before(windows[i+1]) {
			return windows[i], windows[i+1]
		}
	}

	// Fallback - should not happen but handle edge case
	return today, today.Add(5 * time.Hour)
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

	for decoder.More() {
		var rawEntry map[string]interface{}
		if err := decoder.Decode(&rawEntry); err != nil {
			log.Printf("Warning: failed to decode JSON line in %s: %v", filePath, err)
			continue
		}

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

// CountMessagesInWindow counts the number of messages in the current window
func (r *ClaudeDataReader) CountMessagesInWindow(resetHour int) (int, time.Time, time.Time, error) {
	entries, windowStart, windowEnd, err := r.GetCurrentWindowEntries(resetHour)
	if err != nil {
		return 0, time.Time{}, time.Time{}, err
	}

	return len(entries), windowStart, windowEnd, nil
}

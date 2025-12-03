package downloader

import (
	"encoding/json"
	"os"
	"sync"
)

type ChunkState struct {
	ID         int   `json:"id"`
	Start      int64 `json:"start"`
	End        int64 `json:"end"`
	Downloaded int64 `json:"downloaded"`
}

type DownloadState struct {
	URL         string        `json:"url"`
	File        string        `json:"file"`
	Size        int64         `json:"size"`
	Concurrency int           `json:"concurrency"`
	Chunks      []*ChunkState `json:"chunks"`
	mu          sync.Mutex
}

func LoadState(filename string) (*DownloadState, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var state DownloadState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *DownloadState) Save(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// We create a copy or just marshal directly if we assume workers 
	// update fields that are safe (e.g. via atomic, or we lock here).
	// For simplicity, we assume the lock protects the entire struct 
	// during the marshal process.
	
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

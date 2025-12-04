package downloader

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
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
	
	// Create a snapshot to avoid race conditions during json.Marshal
	// specifically for the Downloaded field which is updated atomically
	snapshot := DownloadState{
		URL:         s.URL,
		File:        s.File,
		Size:        s.Size,
		Concurrency: s.Concurrency,
		Chunks:      make([]*ChunkState, len(s.Chunks)),
	}

	for i, c := range s.Chunks {
		snapshot.Chunks[i] = &ChunkState{
			ID:         c.ID,
			Start:      c.Start,
			End:        c.End,
			Downloaded: atomic.LoadInt64(&c.Downloaded),
		}
	}
	
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

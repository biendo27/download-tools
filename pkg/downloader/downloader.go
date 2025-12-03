package downloader

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"sync/atomic"

	"gdl/pkg/resolver"
)

type FileInfo struct {
	Url            string
	Name           string
	Size           int64
	RangeSupported bool
}

type Downloader struct {
	Client *http.Client
}

func NewDownloader() *Downloader {
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // We want raw bytes for range requests
		ForceAttemptHTTP2:   false,
		TLSNextProto:        make(map[string]func(authority string, c *tls.Conn) http.RoundTripper), // Disable HTTP/2
	}
	return &Downloader{
		Client: &http.Client{
			Transport: t,
		},
	}
}

// ... Probe and Download methods ...


func (d *Downloader) Probe(url string, headers map[string]string) (*FileInfo, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	
	// Set default User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	size := resp.ContentLength
	rangeSupported := resp.Header.Get("Accept-Ranges") == "bytes"

	name := parseFilename(resp.Header.Get("Content-Disposition"), url)

	return &FileInfo{
		Url:            url,
		Name:           name,
		Size:           size,
		RangeSupported: rangeSupported,
	}, nil
}

func parseFilename(contentDisposition, url string) string {
	if contentDisposition != "" {
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err == nil {
			if filename, ok := params["filename"]; ok {
				return filename
			}
		}
	}
	return filepath.Base(url)
}

type DownloadConfig struct {
	Url         string
	Concurrency int
	OutputName  string
	OutputDir   string
}

// ...

func (d *Downloader) Download(cfg DownloadConfig) error {
	resolvedUrl, headers, err := resolver.Resolve(cfg.Url)
	if err != nil {
		fmt.Printf("Warning: Failed to resolve URL %s: %v. Using original.\n", cfg.Url, err)
		resolvedUrl = cfg.Url
	} else if resolvedUrl != cfg.Url {
		fmt.Printf("Resolved URL: %s\n", resolvedUrl)
	}

	info, err := d.Probe(resolvedUrl, headers)
	if err != nil {
		return err
	}

	if !info.RangeSupported {
		cfg.Concurrency = 1
	}

	fileName := info.Name
	if cfg.OutputName != "" {
		fileName = cfg.OutputName
	}

	if cfg.OutputDir != "" {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			return err
		}
		fileName = filepath.Join(cfg.OutputDir, fileName)
	}

	stateFile := fileName + ".gdl.json"
	var state *DownloadState

	// Try to load existing state
	if loadedState, err := LoadState(stateFile); err == nil {
		// Verify if state matches current file
		if loadedState.Size == info.Size && loadedState.File == fileName {
			fmt.Println("Resuming download from state file...")
			state = loadedState
			// Update URL in case it changed (e.g. signed link expired)
			state.URL = resolvedUrl 
		}
	}

	// Initialize new state if needed
	if state == nil {
		state = &DownloadState{
			URL:         resolvedUrl,
			File:        fileName,
			Size:        info.Size,
			Concurrency: cfg.Concurrency,
			Chunks:      make([]*ChunkState, cfg.Concurrency),
		}

		chunkSize := info.Size / int64(cfg.Concurrency)
		for i := 0; i < cfg.Concurrency; i++ {
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if i == cfg.Concurrency-1 {
				end = info.Size - 1
			}
			state.Chunks[i] = &ChunkState{
				ID:    i,
				Start: start,
				End:   end,
				Downloaded: 0,
			}
		}
	}

	out, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if info.Size > 0 {
		// Only truncate if new file, otherwise we might wipe existing data?
		// Actually os.Create truncates. os.OpenFile with O_CREATE doesn't if exists.
		// But we need to ensure size.
		stat, _ := out.Stat()
		if stat.Size() != info.Size {
			if err := out.Truncate(info.Size); err != nil {
				return err
			}
		}
	}

	p := mpb.New(mpb.WithWidth(64))
	bar := p.AddBar(info.Size,
		mpb.PrependDecorators(
			decor.Name(filepath.Base(fileName)),
			decor.Percentage(decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_GO, 90),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.SizeB1024(0), "% .2f", 60),
		),
	)

	// Pre-fill bar with already downloaded amount
	var totalDownloaded int64
	for _, chunk := range state.Chunks {
		totalDownloaded += chunk.Downloaded
	}
	bar.IncrInt64(totalDownloaded)

	// Start background saver
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				state.Save(stateFile)
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i, chunk := range state.Chunks {
		if chunk.Downloaded >= (chunk.End - chunk.Start + 1) {
			continue // Chunk already done
		}

		wg.Add(1)
		go func(i int, c *ChunkState) {
			defer wg.Done()
			// Resume from Start + Downloaded
			currentStart := c.Start + c.Downloaded
			if err := d.downloadChunkWithRetry(resolvedUrl, out, currentStart, c.End, bar, headers, c); err != nil {
				fmt.Printf("Error downloading chunk %d: %v\n", i, err)
			}
		}(i, chunk)
	}

	wg.Wait()
	close(done)
	p.Wait()

	// Clean up state file if successful
	os.Remove(stateFile)
	return nil
}

func (d *Downloader) downloadChunkWithRetry(url string, file *os.File, start, end int64, bar *mpb.Bar, headers map[string]string, chunkState *ChunkState) error {
	maxRetries := 5
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		// Always resume from current state
		currentStart := chunkState.Start + chunkState.Downloaded
		if currentStart > chunkState.End {
			return nil
		}

		_, err := d.downloadChunk(url, file, currentStart, end, bar, headers, chunkState)
		
		// downloadChunk now updates chunkState.Downloaded atomically or via mutex?
		// We'll pass chunkState to it.
		
		if chunkState.Start + chunkState.Downloaded > chunkState.End {
			return nil
		}
		if err == nil {
			return nil
		}
		
		lastErr = err
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return fmt.Errorf("failed after %d retries, last error: %v", maxRetries, lastErr)
}

func (d *Downloader) downloadChunk(url string, file *os.File, start, end int64, bar *mpb.Bar, headers map[string]string, chunkState *ChunkState) (int64, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return 0, fmt.Errorf("server returned 200 OK instead of 206 Partial Content (Range ignored)")
	}
	if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return 0, nil
		}
		return 0, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	reader := bar.ProxyReader(resp.Body)
	buf := make([]byte, 256*1024)
	var totalWritten int64

	timer := time.AfterFunc(30*time.Second, func() {
		cancel()
	})
	defer timer.Stop()

	for {
		timer.Reset(30 * time.Second)
		n, err := reader.Read(buf)
		if n > 0 {
			_, wErr := file.WriteAt(buf[:n], start+totalWritten)
			if wErr != nil {
				return totalWritten, wErr
			}
			nInt64 := int64(n)
			totalWritten += nInt64
			
			// Update state safely
			// Since we are the only writer to this ChunkState (one goroutine per chunk),
			// we can just update it. But SaveState reads it concurrently.
			// Atomic store is safest.
			atomic.AddInt64(&chunkState.Downloaded, nInt64)
		}
		if err == io.EOF {
			return totalWritten, nil
		}
		if err != nil {
			return totalWritten, err
		}
	}
}

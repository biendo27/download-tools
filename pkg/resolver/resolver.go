package resolver

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type Resolver interface {
	CanResolve(url string) bool
	Resolve(url string) (string, map[string]string, error)
}

var (
	gdriveRegex  = regexp.MustCompile(`drive\.google\.com`)
	onedriveRegex = regexp.MustCompile(`1drv\.ms|onedrive\.live\.com`)
)

func Resolve(inputUrl string) (string, map[string]string, error) {
	resolvers := []Resolver{
		&GoogleDriveResolver{},
		&OneDriveResolver{},
	}

	for _, r := range resolvers {
		if r.CanResolve(inputUrl) {
			return r.Resolve(inputUrl)
		}
	}
	return inputUrl, nil, nil
}

// --- Google Drive Resolver ---

type GoogleDriveResolver struct{}

func (r *GoogleDriveResolver) CanResolve(u string) bool {
	return gdriveRegex.MatchString(u)
}

func (r *GoogleDriveResolver) Resolve(u string) (string, map[string]string, error) {
	// 1. Extract File ID to construct initial export URL
	fileID := extractGDriveFileID(u)
	if fileID == "" {
		return u, nil, nil
	}
	exportUrl := fmt.Sprintf("https://drive.google.com/uc?export=download&id=%s", fileID)

	// 2. Request with Range to avoid downloading large files, following redirects
	req, err := http.NewRequest("GET", exportUrl, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Range", "bytes=0-4096")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{} // Default client follows redirects
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	// Capture cookies from the response
	var cookies []string
	for _, cookie := range resp.Cookies() {
		cookies = append(cookies, cookie.String())
	}
	headers := make(map[string]string)
	if len(cookies) > 0 {
		headers["Cookie"] = strings.Join(cookies, "; ")
	}

	// 3. Check if we landed on a warning page
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		// Check for the download form
		if strings.Contains(bodyStr, "uc-download-link") || strings.Contains(bodyStr, "confirm=") {
			// Extract form action
			actionRe := regexp.MustCompile(`action="([^"]+)"`)
			actionMatch := actionRe.FindStringSubmatch(bodyStr)
			
			// Extract confirm token
			confirmRe := regexp.MustCompile(`name="confirm" value="([^"]+)"`)
			confirmMatch := confirmRe.FindStringSubmatch(bodyStr)

			// Extract UUID (sometimes needed)
			uuidRe := regexp.MustCompile(`name="uuid" value="([^"]+)"`)
			uuidMatch := uuidRe.FindStringSubmatch(bodyStr)

			if len(actionMatch) > 1 && len(confirmMatch) > 1 {
				baseAction := actionMatch[1]
				// Handle relative action URL
				if strings.HasPrefix(baseAction, "/") {
					baseAction = "https://drive.usercontent.google.com" + baseAction
				}
				
				// Reconstruct URL with params
				values := url.Values{}
				values.Set("id", fileID)
				values.Set("export", "download")
				values.Set("confirm", confirmMatch[1])
				if len(uuidMatch) > 1 {
					values.Set("uuid", uuidMatch[1])
				}

				finalUrl := baseAction
				if strings.Contains(baseAction, "?") {
					finalUrl += "&" + values.Encode()
				} else {
					finalUrl += "?" + values.Encode()
				}
				return finalUrl, headers, nil
			}
		}
	}

	// If it's not HTML (e.g. binary) or we couldn't parse it, 
	// return the final URL we landed on (it might be the direct link)
	return resp.Request.URL.String(), headers, nil
}

func extractGDriveFileID(u string) string {
	// Patterns:
	// /file/d/FILE_ID/view
	// ?id=FILE_ID
	
	re1 := regexp.MustCompile(`/file/d/([a-zA-Z0-9_-]+)`)
	matches := re1.FindStringSubmatch(u)
	if len(matches) > 1 {
		return matches[1]
	}

	parsed, err := url.Parse(u)
	if err == nil {
		return parsed.Query().Get("id")
	}
	return ""
}

// --- OneDrive Resolver ---

type OneDriveResolver struct{}

func (r *OneDriveResolver) CanResolve(u string) bool {
	return onedriveRegex.MatchString(u)
}

func (r *OneDriveResolver) Resolve(u string) (string, map[string]string, error) {
	// Replace ?usp=sharing or similar with ?download=1
	// Or simply append &download=1 if not present.
	// OneDrive direct link usually works by appending `?download=1` 
	// or changing `embed` to `download`.

	parsed, err := url.Parse(u)
	if err != nil {
		return u, nil, err
	}

	q := parsed.Query()
	q.Set("download", "1")
	parsed.RawQuery = q.Encode()

	return parsed.String(), nil, nil
}

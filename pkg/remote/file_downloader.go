package remote

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type FileDownloader struct {
	Cookies map[string]string
	Headers map[string]string
}

// DownloadFile downloads a file with cookies and headers
func (p *FileDownloader) DownloadFile(ctx context.Context, url *url.URL) ([]byte, error) {
	req, err := p.newRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Probe checks that the URL still serves, fetching its first byte.
func (p *FileDownloader) Probe(ctx context.Context, url *url.URL) error {
	req, err := p.newRequest(ctx, url)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("%s: status code %d", url.Host, resp.StatusCode)
	}
	return nil
}

// newRequest is a GET carrying the headers and cookies the page's browser used.
func (p *FileDownloader) newRequest(ctx context.Context, url *url.URL) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	for key, value := range p.Headers {
		req.Header.Set(key, value)
	}

	if len(p.Cookies) > 0 {
		var cookieParts []string
		for key, value := range p.Cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", key, value))
		}
		req.Header.Set("Cookie", strings.Join(cookieParts, "; "))
	}
	return req, nil
}

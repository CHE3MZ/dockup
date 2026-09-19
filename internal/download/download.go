// Package download fetches the Debian nocloud tarball with MB progress.
package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Progress is called with downloaded and total bytes (-1 if unknown).
type Progress func(downloaded, total int64)

// Size issues a HEAD request for Content-Length. Returns -1 if unknown.
func Size(url string) int64 {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return -1
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.ContentLength
}

// Fetch streams url to dest, calling onProgress roughly every 500ms.
func Fetch(url, dest string, onProgress Progress) error {
	client := &http.Client{Timeout: 0}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	total := resp.ContentLength
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var done int64
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	buf := make([]byte, 1<<20)
	lastReport := time.Now()
	report := func() {
		if onProgress != nil {
			onProgress(done, total)
		}
	}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
		}
		select {
		case <-tick.C:
			report()
			lastReport = time.Now()
		default:
			if time.Since(lastReport) > 2*time.Second {
				report()
				lastReport = time.Now()
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return fmt.Errorf("download body: %w", rerr)
		}
	}
	report()
	return nil
}

// FormatMB renders bytes as whole MB.
func FormatMB(b int64) string {
	if b < 0 {
		return "?"
	}
	return fmt.Sprintf("%d", b/(1024*1024))
}

package internal

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// maxImageBytes caps the size of a single image accepted from disk or HTTP.
// 50 MB is enough for typical hi-res icons / artwork; larger files should be
// downsampled by the caller before import.
const maxImageBytes = 50 * 1024 * 1024

// httpFetchTimeout bounds an `imageUrl` download.
const httpFetchTimeout = 10 * time.Second

// ResolvedImage holds raw bytes plus metadata derived from the source.
type ResolvedImage struct {
	Bytes       []byte
	ContentHash string // sha256 hex of Bytes
	Source      string // "imageData", "imagePath", or "imageUrl"
}

// Base64Data returns the raw bytes encoded as a base64 string for transport
// over the JSON bridge. M3 will replace this with binary frames.
func (r *ResolvedImage) Base64Data() string {
	return base64.StdEncoding.EncodeToString(r.Bytes)
}

// ResolveImage extracts image bytes from an `import_image` argument map.
// Exactly one of `imageData` (base64), `imagePath` (local file), or
// `imageUrl` (HTTP/HTTPS) must be set.
func ResolveImage(ctx context.Context, args map[string]interface{}) (*ResolvedImage, error) {
	imgData, _ := args["imageData"].(string)
	imgPath, _ := args["imagePath"].(string)
	imgURL, _ := args["imageUrl"].(string)

	provided := 0
	if imgData != "" {
		provided++
	}
	if imgPath != "" {
		provided++
	}
	if imgURL != "" {
		provided++
	}

	if provided == 0 {
		return nil, errors.New("one of imageData, imagePath, or imageUrl is required")
	}
	if provided > 1 {
		return nil, errors.New("only one of imageData, imagePath, or imageUrl may be set")
	}

	switch {
	case imgData != "":
		bytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(imgData))
		if err != nil {
			// Some clients prepend a data: URL prefix — strip it and retry.
			if i := strings.Index(imgData, ","); i >= 0 && strings.HasPrefix(imgData, "data:") {
				bytes, err = base64.StdEncoding.DecodeString(strings.TrimSpace(imgData[i+1:]))
			}
			if err != nil {
				return nil, fmt.Errorf("invalid base64 imageData: %w", err)
			}
		}
		if len(bytes) > maxImageBytes {
			return nil, fmt.Errorf("imageData too large: %d bytes (max %d)", len(bytes), maxImageBytes)
		}
		return wrap(bytes, "imageData"), nil

	case imgPath != "":
		bytes, err := readFileCapped(imgPath)
		if err != nil {
			return nil, err
		}
		return wrap(bytes, "imagePath"), nil

	case imgURL != "":
		bytes, err := fetchURLCapped(ctx, imgURL)
		if err != nil {
			return nil, err
		}
		return wrap(bytes, "imageUrl"), nil
	}

	return nil, errors.New("unreachable")
}

func wrap(bytes []byte, source string) *ResolvedImage {
	sum := sha256.Sum256(bytes)
	return &ResolvedImage{
		Bytes:       bytes,
		ContentHash: hex.EncodeToString(sum[:]),
		Source:      source,
	}
}

func readFileCapped(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("imagePath: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("imagePath %q is a directory", path)
	}
	if info.Size() > maxImageBytes {
		return nil, fmt.Errorf("imagePath too large: %d bytes (max %d)", info.Size(), maxImageBytes)
	}
	return os.ReadFile(path)
}

func fetchURLCapped(ctx context.Context, url string) ([]byte, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("imageUrl must be http:// or https://")
	}
	c, cancel := context.WithTimeout(ctx, httpFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imageUrl fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imageUrl status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") {
		return nil, fmt.Errorf("imageUrl content-type %q is not image/*", ct)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("imageUrl read: %w", err)
	}
	if len(body) > maxImageBytes {
		return nil, fmt.Errorf("imageUrl too large (>%d bytes)", maxImageBytes)
	}
	return body, nil
}

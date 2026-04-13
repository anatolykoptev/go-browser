package browser

import (
	"encoding/base64"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// FailureSnapshot is the compact context captured automatically on action failure.
// Capped in size to avoid bloating responses on common failures.
type FailureSnapshot struct {
	URL          string `json:"url"`
	Title        string `json:"title,omitempty"`
	Snapshot     string `json:"snapshot,omitempty"`     // accessibility tree, depth=3, max 4 KB
	ScreenshotB64 string `json:"screenshot_b64,omitempty"` // JPEG 400x300, ~20 KB
}

const (
	failureSnapshotMaxChars = 4096
	failureThumbWidth       = 400
	failureThumbHeight      = 300
	failureThumbQuality     = 40
)

// CaptureFailureSnapshot grabs URL, title, short AXTree, and a thumbnail.
// Best-effort: returns what it can; partial failure still yields a useful snapshot.
func CaptureFailureSnapshot(page *rod.Page) *FailureSnapshot {
	if page == nil {
		return nil
	}
	fs := &FailureSnapshot{}

	if info, err := page.Info(); err == nil {
		fs.URL = info.URL
		fs.Title = info.Title
	}

	// Small AXTree for context (reuse existing snapshot infra).
	if s, err := captureSnapshotShort(page); err == nil {
		if len(s) > failureSnapshotMaxChars {
			s = s[:failureSnapshotMaxChars] + "\n…[truncated]"
		}
		fs.Snapshot = s
	}

	// Thumbnail JPEG.
	shot, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: ptrInt(failureThumbQuality),
		Clip: &proto.PageViewport{
			X: 0, Y: 0, Width: failureThumbWidth, Height: failureThumbHeight, Scale: 1,
		},
	})
	if err == nil && len(shot) > 0 {
		fs.ScreenshotB64 = base64.StdEncoding.EncodeToString(shot)
	}
	return fs
}

// captureSnapshotShort creates a compact accessibility snapshot with depth=3.
func captureSnapshotShort(page *rod.Page) (string, error) {
	return doSnapshot(page, 3, "", "", "", nil)
}

func ptrInt(v int) *int { return &v }

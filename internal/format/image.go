package format

import (
	"encoding/base64"
	"strings"
)

// ImageFormatter decodes base64-encoded image lines produced by the b64:
// pipeline control sequence and renders them as inline <img> elements.
// Each line must be a raw base64 string; the MIME type is inferred from
// the magic bytes of the decoded content.
type ImageFormatter struct{}

func (f *ImageFormatter) Name() string { return NameImage }

func (f *ImageFormatter) Format(lines []string) (string, error) {
	var b strings.Builder

	b.WriteString("<div class=\"rdw-image\">")

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}

		// Lines may carry the control prefix b64: — strip it.
		l = strings.TrimPrefix(l, "b64:")

		data, err := base64.StdEncoding.DecodeString(l)
		if err != nil {
			// Try URL-safe encoding.
			data, err = base64.RawURLEncoding.DecodeString(l)
			if err != nil {
				b.WriteString("<p class=\"rdw-image-err\">invalid base64 data</p>")
				continue
			}
		}

		mime := detectMIME(data)

		b.WriteString("<img class=\"rdw-image-img\" src=\"data:")
		b.WriteString(mime)
		b.WriteString(";base64,")
		b.WriteString(base64.StdEncoding.EncodeToString(data))
		b.WriteString("\" alt=\"rdw image\">")
	}

	b.WriteString("</div>")

	return b.String(), nil
}

// detectMIME returns the MIME type of data by inspecting its magic bytes.
func detectMIME(data []byte) string {
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}

	if len(data) >= 3 && string(data[:3]) == "\xff\xd8\xff" {
		return "image/jpeg"
	}

	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif"
	}

	if len(data) >= 4 && string(data[:4]) == "<svg" {
		return "image/svg+xml"
	}

	if len(data) > 5 && strings.HasPrefix(strings.TrimSpace(string(data[:100])), "<svg") {
		return "image/svg+xml"
	}

	// WebP: RIFF....WEBP
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}

	return "image/png" // safe default
}

package output

import (
	"encoding/json"
	"io"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func RenderJSON(w io.Writer, report domain.Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}

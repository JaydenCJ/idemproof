package report

import (
	"encoding/json"

	"github.com/JaydenCJ/idemproof/internal/fsdiff"
)

// RenderJSON renders the report as pretty-printed JSON with a stable key
// order and a trailing newline. Consumers should check schema_version
// before parsing further; additive changes bump it.
func RenderJSON(r *Report) (string, error) {
	// Normalize nil slices so the JSON shape is stable for consumers.
	for i := range r.Runs {
		if r.Runs[i].Changes == nil {
			r.Runs[i].Changes = []fsdiff.Change{}
		}
	}
	if r.Violations == nil {
		r.Violations = []string{}
	}
	buf, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf) + "\n", nil
}

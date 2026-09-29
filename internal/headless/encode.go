package headless

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/FelineStateMachine/012/internal/nuon"
)

// Encode writes v, a value of this package's result types, as JSON
// (indented, one value) or NUON, the forms --format json and nuon ask
// for. NUON is the JSON read as nushell reads it, so both carry the
// same schema (docs/reference/json.md).
func Encode(w io.Writer, format string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	switch format {
	case "json":
	case "nuon":
		val, err := nuon.Parse(data)
		if err != nil {
			return err
		}
		data = nuon.Append(nil, val)
	default:
		return fmt.Errorf("can't write %s", format)
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

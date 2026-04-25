// Package render pre-renders human-readable summaries for audit events at
// write time; the rendered string is stored directly in the audit_log row.
//
// IMPORTANT: renderer changes do NOT retroactively update existing rows.
// When adding a new (category, action) pair, register a new Renderer via
// register() in the appropriate init file. Do NOT change the wording of
// existing renderers — stored rows will not be updated and UI display will
// become inconsistent with historical data.
package render

import "fmt"

type Renderer func(before, after []byte) (summary, target string)

var registry = map[string]Renderer{}

func register(category, action string, r Renderer) {
	registry[category+":"+action] = r
}

func Render(category, action string, before, after []byte) (summary, target string) {
	if r, ok := registry[category+":"+action]; ok {
		return r(before, after)
	}
	return fmt.Sprintf("%s: %s", category, action), ""
}

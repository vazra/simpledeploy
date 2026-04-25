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

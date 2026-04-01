package terminal

import "fmt"

type Colorizer struct {
	enabled bool
}

func NewColorizer(features Features) Colorizer {
	return Colorizer{enabled: features.HasColor}
}

func (c Colorizer) Bold(value string) string {
	return c.wrap("1", value)
}

func (c Colorizer) Muted(value string) string {
	return c.wrap("2;37", value)
}

func (c Colorizer) Accent(value string) string {
	return c.wrap("1;36", value)
}

func (c Colorizer) Notice(value string) string {
	return c.wrap("1;34", value)
}

func (c Colorizer) Warning(value string) string {
	return c.wrap("1;33", value)
}

func (c Colorizer) Success(value string) string {
	return c.wrap("1;32", value)
}

func (c Colorizer) wrap(code, value string) string {
	if !c.enabled || value == "" {
		return value
	}

	return fmt.Sprintf("\033[%sm%s\033[0m", code, value)
}

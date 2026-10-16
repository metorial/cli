package commandutil

import "strconv"

func HumanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return strconv.FormatInt(size, 10) + "B"
	}
	value, suffix := float64(size), ""
	for _, next := range []string{"K", "M", "G", "T", "P"} {
		value /= unit
		suffix = next
		if value < unit {
			break
		}
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + suffix
}

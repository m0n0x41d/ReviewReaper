package utils

const (
	HH_MM = "15:04"
)

func IsContains[T comparable](elements []T, findThis T) bool {
	found := false
	for _, element := range elements {
		if findThis == element {
			found = true
		}
	}
	return found
}

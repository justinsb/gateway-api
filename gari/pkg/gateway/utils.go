package gateway

func withDefault[T any](p *T, defaultValue T) T {
	if p == nil {
		return defaultValue
	}
	return *p
}

func filter[T any](items []T, predicate func(t T) bool) []T {
	var keep []T
	for i := range items {
		if predicate(items[i]) {
			keep = append(keep, items[i])
		}
	}
	return keep
}

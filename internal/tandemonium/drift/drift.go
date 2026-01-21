package drift

func DetectDrift(allowed, changed []string) []string {
	allowedSet := make(map[string]struct{})
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	var drift []string
	for _, c := range changed {
		if _, ok := allowedSet[c]; !ok {
			drift = append(drift, c)
		}
	}
	return drift
}

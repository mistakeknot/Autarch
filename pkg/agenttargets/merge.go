package agenttargets

func Merge(global, project Registry) Registry {
	merged := Registry{Targets: map[string]Target{}}
	for k, v := range global.Targets {
		merged.Targets[k] = v
	}
	for k, v := range project.Targets {
		merged.Targets[k] = v
	}
	return merged
}

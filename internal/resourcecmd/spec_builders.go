package resourcecmd

func arg(name, target, description string, required bool) ArgumentSpec {
	return ArgumentSpec{
		Name:        name,
		Target:      target,
		Required:    required,
		Description: description,
	}
}

func stringFlag(name, target, usage string, required bool) FlagSpec {
	return FlagSpec{
		Name:     name,
		Type:     FlagString,
		Target:   target,
		Usage:    usage,
		Required: required,
	}
}

func boolFlag(name, target, usage string) FlagSpec {
	return FlagSpec{
		Name:   name,
		Type:   FlagBool,
		Target: target,
		Usage:  usage,
	}
}

func floatFlag(name, target, usage string) FlagSpec {
	return FlagSpec{
		Name:   name,
		Type:   FlagFloat,
		Target: target,
		Usage:  usage,
	}
}

func stringSliceFlag(name, target, usage string) FlagSpec {
	return FlagSpec{
		Name:     name,
		Type:     FlagStringSlice,
		Target:   target,
		Usage:    usage,
		Repeated: true,
	}
}

func jsonFlag(name, target, usage string) FlagSpec {
	return FlagSpec{
		Name:   name,
		Type:   FlagJSON,
		Target: target,
		Usage:  usage,
	}
}

func jsonFileFlag(name, target, usage string) FlagSpec {
	return FlagSpec{
		Name:   name,
		Type:   FlagJSONFile,
		Target: target,
		Usage:  usage,
	}
}

func paginationFlags(label string) []FlagSpec {
	return []FlagSpec{
		floatFlag("limit", "params.Limit", "Limit the number of "+label),
		stringFlag("after", "params.After", "Fetch "+label+" after this cursor", false),
		stringFlag("before", "params.Before", "Fetch "+label+" before this cursor", false),
		stringFlag("cursor", "params.Cursor", "Fetch "+label+" using an opaque cursor", false),
		stringFlag("order", "params.Order", "Sort order", false),
	}
}

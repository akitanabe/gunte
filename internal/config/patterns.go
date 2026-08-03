package config

import "regexp"

var (
	targetIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	termNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
	generalIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	fieldPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
)

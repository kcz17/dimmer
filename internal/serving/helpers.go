package serving

func prependLeadingSlashIfMissing(path string) string {
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}
	return path
}

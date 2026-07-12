package transloadit

// This file is generated from Transloadit API2 contracts. If it looks wrong,
// please report the issue instead of editing this file by hand; the source fix
// belongs in the contract generator so all SDKs stay in sync.

import "net/url"

func escapePathSegment(value string) string {
	return url.PathEscape(value)
}

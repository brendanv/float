//go:build !webui

package webui

import "net/http"

func Handler() http.Handler {
	return http.NotFoundHandler()
}

package client

import (
	"net/http"

	"github.com/darrenburns/posting/v3/internal/model"
)

// Populate the same cookie model used by Posting's response table. Add each
// header independently: Set-Cookie cannot be safely comma-joined (Expires has
// a comma), and multiple cookies may have the same name with different paths.
func postmanResponseCookies(response *model.Response) {
	headers := make(http.Header)
	for _, header := range response.Headers {
		headers.Add(header.Name, header.Value)
	}
	wire := &http.Response{Header: headers}
	response.Cookies = cookies(wire.Cookies())
}

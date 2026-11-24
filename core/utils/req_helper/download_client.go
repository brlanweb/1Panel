package req_helper

import (
	"net/http"
	"time"

	"github.com/1Panel-dev/1Panel/core/utils/xpack"
)

// NewDownloadClientWithProxy returns a verified-TLS client honouring the
// panel proxy settings, for long downloads (GitHub release assets redirect
// to objects.githubusercontent.com; redirects stay HTTPS-only).
func NewDownloadClientWithProxy(timeout time.Duration) *http.Client {
	return newExternalHTTPClient(xpack.MultiNodeProvider.LoadRequestTransport(), timeout)
}

// NewDownloadClientWithTransport is NewDownloadClientWithProxy with an
// explicit transport (nil = the panel proxy settings).
func NewDownloadClientWithTransport(transport *http.Transport, timeout time.Duration) *http.Client {
	if transport == nil {
		transport = xpack.MultiNodeProvider.LoadRequestTransport()
	}
	return newExternalHTTPClient(transport, timeout)
}

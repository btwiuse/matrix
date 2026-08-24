package matrix

import (
	"fmt"
	"io"
	"net/http"
)

// DeployAPI serves POST /api/deploy: the request body is a .tar.gz or .zip
// archive (up to 64 MiB) and the response is the same deploy result JSON as
// the remote_deploy tool. This is the plain-HTTP counterpart of the MCP
// tools for callers holding the archive locally: one curl command instead
// of base64-in-JSON uploads.
type DeployAPI struct {
	deploy *LocalDeploy
}

// NewDeployAPI builds the upload endpoint around a LocalDeploy.
func NewDeployAPI(d *LocalDeploy) *DeployAPI {
	return &DeployAPI{deploy: d}
}

// ServeHTTP implements the endpoint. It only accepts POST; anything else is
// a 405.
func (a *DeployAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20+1))
	if err != nil {
		http.Error(w, fmt.Sprintf("reading body: %v", err), http.StatusBadRequest)
		return
	}
	if len(data) > 64<<20 {
		http.Error(w, "archive exceeds 64 MiB", http.StatusRequestEntityTooLarge)
		return
	}

	out, err := a.deploy.publishBytes(data)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

package routes

import (
	"net/http"
	"testing"

	util "github.com/redhat-appstudio/managed-gitops/backend/util"
)

func TestServer(t *testing.T) {
	serverURL := "http://localhost:8090"
	go func() {
		RunRestfulCurlyRouterServer()
	}()
	if err := util.WaitForServerUp(serverURL); err != nil {
		t.Errorf("%v", err)
	}

	// GET should successfully pass with 200
	resp, err := http.Get(serverURL + "/api/v1/application/")
	if err != nil {
		t.Errorf("unexpected error in GET /api/v1/application/: %v", err)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}

	// //Can't test it right now, unpatch it when you have a post request
	// // Test that GET works.
	// resp, err = http.Get(serverURL + "/api/v1/application/1")
	// if err != nil {
	// 	t.Errorf("unexpected error in GET /api/v1/application/1: %v", err)
	// }
	// if resp.StatusCode != http.StatusOK {
	// 	t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	// }
}

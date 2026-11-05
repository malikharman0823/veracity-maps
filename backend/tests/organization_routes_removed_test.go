package tests

import (
	"net/http"
	"testing"
)

func TestOrganizationRoutesAreRemoved(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer cleanupTestDir(t, tempDir)

	response, err := http.Get(server.URL + "/api/organizations")
	if err != nil {
		t.Fatalf("request organization route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected removed organization route to return 404, got %d", response.StatusCode)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/store"
)

const sampleBindingYAML = `contract_id: dungeon-session-list
contract_version: "1.0"
proto:
  service: SessionService
  method: List
  file: proto/sessions.proto
  request: ListSessionsRequest
  response: ListSessionsResponse
`

func TestBindings_CRUDFlow(t *testing.T) {
	projDir := t.TempDir()
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject(projDir)
	ts := setupTestServerWithOpts(t, testServerOptions{store: sessionStore})
	defer ts.Close()

	projName := filepath.Base(projDir)

	t.Run("GET 列表：初始為空", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/bindings/" + projName)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var out struct {
			ContractIDs []string `json:"contractIds"`
		}
		json.NewDecoder(resp.Body).Decode(&out)
		if len(out.ContractIDs) != 0 {
			t.Errorf("expected empty, got %v", out.ContractIDs)
		}
	})

	t.Run("GET 單筆：不存在時 404", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/bindings/" + projName + "/missing-id")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("PUT：寫入有效 binding", func(t *testing.T) {
		req, _ := http.NewRequest("PUT",
			ts.URL+"/api/bindings/"+projName+"/dungeon-session-list",
			bytes.NewBufferString(sampleBindingYAML))
		req.Header.Set("Content-Type", "text/yaml")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, body)
		}
		path, _ := plan.BindingPath(projDir, "dungeon-session-list")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("binding file not written: %v", err)
		}
	})

	t.Run("GET 單筆：寫入後可讀回", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/bindings/" + projName + "/dungeon-session-list")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "SessionService") {
			t.Errorf("response missing SessionService: %s", body)
		}
	})

	t.Run("GET 列表：寫入後包含該 id", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/bindings/" + projName)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		var out struct {
			ContractIDs []string `json:"contractIds"`
		}
		json.NewDecoder(resp.Body).Decode(&out)
		if len(out.ContractIDs) != 1 || out.ContractIDs[0] != "dungeon-session-list" {
			t.Errorf("unexpected list: %v", out.ContractIDs)
		}
	})

	t.Run("PUT：缺 proto 欄位時 400", func(t *testing.T) {
		bad := `contract_id: bad-contract
contract_version: "1.0"
`
		req, _ := http.NewRequest("PUT",
			ts.URL+"/api/bindings/"+projName+"/bad-contract",
			bytes.NewBufferString(bad))
		req.Header.Set("Content-Type", "text/yaml")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("PUT：body id 與 URL id 不一致時 400", func(t *testing.T) {
		req, _ := http.NewRequest("PUT",
			ts.URL+"/api/bindings/"+projName+"/different-id",
			bytes.NewBufferString(sampleBindingYAML))
		req.Header.Set("Content-Type", "text/yaml")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("DELETE：刪除後 GET 回 404", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE",
			ts.URL+"/api/bindings/"+projName+"/dungeon-session-list", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		// Confirm gone
		resp2, _ := http.Get(ts.URL + "/api/bindings/" + projName + "/dungeon-session-list")
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusNotFound {
			t.Fatalf("after delete: status = %d, want 404", resp2.StatusCode)
		}
	})

	t.Run("DELETE：不存在時 404", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE",
			ts.URL+"/api/bindings/"+projName+"/never-existed", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})
}

func TestBindings_UnknownProject404(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/bindings/unknown-project")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/server/internal/cache/blob"
	"github.com/ollama/ollama/server/internal/testutil"
	"github.com/ollama/ollama/types/model"
)

func TestDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	s := newServerTester(t)

	// make a minimal model that CreateHandler will accept. No tensors,
	// just a config.
	_, digest := createBinFile(t, nil, nil)

	// Create the model
	got := callHandler(t, s.CreateHandler, api.CreateRequest{
		Name:  "test",
		Files: map[string]string{"test.gguf": digest},
	})
	if got.Code != http.StatusOK {
		t.Fatalf("expected status code 200, actual %d", got.Code)
	}

	// Create a second model that utilizes the same layers. This will be
	// used to check that when we delete a model, we don't delete layers
	// that are shared with other models.
	got = callHandler(t, s.CreateHandler, api.CreateRequest{
		Name:     "test2",
		Files:    map[string]string{"test.gguf": digest},
		Template: "{{ .System }} {{ .Prompt }}",
	})
	if got.Code != http.StatusOK {
		t.Fatalf("expected status code 200, actual %d", got.Code)
	}
	checkFileExists(t, filepath.Join("manifests", "*", "*", "*", "*"), []string{
		filepath.Join("manifests", "registry.ollama.ai", "library", "test", "latest"),
		filepath.Join("manifests", "registry.ollama.ai", "library", "test2", "latest"),
	})
	checkFileExists(t, filepath.Join("blobs", "*"), []string{
		filepath.Join("blobs", "sha256-8f2c2167d789c6b2302dff965160fa5029f6a24096d262c1cbb469f21a045382"),
		filepath.Join("blobs", "sha256-a4e5e156ddec27e286f75328784d7106b60a4eb1d246e950a001a3f944fbda99"),
		filepath.Join("blobs", "sha256-ca239d7bd8ea90e4a5d2e6bf88f8d74a47b14336e73eb4e18bed4dd325018116"),
		filepath.Join("blobs", "sha256-fe7ac77b725cda2ccad03f88a880ecdfd7a33192d6cae08fce2c0ee1455991ed"),
	})

	// Delete the first model, and then check we didn't delete the layers
	// we shared with the second model.
	got = callHandler(t, gin.WrapH(s.handle(s.handleModelDelete)), api.DeleteRequest{Name: "test"})
	if got.Code != http.StatusOK {
		t.Fatalf("res.Status = %d, want %d", got.Code, 200)
	}
	checkFileExists(t, filepath.Join("manifests", "*", "*", "*", "*"), []string{
		filepath.Join("manifests", "registry.ollama.ai", "library", "test2", "latest"),
	})
	checkFileExists(t, filepath.Join("blobs", "*"), []string{
		filepath.Join("blobs", "sha256-8f2c2167d789c6b2302dff965160fa5029f6a24096d262c1cbb469f21a045382"),
		filepath.Join("blobs", "sha256-a4e5e156ddec27e286f75328784d7106b60a4eb1d246e950a001a3f944fbda99"),
		filepath.Join("blobs", "sha256-fe7ac77b725cda2ccad03f88a880ecdfd7a33192d6cae08fce2c0ee1455991ed"),
	})

	// Delete the second model, and then check the layers are deleted.
	got = callHandler(t, gin.WrapH(s.handle(s.handleModelDelete)), api.DeleteRequest{Name: "test2"})
	if got.Code != http.StatusOK {
		t.Fatalf("expected status code 200, actual %d", got.Code)
	}
	checkFileExists(t, filepath.Join("manifests", "*", "*", "*", "*"), []string{})
	checkFileExists(t, filepath.Join("blobs", "*"), []string{})
}

func TestDeleteDuplicateLayers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := t.TempDir()
	t.Setenv("OLLAMA_MODELS", p)

	s := newServerTester(t)

	n := model.ParseName("test")

	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(&ConfigV2{}); err != nil {
		t.Fatal(err)
	}

	config, err := NewLayer(&b, "application/vnd.docker.container.image.v1+json")
	if err != nil {
		t.Fatal(err)
	}

	// create a manifest with duplicate layers
	if err := WriteManifest(n, config, []Layer{config}); err != nil {
		t.Fatal(err)
	}

	w := callHandler(t, gin.WrapH(s.handle(s.handleModelDelete)), api.DeleteRequest{Name: "test"})
	if w.Code != http.StatusOK {
		t.Errorf("expected status code 200, actual %d", w.Code)
	}

	checkFileExists(t, filepath.Join(p, "manifests", "*", "*", "*", "*"), []string{})
}

func newServerTester(t *testing.T) *Server {
	dir := t.TempDir()
	t.Chdir(dir)
	c, err := blob.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OLLAMA_MODELS", dir) // TODO(bmizerany): remove this
	return &Server{
		log:   testutil.Slogger(t),
		cache: c,
	}
}

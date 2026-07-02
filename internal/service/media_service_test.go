package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.orx.me/apps/hyper-sync/internal/media"
	"go.orx.me/apps/hyper-sync/internal/service"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
	"go.orx.me/apps/hyper-sync/pkg/proto/api/v1/v1connect"
)

type mediaTestEnv struct {
	mediaClient   v1connect.MediaServiceClient
	uploadURL     string
	server        *httptest.Server
	objectStorage *media.MemoryObjectStorage
}

func setupMediaTest(t *testing.T) (*mediaTestEnv, func()) {
	t.Helper()

	store := media.NewMemoryStore()
	objectStorage := media.NewMemoryObjectStorage()
	cdnDomain := "https://cdn.example.com"

	svc := service.NewMediaService(store, objectStorage, cdnDomain)

	mux := http.NewServeMux()
	path, handler := v1connect.NewMediaServiceHandler(svc)
	mux.Handle(path, handler)
	mux.HandleFunc("POST /api/media/upload", svc.HandleUpload)

	server := httptest.NewServer(mux)
	client := v1connect.NewMediaServiceClient(server.Client(), server.URL)

	env := &mediaTestEnv{
		mediaClient:   client,
		uploadURL:     server.URL + "/api/media/upload",
		server:        server,
		objectStorage: objectStorage,
	}

	return env, server.Close
}

func uploadFile(t *testing.T, url string, filename string, contentType string, content []byte) *service.UploadResponse {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest("POST", url, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result service.UploadResponse
	require.NoError(t, json.Unmarshal(body, &result))
	return &result
}

func TestUploadMedia_ReturnsIDAndCDNUrl(t *testing.T) {
	env, cleanup := setupMediaTest(t)
	defer cleanup()

	content := []byte("fake image data")
	result := uploadFile(t, env.uploadURL, "photo.jpg", "image/jpeg", content)

	assert.NotEmpty(t, result.ID)
	assert.Contains(t, result.CDNUrl, "https://cdn.example.com/")
	assert.Equal(t, "image/jpeg", result.ContentType)
	assert.Equal(t, int64(len(content)), result.SizeBytes)
}

func TestUploadMedia_StoresFileInObjectStorage(t *testing.T) {
	env, cleanup := setupMediaTest(t)
	defer cleanup()

	content := []byte("file bytes")
	result := uploadFile(t, env.uploadURL, "test.png", "image/png", content)

	assert.NotEmpty(t, result.S3Key)
	assert.True(t, env.objectStorage.Has(result.S3Key))
}

func TestGetMedia_AfterUpload_ReturnsMetadata(t *testing.T) {
	env, cleanup := setupMediaTest(t)
	defer cleanup()

	uploaded := uploadFile(t, env.uploadURL, "doc.pdf", "application/pdf", []byte("pdf content"))

	resp, err := env.mediaClient.GetMedia(context.Background(), connect.NewRequest(&v1.GetMediaRequest{
		Id: uploaded.ID,
	}))

	require.NoError(t, err)
	assert.Equal(t, uploaded.ID, resp.Msg.Media.Id)
	assert.Equal(t, uploaded.CDNUrl, resp.Msg.Media.CdnUrl)
	assert.Equal(t, "application/pdf", resp.Msg.Media.ContentType)
	assert.Equal(t, "doc.pdf", resp.Msg.Media.OriginalFilename)
}

func TestListMedia_ReturnsPaginatedResults(t *testing.T) {
	env, cleanup := setupMediaTest(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		uploadFile(t, env.uploadURL, "file.jpg", "image/jpeg", []byte("data"))
	}

	resp, err := env.mediaClient.ListMedia(context.Background(), connect.NewRequest(&v1.ListMediaRequest{
		PageSize: 10,
		Page:     1,
	}))

	require.NoError(t, err)
	assert.Equal(t, int32(3), resp.Msg.Total)
	assert.Len(t, resp.Msg.Items, 3)
}

func TestDeleteMedia_RemovesFromStoreAndS3(t *testing.T) {
	env, cleanup := setupMediaTest(t)
	defer cleanup()

	uploaded := uploadFile(t, env.uploadURL, "todelete.jpg", "image/jpeg", []byte("data"))
	assert.True(t, env.objectStorage.Has(uploaded.S3Key))

	_, err := env.mediaClient.DeleteMedia(context.Background(), connect.NewRequest(&v1.DeleteMediaRequest{
		Id: uploaded.ID,
	}))
	require.NoError(t, err)

	assert.False(t, env.objectStorage.Has(uploaded.S3Key))

	_, err = env.mediaClient.GetMedia(context.Background(), connect.NewRequest(&v1.GetMediaRequest{
		Id: uploaded.ID,
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

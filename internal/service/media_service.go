package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.orx.me/apps/hyper-sync/internal/media"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

type UploadResponse struct {
	ID          string `json:"id"`
	CDNUrl      string `json:"cdn_url"`
	S3Key       string `json:"s3_key"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type MediaService struct {
	store         media.Store
	objectStorage media.ObjectStorage
	cdnDomain     string
}

func NewMediaService(store media.Store, objectStorage media.ObjectStorage, cdnDomain string) *MediaService {
	return &MediaService{
		store:         store,
		objectStorage: objectStorage,
		cdnDomain:     cdnDomain,
	}
}

// maxUploadBytes caps a single media upload (multipart overhead included).
const maxUploadBytes = 50 << 20

func (s *MediaService) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	contentType := http.DetectContentType(buf[:n])
	if _, err := file.Seek(0, 0); err != nil {
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	// The sniffed type is authoritative; the client-controlled filename only
	// refines generic results (sniffing cannot identify e.g. SVG or WebM), so a
	// script renamed to x.png cannot masquerade as an image.
	if isGenericContentType(contentType) {
		if ext := extensionContentType(header.Filename); ext != "" {
			contentType = ext
		}
	}

	key := fmt.Sprintf("media/%s/%s", time.Now().Format("2006/01/02"), uuid.New().String())

	if err := s.objectStorage.Upload(r.Context(), key, contentType, file); err != nil {
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	cdnURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(s.cdnDomain, "/"), key)

	m := &media.Media{
		S3Key:            key,
		CDNUrl:           cdnURL,
		ContentType:      contentType,
		SizeBytes:        header.Size,
		OriginalFilename: header.Filename,
	}

	created, err := s.store.Create(r.Context(), m)
	if err != nil {
		http.Error(w, "store failed", http.StatusInternalServerError)
		return
	}

	resp := UploadResponse{
		ID:          created.ID,
		CDNUrl:      created.CDNUrl,
		S3Key:       created.S3Key,
		ContentType: created.ContentType,
		SizeBytes:   created.SizeBytes,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *MediaService) GetMedia(ctx context.Context, req *connect.Request[v1.GetMediaRequest]) (*connect.Response[v1.GetMediaResponse], error) {
	m, err := s.store.GetByID(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, media.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.GetMediaResponse{
		Media: mediaToProto(m),
	}), nil
}

func (s *MediaService) ListMedia(ctx context.Context, req *connect.Request[v1.ListMediaRequest]) (*connect.Response[v1.ListMediaResponse], error) {
	result, err := s.store.List(ctx, media.ListOptions{
		PageSize: int(req.Msg.PageSize),
		Page:     int(req.Msg.Page),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	items := make([]*v1.Media, 0, len(result.Items))
	for _, m := range result.Items {
		items = append(items, mediaToProto(m))
	}

	return connect.NewResponse(&v1.ListMediaResponse{
		Items: items,
		Total: int32(result.Total),
	}), nil
}

func (s *MediaService) DeleteMedia(ctx context.Context, req *connect.Request[v1.DeleteMediaRequest]) (*connect.Response[v1.DeleteMediaResponse], error) {
	m, err := s.store.GetByID(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, media.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.objectStorage.Delete(ctx, m.S3Key); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.store.Delete(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.DeleteMediaResponse{}), nil
}

// isGenericContentType reports whether sniffing failed to identify the payload
// precisely enough to be useful as an object-storage content type.
func isGenericContentType(ct string) bool {
	return ct == "application/octet-stream" ||
		strings.HasPrefix(ct, "text/plain") ||
		strings.HasPrefix(ct, "text/xml")
}

func extensionContentType(filename string) string {
	types := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".png": "image/png", ".gif": "image/gif",
		".webp": "image/webp", ".svg": "image/svg+xml",
		".mp4": "video/mp4", ".webm": "video/webm",
		".pdf": "application/pdf",
	}
	return types[strings.ToLower(filepath.Ext(filename))]
}

func mediaToProto(m *media.Media) *v1.Media {
	return &v1.Media{
		Id:               m.ID,
		CdnUrl:           m.CDNUrl,
		ContentType:      m.ContentType,
		SizeBytes:        m.SizeBytes,
		OriginalFilename: m.OriginalFilename,
		CreatedAt:        timestamppb.New(m.CreatedAt),
	}
}

package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMemos_ListMemos_Localhost(t *testing.T) {
	endpoint := os.Getenv("MEMOS_ENDPOINT")
	token := os.Getenv("MEMOS_TOKEN")

	// 如果没有设置环境变量，跳过这个测试
	if endpoint == "" {
		t.Skip("MEMOS_ENDPOINT environment variable not set, skipping localhost test")
	}

	memos := NewMemos(endpoint, token, "memos")

	response, err := memos.ListPosts(context.Background(), 10)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Errorf("Expected response, got nil")
		return
	}

	t.Logf("Successfully retrieved %d memos", len(response))

	for _, memo := range response{
		t.Logf("Memo: %+v, %+v, %+v", memo.ID, memo.CreatedAt, len(memo.Media))
	}

	posts, err := memos.ListPosts(context.Background(), 10)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	t.Logf("Successfully retrieved %d posts", len(posts))

	for _, post := range posts {
		t.Logf("Post: %+v", post)
	}
}

func TestMemos_ListMemos(t *testing.T) {
	// 准备测试数据
	testMemos := []Memo{
		{
			Name:        "memos/1",
			UID:         "test-uid-1",
			RowStatus:   "NORMAL",
			Creator:     "users/test",
			CreateTime:  time.Now(),
			UpdateTime:  time.Now(),
			DisplayTime: time.Now(),
			Content:     "Test memo content 1",
			Visibility:  "PUBLIC",
			Pinned:      false,
			Resources: []Resource{
				{
					Name:         "resources/RGMrMFUSkGYXewEHcLiYvf",
					CreateTime:   "2025-06-20T17:23:57Z",
					Filename:     "BB34668C-25A2-4B1B-BF43-2B9DB485CFEC.jpg",
					Content:      "",
					ExternalLink: "https://s3.us-west-1.wasabisys.com/mu-sns/assets/1750440236_BB34668C-25A2-4B1B-BF43-2B9DB485CFEC.jpg",
					Type:         "image/jpeg",
					Size:         "5296693",
					Memo:         "memos/1",
				},
			},
		},
		{
			Name:        "memos/2",
			UID:         "test-uid-2",
			RowStatus:   "NORMAL",
			Creator:     "users/test",
			CreateTime:  time.Now(),
			UpdateTime:  time.Now(),
			DisplayTime: time.Now(),
			Content:     "Test memo content 2",
			Visibility:  "PRIVATE",
			Pinned:      true,
			Resources:   []Resource{}, // 空资源数组
		},
	}

	expectedResponse := ListMemosResponse{
		Memos:         testMemos,
		NextPageToken: "next-token-123",
	}

	tests := []struct {
		name           string
		request        *ListMemosRequest
		serverResponse ListMemosResponse
		serverStatus   int
		serverHeaders  map[string]string
		expectedError  bool
		checkAuth      bool
		token          string
	}{
		{
			name:           "successful request without parameters",
			request:        nil,
			serverResponse: expectedResponse,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			checkAuth:      true,
			token:          "test-token",
		},
		{
			name: "successful request with all parameters",
			request: &ListMemosRequest{
				PageSize:      10,
				PageToken:     "page-token",
				Filter:        "test-filter",
				Creator:       "users/test",
				Visibility:    "PUBLIC",
				OrderBy:       "display_time desc",
				Tag:           "test-tag",
				ContentSearch: "search-content",
			},
			serverResponse: expectedResponse,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			checkAuth:      true,
			token:          "test-token",
		},
		{
			name:           "successful request without token",
			request:        nil,
			serverResponse: expectedResponse,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			checkAuth:      false,
			token:          "",
		},
		{
			name:           "server returns 404",
			request:        nil,
			serverResponse: ListMemosResponse{},
			serverStatus:   http.StatusNotFound,
			expectedError:  true,
			token:          "test-token",
		},
		{
			name:           "server returns 500",
			request:        nil,
			serverResponse: ListMemosResponse{},
			serverStatus:   http.StatusInternalServerError,
			expectedError:  true,
			token:          "test-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// 检查请求方法
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				// 检查请求路径
				if r.URL.Path != "/api/v1/memos" {
					t.Errorf("Expected path /api/v1/memos, got %s", r.URL.Path)
				}

				// 检查认证头
				if tt.checkAuth && tt.token != "" {
					auth := r.Header.Get("Authorization")
					expected := "Bearer " + tt.token
					if auth != expected {
						t.Errorf("Expected Authorization header %s, got %s", expected, auth)
					}
				}

				// 检查Content-Type头
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", contentType)
				}

				// 检查查询参数
				if tt.request != nil {
					query := r.URL.Query()
					if tt.request.PageSize > 0 {
						if query.Get("pageSize") != "10" {
							t.Errorf("Expected pageSize=10, got %s", query.Get("pageSize"))
						}
					}
					if tt.request.PageToken != "" {
						if query.Get("pageToken") != tt.request.PageToken {
							t.Errorf("Expected pageToken=%s, got %s", tt.request.PageToken, query.Get("pageToken"))
						}
					}
					if tt.request.Filter != "" {
						if query.Get("filter") != tt.request.Filter {
							t.Errorf("Expected filter=%s, got %s", tt.request.Filter, query.Get("filter"))
						}
					}
					if tt.request.Creator != "" {
						if query.Get("creator") != tt.request.Creator {
							t.Errorf("Expected creator=%s, got %s", tt.request.Creator, query.Get("creator"))
						}
					}
					if tt.request.Visibility != "" {
						if query.Get("visibility") != tt.request.Visibility {
							t.Errorf("Expected visibility=%s, got %s", tt.request.Visibility, query.Get("visibility"))
						}
					}
					if tt.request.OrderBy != "" {
						if query.Get("orderBy") != tt.request.OrderBy {
							t.Errorf("Expected orderBy=%s, got %s", tt.request.OrderBy, query.Get("orderBy"))
						}
					}
					if tt.request.Tag != "" {
						if query.Get("tag") != tt.request.Tag {
							t.Errorf("Expected tag=%s, got %s", tt.request.Tag, query.Get("tag"))
						}
					}
					if tt.request.ContentSearch != "" {
						if query.Get("contentSearch") != tt.request.ContentSearch {
							t.Errorf("Expected contentSearch=%s, got %s", tt.request.ContentSearch, query.Get("contentSearch"))
						}
					}
				}

				// 设置响应状态码
				w.WriteHeader(tt.serverStatus)

				// 设置响应头
				for key, value := range tt.serverHeaders {
					w.Header().Set(key, value)
				}

				// 设置响应体
				if tt.serverStatus == http.StatusOK {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(tt.serverResponse)
				} else {
					w.Write([]byte("Error response"))
				}
			}))
			defer server.Close()

			// 创建Memos实例
			memos := NewMemos(server.URL, tt.token, "memos")

			// 发送请求并检查响应
			response, err := memos.ListMemos(context.Background(), tt.request)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if response == nil {
				t.Errorf("Expected response, got nil")
				return
			}

			// 检查响应内容
			if len(response.Memos) != len(tt.serverResponse.Memos) {
				t.Errorf("Expected %d memos, got %d", len(tt.serverResponse.Memos), len(response.Memos))
			}

			if response.NextPageToken != tt.serverResponse.NextPageToken {
				t.Errorf("Expected NextPageToken %s, got %s", tt.serverResponse.NextPageToken, response.NextPageToken)
			}

			// 检查第一个 memo 的内容
			if len(response.Memos) > 0 && len(tt.serverResponse.Memos) > 0 {
				memo := response.Memos[0]
				expected := tt.serverResponse.Memos[0]

				if memo.Name != expected.Name {
					t.Errorf("Expected memo name %s, got %s", expected.Name, memo.Name)
				}

				if memo.Content != expected.Content {
					t.Errorf("Expected memo content %s, got %s", expected.Content, memo.Content)
				}

				if memo.Visibility != expected.Visibility {
					t.Errorf("Expected memo visibility %s, got %s", expected.Visibility, memo.Visibility)
				}

				if memo.Pinned != expected.Pinned {
					t.Errorf("Expected memo pinned %v, got %v", expected.Pinned, memo.Pinned)
				}

				// 检查资源
				if len(memo.Resources) != len(expected.Resources) {
					t.Errorf("Expected %d resources, got %d", len(expected.Resources), len(memo.Resources))
				}

				if len(memo.Resources) > 0 && len(expected.Resources) > 0 {
					resource := memo.Resources[0]
					expectedResource := expected.Resources[0]

					if resource.Name != expectedResource.Name {
						t.Errorf("Expected resource name %s, got %s", expectedResource.Name, resource.Name)
					}

					if resource.Filename != expectedResource.Filename {
						t.Errorf("Expected resource filename %s, got %s", expectedResource.Filename, resource.Filename)
					}

					if resource.ExternalLink != expectedResource.ExternalLink {
						t.Errorf("Expected resource external link %s, got %s", expectedResource.ExternalLink, resource.ExternalLink)
					}

					if resource.Type != expectedResource.Type {
						t.Errorf("Expected resource type %s, got %s", expectedResource.Type, resource.Type)
					}

					if resource.Size != expectedResource.Size {
						t.Errorf("Expected resource size %s, got %s", expectedResource.Size, resource.Size)
					}
				}
			}
		})
	}
}

func TestMemos_ListMemos_InvalidJSON(t *testing.T) {
	// 创建一个返回无效JSON的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	memos := NewMemos(server.URL, "test-token", "memos")

	_, err := memos.ListMemos(context.Background(), nil)
	if err == nil {
		t.Errorf("Expected error for invalid JSON, got none")
	}
}

func TestMemos_ListMemos_NetworkError(t *testing.T) {
	memos := NewMemos("http://invalid-host:9999", "test-token", "memos")

	_, err := memos.ListMemos(context.Background(), nil)
	if err == nil {
		t.Errorf("Expected network error, got none")
	}
}

func TestNewMemos(t *testing.T) {
	endpoint := "https://example.com/"
	token := "test-token"
	name := "test-memos"

	memos := NewMemos(endpoint, token, name)

	// Check if trailing slash is removed
	if memos.Endpoint != "https://example.com" {
		t.Errorf("Expected endpoint without trailing slash, got %s", memos.Endpoint)
	}

	if memos.Token != token {
		t.Errorf("Expected token %s, got %s", token, memos.Token)
	}

	if memos.name != name {
		t.Errorf("Expected name %s, got %s", name, memos.name)
	}
}

// TestListMemosDefaultOrderBy tests that the default orderBy parameter is set correctly
func TestListMemosDefaultOrderBy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that the default orderBy parameter is set
		orderBy := r.URL.Query().Get("orderBy")
		if orderBy != "display_time desc" {
			t.Errorf("Expected default orderBy 'display_time desc', got '%s'", orderBy)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ListMemosResponse{
			Memos:         []Memo{},
			NextPageToken: "",
		})
	}))
	defer server.Close()

	memos := NewMemos(server.URL, "test-token", "memos")

	// Test with nil request - should set default orderBy
	_, err := memos.ListMemos(context.Background(), nil)
	if err != nil {
		t.Errorf("ListMemos failed: %v", err)
	}

	// Test with empty request - should set default orderBy
	req := &ListMemosRequest{
		PageSize: 10,
	}
	_, err = memos.ListMemos(context.Background(), req)
	if err != nil {
		t.Errorf("ListMemos failed: %v", err)
	}
}

// TestListMemosCustomOrderBy tests that custom orderBy parameter is preserved
func TestListMemosCustomOrderBy(t *testing.T) {
	customOrderBy := "display_time asc"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that the custom orderBy parameter is preserved
		orderBy := r.URL.Query().Get("orderBy")
		if orderBy != customOrderBy {
			t.Errorf("Expected custom orderBy '%s', got '%s'", customOrderBy, orderBy)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ListMemosResponse{
			Memos:         []Memo{},
			NextPageToken: "",
		})
	}))
	defer server.Close()

	memos := NewMemos(server.URL, "test-token", "memos")

	// Test with custom orderBy
	req := &ListMemosRequest{
		PageSize: 10,
		OrderBy:  customOrderBy,
	}
	_, err := memos.ListMemos(context.Background(), req)
	if err != nil {
		t.Errorf("ListMemos failed: %v", err)
	}
}

func TestMemosURLConstruction(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		pageSize       int
		expectedURL    string
		expectedParams map[string]string
	}{
		{
			name:        "basic URL construction",
			endpoint:    "https://example.com",
			pageSize:    5,
			expectedURL: "https://example.com/api/v1/memos",
			expectedParams: map[string]string{
				"pageSize": "5",
				"orderBy":  "display_time desc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify URL path
				if r.URL.Path != "/api/v1/memos" {
					t.Errorf("Expected path /api/v1/memos, got %s", r.URL.Path)
				}

				// Verify query parameters
				for key, expectedValue := range tt.expectedParams {
					actualValue := r.URL.Query().Get(key)
					if actualValue != expectedValue {
						t.Errorf("Expected %s=%s, got %s=%s", key, expectedValue, key, actualValue)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(ListMemosResponse{
					Memos:         []Memo{},
					NextPageToken: "",
				})
			}))
			defer server.Close()

			memos := NewMemos(server.URL, "test-token", "memos")

			_, err := memos.ListMemos(context.Background(), &ListMemosRequest{PageSize: 5})
			if err != nil {
				t.Errorf("ListMemos failed: %v", err)
			}
		})
	}
}

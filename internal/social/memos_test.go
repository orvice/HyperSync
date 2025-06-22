package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

	response, err := memos.ListMemos(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Errorf("Expected response, got nil")
		return
	}

	t.Logf("Successfully retrieved %d memos", len(response.Memos))

	for _, memo := range response.Memos {
		t.Logf("Memo: %+v, %+v, %+v", memo.Name, memo.CreateTime, len(memo.Resources))
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

			// 执行测试
			response, err := memos.ListMemos(tt.request)

			// 检查错误
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

			// 检查响应
			if response == nil {
				t.Errorf("Expected response, but got nil")
				return
			}

			// 检查响应内容
			if len(response.Memos) != len(tt.serverResponse.Memos) {
				t.Errorf("Expected %d memos, got %d", len(tt.serverResponse.Memos), len(response.Memos))
			}

			if response.NextPageToken != tt.serverResponse.NextPageToken {
				t.Errorf("Expected nextPageToken %s, got %s", tt.serverResponse.NextPageToken, response.NextPageToken)
			}

			// 检查第一个memo的内容
			if len(response.Memos) > 0 && len(tt.serverResponse.Memos) > 0 {
				expectedMemo := tt.serverResponse.Memos[0]
				actualMemo := response.Memos[0]

				if actualMemo.Name != expectedMemo.Name {
					t.Errorf("Expected memo name %s, got %s", expectedMemo.Name, actualMemo.Name)
				}
				if actualMemo.Content != expectedMemo.Content {
					t.Errorf("Expected memo content %s, got %s", expectedMemo.Content, actualMemo.Content)
				}
				if actualMemo.Visibility != expectedMemo.Visibility {
					t.Errorf("Expected memo visibility %s, got %s", expectedMemo.Visibility, actualMemo.Visibility)
				}
				if actualMemo.Pinned != expectedMemo.Pinned {
					t.Errorf("Expected memo pinned %v, got %v", expectedMemo.Pinned, actualMemo.Pinned)
				}

				// 检查资源
				if len(actualMemo.Resources) != len(expectedMemo.Resources) {
					t.Errorf("Expected %d resources, got %d", len(expectedMemo.Resources), len(actualMemo.Resources))
				}

				// 如果有资源，检查第一个资源的详细信息
				if len(actualMemo.Resources) > 0 && len(expectedMemo.Resources) > 0 {
					expectedResource := expectedMemo.Resources[0]
					actualResource := actualMemo.Resources[0]

					if actualResource.Name != expectedResource.Name {
						t.Errorf("Expected resource name %s, got %s", expectedResource.Name, actualResource.Name)
					}
					if actualResource.Filename != expectedResource.Filename {
						t.Errorf("Expected resource filename %s, got %s", expectedResource.Filename, actualResource.Filename)
					}
					if actualResource.Type != expectedResource.Type {
						t.Errorf("Expected resource type %s, got %s", expectedResource.Type, actualResource.Type)
					}
					if actualResource.Size != expectedResource.Size {
						t.Errorf("Expected resource size %s, got %s", expectedResource.Size, actualResource.Size)
					}
				}
			}
		})
	}
}

func TestMemos_ListMemos_InvalidJSON(t *testing.T) {
	// 测试无效JSON响应的情况
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	memos := NewMemos(server.URL, "test-token", "memos")
	_, err := memos.ListMemos(nil)

	if err == nil {
		t.Errorf("Expected error for invalid JSON, but got none")
	}
}

func TestMemos_ListMemos_NetworkError(t *testing.T) {
	// 测试网络错误的情况
	memos := NewMemos("http://invalid-url-that-does-not-exist", "test-token", "memos")
	_, err := memos.ListMemos(nil)

	if err == nil {
		t.Errorf("Expected network error, but got none")
	}
}

func TestNewMemos(t *testing.T) {
	endpoint := "https://memos.example.com"
	token := "test-token"

	memos := NewMemos(endpoint, token, "memos")

	if memos == nil {
		t.Fatal("Expected Memos instance, got nil")
	}

	if memos.Endpoint != endpoint {
		t.Errorf("Expected endpoint %s, got %s", endpoint, memos.Endpoint)
	}

	if memos.Token != token {
		t.Errorf("Expected token %s, got %s", token, memos.Token)
	}
}

// TestListMemosDefaultOrderBy tests that the default orderBy parameter is set correctly
func TestListMemosDefaultOrderBy(t *testing.T) {
	// Create a test server to capture the request parameters
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters
		queryParams := r.URL.Query()

		// Check that orderBy parameter is set to "display_time desc"
		orderBy := queryParams.Get("orderBy")
		if orderBy != "display_time desc" {
			t.Errorf("Expected orderBy to be 'display_time desc', got '%s'", orderBy)
		}

		// Return a minimal valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"memos": [], "nextPageToken": ""}`))
	}))
	defer server.Close()

	// Create Memos client with test server URL
	memos := NewMemos(server.URL, "test-token", "test-memos")

	t.Run("Default orderBy with nil request", func(t *testing.T) {
		// Test with nil request - should set default orderBy
		_, err := memos.ListMemos(nil)
		if err != nil {
			t.Errorf("ListMemos failed: %v", err)
		}
	})

	t.Run("Default orderBy with empty OrderBy field", func(t *testing.T) {
		// Test with empty OrderBy field - should set default orderBy
		req := &ListMemosRequest{
			PageSize: 10,
			// OrderBy is empty, should use default
		}
		_, err := memos.ListMemos(req)
		if err != nil {
			t.Errorf("ListMemos failed: %v", err)
		}
	})
}

// TestListMemosCustomOrderBy tests that custom orderBy parameter is preserved
func TestListMemosCustomOrderBy(t *testing.T) {
	customOrderBy := "create_time asc"

	// Create a test server to capture the request parameters
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters
		queryParams := r.URL.Query()

		// Check that orderBy parameter is set to our custom value
		orderBy := queryParams.Get("orderBy")
		if orderBy != customOrderBy {
			t.Errorf("Expected orderBy to be '%s', got '%s'", customOrderBy, orderBy)
		}

		// Return a minimal valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"memos": [], "nextPageToken": ""}`))
	}))
	defer server.Close()

	// Create Memos client with test server URL
	memos := NewMemos(server.URL, "test-token", "test-memos")

	// Test with custom OrderBy field - should preserve the custom value
	req := &ListMemosRequest{
		PageSize: 10,
		OrderBy:  customOrderBy,
	}
	_, err := memos.ListMemos(req)
	if err != nil {
		t.Errorf("ListMemos failed: %v", err)
	}
}

// TestMemosURLConstruction tests that URLs are constructed correctly
func TestMemosURLConstruction(t *testing.T) {
	expectedPath := "/api/v1/memos"

	// Create a test server to capture the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request path
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path to be '%s', got '%s'", expectedPath, r.URL.Path)
		}

		// Check that Authorization header is set
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("Expected Authorization header to start with 'Bearer ', got '%s'", authHeader)
		}

		// Return a minimal valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"memos": [], "nextPageToken": ""}`))
	}))
	defer server.Close()

	// Create Memos client with test server URL
	memos := NewMemos(server.URL, "test-token", "test-memos")

	// Test URL construction
	_, err := memos.ListMemos(&ListMemosRequest{PageSize: 5})
	if err != nil {
		t.Errorf("ListMemos failed: %v", err)
	}
}

package asana

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// serve points the client at a server that answers every request with status
// and counts the calls.
func serve(t *testing.T, status int) (*Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	t.Cleanup(srv.Close)
	old := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = old })
	c := NewClient("tok")
	c.RetryMax = 1
	return c, &calls
}

func TestCreateTaskNotRetriedOn5xx(t *testing.T) {
	c, calls := serve(t, http.StatusInternalServerError)
	if _, err := c.CreateTask(map[string]interface{}{"name": "x"}); err == nil {
		t.Fatal("want error")
	}
	if *calls != 1 {
		t.Fatalf("CreateTask sent %d requests on 500, want 1 (a retry may duplicate the task)", *calls)
	}
}

func TestCreateSubtaskNotRetriedOn5xx(t *testing.T) {
	c, calls := serve(t, http.StatusBadGateway)
	_, _ = c.CreateSubtask("p", map[string]interface{}{"name": "x"})
	if *calls != 1 {
		t.Fatalf("CreateSubtask sent %d requests on 502, want 1", *calls)
	}
}

func TestCreateTaskRetriedOn429(t *testing.T) {
	c, calls := serve(t, http.StatusTooManyRequests)
	_, _ = c.CreateTask(map[string]interface{}{"name": "x"})
	if *calls != 2 {
		t.Fatalf("CreateTask sent %d requests on 429 with RetryMax=1, want 2", *calls)
	}
}

func TestAddProjectRetriedOn5xx(t *testing.T) {
	c, calls := serve(t, http.StatusServiceUnavailable)
	_ = c.AddTaskToProject("t", "p", "s")
	if *calls != 2 {
		t.Fatalf("AddTaskToProject sent %d requests on 503 with RetryMax=1, want 2 (idempotent, safe to retry)", *calls)
	}
}

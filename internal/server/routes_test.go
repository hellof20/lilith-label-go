package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddDataHandler(t *testing.T) {
	s := &Server{}
	server := httptest.NewServer(http.HandlerFunc(s.AddDataHandler))
	defer server.Close()
	// resp, err := http.Get(server.URL)
	data := map[string]interface{}{
		"name": "John Doe",
		"age":  31,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("error marshaling JSON data. Err: %v", err)
	}
	resp, err := http.Post(server.URL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("error making request to server. Err: %v", err)
	}
	t.Log(resp.Body)
	defer resp.Body.Close()
}

//go:build e2e

package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAPILogging проверяет JSON-логи успешных и ошибочных HTTP-запросов:
// одну запись на запрос, обязательные поля, уникальность request_id и совпадение с X-Request-Id.
// Проверяет request_body у POST, PUT и DELETE, маскирование password и user_id: null.
// Выводит SELECT созданного товара и его состояния после архивации.
func TestAPILogging(t *testing.T) {
	since := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	body := validProduct(newCategory())
	body["password"] = "e2e-fake-password"
	created := request(t, "POST", "/products", body, 201)
	id := decode[product](t, created).ID
	readDatabaseProduct(t, id)
	path := "/products/" + id
	cases := []struct {
		method, path string
		status       int
		response     response
	}{
		{"POST", "/products", 201, created},
		{"GET", path, 200, request(t, "GET", path, nil, 200)},
		{"PUT", path, 200, request(t, "PUT", path, body, 200)},
		{"DELETE", path, 204, request(t, "DELETE", path, nil, 204)},
		{"POST", "/products", 400, request(t, "POST", "/products", map[string]any{"password": "e2e-fake-password"}, 400)},
		{"GET", "/missing-route", 404, request(t, "GET", "/missing-route", nil, 404)},
		{"POST", path, 405, request(t, "POST", path, body, 405)},
	}
	readDatabaseProduct(t, id)
	entries := map[string][]map[string]json.RawMessage{}
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries = map[string][]map[string]json.RawMessage{}
		logs := compose(t, "", "logs", "--no-color", "--no-log-prefix", "--since", since, "app")
		for _, line := range strings.Split(logs, "\n") {
			var entry map[string]json.RawMessage
			if json.Unmarshal([]byte(line), &entry) != nil {
				continue
			}
			var id string
			if json.Unmarshal(entry["request_id"], &id) == nil && id != "" {
				entries[id] = append(entries[id], entry)
			}
		}
		complete := true
		for _, tc := range cases {
			if len(entries[tc.response.Header.Get("X-Request-Id")]) == 0 {
				complete = false
			}
		}
		if complete || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		id := tc.response.Header.Get("X-Request-Id")
		if seen[id] {
			t.Errorf("request_id повторился: %s", id)
		}
		seen[id] = true
		found := entries[id]
		if len(found) != 1 {
			t.Errorf("%s %s: ожидалась одна запись с request_id=%s, получили %d", tc.method, tc.path, id, len(found))
			continue
		}
		entry := found[0]
		data, _ := json.Marshal(entry)
		t.Logf("JSON-лог для %s %s:\n%s", tc.method, tc.path, shortJSON(data))
		for _, field := range []string{"request_id", "method", "endpoint", "status_code", "duration_ms", "user_id", "timestamp"} {
			if _, ok := entry[field]; !ok {
				t.Errorf("В журнале нет обязательного поля %s", field)
			}
		}
		var record struct {
			Method     string         `json:"method"`
			Endpoint   string         `json:"endpoint"`
			StatusCode int            `json:"status_code"`
			DurationMS float64        `json:"duration_ms"`
			Timestamp  time.Time      `json:"timestamp"`
			Body       map[string]any `json:"request_body"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatalf("Неверный формат полей журнала: %v", err)
		}
		if record.Method != tc.method || record.Endpoint != tc.path || record.StatusCode != tc.status ||
			record.DurationMS < 0 || record.Timestamp.IsZero() || string(entry["user_id"]) != "null" {
			t.Errorf("Неверные поля журнала: %s", data)
		}
		if tc.method == "POST" || tc.method == "PUT" || tc.method == "DELETE" {
			if _, ok := entry["request_body"]; !ok {
				t.Error("В журнале мутирующего запроса нет request_body")
			}
		}
		if tc.method == "POST" || tc.method == "PUT" {
			if record.Body["password"] != "***" || strings.Contains(string(data), "e2e-fake-password") {
				t.Errorf("Пароль не замаскирован в журнале запроса %s", id)
			}
			if tc.status != 400 && record.Body["name"] != body["name"] {
				t.Error("В журнале потерялись обычные поля тела запроса")
			}
		}
	}
}

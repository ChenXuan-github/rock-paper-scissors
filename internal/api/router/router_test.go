package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testResponse 对应统一响应外壳；RawMessage 允许测试稍后再按具体接口解析 data。
type testResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// evaluateRoundData 只描述本测试关注的回合响应字段。
type evaluateRoundData struct {
	PlayerMove   string `json:"playerMove"`
	OpponentMove string `json:"opponentMove"`
	Result       string `json:"result"`
}

func TestEvaluateRoundSuccess(t *testing.T) {
	// httptest 在内存中请求 Gin Router，不需要真的监听 8080 端口。
	response := performRequest(
		t,
		http.MethodPost,
		"/api/v1/rounds/evaluate",
		`{"playerMove":"rock","opponentMove":"scissors"}`,
	)

	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusOK)
	}

	// 第一层先解析统一响应的 code/message/data 外壳。
	var body testResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body.Code != http.StatusOK {
		t.Errorf("response code = %d, want %d", body.Code, http.StatusOK)
	}

	// 第二层再把原始 data 解析成当前接口的具体 DTO。
	var data evaluateRoundData
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatal(err)
	}

	if data.Result != "win" {
		t.Errorf("result = %q, want %q", data.Result, "win")
	}
}

func TestEvaluateRoundRejectsInvalidMove(t *testing.T) {
	response := performRequest(
		t,
		http.MethodPost,
		"/api/v1/rounds/evaluate",
		`{"playerMove":"fire","opponentMove":"scissors"}`,
	)

	assertStatus(t, response, http.StatusBadRequest)
}

func TestEvaluateRoundRejectsInvalidJSON(t *testing.T) {
	response := performRequest(
		t,
		http.MethodPost,
		"/api/v1/rounds/evaluate",
		`{"playerMove":`,
	)

	assertStatus(t, response, http.StatusBadRequest)
}

func TestEvaluateRoundRejectsMissingField(t *testing.T) {
	response := performRequest(
		t,
		http.MethodPost,
		"/api/v1/rounds/evaluate",
		`{"playerMove":"rock"}`,
	)

	assertStatus(t, response, http.StatusBadRequest)
}

func TestEvaluateRoundRejectsWrongMethod(t *testing.T) {
	response := performRequest(
		t,
		http.MethodGet,
		"/api/v1/rounds/evaluate",
		"",
	)

	assertStatus(t, response, http.StatusNotFound)
}

func performRequest(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	// 标记为测试辅助函数；失败时 testing 会把行号定位到调用它的具体用例。
	t.Helper()

	// NewRequest 构造内存 HTTP 请求，strings.Reader 提供 JSON 请求体。
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	// Recorder 捕获 Router 写出的状态码、Header 和响应 Body。
	response := httptest.NewRecorder()
	// nil 依赖让测试聚焦 Day 2 路由和回合接口，不接入真实用户与 JWT。
	New(nil, nil, nil).ServeHTTP(response, request)

	return response
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	// 共享断言同样标记为 Helper，让错误位置指向实际测试用例。
	t.Helper()

	if response.Code != want {
		t.Errorf("HTTP status = %d, want %d; body = %s", response.Code, want, response.Body.String())
	}
}

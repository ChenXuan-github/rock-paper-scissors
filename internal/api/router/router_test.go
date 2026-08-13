package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type evaluateRoundData struct {
	PlayerMove   string `json:"playerMove"`
	OpponentMove string `json:"opponentMove"`
	Result       string `json:"result"`
}

func TestEvaluateRoundSuccess(t *testing.T) {
	response := performRequest(
		t,
		http.MethodPost,
		"/api/v1/rounds/evaluate",
		`{"playerMove":"rock","opponentMove":"scissors"}`,
	)

	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusOK)
	}

	var body testResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body.Code != http.StatusOK {
		t.Errorf("response code = %d, want %d", body.Code, http.StatusOK)
	}

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
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	New().ServeHTTP(response, request)

	return response
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()

	if response.Code != want {
		t.Errorf("HTTP status = %d, want %d; body = %s", response.Code, want, response.Body.String())
	}
}

package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLegacyEvaluateRouteIsNotRegistered 防止已经废弃的“双拳一次提交”接口被误加回来。
// Round 与 Evaluate 的领域单元测试继续保留，正式 HTTP 流程只允许玩家分别在房间中出拳。
func TestLegacyEvaluateRouteIsNotRegistered(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/rounds/evaluate",
		strings.NewReader(`{"playerMove":"rock","opponentMove":"scissors"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	New(nil, nil, nil, nil, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"legacy evaluate HTTP status = %d, want %d; body = %s",
			recorder.Code,
			http.StatusNotFound,
			recorder.Body.String(),
		)
	}
}

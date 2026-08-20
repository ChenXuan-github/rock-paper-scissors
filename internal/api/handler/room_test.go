package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/gin-gonic/gin"
)

type roomTestTokenVerifier struct {
	userID   int64
	username string
}

type roomTestEventPusher struct {
	events map[int64][]realtime.Event
}

func (p *roomTestEventPusher) SendToUser(userID int64, event realtime.Event) error {
	p.events[userID] = append(p.events[userID], event)
	return nil
}

func (v roomTestTokenVerifier) Parse(string) (auth.Claims, error) {
	if v.userID == 0 {
		return auth.Claims{UserID: 1, Username: "chenxuan"}, nil
	}
	return auth.Claims{UserID: v.userID, Username: v.username}, nil
}

func TestRoomHandlerCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	handler := NewRoomHandler(service)

	router := gin.New()
	router.POST(
		"/rooms",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.Create,
	)

	request := httptest.NewRequest(http.MethodPost, "/rooms", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}

	var body response.Response[roomResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.ID) != 6 {
		t.Fatalf("room ID length = %d, want 6", len(body.Data.ID))
	}
	if len(body.Data.Players) != 1 || body.Data.Players[0].UserID != 1 {
		t.Fatalf("players = %#v, want current user", body.Data.Players)
	}
	if body.Data.Status != game.RoomStatusWaiting {
		t.Fatalf("status = %q, want %q", body.Data.Status, game.RoomStatusWaiting)
	}
}

func TestRoomHandlerList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	if _, err := service.CreateRoom(&game.Player{UserID: 1, Username: "player-1"}); err != nil {
		t.Fatalf("first CreateRoom() error = %v", err)
	}
	if _, err := service.CreateRoom(&game.Player{UserID: 2, Username: "player-2"}); err != nil {
		t.Fatalf("second CreateRoom() error = %v", err)
	}
	handler := NewRoomHandler(service)

	router := gin.New()
	router.GET(
		"/rooms",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.List,
	)

	request := httptest.NewRequest(http.MethodGet, "/rooms", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var body response.Response[[]roomResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("room count = %d, want 2", len(body.Data))
	}
}

func TestRoomHandlerCreateRejectsPlayerAlreadyInRoom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	handler := NewRoomHandler(service)

	router := gin.New()
	router.POST(
		"/rooms",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.Create,
	)

	performCreate := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/rooms", nil)
		request.Header.Set("Authorization", "Bearer test-token")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	if firstResponse := performCreate(); firstResponse.Code != http.StatusCreated {
		t.Fatalf("first HTTP status = %d, want %d", firstResponse.Code, http.StatusCreated)
	}
	if secondResponse := performCreate(); secondResponse.Code != http.StatusConflict {
		t.Fatalf(
			"second HTTP status = %d, want %d; body = %s",
			secondResponse.Code,
			http.StatusConflict,
			secondResponse.Body.String(),
		)
	}
	if rooms := manager.ListRooms(); len(rooms) != 1 {
		t.Fatalf("room count = %d, want 1", len(rooms))
	}
}

func TestRoomHandlerLeaveCurrentRoom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	if _, err := service.CreateRoom(&game.Player{UserID: 1, Username: "chenxuan"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	handler := NewRoomHandler(service)

	router := gin.New()
	router.DELETE(
		"/rooms/me",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.LeaveCurrent,
	)

	performLeave := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodDelete, "/rooms/me", nil)
		request.Header.Set("Authorization", "Bearer test-token")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	firstResponse := performLeave()
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first HTTP status = %d, want %d; body = %s", firstResponse.Code, http.StatusOK, firstResponse.Body.String())
	}
	var body response.Response[leaveRoomResponse]
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.RoomDeleted {
		t.Fatal("roomDeleted = false, want true")
	}

	secondResponse := performLeave()
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("second HTTP status = %d, want %d; body = %s", secondResponse.Code, http.StatusConflict, secondResponse.Body.String())
	}
}

func TestRoomHandlerJoin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	room, err := service.CreateRoom(&game.Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	handler := NewRoomHandler(service)

	router := gin.New()
	router.POST(
		"/rooms/:roomID/join",
		middleware.Authenticate(roomTestTokenVerifier{userID: 2, username: "guest"}),
		handler.Join,
	)

	request := httptest.NewRequest(http.MethodPost, "/rooms/"+room.ID+"/join", nil)
	request.Header.Set("Authorization", "Bearer guest-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var body response.Response[roomResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID != room.ID {
		t.Fatalf("room ID = %q, want %q", body.Data.ID, room.ID)
	}
	if len(body.Data.Players) != 2 {
		t.Fatalf("player count = %d, want 2", len(body.Data.Players))
	}
	if body.Data.Status != game.RoomStatusReady {
		t.Fatalf("status = %q, want %q", body.Data.Status, game.RoomStatusReady)
	}
}

func TestRoomHandlerStartByHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	room, err := service.CreateRoom(&game.Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, &game.Player{UserID: 2, Username: "guest"}); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	handler := NewRoomHandler(service)

	router := gin.New()
	router.POST(
		"/rooms/me/start",
		middleware.Authenticate(roomTestTokenVerifier{userID: 1, username: "host"}),
		handler.Start,
	)

	request := httptest.NewRequest(http.MethodPost, "/rooms/me/start", nil)
	request.Header.Set("Authorization", "Bearer host-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var body response.Response[roomResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.HostUserID != 1 {
		t.Fatalf("hostUserId = %d, want 1", body.Data.HostUserID)
	}
	if body.Data.Status != game.RoomStatusPlaying {
		t.Fatalf("status = %q, want %q", body.Data.Status, game.RoomStatusPlaying)
	}
}

func TestRoomHandlerStartRejectsGuest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	room, err := service.CreateRoom(&game.Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, &game.Player{UserID: 2, Username: "guest"}); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	handler := NewRoomHandler(service)

	router := gin.New()
	router.POST(
		"/rooms/me/start",
		middleware.Authenticate(roomTestTokenVerifier{userID: 2, username: "guest"}),
		handler.Start,
	)

	request := httptest.NewRequest(http.MethodPost, "/rooms/me/start", nil)
	request.Header.Set("Authorization", "Bearer guest-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if room.Status() != game.RoomStatusReady {
		t.Fatalf("status = %q, want %q", room.Status(), game.RoomStatusReady)
	}
}

func TestRoomHandlerSubmitMoveWaitsThenSettles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	room, err := service.CreateRoom(&game.Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, &game.Player{UserID: 2, Username: "guest"}); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := service.StartCurrentRoom(1); err != nil {
		t.Fatalf("StartCurrentRoom() error = %v", err)
	}
	pusher := &roomTestEventPusher{events: make(map[int64][]realtime.Event)}
	handler := NewRoomHandler(service, pusher)

	performMove := func(userID int64, username, move string) *httptest.ResponseRecorder {
		router := gin.New()
		router.POST(
			"/rooms/me/move",
			middleware.Authenticate(roomTestTokenVerifier{userID: userID, username: username}),
			handler.SubmitMove,
		)

		requestBody, err := json.Marshal(map[string]string{"move": move})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(
			http.MethodPost,
			"/rooms/me/move",
			bytes.NewReader(requestBody),
		)
		request.Header.Set("Authorization", "Bearer test-token")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	hostResponse := performMove(1, "host", "rock")
	if hostResponse.Code != http.StatusOK {
		t.Fatalf("host HTTP status = %d, want %d; body = %s", hostResponse.Code, http.StatusOK, hostResponse.Body.String())
	}
	var waitingBody response.Response[submitMoveResponse]
	if err := json.Unmarshal(hostResponse.Body.Bytes(), &waitingBody); err != nil {
		t.Fatal(err)
	}
	if waitingBody.Data.Settled || waitingBody.Data.Result != game.ResultPending.String() {
		t.Fatalf("waiting response = %#v, want unsettled pending", waitingBody.Data)
	}
	if waitingBody.Data.OpponentMove != nil {
		t.Fatalf("waiting opponentMove = %q, want nil", *waitingBody.Data.OpponentMove)
	}
	if len(pusher.events[1]) != 0 || len(pusher.events[2]) != 1 {
		t.Fatalf("events after first move = %#v", pusher.events)
	}
	moveEvent := pusher.events[2][0]
	if moveEvent.Type != "move_submitted" {
		t.Fatalf("event type = %q, want move_submitted", moveEvent.Type)
	}
	moveData, ok := moveEvent.Data.(moveSubmittedEventData)
	if !ok || moveData.SubmittedCount != 1 {
		t.Fatalf("move event data = %#v", moveEvent.Data)
	}

	guestResponse := performMove(2, "guest", "scissors")
	if guestResponse.Code != http.StatusOK {
		t.Fatalf("guest HTTP status = %d, want %d; body = %s", guestResponse.Code, http.StatusOK, guestResponse.Body.String())
	}
	var settledBody response.Response[submitMoveResponse]
	if err := json.Unmarshal(guestResponse.Body.Bytes(), &settledBody); err != nil {
		t.Fatal(err)
	}
	if !settledBody.Data.Settled || settledBody.Data.Result != game.Lose.String() {
		t.Fatalf("settled response = %#v, want settled lose", settledBody.Data)
	}
	if settledBody.Data.OpponentMove == nil || *settledBody.Data.OpponentMove != game.Rock.String() {
		t.Fatalf("settled opponentMove = %#v, want rock", settledBody.Data.OpponentMove)
	}

	if len(pusher.events[1]) != 1 || len(pusher.events[2]) != 2 {
		t.Fatalf("pushed events = %#v, want settlement for both players", pusher.events)
	}
	assertRoundSettledEvent(t, pusher.events[1][0], "rock", "scissors", "win")
	assertRoundSettledEvent(t, pusher.events[2][1], "scissors", "rock", "lose")
}

func assertRoundSettledEvent(
	t *testing.T,
	event realtime.Event,
	wantMove string,
	wantOpponentMove string,
	wantResult string,
) {
	t.Helper()
	if event.Type != "round_settled" {
		t.Fatalf("event type = %q, want round_settled", event.Type)
	}
	data, ok := event.Data.(roundSettledEventData)
	if !ok {
		t.Fatalf("event data type = %T, want roundSettledEventData", event.Data)
	}
	if data.Move != wantMove || data.OpponentMove != wantOpponentMove || data.Result != wantResult {
		t.Fatalf("event data = %#v", data)
	}
}

func TestRoomHandlerSubmitMoveRejectsInvalidMove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRoomHandler(game.NewRoomService(game.NewRoomManager()))
	router := gin.New()
	router.POST(
		"/rooms/me/move",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.SubmitMove,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/rooms/me/move",
		bytes.NewBufferString(`{"move":"fire"}`),
	)
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestRoomHandlerCurrentHidesMoveUntilSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	room, err := service.CreateRoom(&game.Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, &game.Player{UserID: 2, Username: "guest"}); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := service.StartCurrentRoom(1); err != nil {
		t.Fatalf("StartCurrentRoom() error = %v", err)
	}
	// 客人先出剪刀，房主此时只能知道已经有一人提交，不能看到剪刀。
	if _, err := service.SubmitMove(2, game.Scissors); err != nil {
		t.Fatalf("guest SubmitMove() error = %v", err)
	}
	handler := NewRoomHandler(service)

	performCurrent := func() *httptest.ResponseRecorder {
		router := gin.New()
		router.GET(
			"/rooms/me",
			middleware.Authenticate(roomTestTokenVerifier{userID: 1, username: "host"}),
			handler.Current,
		)
		request := httptest.NewRequest(http.MethodGet, "/rooms/me", nil)
		request.Header.Set("Authorization", "Bearer host-token")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	waitingResponse := performCurrent()
	if waitingResponse.Code != http.StatusOK {
		t.Fatalf("waiting HTTP status = %d, want %d; body = %s", waitingResponse.Code, http.StatusOK, waitingResponse.Body.String())
	}
	var waitingBody response.Response[currentRoomResponse]
	if err := json.Unmarshal(waitingResponse.Body.Bytes(), &waitingBody); err != nil {
		t.Fatal(err)
	}
	if waitingBody.Data.Round == nil || waitingBody.Data.Round.SubmittedCount != 1 {
		t.Fatalf("waiting round = %#v, want one submission", waitingBody.Data.Round)
	}
	if waitingBody.Data.Round.Submitted || waitingBody.Data.Round.Move != nil || waitingBody.Data.Round.OpponentMove != nil {
		t.Fatalf("waiting round leaked a move: %#v", waitingBody.Data.Round)
	}

	// 房主随后出石头并触发结算，再查询时双方选择和房主结果才允许返回。
	if _, err := service.SubmitMove(1, game.Rock); err != nil {
		t.Fatalf("host SubmitMove() error = %v", err)
	}
	settledResponse := performCurrent()
	var settledBody response.Response[currentRoomResponse]
	if err := json.Unmarshal(settledResponse.Body.Bytes(), &settledBody); err != nil {
		t.Fatal(err)
	}
	if settledBody.Data.Round == nil || !settledBody.Data.Round.Settled {
		t.Fatalf("settled round = %#v, want settled", settledBody.Data.Round)
	}
	if settledBody.Data.Round.Move == nil || *settledBody.Data.Round.Move != game.Rock.String() {
		t.Fatalf("settled own move = %#v, want rock", settledBody.Data.Round.Move)
	}
	if settledBody.Data.Round.OpponentMove == nil || *settledBody.Data.Round.OpponentMove != game.Scissors.String() {
		t.Fatalf("settled opponent move = %#v, want scissors", settledBody.Data.Round.OpponentMove)
	}
	if settledBody.Data.Round.Result != game.Win.String() {
		t.Fatalf("settled result = %q, want win", settledBody.Data.Round.Result)
	}
}

func TestRoomHandlerCurrentRejectsPlayerWithoutRoom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRoomHandler(game.NewRoomService(game.NewRoomManager()))
	router := gin.New()
	router.GET(
		"/rooms/me",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.Current,
	)

	request := httptest.NewRequest(http.MethodGet, "/rooms/me", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

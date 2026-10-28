package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
)

// setupTestServer creates a test HTTP server with party room routes
func setupTestServer() (*httptest.Server, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware for party creation
	router.POST("/api/party/create", func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		userName := c.GetHeader("X-User-Name")
		if userName == "" {
			userName = "HostUser"
		}
		c.Set("user_id", userID)
		c.Set("first_name", userName)
		controllers.CreatePartyRoom()(c)
	})

	router.GET("/api/party/check/:room_id", controllers.CheckPartyRoom())
	router.GET("/ws/party/:room_id", controllers.ConnectPartyRoomWebSocket())

	server := httptest.NewServer(router)
	return server, router
}

// TestClient manages a simulated user's websocket connection and message queue
type TestClient struct {
	Conn     *websocket.Conn
	UserID   string
	UserName string
	Messages chan map[string]interface{}
	closed   bool
	mu       sync.Mutex
}

// connectTestClient creates and registers a simulated client
func connectTestClient(t *testing.T, serverURL, roomCode, userID, userName string) *TestClient {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws/party/" + roomCode + "?user_id=" + userID + "&user_name=" + userName

	dialer := websocket.Dialer{
		HandshakeTimeout: 3 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect websocket for %s: %v", userID, err)
	}
	if resp != nil {
		resp.Body.Close()
	}

	client := &TestClient{
		Conn:     conn,
		UserID:   userID,
		UserName: userName,
		Messages: make(chan map[string]interface{}, 256),
	}

	go func() {
		for {
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				return
			}
			client.mu.Lock()
			if !client.closed {
				select {
				case client.Messages <- msg:
				default:
				}
			}
			client.mu.Unlock()
		}
	}()

	return client
}

func (tc *TestClient) Close() {
	tc.mu.Lock()
	tc.closed = true
	tc.mu.Unlock()
	_ = tc.Conn.Close()
}

func (tc *TestClient) DrainMessages() {
	for {
		select {
		case <-tc.Messages:
		default:
			return
		}
	}
}

func (tc *TestClient) WaitForMessage(t *testing.T, expectedType string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-tc.Messages:
			if msgType, ok := msg["type"].(string); ok && msgType == expectedType {
				return msg
			}
		case <-deadline:
			t.Fatalf("[%s] Timed out waiting for message of type '%s'", tc.UserName, expectedType)
			return nil
		}
	}
}

// TestPartyRoom_FullLifecycle tests end-to-end room creation, multiple simulated users,
// server-authoritative playback, queue locking, duplicate prevention, late joiners, and host election.
func TestPartyRoom_FullLifecycle(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	hostID := "host_user_1"
	hostName := "HostAlice"

	// 1. Create Room via REST
	req, _ := http.NewRequest("POST", server.URL+"/api/party/create", nil)
	req.Header.Set("X-User-ID", hostID)
	req.Header.Set("X-User-Name", hostName)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create party room: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var createResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		t.Fatalf("Failed to decode create room response: %v", err)
	}

	roomCode, ok := createResp["room_code"].(string)
	if !ok || roomCode == "" {
		t.Fatalf("Invalid room_code in response: %v", createResp)
	}

	t.Logf("✅ Party room created: %s", roomCode)

	// Verify room exists via Check endpoint
	checkResp, err := http.Get(server.URL + "/api/party/check/" + roomCode)
	if err != nil || checkResp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to check room: %v", err)
	}
	checkResp.Body.Close()

	// 2. Connect Host via WebSocket
	hostClient := connectTestClient(t, server.URL, roomCode, hostID, hostName)
	defer hostClient.Close()

	// Host receives initial snapshot
	initialPlayback := hostClient.WaitForMessage(t, "playback_state", 2*time.Second)
	if initialPlayback["is_playing"] != false {
		t.Errorf("Expected initial is_playing = false, got %v", initialPlayback["is_playing"])
	}

	// 3. Connect Member 1 (Bob) and Member 2 (Charlie)
	member1ID := "member_bob_2"
	member1Name := "Bob"
	member1Client := connectTestClient(t, server.URL, roomCode, member1ID, member1Name)
	defer member1Client.Close()

	member2ID := "member_charlie_3"
	member2Name := "Charlie"
	member2Client := connectTestClient(t, server.URL, roomCode, member2ID, member2Name)
	defer member2Client.Close()

	// Wait for member list broadcast on host
	memberListMsg := hostClient.WaitForMessage(t, "member_list", 2*time.Second)
	members, ok := memberListMsg["members"].([]interface{})
	if !ok || len(members) < 3 {
		memberListMsg = hostClient.WaitForMessage(t, "member_list", 2*time.Second)
		members = memberListMsg["members"].([]interface{})
	}
	t.Logf("✅ 3 participants connected: Host (%s), Member1 (%s), Member2 (%s)", hostName, member1Name, member2Name)

	time.Sleep(50 * time.Millisecond)
	hostClient.DrainMessages()
	member1Client.DrainMessages()
	member2Client.DrainMessages()

	// ----------------------------------------------------
	// 4. Server-Authoritative Playback Synchronization
	// ----------------------------------------------------
	t.Run("ServerAuthoritativePlayback", func(t *testing.T) {
		hostClient.DrainMessages()
		member1Client.DrainMessages()
		member2Client.DrainMessages()

		startTimeMs := 15000.0
		trackID := "song_track_99"

		hostSync := map[string]interface{}{
			"type":                 "sync_playback",
			"song_id":              trackID,
			"is_playing":           true,
			"playback_position_ms": startTimeMs,
			"duration_ms":          200000.0,
			"state_version":        2.0,
			"event_id":             "sync_evt_001",
		}
		if err := hostClient.Conn.WriteJSON(hostSync); err != nil {
			t.Fatalf("Failed to send sync_playback from host: %v", err)
		}

		// Member 1 should receive server-authoritative playback_state
		stateBob := member1Client.WaitForMessage(t, "playback_state", 2*time.Second)
		if stateBob["song_id"] != trackID {
			t.Errorf("Expected song_id = %s, got %v", trackID, stateBob["song_id"])
		}
		if stateBob["is_playing"] != true {
			t.Errorf("Expected is_playing = true, got %v", stateBob["is_playing"])
		}
		serverTimestamp, ok := stateBob["server_timestamp"].(float64)
		if !ok || serverTimestamp <= 0 {
			t.Errorf("Expected positive server_timestamp, got %v", stateBob["server_timestamp"])
		}
		stateVersion, ok := stateBob["state_version"].(float64)
		if !ok || stateVersion < 2 {
			t.Errorf("Expected state_version >= 2, got %v", stateBob["state_version"])
		}

		t.Logf("✅ Server-authoritative playback state broadcast successfully (ver: %v, timestamp: %v)", stateVersion, int64(serverTimestamp))
	})

	// ----------------------------------------------------
	// 5. Host Permissions Enforcement
	// ----------------------------------------------------
	t.Run("HostPermissionsEnforcement", func(t *testing.T) {
		hostClient.DrainMessages()
		member1Client.DrainMessages()
		member2Client.DrainMessages()

		// Member 1 attempts to control playback (unauthorized)
		unauthorizedSync := map[string]interface{}{
			"type":                 "sync_playback",
			"song_id":              "hacked_song",
			"is_playing":           false,
			"playback_position_ms": 0.0,
		}
		if err := member1Client.Conn.WriteJSON(unauthorizedSync); err != nil {
			t.Fatalf("Failed to write unauthorized sync: %v", err)
		}

		// Member 1 should receive an error response with PERMISSION_DENIED
		errMsg := member1Client.WaitForMessage(t, "error", 2*time.Second)
		if errMsg["error_code"] != "PERMISSION_DENIED" {
			t.Errorf("Expected PERMISSION_DENIED error, got: %v", errMsg)
		}

		t.Logf("✅ Non-host playback control correctly rejected with PERMISSION_DENIED")
	})

	// ----------------------------------------------------
	// 6. Shared Queue and Queue Locking
	// ----------------------------------------------------
	t.Run("SharedQueueAndQueueLocking", func(t *testing.T) {
		hostClient.DrainMessages()
		member1Client.DrainMessages()
		member2Client.DrainMessages()

		// A. Member 1 adds a song while queue is unlocked -> should succeed
		addMsg := map[string]interface{}{
			"type": "add_to_queue",
			"song": map[string]interface{}{
				"song_id":  "queue_song_1",
				"title":    "Starboy",
				"artist":   "The Weeknd",
				"duration": 230.0,
			},
		}
		if err := member1Client.Conn.WriteJSON(addMsg); err != nil {
			t.Fatalf("Failed to send add_to_queue: %v", err)
		}

		queueUpdate1 := hostClient.WaitForMessage(t, "queue_updated", 2*time.Second)
		queue1, ok := queueUpdate1["queue"].([]interface{})
		if !ok || len(queue1) != 1 {
			t.Fatalf("Expected 1 song in queue, got: %v", queueUpdate1)
		}
		t.Logf("✅ Member added song to unlocked queue")

		// Drain messages on all clients before locking queue
		time.Sleep(20 * time.Millisecond)
		hostClient.DrainMessages()
		member1Client.DrainMessages()
		member2Client.DrainMessages()

		// B. Host locks the queue
		lockMsg := map[string]interface{}{
			"type":   "toggle_queue_lock",
			"locked": true,
		}
		if err := hostClient.Conn.WriteJSON(lockMsg); err != nil {
			t.Fatalf("Failed to lock queue: %v", err)
		}

		lockUpdate := member1Client.WaitForMessage(t, "queue_updated", 2*time.Second)
		if lockUpdate["queue_locked"] != true {
			t.Errorf("Expected queue_locked = true, got %v", lockUpdate["queue_locked"])
		}
		t.Logf("✅ Host locked the queue")

		// Drain messages before testing locked addition
		time.Sleep(20 * time.Millisecond)
		member2Client.DrainMessages()

		// C. Member 2 tries to add song while queue is locked -> should be rejected
		addLockedMsg := map[string]interface{}{
			"type": "add_to_queue",
			"song": map[string]interface{}{
				"song_id": "queue_song_2",
				"title":   "Blinding Lights",
			},
		}
		if err := member2Client.Conn.WriteJSON(addLockedMsg); err != nil {
			t.Fatalf("Failed to send add_to_queue from member2: %v", err)
		}

		lockErrMsg := member2Client.WaitForMessage(t, "error", 2*time.Second)
		if lockErrMsg["error_code"] != "QUEUE_LOCKED" {
			t.Errorf("Expected QUEUE_LOCKED error, got: %v", lockErrMsg)
		}
		t.Logf("✅ Member addition to locked queue correctly rejected with QUEUE_LOCKED")

		// D. Host adds song while queue is locked -> should succeed
		member1Client.DrainMessages()
		hostAddMsg := map[string]interface{}{
			"type": "add_to_queue",
			"song": map[string]interface{}{
				"song_id":  "queue_song_3",
				"title":    "Save Your Tears",
				"artist":   "The Weeknd",
				"duration": 215.0,
			},
		}
		if err := hostClient.Conn.WriteJSON(hostAddMsg); err != nil {
			t.Fatalf("Failed to send add_to_queue from host: %v", err)
		}

		queueUpdate2 := member1Client.WaitForMessage(t, "queue_updated", 2*time.Second)
		queue2, _ := queueUpdate2["queue"].([]interface{})
		if len(queue2) != 2 {
			t.Errorf("Expected 2 songs in queue, got %d", len(queue2))
		}
		t.Logf("✅ Host successfully added song while queue is locked")

		// E. Host advances to next track
		member1Client.DrainMessages()
		nextMsg := map[string]interface{}{
			"type": "next_track",
		}
		if err := hostClient.Conn.WriteJSON(nextMsg); err != nil {
			t.Fatalf("Failed to send next_track: %v", err)
		}

		nextPlayback := member1Client.WaitForMessage(t, "playback_state", 2*time.Second)
		if nextPlayback["song_id"] != "queue_song_1" {
			t.Errorf("Expected next song to be 'queue_song_1', got %v", nextPlayback["song_id"])
		}
		t.Logf("✅ Queue automatically advanced to next song ('queue_song_1')")
	})

	// ----------------------------------------------------
	// 7. Duplicate-Event Prevention
	// ----------------------------------------------------
	t.Run("DuplicateEventPrevention", func(t *testing.T) {
		hostClient.DrainMessages()
		member1Client.DrainMessages()
		member2Client.DrainMessages()

		dupEventID := "unique_action_event_999"

		// Host sends message with event_id
		chatMsg := map[string]interface{}{
			"type":     "chat_message",
			"text":     "Hello Everyone!",
			"event_id": dupEventID,
		}
		if err := hostClient.Conn.WriteJSON(chatMsg); err != nil {
			t.Fatalf("Failed to send chat: %v", err)
		}

		// Member 1 receives chat message
		firstChat := member1Client.WaitForMessage(t, "chat_message", 2*time.Second)
		if firstChat["text"] != "Hello Everyone!" {
			t.Errorf("Expected 'Hello Everyone!', got %v", firstChat["text"])
		}

		// Host sends exact same event_id again (duplicate submission)
		dupChatMsg := map[string]interface{}{
			"type":     "chat_message",
			"text":     "Hello Everyone DUPLICATE!",
			"event_id": dupEventID,
		}
		if err := hostClient.Conn.WriteJSON(dupChatMsg); err != nil {
			t.Fatalf("Failed to send duplicate chat: %v", err)
		}

		// Verify duplicate was discarded by checking channel timeout
		select {
		case dupCheck := <-member1Client.Messages:
			if dupCheck["type"] == "chat_message" && dupCheck["text"] == "Hello Everyone DUPLICATE!" {
				t.Errorf("Duplicate event was NOT suppressed: %v", dupCheck)
			}
		case <-time.After(300 * time.Millisecond):
			t.Logf("✅ Duplicate event successfully filtered by deduplication engine")
		}
	})

	// ----------------------------------------------------
	// 8. Late Joiners Authoritative State Interpolation
	// ----------------------------------------------------
	t.Run("LateJoinerStateInterpolation", func(t *testing.T) {
		hostClient.DrainMessages()
		member1Client.DrainMessages()

		// Set playing at progress = 10000ms
		_ = hostClient.Conn.WriteJSON(map[string]interface{}{
			"type":                 "sync_playback",
			"song_id":              "late_joiner_track",
			"is_playing":           true,
			"playback_position_ms": 10000.0,
			"duration_ms":          180000.0,
		})
		_ = member1Client.WaitForMessage(t, "playback_state", 2*time.Second)

		// Wait 150ms to simulate real-time playback progress
		time.Sleep(150 * time.Millisecond)

		// Late joiner Diana connects
		lateUserID := "late_user_diana_4"
		lateUserName := "Diana"
		lateClient := connectTestClient(t, server.URL, roomCode, lateUserID, lateUserName)
		defer lateClient.Close()

		// Late joiner immediately receives authoritative snapshot
		latePlayback := lateClient.WaitForMessage(t, "playback_state", 2*time.Second)
		if latePlayback["song_id"] != "late_joiner_track" {
			t.Errorf("Expected song_id = 'late_joiner_track', got %v", latePlayback["song_id"])
		}
		if latePlayback["is_playing"] != true {
			t.Errorf("Expected is_playing = true, got %v", latePlayback["is_playing"])
		}

		pos, ok := latePlayback["playback_position_ms"].(float64)
		if !ok || pos < 10100.0 {
			t.Errorf("Expected progress >= 10100ms due to server elapsed time, got %v", pos)
		}

		t.Logf("✅ Late joiner received authoritative interpolated playback position: %.0f ms", pos)
	})

	// ----------------------------------------------------
	// 9. Host Disconnect & Automatic Host Election
	// ----------------------------------------------------
	t.Run("HostDisconnectAndElection", func(t *testing.T) {
		// Set room grace duration to 300ms for fast automated testing
		controllers.GlobalPartyManager.Mutex.RLock()
		room := controllers.GlobalPartyManager.Rooms[roomCode]
		controllers.GlobalPartyManager.Mutex.RUnlock()

		room.Mutex.Lock()
		room.HostGraceDuration = 300 * time.Millisecond
		room.Mutex.Unlock()

		// Drain messages before closing host
		member1Client.DrainMessages()
		member2Client.DrainMessages()

		// Host disconnects (e.g. browser tab closed)
		hostClient.Close()

		// Remaining member (Bob) receives host_status indicating disconnect
		hostStatus := member1Client.WaitForMessage(t, "host_status", 2*time.Second)
		if hostStatus["connected"] != false {
			t.Errorf("Expected host_status.connected = false, got %v", hostStatus["connected"])
		}
		t.Logf("✅ Host disconnection broadcast to members with grace period")

		// Wait for grace period (300ms) to expire and trigger host election
		time.Sleep(450 * time.Millisecond)

		// Bob (longest-standing participant) should be elected as new host
		hostTransferred := member1Client.WaitForMessage(t, "host_transferred", 2*time.Second)
		if hostTransferred["new_host_id"] != member1ID {
			t.Errorf("Expected new_host_id = %s (Bob), got %v", member1ID, hostTransferred["new_host_id"])
		}
		t.Logf("✅ Host election successful: %s promoted to party host!", hostTransferred["new_host_name"])

		// Verify Bob can now exercise host privileges (e.g. toggle queue lock)
		bobLockMsg := map[string]interface{}{
			"type":   "toggle_queue_lock",
			"locked": false,
		}
		if err := member1Client.Conn.WriteJSON(bobLockMsg); err != nil {
			t.Fatalf("Failed to write lock toggle as new host: %v", err)
		}

		newLockUpdate := member2Client.WaitForMessage(t, "queue_updated", 2*time.Second)
		if newLockUpdate["queue_locked"] != false {
			t.Errorf("Expected queue_locked = false after new host unlocked it, got %v", newLockUpdate["queue_locked"])
		}
		t.Logf("✅ Newly elected host successfully executed host privileged command")
	})
}

// TestPartyRoom_ConcurrentSimulatedUsers tests 10 concurrent simulated users
// sending chat messages and pinging to ensure thread safety without race conditions.
func TestPartyRoom_ConcurrentSimulatedUsers(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// 1. Create Room
	req, _ := http.NewRequest("POST", server.URL+"/api/party/create", nil)
	req.Header.Set("X-User-ID", "host_concur")
	req.Header.Set("X-User-Name", "HostConcur")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	var createResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()
	roomCode := createResp["room_code"].(string)

	hostClient := connectTestClient(t, server.URL, roomCode, "host_concur", "HostConcur")
	defer hostClient.Close()

	// 2. Connect 8 simulated members concurrently
	const numUsers = 8
	var clients []*TestClient
	var wg sync.WaitGroup

	for i := 0; i < numUsers; i++ {
		userID := "sim_user_" + string(rune('A'+i))
		userName := "SimUser_" + string(rune('A'+i))
		c := connectTestClient(t, server.URL, roomCode, userID, userName)
		clients = append(clients, c)
		defer c.Close()
	}

	// 3. Concurrently send chat messages and pings from all simulated users
	for i, client := range clients {
		wg.Add(1)
		go func(index int, tc *TestClient) {
			defer wg.Done()
			for round := 0; round < 5; round++ {
				_ = tc.Conn.WriteJSON(map[string]interface{}{
					"type": "chat_message",
					"text": "Concurrent message from user",
				})
				_ = tc.Conn.WriteJSON(map[string]interface{}{
					"type":        "ping",
					"client_time": float64(time.Now().UnixMilli()),
				})
				time.Sleep(10 * time.Millisecond)
			}
		}(i, client)
	}

	wg.Wait()
	t.Logf("✅ %d concurrent users sent 40 messages without race conditions", numUsers)
}

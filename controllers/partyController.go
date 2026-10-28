package controllers

import (
	"crypto/rand"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Client represents a connected websocket client
type Client struct {
	Conn       *websocket.Conn
	UserID     string
	UserName   string
	Send       chan []byte
	JoinedAt   int64 // Epoch ms when client connected
	writeMutex sync.Mutex
	isClosed   bool
	closeMutex sync.Mutex
}

// SendJSON writes a JSON message safely using the client's write mutex
func (c *Client) SendJSON(v interface{}) error {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	c.closeMutex.Lock()
	closed := c.isClosed
	c.closeMutex.Unlock()

	if closed || c.Conn == nil {
		return websocket.ErrCloseSent
	}

	_ = c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.Conn.WriteJSON(v)
}

// Close safely closes the client websocket and send channel
func (c *Client) Close() {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()

	if !c.isClosed {
		c.isClosed = true
		if c.Conn != nil {
			_ = c.Conn.Close()
		}
		// Drain and close send channel safely if open
		select {
		case <-c.Send:
		default:
		}
	}
}

// PlaybackState represents current authoritative music state of the room
type PlaybackState struct {
	TrackID          string `json:"track_id"`
	SongID           string `json:"song_id"`              // Backward compatibility
	IsPlaying        bool   `json:"is_playing"`
	PlaybackPosition int64  `json:"playback_position_ms"` // Authoritative position at ServerTimestamp (ms)
	ProgressMs       int    `json:"progress_ms"`          // Backward compatibility (ms)
	ServerTimestamp  int64  `json:"server_timestamp"`     // Server epoch timestamp in ms
	DurationMs       int64  `json:"duration_ms"`          // Track duration in ms (if known)
	StateVersion     int64  `json:"state_version"`        // Monotonically increasing sequence number
	UpdatedBy        string `json:"updated_by"`           // User ID who made the change
}

// CurrentPositionMs calculates authoritative progress based on elapsed server time
func (ps *PlaybackState) CurrentPositionMs() int64 {
	if !ps.IsPlaying {
		return ps.PlaybackPosition
	}
	now := time.Now().UnixMilli()
	elapsed := now - ps.ServerTimestamp
	if elapsed < 0 {
		elapsed = 0
	}
	current := ps.PlaybackPosition + elapsed
	if ps.DurationMs > 0 && current > ps.DurationMs {
		current = ps.DurationMs
	}
	return current
}

// QueueItem represents a song in the room's shared queue
type QueueItem struct {
	ID        string `json:"id"`
	SongID    string `json:"song_id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Duration  int    `json:"duration"`
	ImageURL  string `json:"image_url"`
	AddedBy   string `json:"added_by"`
	AddedByID string `json:"added_by_id"`
	AddedAt   int64  `json:"added_at"`
}

// ChatMessage represents a chat bubble in the room
type ChatMessage struct {
	SenderName string    `json:"sender_name"`
	SenderID   string    `json:"sender_id"`
	Text       string    `json:"text"`
	Timestamp  time.Time `json:"timestamp"`
}

// MemberInfo represents a member in the room
type MemberInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsHost   bool   `json:"is_host"`
	JoinedAt int64  `json:"joined_at"`
}

// PartyRoom represents a live listening room
type PartyRoom struct {
	ID                   string
	HostID               string
	HostName             string
	Clients              map[string]*Client
	Playback             PlaybackState
	Queue                []QueueItem
	QueueLocked          bool
	Chat                 []ChatMessage
	Mutex                sync.RWMutex
	Broadcast            chan []byte
	HostGraceTimer       *time.Timer
	HostGraceDuration    time.Duration
	HostIntentionalLeave bool
	HostConnected        bool
	ClientHistory        map[string]int64 // UserID -> JoinedAt epoch ms for election order
	ProcessedEvents      map[string]int64 // EventID -> timestamp ms for deduplication
	LastStateUpdateMs    int64
}

// PartyManager holds all active listening parties
type PartyManager struct {
	Rooms map[string]*PartyRoom
	Mutex sync.RWMutex
}

var GlobalPartyManager = &PartyManager{
	Rooms: make(map[string]*PartyRoom),
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev/testing
	},
}

// CheckAndRecordEvent returns true if event is duplicate
func (r *PartyRoom) CheckAndRecordEvent(eventID string) bool {
	if eventID == "" {
		return false
	}
	now := time.Now().UnixMilli()

	// Prune old events (> 60 seconds)
	if len(r.ProcessedEvents) > 500 {
		cutoff := now - 60000
		for id, ts := range r.ProcessedEvents {
			if ts < cutoff {
				delete(r.ProcessedEvents, id)
			}
		}
	}

	if _, exists := r.ProcessedEvents[eventID]; exists {
		return true
	}
	r.ProcessedEvents[eventID] = now
	return false
}

// ElectNewHost selects the longest-standing connected client to become new host
func (r *PartyRoom) ElectNewHost() *Client {
	var oldestClient *Client
	var oldestTime int64 = 1<<62 - 1

	for _, client := range r.Clients {
		if client.JoinedAt < oldestTime {
			oldestTime = client.JoinedAt
			oldestClient = client
		}
	}
	return oldestClient
}

// REST: Create a new room code
func CreatePartyRoom() gin.HandlerFunc {
	return func(c *gin.Context) {
		hostIDVal, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}
		hostID := hostIDVal.(string)

		hostName := "Host"
		if nameVal, ok := c.Get("first_name"); ok && nameVal != "" {
			hostName = nameVal.(string)
		}

		roomCode := GenerateRoomCode()
		now := time.Now().UnixMilli()

		room := &PartyRoom{
			ID:       roomCode,
			HostID:   hostID,
			HostName: hostName,
			Clients:  make(map[string]*Client),
			Playback: PlaybackState{
				TrackID:          "",
				SongID:           "",
				IsPlaying:        false,
				PlaybackPosition: 0,
				ProgressMs:       0,
				ServerTimestamp:  now,
				DurationMs:       0,
				StateVersion:     1,
				UpdatedBy:        hostID,
			},
			Queue:              []QueueItem{},
			QueueLocked:        false,
			Chat:               []ChatMessage{},
			Broadcast:          make(chan []byte, 256),
			HostGraceDuration:  15 * time.Second,
			HostConnected:      true,
			ClientHistory:      make(map[string]int64),
			ProcessedEvents:    make(map[string]int64),
		}

		GlobalPartyManager.Mutex.Lock()
		GlobalPartyManager.Rooms[roomCode] = room
		GlobalPartyManager.Mutex.Unlock()

		go room.RunBroadcaster()

		log.Printf("🎉 [CreatePartyRoom] Created room: %s with host: %s (%s)\n", roomCode, hostName, hostID)
		c.JSON(http.StatusOK, gin.H{
			"room_code":    roomCode,
			"host_id":      hostID,
			"host_name":    hostName,
			"queue_locked": false,
		})
	}
}

// REST: Check room exists
func CheckPartyRoom() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomCode := c.Param("room_id")
		GlobalPartyManager.Mutex.RLock()
		room, exists := GlobalPartyManager.Rooms[roomCode]
		GlobalPartyManager.Mutex.RUnlock()

		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Room not found"})
			return
		}

		room.Mutex.RLock()
		membersCount := len(room.Clients)
		hostID := room.HostID
		hostName := room.HostName
		queueLocked := room.QueueLocked
		songID := room.Playback.SongID
		isPlaying := room.Playback.IsPlaying
		room.Mutex.RUnlock()

		c.JSON(http.StatusOK, gin.H{
			"room_code":      roomCode,
			"host_id":        hostID,
			"host_name":      hostName,
			"members_count":  membersCount,
			"queue_locked":   queueLocked,
			"current_song":   songID,
			"is_playing":     isPlaying,
		})
	}
}

// WebSocket: Handle client connection and synchronization
func ConnectPartyRoomWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomCode := c.Param("room_id")
		userID := c.Query("user_id")
		userName := c.Query("user_name")

		if userID == "" {
			userID = "guest_" + time.Now().Format("05.000")
		}
		if userName == "" {
			userName = "Guest"
		}

		GlobalPartyManager.Mutex.RLock()
		room, exists := GlobalPartyManager.Rooms[roomCode]
		GlobalPartyManager.Mutex.RUnlock()

		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Room does not exist"})
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println("❌ [WebSocket Upgrade] Error upgrading connection:", err)
			return
		}

		now := time.Now().UnixMilli()

		room.Mutex.Lock()
		// Track or preserve original joined timestamp
		joinedAt, hadJoined := room.ClientHistory[userID]
		if !hadJoined {
			joinedAt = now
			room.ClientHistory[userID] = joinedAt
		}

		// Close any previous stale connection for this userID
		if oldClient, ok := room.Clients[userID]; ok {
			oldClient.Close()
		}

		client := &Client{
			Conn:     conn,
			UserID:   userID,
			UserName: userName,
			Send:     make(chan []byte, 256),
			JoinedAt: joinedAt,
		}

		room.Clients[userID] = client

		// If this is the host reconnecting, cancel the grace timer and restore host connected status
		if userID == room.HostID {
			room.HostConnected = true
			if room.HostGraceTimer != nil {
				room.HostGraceTimer.Stop()
				room.HostGraceTimer = nil
				log.Printf("✅ [Host Reconnected] Grace timer cancelled for room %s\n", roomCode)
			}
		}
		room.Mutex.Unlock()

		log.Printf("🔌 [WebSocket Connect] User %s (%s) joined room %s\n", userName, userID, roomCode)

		// 1. Send authoritative initial snapshot to this client immediately
		room.SendAuthoritativeInitialState(client)

		// 2. Broadcast updated member list to all clients
		room.BroadcastMemberList()

		// 3. Broadcast host status to all clients
		room.BroadcastHostStatus()

		// Start message reader
		go client.ReadPump(room)
	}
}

// RunBroadcaster handles room channel notifications
func (r *PartyRoom) RunBroadcaster() {
	for message := range r.Broadcast {
		r.Mutex.RLock()
		for _, client := range r.Clients {
			_ = client.SendJSON(message)
		}
		r.Mutex.RUnlock()
	}
}

// ReadPump handles incoming WebSocket messages from client
func (c *Client) ReadPump(room *PartyRoom) {
	defer func() {
		c.Close()
		log.Printf("🔌 [WebSocket Disconnect] User %s (%s) disconnected from room %s\n", c.UserName, c.UserID, room.ID)

		room.Mutex.Lock()
		// Remove only if this client is the current active client for this UserID
		if activeClient, ok := room.Clients[c.UserID]; ok && activeClient == c {
			delete(room.Clients, c.UserID)
		}

		isHost := (c.UserID == room.HostID)
		intentionalLeave := room.HostIntentionalLeave
		remainingClients := len(room.Clients)
		room.Mutex.Unlock()

		if isHost {
			if intentionalLeave {
				// Host intentionally closed party room
				log.Printf("🧹 [Host Left] Room %s destroyed (host intentionally closed party)\n", room.ID)
				room.DestroyRoom("Host ended the party room")
				return
			}

			// Host unintentional disconnect (page refresh, network drop)
			room.Mutex.Lock()
			room.HostConnected = false
			if room.HostGraceTimer != nil {
				room.HostGraceTimer.Stop()
			}

			graceDuration := room.HostGraceDuration
			if graceDuration == 0 {
				graceDuration = 15 * time.Second
			}

			log.Printf("⏳ [Host Grace] Starting %v grace period for room %s\n", graceDuration, room.ID)
			room.HostGraceTimer = time.AfterFunc(graceDuration, func() {
				room.HandleHostTimeout()
			})
			room.Mutex.Unlock()

			// Broadcast host disconnected notice with grace duration
			room.BroadcastHostStatus()
			room.BroadcastMemberList()

		} else {
			// Regular member disconnected
			if remainingClients == 0 {
				// No one left in room, destroy
				GlobalPartyManager.Mutex.Lock()
				delete(GlobalPartyManager.Rooms, room.ID)
				GlobalPartyManager.Mutex.Unlock()
				log.Printf("🧹 [Cleanup] Destroyed empty room: %s\n", room.ID)
			} else {
				room.BroadcastMemberList()
			}
		}
	}()

	for {
		var msg map[string]interface{}
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ [ReadPump] Connection read error: %v", err)
			}
			break
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		// Duplicate event prevention
		if eventID, hasEvent := msg["event_id"].(string); hasEvent && eventID != "" {
			room.Mutex.Lock()
			isDuplicate := room.CheckAndRecordEvent(eventID)
			room.Mutex.Unlock()
			if isDuplicate {
				log.Printf("⚠️ [Deduplication] Ignored duplicate event %s from %s\n", eventID, c.UserID)
				continue
			}
		}

		switch msgType {
		// ----------------------------------------------------
		// 1. Authoritative Playback Synchronization (Host only)
		// ----------------------------------------------------
		case "sync_playback":
			room.Mutex.RLock()
			isHost := (c.UserID == room.HostID)
			room.Mutex.RUnlock()

			if !isHost {
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "PERMISSION_DENIED",
					"message":    "Only the room host can control playback synchronization.",
				})
				continue
			}

			// Validate incoming state version to prevent out-of-order execution
			room.Mutex.Lock()
			if clientVersion, ok := msg["state_version"].(float64); ok && int64(clientVersion) > 0 {
				if int64(clientVersion) < room.Playback.StateVersion {
					room.Mutex.Unlock()
					log.Printf("⚠️ [StateVersion] Ignored stale sync (client ver: %d, server ver: %d)\n", int64(clientVersion), room.Playback.StateVersion)
					continue
				}
			}

			now := time.Now().UnixMilli()
			room.Playback.StateVersion++
			room.Playback.UpdatedBy = c.UserID

			if songID, ok := msg["song_id"].(string); ok && songID != "" {
				room.Playback.SongID = songID
				room.Playback.TrackID = songID
			} else if trackID, ok := msg["track_id"].(string); ok && trackID != "" {
				room.Playback.TrackID = trackID
				room.Playback.SongID = trackID
			}

			if durationMs, ok := msg["duration_ms"].(float64); ok {
				room.Playback.DurationMs = int64(durationMs)
			}

			if isPlaying, ok := msg["is_playing"].(bool); ok {
				room.Playback.IsPlaying = isPlaying
			}

			if progressMs, ok := msg["progress_ms"].(float64); ok {
				room.Playback.PlaybackPosition = int64(progressMs)
				room.Playback.ProgressMs = int(progressMs)
			} else if posMs, ok := msg["playback_position_ms"].(float64); ok {
				room.Playback.PlaybackPosition = int64(posMs)
				room.Playback.ProgressMs = int(posMs)
			}

			room.Playback.ServerTimestamp = now
			room.Mutex.Unlock()

			// Broadcast authoritative state to all members
			room.BroadcastPlaybackState(c.UserID)

		// ----------------------------------------------------
		// 2. Queue Management & Locking
		// ----------------------------------------------------
		case "add_to_queue":
			room.Mutex.RLock()
			isHost := (c.UserID == room.HostID)
			isLocked := room.QueueLocked
			room.Mutex.RUnlock()

			if isLocked && !isHost {
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "QUEUE_LOCKED",
					"message":    "Queue is locked. Only the host can add songs.",
				})
				continue
			}

			songData, ok := msg["song"].(map[string]interface{})
			if !ok {
				continue
			}

			songID, _ := songData["song_id"].(string)
			if songID == "" {
				songID, _ = songData["id"].(string)
			}
			if songID == "" {
				continue
			}

			title, _ := songData["title"].(string)
			artist, _ := songData["artist"].(string)
			imageURL, _ := songData["image_url"].(string)
			duration := 0
			if d, ok := songData["duration"].(float64); ok {
				duration = int(d)
			}

			newItem := QueueItem{
				ID:        GenerateRandomID("q_"),
				SongID:    songID,
				Title:     title,
				Artist:    artist,
				Duration:  duration,
				ImageURL:  imageURL,
				AddedBy:   c.UserName,
				AddedByID: c.UserID,
				AddedAt:   time.Now().UnixMilli(),
			}

			room.Mutex.Lock()
			room.Queue = append(room.Queue, newItem)
			room.Mutex.Unlock()

			room.BroadcastQueueState()

		case "remove_from_queue":
			queueItemID, ok := msg["queue_item_id"].(string)
			if !ok || queueItemID == "" {
				continue
			}

			room.Mutex.Lock()
			isHost := (c.UserID == room.HostID)
			isLocked := room.QueueLocked

			itemIndex := -1
			var targetItem QueueItem
			for idx, item := range room.Queue {
				if item.ID == queueItemID {
					itemIndex = idx
					targetItem = item
					break
				}
			}

			if itemIndex == -1 {
				room.Mutex.Unlock()
				continue
			}

			// If locked, only host can remove. If unlocked, host or adder can remove.
			if isLocked && !isHost {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "QUEUE_LOCKED",
					"message":    "Queue is locked. Only the host can remove songs.",
				})
				continue
			}

			if !isHost && targetItem.AddedByID != c.UserID {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "PERMISSION_DENIED",
					"message":    "You can only remove songs you added.",
				})
				continue
			}

			// Remove item from queue
			room.Queue = append(room.Queue[:itemIndex], room.Queue[itemIndex+1:]...)
			room.Mutex.Unlock()

			room.BroadcastQueueState()

		case "toggle_queue_lock":
			room.Mutex.Lock()
			if c.UserID != room.HostID {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "PERMISSION_DENIED",
					"message":    "Only the host can lock or unlock the queue.",
				})
				continue
			}

			if lockVal, ok := msg["locked"].(bool); ok {
				room.QueueLocked = lockVal
			} else {
				room.QueueLocked = !room.QueueLocked
			}
			lockedStatus := room.QueueLocked
			room.Mutex.Unlock()

			room.BroadcastQueueState()

			// Broadcast notice in chat
			systemMsg := "🔓 Host unlocked the shared queue. Anyone can add tracks!"
			if lockedStatus {
				systemMsg = "🔒 Host locked the shared queue. Only the host can add tracks."
			}
			room.BroadcastSystemMessage(systemMsg)

		case "next_track":
			// Host triggers next track from queue
			room.Mutex.Lock()
			if c.UserID != room.HostID {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "PERMISSION_DENIED",
					"message":    "Only the host can advance the track.",
				})
				continue
			}

			if len(room.Queue) > 0 {
				nextSong := room.Queue[0]
				room.Queue = room.Queue[1:]

				now := time.Now().UnixMilli()
				room.Playback.TrackID = nextSong.SongID
				room.Playback.SongID = nextSong.SongID
				room.Playback.IsPlaying = true
				room.Playback.PlaybackPosition = 0
				room.Playback.ProgressMs = 0
				room.Playback.ServerTimestamp = now
				room.Playback.DurationMs = int64(nextSong.Duration * 1000)
				room.Playback.StateVersion++
				room.Playback.UpdatedBy = c.UserID
				room.Mutex.Unlock()

				room.BroadcastPlaybackState("")
				room.BroadcastQueueState()
			} else {
				room.Mutex.Unlock()
			}

		case "clear_queue":
			room.Mutex.Lock()
			if c.UserID != room.HostID {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "PERMISSION_DENIED",
					"message":    "Only the host can clear the queue.",
				})
				continue
			}
			room.Queue = []QueueItem{}
			room.Mutex.Unlock()

			room.BroadcastQueueState()

		// ----------------------------------------------------
		// 3. Host Transfer & Explicit Leave
		// ----------------------------------------------------
		case "transfer_host":
			targetUserID, ok := msg["target_user_id"].(string)
			if !ok || targetUserID == "" {
				continue
			}

			room.Mutex.Lock()
			if c.UserID != room.HostID {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "PERMISSION_DENIED",
					"message":    "Only the current host can transfer host permissions.",
				})
				continue
			}

			targetClient, exists := room.Clients[targetUserID]
			if !exists {
				room.Mutex.Unlock()
				_ = c.SendJSON(gin.H{
					"type":       "error",
					"error_code": "USER_NOT_FOUND",
					"message":    "Target member is not active in this room.",
				})
				continue
			}

			room.HostID = targetClient.UserID
			room.HostName = targetClient.UserName
			room.HostConnected = true
			newHostID := room.HostID
			newHostName := room.HostName
			room.Mutex.Unlock()

			room.BroadcastHostTransferred(newHostID, newHostName, "transferred_by_host")
			room.BroadcastMemberList()
			room.BroadcastSystemMessage("👑 " + c.UserName + " transferred host permissions to " + newHostName)

		case "host_leave":
			// Host deliberately leaves / ends room
			if c.UserID == room.HostID {
				room.Mutex.Lock()
				room.HostIntentionalLeave = true
				room.Mutex.Unlock()
				log.Printf("👋 [Host Leave] Host intentionally closed room %s\n", room.ID)
				room.DestroyRoom("Host ended the party room")
				return
			}

		// ----------------------------------------------------
		// 4. Live Chat
		// ----------------------------------------------------
		case "chat_message":
			text, ok := msg["text"].(string)
			if ok && text != "" {
				chatMsg := ChatMessage{
					SenderName: c.UserName,
					SenderID:   c.UserID,
					Text:       text,
					Timestamp:  time.Now(),
				}
				room.Mutex.Lock()
				room.Chat = append(room.Chat, chatMsg)
				// Limit stored chat logs to 100
				if len(room.Chat) > 100 {
					room.Chat = room.Chat[len(room.Chat)-100:]
				}
				room.Mutex.Unlock()

				room.BroadcastChatMessage(chatMsg)
			}

		// ----------------------------------------------------
		// 5. Ping / Pong for latency & clock offset sync
		// ----------------------------------------------------
		case "ping":
			clientTime, _ := msg["client_time"].(float64)
			_ = c.SendJSON(gin.H{
				"type":             "pong",
				"client_time":      clientTime,
				"server_timestamp": time.Now().UnixMilli(),
			})
		}
	}
}

// SendAuthoritativeInitialState provides full state snapshot to late joiners & reconnecting clients
func (r *PartyRoom) SendAuthoritativeInitialState(c *Client) {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	// 1. Authoritative playback state with server calculated progress
	calculatedPos := r.Playback.CurrentPositionMs()
	_ = c.SendJSON(gin.H{
		"type":                 "playback_state",
		"track_id":             r.Playback.TrackID,
		"song_id":              r.Playback.SongID,
		"is_playing":           r.Playback.IsPlaying,
		"playback_position_ms": calculatedPos,
		"progress_ms":          int(calculatedPos),
		"server_timestamp":     time.Now().UnixMilli(),
		"duration_ms":          r.Playback.DurationMs,
		"state_version":        r.Playback.StateVersion,
		"sender_id":            r.Playback.UpdatedBy,
	})

	// 2. Shared queue and queue lock status
	_ = c.SendJSON(gin.H{
		"type":         "queue_updated",
		"queue":        r.Queue,
		"queue_locked": r.QueueLocked,
	})

	// 3. Room metadata
	_ = c.SendJSON(gin.H{
		"type":           "room_info",
		"room_code":      r.ID,
		"host_id":        r.HostID,
		"host_name":      r.HostName,
		"host_connected": r.HostConnected,
	})

	// 4. Recent chat history (last 30)
	logsLimit := 30
	startIndex := 0
	if len(r.Chat) > logsLimit {
		startIndex = len(r.Chat) - logsLimit
	}
	for i := startIndex; i < len(r.Chat); i++ {
		_ = c.SendJSON(gin.H{
			"type":        "chat_message",
			"sender_name": r.Chat[i].SenderName,
			"sender_id":   r.Chat[i].SenderID,
			"text":        r.Chat[i].Text,
			"timestamp":   r.Chat[i].Timestamp,
		})
	}
}

// BroadcastPlaybackState broadcasts updated playback state
func (r *PartyRoom) BroadcastPlaybackState(senderID string) {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	payload := gin.H{
		"type":                 "playback_state",
		"track_id":             r.Playback.TrackID,
		"song_id":              r.Playback.SongID,
		"is_playing":           r.Playback.IsPlaying,
		"playback_position_ms": r.Playback.PlaybackPosition,
		"progress_ms":          r.Playback.ProgressMs,
		"server_timestamp":     r.Playback.ServerTimestamp,
		"duration_ms":          r.Playback.DurationMs,
		"state_version":        r.Playback.StateVersion,
		"sender_id":            senderID,
	}

	for _, client := range r.Clients {
		_ = client.SendJSON(payload)
	}
}

// BroadcastQueueState broadcasts updated shared queue & lock state
func (r *PartyRoom) BroadcastQueueState() {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	payload := gin.H{
		"type":         "queue_updated",
		"queue":        r.Queue,
		"queue_locked": r.QueueLocked,
	}

	for _, client := range r.Clients {
		_ = client.SendJSON(payload)
	}
}

// BroadcastMemberList broadcasts members list with host indicator
func (r *PartyRoom) BroadcastMemberList() {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	var memberNames []string
	var memberDetails []MemberInfo

	for _, client := range r.Clients {
		memberNames = append(memberNames, client.UserName)
		memberDetails = append(memberDetails, MemberInfo{
			ID:       client.UserID,
			Name:     client.UserName,
			IsHost:   client.UserID == r.HostID,
			JoinedAt: client.JoinedAt,
		})
	}

	payload := gin.H{
		"type":           "member_list",
		"members":        memberNames,
		"member_details": memberDetails,
		"host_id":        r.HostID,
		"host_name":      r.HostName,
		"host_connected": r.HostConnected,
	}

	for _, client := range r.Clients {
		_ = client.SendJSON(payload)
	}
}

// BroadcastHostStatus sends current host connectivity status
func (r *PartyRoom) BroadcastHostStatus() {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	payload := gin.H{
		"type":           "host_status",
		"connected":      r.HostConnected,
		"host_id":        r.HostID,
		"host_name":      r.HostName,
		"grace_duration": r.HostGraceDuration.Seconds(),
	}

	for _, client := range r.Clients {
		_ = client.SendJSON(payload)
	}
}

// BroadcastHostTransferred informs room of host change
func (r *PartyRoom) BroadcastHostTransferred(newHostID, newHostName, reason string) {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	payload := gin.H{
		"type":          "host_transferred",
		"new_host_id":   newHostID,
		"new_host_name": newHostName,
		"reason":        reason,
	}

	for _, client := range r.Clients {
		_ = client.SendJSON(payload)
	}
}

// BroadcastChatMessage sends a chat message to everyone in room
func (r *PartyRoom) BroadcastChatMessage(msg ChatMessage) {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	payload := gin.H{
		"type":        "chat_message",
		"sender_name": msg.SenderName,
		"sender_id":   msg.SenderID,
		"text":        msg.Text,
		"timestamp":   msg.Timestamp,
	}

	for _, client := range r.Clients {
		_ = client.SendJSON(payload)
	}
}

// BroadcastSystemMessage sends a system notification to the chat
func (r *PartyRoom) BroadcastSystemMessage(text string) {
	systemMsg := ChatMessage{
		SenderName: "System",
		SenderID:   "system",
		Text:       text,
		Timestamp:  time.Now(),
	}
	r.BroadcastChatMessage(systemMsg)
}

// HandleHostTimeout is called when host grace period expires without reconnection
func (r *PartyRoom) HandleHostTimeout() {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	if r.HostConnected {
		return // Host reconnected in time
	}

	log.Printf("⏳ [Host Timeout] Host did not reconnect to room %s\n", r.ID)

	// If no connected clients remaining, clean up the room
	if len(r.Clients) == 0 {
		GlobalPartyManager.Mutex.Lock()
		delete(GlobalPartyManager.Rooms, r.ID)
		GlobalPartyManager.Mutex.Unlock()
		log.Printf("🧹 [Cleanup] Destroyed abandoned room %s\n", r.ID)
		return
	}

	// Host election: Elect longest-standing active member
	newHost := r.ElectNewHost()
	if newHost != nil {
		r.HostID = newHost.UserID
		r.HostName = newHost.UserName
		r.HostConnected = true
		r.HostGraceTimer = nil

		log.Printf("👑 [Host Election] Room %s elected new host: %s (%s)\n", r.ID, newHost.UserName, newHost.UserID)

		// Broadcast host transferred and updated member list
		go func(hostID, hostName string) {
			r.BroadcastHostTransferred(hostID, hostName, "host_timeout")
			r.BroadcastMemberList()
			r.BroadcastSystemMessage("👑 Host disconnected. " + hostName + " has been elected as the new party host!")
		}(newHost.UserID, newHost.UserName)
	}
}

// DestroyRoom ends the party room and disconnects all clients
func (r *PartyRoom) DestroyRoom(reason string) {
	GlobalPartyManager.Mutex.Lock()
	delete(GlobalPartyManager.Rooms, r.ID)
	GlobalPartyManager.Mutex.Unlock()

	r.Mutex.Lock()
	for _, member := range r.Clients {
		_ = member.SendJSON(gin.H{
			"type":   "room_ended",
			"reason": reason,
		})
		member.Close()
	}
	r.Clients = make(map[string]*Client)
	r.Mutex.Unlock()
}

// GenerateRandomID creates a prefixed random ID string
func GenerateRandomID(prefix string) string {
	letters := "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		} else {
			b[i] = letters[n.Int64()]
		}
	}
	return prefix + string(b)
}

// Helper: Generates unique crypto-random 6-character uppercase Room IDs like "GH-X8F9A2"
func GenerateRoomCode() string {
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := "GH-"
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			code += string(letters[time.Now().UnixNano()%int64(len(letters))])
		} else {
			code += string(letters[n.Int64()])
		}
	}
	return code
}

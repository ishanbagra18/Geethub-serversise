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
	Conn     *websocket.Conn
	UserID   string
	UserName string
	Send     chan []byte
}

// PlaybackState represents current music state of the room
type PlaybackState struct {
	SongID     string `json:"song_id"`
	IsPlaying  bool   `json:"is_playing"`
	ProgressMs int    `json:"progress_ms"`
}

// ChatMessage represents a chat bubble in the room
type ChatMessage struct {
	SenderName string    `json:"sender_name"`
	Text       string    `json:"text"`
	Timestamp  time.Time `json:"timestamp"`
}

// PartyRoom represents a live listening room
type PartyRoom struct {
	ID                   string
	HostID               string
	Clients              map[string]*Client
	Playback             PlaybackState
	Chat                 []ChatMessage
	Mutex                sync.Mutex
	Broadcast            chan []byte
	HostGraceTimer       *time.Timer // Grace period timer for host reconnection on refresh
	HostIntentionalLeave bool        // Set when host explicitly clicks Leave Party
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
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev/testing
	},
}

// REST: Create a new room code
func CreatePartyRoom() gin.HandlerFunc {
	return func(c *gin.Context) {
		hostID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Generate a simple unique room code
		roomCode := GenerateRoomCode()

		GlobalPartyManager.Mutex.Lock()
		room := &PartyRoom{
			ID:      roomCode,
			HostID:  hostID.(string),
			Clients: make(map[string]*Client),
			Playback: PlaybackState{
				SongID:     "",
				IsPlaying:  false,
				ProgressMs: 0,
			},
			Chat:      []ChatMessage{},
			Broadcast: make(chan []byte, 256),
		}
		GlobalPartyManager.Rooms[roomCode] = room
		GlobalPartyManager.Mutex.Unlock()

		// Start broadcast routine for this room
		go room.RunBroadcaster()

		log.Printf("🎉 [CreatePartyRoom] Created room: %s with host: %s\n", roomCode, hostID)
		c.JSON(http.StatusOK, gin.H{
			"room_code": roomCode,
			"host_id":   hostID,
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

		c.JSON(http.StatusOK, gin.H{
			"room_code": room.ID,
			"host_id":   room.HostID,
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

		client := &Client{
			Conn:     conn,
			UserID:   userID,
			UserName: userName,
			Send:     make(chan []byte, 256),
		}

		// Register client in the room
		room.Mutex.Lock()
		room.Clients[userID] = client

		// If the host is reconnecting (e.g. page refresh), cancel the grace timer
		if userID == room.HostID && room.HostGraceTimer != nil {
			room.HostGraceTimer.Stop()
			room.HostGraceTimer = nil
			log.Printf("✅ [Host Reconnected] Grace timer cancelled for room %s\n", roomCode)
		}
		room.Mutex.Unlock()

		log.Printf("🔌 [WebSocket Connect] User %s (%s) joined room %s\n", userName, userID, roomCode)

		// Send initial sync state and member list
		room.SendInitialState(client)
		room.BroadcastMemberList()

		// Start reader and writer routines
		go client.WritePump()
		go client.ReadPump(room)
	}
}

// RunBroadcaster handles room channel notifications and forwards them to all connections
func (r *PartyRoom) RunBroadcaster() {
	for message := range r.Broadcast {
		r.Mutex.Lock()
		for userID, client := range r.Clients {
			select {
			case client.Send <- message:
			default:
				close(client.Send)
				delete(r.Clients, userID)
			}
		}
		r.Mutex.Unlock()
	}
}

// ReadPump handles messages received from the client
func (c *Client) ReadPump(room *PartyRoom) {
	defer func() {
		c.Conn.Close()
		log.Printf("🔌 [WebSocket Disconnect] User %s (%s) left room %s\n", c.UserName, c.UserID, room.ID)

		if c.UserID == room.HostID {
			// Host disconnected: remove from clients
			room.Mutex.Lock()
			delete(room.Clients, c.UserID)
			intentionalLeave := room.HostIntentionalLeave
			room.Mutex.Unlock()

			if intentionalLeave {
				// Host clicked "Leave Party" — destroy room immediately, no grace period
				log.Printf("🧹 [Host Left] Room %s destroyed (host intentionally left)\n", room.ID)

				GlobalPartyManager.Mutex.Lock()
				delete(GlobalPartyManager.Rooms, room.ID)
				GlobalPartyManager.Mutex.Unlock()

				room.Mutex.Lock()
				for _, member := range room.Clients {
					member.Conn.WriteJSON(map[string]string{
						"type": "room_ended",
					})
					member.Conn.Close()
				}
				room.Mutex.Unlock()
			} else {
				// Page refresh / accidental disconnect — start grace period
				log.Printf("⏳ [Host Grace] Starting 10s reconnection grace period for room %s\n", room.ID)

				room.Mutex.Lock()
				if room.HostGraceTimer != nil {
					room.HostGraceTimer.Stop()
				}
				room.HostGraceTimer = time.AfterFunc(10*time.Second, func() {
					log.Printf("🧹 [Host Left] Room %s destroyed (host did not reconnect within grace period)\n", room.ID)

					GlobalPartyManager.Mutex.Lock()
					delete(GlobalPartyManager.Rooms, room.ID)
					GlobalPartyManager.Mutex.Unlock()

					room.Mutex.Lock()
					for _, member := range room.Clients {
						member.Conn.WriteJSON(map[string]string{
							"type": "room_ended",
						})
						member.Conn.Close()
					}
					room.Mutex.Unlock()
				})
				room.Mutex.Unlock()

				// Update member list (host temporarily absent)
				room.BroadcastMemberList()
			}
		} else {
			// A regular member left
			room.Mutex.Lock()
			delete(room.Clients, c.UserID)
			clientsCount := len(room.Clients)
			room.Mutex.Unlock()

			if clientsCount == 0 {
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
				log.Printf("❌ [ReadPump] error: %v", err)
			}
			break
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "sync_playback":
			// Only allow the host to broadcast playback updates
			if c.UserID == room.HostID {
				room.Mutex.Lock()
				if songID, ok := msg["song_id"].(string); ok {
					room.Playback.SongID = songID
				}
				if isPlaying, ok := msg["is_playing"].(bool); ok {
					room.Playback.IsPlaying = isPlaying
				}
				if progressMs, ok := msg["progress_ms"].(float64); ok {
					room.Playback.ProgressMs = int(progressMs)
				}
				room.Mutex.Unlock()

				// Broadcast to all members (except host to prevent loops)
				room.BroadcastPlaybackState(c.UserID)
			}
		case "chat_message":
			text, ok := msg["text"].(string)
			if ok && text != "" {
				chatMsg := ChatMessage{
					SenderName: c.UserName,
					Text:       text,
					Timestamp:  time.Now(),
				}
				room.Mutex.Lock()
				room.Chat = append(room.Chat, chatMsg)
				room.Mutex.Unlock()

				room.BroadcastChatMessage(chatMsg)
			}
		case "host_leave":
			// Host is intentionally leaving — mark the room for immediate destruction
			if c.UserID == room.HostID {
				room.Mutex.Lock()
				room.HostIntentionalLeave = true
				room.Mutex.Unlock()
				log.Printf("👋 [Host Leave] Host intentionally left room %s\n", room.ID)
			}
		}
	}
}

// WritePump writes messages from the queue to the client websocket connection
func (c *Client) WritePump() {
	defer func() {
		c.Conn.Close()
	}()
	for message := range c.Send {
		err := c.Conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}

// Helper: Sends initial state to newly connected client
func (r *PartyRoom) SendInitialState(c *Client) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	// 1. Send current playback state
	c.Conn.WriteJSON(gin.H{
		"type":        "playback_state",
		"song_id":     r.Playback.SongID,
		"is_playing":  r.Playback.IsPlaying,
		"progress_ms": r.Playback.ProgressMs,
	})

	// 2. Send previous chat logs (limit to last 20)
	logsLimit := 20
	startIndex := 0
	if len(r.Chat) > logsLimit {
		startIndex = len(r.Chat) - logsLimit
	}
	for i := startIndex; i < len(r.Chat); i++ {
		c.Conn.WriteJSON(gin.H{
			"type":        "chat_message",
			"sender_name": r.Chat[i].SenderName,
			"text":        r.Chat[i].Text,
			"timestamp":   r.Chat[i].Timestamp,
		})
	}
}

// Broadcasts playback details to members in the room
func (r *PartyRoom) BroadcastPlaybackState(senderID string) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	payload := gin.H{
		"type":        "playback_state",
		"song_id":     r.Playback.SongID,
		"is_playing":  r.Playback.IsPlaying,
		"progress_ms": r.Playback.ProgressMs,
		"sender_id":   senderID,
	}

	for _, client := range r.Clients {
		if client.UserID == senderID {
			continue // Skip sending back to host
		}
		client.Conn.WriteJSON(payload)
	}
}

// Broadcasts chat messages to everyone in the room
func (r *PartyRoom) BroadcastChatMessage(msg ChatMessage) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	payload := gin.H{
		"type":        "chat_message",
		"sender_name": msg.SenderName,
		"text":        msg.Text,
		"timestamp":   msg.Timestamp,
	}

	for _, client := range r.Clients {
		client.Conn.WriteJSON(payload)
	}
}

// Broadcasts current list of members in the room
func (r *PartyRoom) BroadcastMemberList() {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	var members []string
	for _, client := range r.Clients {
		members = append(members, client.UserName)
	}

	payload := gin.H{
		"type":    "member_list",
		"members": members,
	}

	for _, client := range r.Clients {
		client.Conn.WriteJSON(payload)
	}
}

// Helper: Generates unique crypto-random 6-character uppercase Room IDs like "GH-X8F9A2"
func GenerateRoomCode() string {
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := "GH-"
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			// Fallback if crypto/rand fails (extremely rare)
			code += string(letters[time.Now().UnixNano()%int64(len(letters))])
		} else {
			code += string(letters[n.Int64()])
		}
	}
	return code
}

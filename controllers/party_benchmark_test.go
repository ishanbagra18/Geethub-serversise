package controllers_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/controllers"
)

// BenchmarkPartyRoom_EventDeduplication measures cryptographic event deduplication throughput
func BenchmarkPartyRoom_EventDeduplication(b *testing.B) {
	room := &controllers.PartyRoom{
		ProcessedEvents: make(map[string]int64),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eventID := fmt.Sprintf("evt_%d", i%500)
		_ = room.CheckAndRecordEvent(eventID)
	}
}

// BenchmarkPartyRoom_CurrentPositionCalculation measures elapsed server timestamp interpolation
func BenchmarkPartyRoom_CurrentPositionCalculation(b *testing.B) {
	state := &controllers.PlaybackState{
		TrackID:          "track_123",
		IsPlaying:        true,
		PlaybackPosition: 15000,
		ServerTimestamp:  time.Now().UnixMilli() - 5000,
		DurationMs:       240000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pos := state.CurrentPositionMs()
		if pos < 15000 {
			b.Fatal("Expected progress >= 15000")
		}
	}
}

// BenchmarkPartyRoom_BroadcastFanout measures in-memory message fan-out to 100 concurrent clients
func BenchmarkPartyRoom_BroadcastFanout(b *testing.B) {
	const clientCount = 100
	clients := make([]chan []byte, clientCount)
	for i := 0; i < clientCount; i++ {
		clients[i] = make(chan []byte, 64)
	}

	// Consumer drain goroutines
	var wg sync.WaitGroup
	stopConsumers := make(chan struct{})

	for i := 0; i < clientCount; i++ {
		ch := clients[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ch:
				case <-stopConsumers:
					return
				}
			}
		}()
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"type":        "PLAYBACK_STATE",
		"track_id":    "song_456",
		"is_playing":  true,
		"position_ms": 34500,
		"timestamp":   time.Now().UnixMilli(),
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for c := 0; c < clientCount; c++ {
			select {
			case clients[c] <- payload:
			default:
			}
		}
	}

	b.StopTimer()
	close(stopConsumers)
	wg.Wait()
}

package services

import (
	"context"
	"strings"
	"testing"
)

func TestMapJamendoTrackToSong(t *testing.T) {
	track := JamendoTrack{
		ID:         "1885652",
		Name:       "Workout Beat Machine",
		Duration:   210,
		ArtistName: "MaxKoMusic",
		AlbumName:  "High Energy Workout",
		Image:      "https://usercontent.jamendo.com?type=album&id=391204&width=300",
		Audio:      "https://prod-1.storage.jamendo.com/download/track/1885652/mp32/",
		LicenseCCURL: "https://creativecommons.org/licenses/by/3.0/",
	}
	track.MusicInfo.Tags.Genres = []string{"dance", "edm", "workout"}

	song := MapJamendoTrackToSong(track)

	if song.SongID != "jamendo_1885652" {
		t.Errorf("Expected SongID 'jamendo_1885652', got '%s'", song.SongID)
	}

	if song.Title == nil || *song.Title != "Workout Beat Machine" {
		t.Errorf("Expected title 'Workout Beat Machine'")
	}

	if song.Artist == nil || *song.Artist != "MaxKoMusic" {
		t.Errorf("Expected artist 'MaxKoMusic'")
	}

	if song.FileURL == nil || !strings.Contains(*song.FileURL, "jamendo.com") {
		t.Errorf("Expected valid audio URL on Jamendo CDN, got '%v'", song.FileURL)
	}

	if song.Info == nil || !strings.Contains(*song.Info, "Creative Commons") {
		t.Errorf("Expected Info to contain Creative Commons attribution")
	}

	if song.IsPrivate != nil && *song.IsPrivate {
		t.Errorf("Expected IsPrivate to be false for CC tracks")
	}
}

func TestSearchJamendoTracksFallback(t *testing.T) {
	ctx := context.Background()
	songs, err := SearchJamendoTracks(ctx, "workout", 10)
	if err != nil {
		t.Fatalf("SearchJamendoTracks failed: %v", err)
	}

	if len(songs) == 0 {
		t.Fatalf("Expected at least 1 track returned for 'workout'")
	}

	first := songs[0]
	if !strings.HasPrefix(first.SongID, "jamendo_") {
		t.Errorf("Expected SongID to start with 'jamendo_', got '%s'", first.SongID)
	}

	if first.FileURL == nil || *first.FileURL == "" {
		t.Errorf("Expected playable audio URL for track")
	}
}

func TestGetJamendoTrackByID(t *testing.T) {
	ctx := context.Background()
	song, err := GetJamendoTrackByID(ctx, "jamendo_1885652")
	if err != nil {
		t.Fatalf("GetJamendoTrackByID error: %v", err)
	}

	if song == nil {
		t.Fatalf("Expected song object to be returned")
	}

	if song.SongID != "jamendo_1885652" {
		t.Errorf("Expected SongID 'jamendo_1885652', got '%s'", song.SongID)
	}
}

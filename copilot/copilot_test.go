package copilot

import (
	"strings"
	"testing"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

func TestExtractIntentSemantic(t *testing.T) {
	prompt := "Play something energetic for a 30-minute workout, but avoid songs I've already heard today."
	intent := parseSemantically(prompt)

	if intent.Mood != "Energetic" {
		t.Errorf("Expected mood 'Energetic', got '%s'", intent.Mood)
	}

	if intent.Activity != "Workout" {
		t.Errorf("Expected activity 'Workout', got '%s'", intent.Activity)
	}

	if intent.DurationMinutes != 30 {
		t.Errorf("Expected duration 30, got %d", intent.DurationMinutes)
	}

	if !intent.AvoidHeardToday {
		t.Errorf("Expected AvoidHeardToday to be true")
	}

	if intent.Action != "play_now" {
		t.Errorf("Expected action 'play_now', got '%s'", intent.Action)
	}
}

func TestExtractIntentVariations(t *testing.T) {
	testCases := []struct {
		query           string
		expectedMood    string
		expectedAct     string
		expectedDur     int
		expectedAvoid   bool
		expectedLang    string
		expectedAction  string
	}{
		{
			query:          "Create playlist with chill acoustic songs for 45 minutes of study",
			expectedMood:   "Chill",
			expectedAct:    "Study",
			expectedDur:    45,
			expectedAvoid:  false,
			expectedLang:   "Any",
			expectedAction: "create_playlist",
		},
		{
			query:          "Queue up upbeat Punjabi party bangers for a 1 hour road trip, skip songs heard today",
			expectedMood:   "Party",
			expectedAct:    "Driving",
			expectedDur:    60,
			expectedAvoid:  true,
			expectedLang:   "Punjabi",
			expectedAction: "queue_songs",
		},
		{
			query:          "Romantic Hindi melodies for 20m relaxation",
			expectedMood:   "Romantic",
			expectedAct:    "Relaxation",
			expectedDur:    20,
			expectedAvoid:  false,
			expectedLang:   "Hindi",
			expectedAction: "play_now",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			intent := parseSemantically(tc.query)
			if intent.Mood != tc.expectedMood {
				t.Errorf("Query '%s': Expected mood '%s', got '%s'", tc.query, tc.expectedMood, intent.Mood)
			}
			if intent.Activity != tc.expectedAct {
				t.Errorf("Query '%s': Expected act '%s', got '%s'", tc.query, tc.expectedAct, intent.Activity)
			}
			if intent.DurationMinutes != tc.expectedDur {
				t.Errorf("Query '%s': Expected dur %d, got %d", tc.query, tc.expectedDur, intent.DurationMinutes)
			}
			if intent.AvoidHeardToday != tc.expectedAvoid {
				t.Errorf("Query '%s': Expected avoid %v, got %v", tc.query, tc.expectedAvoid, intent.AvoidHeardToday)
			}
			if intent.Language != tc.expectedLang {
				t.Errorf("Query '%s': Expected lang '%s', got '%s'", tc.query, tc.expectedLang, intent.Language)
			}
			if intent.Action != tc.expectedAction {
				t.Errorf("Query '%s': Expected action '%s', got '%s'", tc.query, tc.expectedAction, intent.Action)
			}
		})
	}
}

func TestScoreSongForIntent(t *testing.T) {
	workoutTitle := "Gym Power Beat"
	workoutGenre := "EDM Dance Workout"
	workoutSong := models.Song{
		Title:     &workoutTitle,
		Genre:     &workoutGenre,
		PlayCount: 50,
	}

	chillTitle := "Midnight Rain Lo-Fi"
	chillGenre := "Lo-fi Acoustic"
	chillSong := models.Song{
		Title:     &chillTitle,
		Genre:     &chillGenre,
		PlayCount: 10,
	}

	intent := models.ExtractedIntent{
		Mood:            "Energetic",
		Activity:        "Workout",
		Language:        "Any",
		DurationMinutes: 30,
	}

	wScore, _, _ := scoreSongForIntent(workoutSong, intent)
	cScore, _, _ := scoreSongForIntent(chillSong, intent)

	if wScore <= cScore {
		t.Errorf("Expected workout song score (%f) to be significantly higher than chill song (%f)", wScore, cScore)
	}
}

func TestBuildConversationalSummary(t *testing.T) {
	intent := models.ExtractedIntent{
		Mood:            "Energetic",
		Activity:        "Workout",
		AvoidHeardToday: true,
	}
	msg := buildConversationalSummary(intent, 8, 28, 3)
	if !strings.Contains(msg, "excluded 3 song(s)") {
		t.Errorf("Expected summary to mention excluded songs, got: %s", msg)
	}
}

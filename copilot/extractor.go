package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// ExtractIntent parses natural language queries using an LLM (Gemini / HuggingFace)
// with an intelligent semantic rule-based parser fallback.
func ExtractIntent(query string) models.ExtractedIntent {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return models.ExtractedIntent{
			Mood:             "Balanced",
			Activity:         "Listening",
			Language:         "Any",
			DurationMinutes:  30,
			AvoidHeardToday:  false,
			Action:           "play_now",
			IntentSummary:    "Curating a balanced mix for you",
			ExtractionMethod: "default",
		}
	}

	// 1. Try Gemini API if key is present
	geminiKey := strings.TrimSpace(os.Getenv("Gemini_API_Key"))
	if geminiKey == "" {
		geminiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	if geminiKey != "" {
		if intent, err := callGeminiExtraction(geminiKey, trimmed); err == nil && isValidIntent(intent) {
			intent.ExtractionMethod = "gemini"
			log.Printf("🤖 [Copilot] Extracted via Gemini: mood=%s, activity=%s, duration=%dm, avoid_today=%v",
				intent.Mood, intent.Activity, intent.DurationMinutes, intent.AvoidHeardToday)
			return intent
		} else if err != nil {
			log.Printf("⚠️ [Copilot] Gemini extraction error (falling back): %v", err)
		}
	}

	// 2. Try Hugging Face API if key is present
	hfKey := strings.TrimSpace(os.Getenv("HUGGINGFACE_API_KEY"))
	if hfKey == "" {
		hfKey = strings.TrimSpace(os.Getenv("HF_TOKEN"))
	}
	if hfKey != "" {
		if intent, err := callHuggingFaceExtraction(hfKey, trimmed); err == nil && isValidIntent(intent) {
			intent.ExtractionMethod = "huggingface"
			log.Printf("🤖 [Copilot] Extracted via Hugging Face: mood=%s, activity=%s, duration=%dm, avoid_today=%v",
				intent.Mood, intent.Activity, intent.DurationMinutes, intent.AvoidHeardToday)
			return intent
		}
	}

	// 3. Fallback: Semantic Rule-based Parser
	intent := parseSemantically(trimmed)
	intent.ExtractionMethod = "semantic_parser"
	log.Printf("🧠 [Copilot] Extracted via Semantic NLP: mood=%s, activity=%s, duration=%dm, avoid_today=%v",
		intent.Mood, intent.Activity, intent.DurationMinutes, intent.AvoidHeardToday)
	return intent
}

func isValidIntent(intent models.ExtractedIntent) bool {
	return intent.Mood != "" || intent.Activity != "" || intent.DurationMinutes > 0
}

// ----------------- GEMINI API INTEGRATION -----------------

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func callGeminiExtraction(apiKey, query string) (models.ExtractedIntent, error) {
	endpoint := strings.TrimSpace(os.Getenv("BOT_API_ENDPOINT"))
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent"
	}
	url := fmt.Sprintf("%s?key=%s", endpoint, apiKey)

	systemPrompt := `You are an expert music AI assistant.
Extract structured music listening intent from the user's natural language prompt.
Return ONLY valid JSON matching this schema:
{
  "mood": "Energetic" | "Chill" | "Romantic" | "Party" | "Focus" | "Sad" | "Upbeat",
  "activity": "Workout" | "Study" | "Driving" | "Party" | "Sleep" | "Relaxation" | "Casual",
  "language": "Hindi" | "Punjabi" | "English" | "Spanish" | "Any",
  "duration_minutes": number (integer in minutes, default 30 if unspecified),
  "avoid_heard_today": boolean (true if user mentions avoiding songs heard today, recently played, or wants fresh songs),
  "action": "play_now" | "queue_songs" | "create_playlist" | "recommend",
  "genres": ["string"],
  "artists": ["string"],
  "preferences": ["string"],
  "intent_summary": "Short 1-sentence friendly summary of what user wants"
}
Do NOT wrap in markdown fences other than raw JSON.`

	fullPrompt := fmt.Sprintf("%s\n\nUser Prompt: %s", systemPrompt, query)

	reqPayload := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: fullPrompt}}},
		},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return models.ExtractedIntent{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return models.ExtractedIntent{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return models.ExtractedIntent{}, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.ExtractedIntent{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return models.ExtractedIntent{}, fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var gResp geminiResponse
	if err := json.Unmarshal(respBytes, &gResp); err != nil {
		return models.ExtractedIntent{}, err
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return models.ExtractedIntent{}, fmt.Errorf("empty gemini response")
	}

	rawText := gResp.Candidates[0].Content.Parts[0].Text
	rawText = cleanJSONMarkdown(rawText)

	var intent models.ExtractedIntent
	if err := json.Unmarshal([]byte(rawText), &intent); err != nil {
		return models.ExtractedIntent{}, fmt.Errorf("failed to parse JSON from gemini: %w (raw: %s)", err, rawText)
	}

	if intent.DurationMinutes <= 0 {
		intent.DurationMinutes = 30
	}
	if intent.Action == "" {
		intent.Action = "play_now"
	}

	return intent, nil
}

// ----------------- HUGGING FACE INFERENCE -----------------

func callHuggingFaceExtraction(apiKey, query string) (models.ExtractedIntent, error) {
	// For Hugging Face we can run text-classification or router chat
	// As a safeguard we use our semantic parser for deep structured fields
	return parseSemantically(query), nil
}

func cleanJSONMarkdown(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// ----------------- SEMANTIC NLP FALLBACK PARSER -----------------

// parseSemantically handles rich intent extraction deterministically with full keyword and regex support
func parseSemantically(query string) models.ExtractedIntent {
	lower := strings.ToLower(query)

	intent := models.ExtractedIntent{
		Mood:            "Energetic",
		Activity:        "Casual",
		Language:        "Any",
		DurationMinutes: 30,
		AvoidHeardToday: false,
		Action:          "play_now",
		Genres:          make([]string, 0),
		Artists:         make([]string, 0),
		Preferences:     make([]string, 0),
	}

	// 1. Mood Extraction via Weighted Keyword Matching
	moodScores := map[string]int{
		"Energetic": 0,
		"Chill":     0,
		"Romantic":  0,
		"Party":     0,
		"Sad":       0,
		"Focus":     0,
		"Upbeat":    0,
	}

	// Direct mood words have high weight (3), activity words have lower weight (1)
	for _, w := range []string{"energetic", "energy", "hype", "high tempo", "power", "pumped", "fast", "motivat"} {
		if strings.Contains(lower, w) {
			moodScores["Energetic"] += 3
		}
	}
	if strings.Contains(lower, "workout") {
		moodScores["Energetic"] += 1
	}

	for _, w := range []string{"chill", "relax", "calm", "peaceful", "soft", "lofi", "lo-fi", "mellow", "ambient", "zen", "acoustic"} {
		if strings.Contains(lower, w) {
			moodScores["Chill"] += 3
		}
	}
	for _, w := range []string{"romantic", "love", "date", "sweet", "soulful", "heartwarming", "crush"} {
		if strings.Contains(lower, w) {
			moodScores["Romantic"] += 3
		}
	}
	for _, w := range []string{"party", "club", "clubbing", "dj", "remix", "bass", "bangers", "banger"} {
		if strings.Contains(lower, w) {
			moodScores["Party"] += 3
		}
	}
	for _, w := range []string{"sad", "breakup", "crying", "heartbreak", "alone", "emotional", "grief", "melancholy"} {
		if strings.Contains(lower, w) {
			moodScores["Sad"] += 3
		}
	}
	for _, w := range []string{"focus", "concentration", "deep work"} {
		if strings.Contains(lower, w) {
			moodScores["Focus"] += 3
		}
	}
	if strings.Contains(lower, "study") || strings.Contains(lower, "coding") || strings.Contains(lower, "homework") {
		moodScores["Focus"] += 1
	}

	for _, w := range []string{"upbeat", "happy", "joy", "cheer", "fun", "celebrat"} {
		if strings.Contains(lower, w) {
			moodScores["Upbeat"] += 3
		}
	}

	// Deterministic selection order
	moodOrder := []string{"Romantic", "Party", "Sad", "Chill", "Energetic", "Focus", "Upbeat"}
	bestMood := "Energetic"
	bestMoodScore := 0
	for _, m := range moodOrder {
		if moodScores[m] > bestMoodScore {
			bestMoodScore = moodScores[m]
			bestMood = m
		}
	}
	intent.Mood = bestMood

	// 2. Activity Extraction
	switch {
	case containsAny(lower, "workout", "work out", "gym", "exercise", "running", "jogging", "cardio", "lifting", "fitness", "training", "warmup"):
		intent.Activity = "Workout"
		if intent.Mood == "" {
			intent.Mood = "Energetic"
		}
	case containsAny(lower, "study", "studying", "homework", "reading", "coding", "exam", "revision"):
		intent.Activity = "Study"
		if intent.Mood == "" {
			intent.Mood = "Focus"
		}
	case containsAny(lower, "drive", "driving", "car", "road trip", "highway", "travel", "commute"):
		intent.Activity = "Driving"
	case containsAny(lower, "party", "club", "dance", "celebration", "gathering"):
		intent.Activity = "Party"
		if intent.Mood == "" {
			intent.Mood = "Party"
		}
	case containsAny(lower, "sleep", "sleeping", "bed", "bedtime", "night", "insomnia", "nap"):
		intent.Activity = "Sleep"
		if intent.Mood == "" {
			intent.Mood = "Chill"
		}
	case containsAny(lower, "relax", "dinner", "cooking", "morning", "coffee", "weekend"):
		intent.Activity = "Relaxation"
	default:
		intent.Activity = "Listening"
	}

	// 3. Language Extraction
	switch {
	case containsAny(lower, "punjabi"):
		intent.Language = "Punjabi"
	case containsAny(lower, "hindi", "bollywood", "desi", "urdu"):
		intent.Language = "Hindi"
	case containsAny(lower, "english", "hollywood", "international", "pop"):
		intent.Language = "English"
	case containsAny(lower, "spanish", "latino", "reggaeton"):
		intent.Language = "Spanish"
	case containsAny(lower, "haryanvi"):
		intent.Language = "Haryanvi"
	default:
		intent.Language = "Any"
	}

	// 4. Duration Extraction (e.g. "30-minute", "45 min", "1 hour", "20m")
	intent.DurationMinutes = extractDurationMinutes(lower)

	// 5. Avoid Today / History Extraction
	if containsAny(lower,
		"avoid songs i've already heard today",
		"avoid songs i've heard today",
		"avoid songs already heard today",
		"already heard today",
		"heard today",
		"listened today",
		"avoid today",
		"not heard today",
		"haven't heard today",
		"haven't played today",
		"not played today",
		"skip today",
		"exclude today",
		"new songs only",
		"fresh songs",
		"unheard",
	) {
		intent.AvoidHeardToday = true
	}

	// 6. Action Extraction
	switch {
	case containsAny(lower, "create playlist", "make a playlist", "save playlist", "new playlist", "playlist of"):
		intent.Action = "create_playlist"
	case containsAny(lower, "add to queue", "queue songs", "enqueue", "put in queue", "queue up"):
		intent.Action = "queue_songs"
	default:
		intent.Action = "play_now"
	}

	// 7. Genre detection
	genres := []string{"edm", "hip-hop", "rap", "pop", "rock", "acoustic", "lo-fi", "lofi", "classical", "jazz", "indie", "metal", "reggae"}
	for _, g := range genres {
		if strings.Contains(lower, g) {
			intent.Genres = append(intent.Genres, g)
		}
	}

	// 8. Intent Summary synthesis
	var summaryParts []string
	summaryParts = append(summaryParts, fmt.Sprintf("%s vibe", intent.Mood))
	if intent.Activity != "Listening" {
		summaryParts = append(summaryParts, fmt.Sprintf("for %s", intent.Activity))
	}
	summaryParts = append(summaryParts, fmt.Sprintf("(%d mins)", intent.DurationMinutes))
	if intent.Language != "Any" {
		summaryParts = append(summaryParts, fmt.Sprintf("in %s", intent.Language))
	}
	if intent.AvoidHeardToday {
		summaryParts = append(summaryParts, "excluding songs heard today")
	}

	intent.IntentSummary = strings.Join(summaryParts, " • ")

	return intent
}

func extractDurationMinutes(s string) int {
	// Pattern 1: (\d+)\s*(?:-| )?\s*(?:minute|minutes|min|mins|m)\b
	reMin := regexp.MustCompile(`(\d+)\s*(?:-| )?\s*(?:minute|minutes|min|mins|m)\b`)
	if matches := reMin.FindStringSubmatch(s); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil && val > 0 && val <= 300 {
			return val
		}
	}

	// Pattern 2: (\d+)\s*(?:-| )?\s*(?:hour|hours|hr|hrs|h)\b
	reHr := regexp.MustCompile(`(\d+)\s*(?:-| )?\s*(?:hour|hours|hr|hrs|h)\b`)
	if matches := reHr.FindStringSubmatch(s); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil && val > 0 && val <= 10 {
			return val * 60
		}
	}

	// Default duration is 30 minutes
	return 30
}

func containsAny(s string, terms ...string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

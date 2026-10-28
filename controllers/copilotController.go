package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/copilot"
	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// InitCopilotController initializes collections used by the Copilot
func InitCopilotController() {
	copilot.InitCollections()
}

// HandleCopilotQuery processes natural language queries, extracts intent via LLM/NLP,
// retrieves songs from catalog, filters listening history today, and ranks tracks.
func HandleCopilotQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.CopilotQueryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query payload", "details": err.Error()})
			return
		}

		// Retrieve authenticated user if available
		userID := ""
		if uid, exists := c.Get("user_id"); exists {
			if uStr, ok := uid.(string); ok {
				userID = uStr
			}
		}

		// 1. Extract Intent via LLM / Semantic NLP
		intent := copilot.ExtractIntent(req.Query)

		// Apply manual overrides if provided in request
		if req.TargetDuration > 0 {
			intent.DurationMinutes = req.TargetDuration
		}
		if req.Action != "" {
			intent.Action = req.Action
		}

		// 2. Retrieve & Rank candidate songs against catalog and history
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := copilot.RetrieveAndRank(ctx, intent, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve and rank songs", "details": err.Error()})
			return
		}

		resp.Query = req.Query
		c.JSON(http.StatusOK, resp)
	}
}

// HandleCopilotAction executes controlled actions requested by the Copilot assistant
// such as creating a new playlist or formatting queue actions.
func HandleCopilotAction() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.CopilotActionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action payload", "details": err.Error()})
			return
		}

		userID := ""
		if uid, exists := c.Get("user_id"); exists {
			if uStr, ok := uid.(string); ok {
				userID = uStr
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		switch req.ActionType {
		case "create_playlist":
			pName := req.PlaylistName
			if pName == "" {
				pName = "AI Copilot Playlist"
			}
			pDesc := req.PlaylistDescription
			if pDesc == "" {
				pDesc = "Curated automatically by Geethub AI Music Copilot."
			}

			playlist, err := copilot.CreateCopilotPlaylist(ctx, userID, pName, pDesc, req.SongIDs, req.IsPublic)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create copilot playlist", "details": err.Error()})
				return
			}

			c.JSON(http.StatusOK, models.CopilotActionResponse{
				Success:    true,
				ActionType: req.ActionType,
				Message:    "Playlist created successfully!",
				PlaylistID: playlist.ID.Hex(),
				Playlist:   playlist,
				Count:      len(req.SongIDs),
			})

		case "queue_songs", "play_now":
			c.JSON(http.StatusOK, models.CopilotActionResponse{
				Success:    true,
				ActionType: req.ActionType,
				Message:    "Action prepared for player execution",
				Count:      len(req.SongIDs),
			})

		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported action type: " + req.ActionType})
		}
	}
}

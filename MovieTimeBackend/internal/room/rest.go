package room

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"movietime-backend/internal/models"
	"net/http"
	"regexp"
)

type RestHandler struct {
	svc *Service
}

func NewRestHandler(svc *Service) *RestHandler {
	return &RestHandler{svc: svc}
}

func (h *RestHandler) Router() http.Handler {
	r := chi.NewRouter()
	r.Post("/rooms", h.createRoom)
	r.Get("/rooms/{id}", h.getRoom)
	return r
}

// Универсальный парсер ID из ссылки (поддерживает YouTube и Rutube)
func extractVideoID(url string) string {
	// Rutube: https://rutube.ru/video/6e5e06ad0f3104ae47fb0f69d2198855/
	reRutube := regexp.MustCompile(`rutube\.ru/video/([a-f0-9]+)`)
	if matches := reRutube.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1]
	}
	// YouTube: https://youtu.be/dQw4w9WgXcQ или https://www.youtube.com/watch?v=dQw4w9WgXcQ
	reYoutube := regexp.MustCompile(`(?:v=|\/)([0-9A-Za-z_-]{11})(?:[?&]|$)`)
	if matches := reYoutube.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1]
	}
	return "dQw4w9WgXcQ" // заглушка
}

func (h *RestHandler) createRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostNickname string `json:"hostNickname"`
		VideoUrl     string `json:"videoUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.HostNickname == "" {
		http.Error(w, "nickname required", http.StatusBadRequest)
		return
	}

	createReq := models.CreateRoomRequest{HostNickname: req.HostNickname}
	roomID, userID, err := h.svc.CreateRoom(r.Context(), createReq)
	if err != nil {
		http.Error(w, "failed to create room", http.StatusInternalServerError)
		return
	}

	// Сохраняем ID видео (если ссылка была введена)
	if req.VideoUrl != "" {
		room, _ := h.svc.GetRoom(r.Context(), roomID)
		room.Mu.Lock()
		room.VideoID = extractVideoID(req.VideoUrl)
		room.Mu.Unlock()
		h.svc.UpdateRoom(r.Context(), room)
	}

	json.NewEncoder(w).Encode(map[string]string{
		"roomId": roomID,
		"userId": userID,
	})
}

func (h *RestHandler) getRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          room.ID,
		"videoId":     room.VideoID,
		"isPlaying":   room.IsPlaying,
		"currentTime": room.CurrentTime,
		"users":       room.Users,
		"history":     room.History,
		"voteState":   room.VoteState,
	})
}

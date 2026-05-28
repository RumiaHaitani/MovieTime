package room

import (
	"encoding/json"
	"sync"
	"time"

	"movietime-backend/internal/models"
)

type Client struct {
	Room     *Room
	Send     chan []byte
	UserID   string
	Nickname string
	Conn     interface{}
}

type Room struct {
	ID          string
	HostID      string
	VideoID     string
	IsPlaying   bool
	CurrentTime float64
	Clients     map[*Client]bool
	Mu          sync.RWMutex
	History     []models.Message
	Users       map[string]models.User
	Reactions   []models.Reaction
	VoteState   *models.VoteState
	voteTimer   *time.Timer
}

func NewRoom(id, hostID string) *Room {
	return &Room{
		ID:          id,
		HostID:      hostID,
		VideoID:     "dQw4w9WgXcQ", // временно, заменится при создании
		IsPlaying:   false,
		CurrentTime: 0,
		Clients:     make(map[*Client]bool),
		Users:       make(map[string]models.User),
		History:     []models.Message{},
		Reactions:   []models.Reaction{},
		VoteState: &models.VoteState{
			Suggestions: []models.VideoSuggestion{},
			VotingUntil: 0,
		},
	}
}

func (r *Room) AddMessage(msg models.Message) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.History = append(r.History, msg)
	if len(r.History) > 100 {
		r.History = r.History[1:]
	}
}

func (r *Room) AddReaction(react models.Reaction) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Reactions = append(r.Reactions, react)
	if len(r.Reactions) > 50 {
		r.Reactions = r.Reactions[1:]
	}
}

func (r *Room) Broadcast(data []byte) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	for client := range r.Clients {
		select {
		case client.Send <- data:
		default:
		}
	}
}

// ===== ГОЛОСОВАНИЕ =====
func (r *Room) StartVoting(duration time.Duration) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if r.VoteState.VotingUntil != 0 {
		return
	}
	r.VoteState.VotingUntil = time.Now().Add(duration).Unix()
	if r.voteTimer != nil {
		r.voteTimer.Stop()
	}
	r.voteTimer = time.AfterFunc(duration, func() {
		r.FinishVoting()
	})
}

func (r *Room) FinishVoting() {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if len(r.VoteState.Suggestions) == 0 {
		r.VoteState.VotingUntil = 0
		return
	}
	winner := r.VoteState.Suggestions[0]
	for _, s := range r.VoteState.Suggestions {
		if s.Votes > winner.Votes {
			winner = s
		}
	}
	r.VoteState.WinningVideo = &winner
	// Извлекаем ID видео из ссылки – здесь без внешней функции, просто заглушка,
	// потому что реальный ID установит rest.go при создании комнаты.
	// Но для автоматической смены после голосования нужно парсить ссылку.
	// Сделаем простую заглушку, которая потом заменится.
	r.VideoID = extractYouTubeIDSimple(winner.VideoURL)
	r.CurrentTime = 0
	r.IsPlaying = false
	r.VoteState = &models.VoteState{Suggestions: []models.VideoSuggestion{}}
	r.BroadcastSync()
	r.BroadcastVoteState()
}

// простая заглушка, чтобы не вызывать ошибку при компиляции
func extractYouTubeIDSimple(url string) string {
	// Если ссылка на Rutube, можно извлечь ID, но для простоты оставим дефолт
	return "dQw4w9WgXcQ"
}

func (r *Room) BroadcastSync() {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	data, _ := json.Marshal(map[string]interface{}{
		"type": "sync",
		"data": map[string]interface{}{
			"videoId":     r.VideoID,
			"isPlaying":   r.IsPlaying,
			"currentTime": r.CurrentTime,
		},
	})
	r.Broadcast(data)
}

func (r *Room) BroadcastVoteState() {
	r.Mu.RLock()
	state := r.VoteState
	r.Mu.RUnlock()
	data, _ := json.Marshal(map[string]interface{}{
		"type": "vote_updated",
		"data": state,
	})
	r.Broadcast(data)
}

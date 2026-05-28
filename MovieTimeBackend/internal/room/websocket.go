package room

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"movietime-backend/internal/models"
	"movietime-backend/internal/utils"
	"net/http"
	"time"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WebSocketHandler struct {
	svc *Service
}

func NewWebSocketHandler(svc *Service) *WebSocketHandler {
	return &WebSocketHandler{svc: svc}
}

func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("roomId")
	nickname := r.URL.Query().Get("nickname")
	userID := r.URL.Query().Get("userId")

	if roomID == "" || nickname == "" || userID == "" {
		http.Error(w, "missing params", http.StatusBadRequest)
		return
	}

	room, err := h.svc.GetRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}

	client := &Client{
		Room:     room,
		Send:     make(chan []byte, 256),
		UserID:   userID,
		Nickname: nickname,
		Conn:     conn,
	}

	room.Mu.Lock()
	room.Clients[client] = true
	room.Users[userID] = models.User{ID: userID, Nickname: nickname}
	room.Mu.Unlock()
	h.svc.UpdateRoom(r.Context(), room)

	h.sendInitialState(client)
	h.broadcastJoin(client)

	go h.writePump(client, conn)
	go h.readPump(client, conn)
}

func (h *WebSocketHandler) sendInitialState(client *Client) {
	room := client.Room
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	state := map[string]interface{}{
		"type": "room_state",
		"data": map[string]interface{}{
			"videoId":     room.VideoID,
			"isPlaying":   room.IsPlaying,
			"currentTime": room.CurrentTime,
			"history":     room.History,
			"users":       room.Users,
			"reactions":   room.Reactions,
			"voteState":   room.VoteState,
		},
	}
	b, _ := json.Marshal(state)
	client.Send <- b
}

func (h *WebSocketHandler) broadcastJoin(client *Client) {
	room := client.Room
	sysMsg := models.Message{
		UserID:    "system",
		Nickname:  "System",
		Text:      client.Nickname + " присоединился",
		Timestamp: time.Now().Unix(),
	}
	room.AddMessage(sysMsg)
	h.broadcastChat(room, sysMsg)
	h.broadcastUsers(room)
	h.svc.UpdateRoom(context.Background(), room)
}

func (h *WebSocketHandler) broadcastChat(room *Room, msg models.Message) {
	b, _ := json.Marshal(map[string]interface{}{
		"type": "new_message",
		"data": msg,
	})
	room.Broadcast(b)
}

func (h *WebSocketHandler) broadcastUsers(room *Room) {
	room.Mu.RLock()
	list := make([]models.User, 0, len(room.Users))
	for _, u := range room.Users {
		list = append(list, u)
	}
	room.Mu.RUnlock()
	b, _ := json.Marshal(map[string]interface{}{
		"type": "user_list",
		"data": list,
	})
	room.Broadcast(b)
}

func (h *WebSocketHandler) broadcastSync(room *Room) {
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	b, _ := json.Marshal(map[string]interface{}{
		"type": "sync",
		"data": map[string]interface{}{
			"videoId":     room.VideoID,
			"isPlaying":   room.IsPlaying,
			"currentTime": room.CurrentTime,
		},
	})
	room.Broadcast(b)
}

func (h *WebSocketHandler) broadcastReaction(room *Room, react models.Reaction) {
	b, _ := json.Marshal(map[string]interface{}{
		"type": "new_reaction",
		"data": react,
	})
	room.Broadcast(b)
}

func (h *WebSocketHandler) broadcastVoteState(room *Room) {
	room.Mu.RLock()
	state := room.VoteState
	room.Mu.RUnlock()
	b, _ := json.Marshal(map[string]interface{}{
		"type": "vote_updated",
		"data": state,
	})
	room.Broadcast(b)
}

func (h *WebSocketHandler) handleMessage(client *Client, raw []byte) {
	var env struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	room := client.Room
	ctx := context.Background()

	switch env.Type {
	case "command":
		var cmd struct {
			Action string  `json:"action"`
			Time   float64 `json:"time"`
		}
		json.Unmarshal(env.Data, &cmd)
		room.Mu.Lock()
		switch cmd.Action {
		case "play":
			room.IsPlaying = true
		case "pause":
			room.IsPlaying = false
		case "seek":
			room.CurrentTime = cmd.Time
		}
		room.Mu.Unlock()
		room.BroadcastSync()
		h.svc.UpdateRoom(ctx, room)

	case "chat_message":
		var msg struct{ Text string }
		json.Unmarshal(env.Data, &msg)
		message := models.Message{
			UserID:    client.UserID,
			Nickname:  client.Nickname,
			Text:      msg.Text,
			Timestamp: time.Now().Unix(),
		}
		room.AddMessage(message)
		h.broadcastChat(room, message)
		h.svc.UpdateRoom(ctx, room)

	case "reaction":
		var react struct{ Emoji string }
		json.Unmarshal(env.Data, &react)
		reaction := models.Reaction{
			UserID:    client.UserID,
			Nickname:  client.Nickname,
			Emoji:     react.Emoji,
			Timestamp: time.Now().Unix(),
		}
		room.AddReaction(reaction)
		h.broadcastReaction(room, reaction)
		h.svc.UpdateRoom(ctx, room)

	case "vote_video":
		var vote struct {
			Action       string `json:"action"`
			VideoURL     string `json:"videoUrl"`
			SuggestionID string `json:"suggestionId"`
		}
		json.Unmarshal(env.Data, &vote)
		if vote.Action == "suggest" {
			sug := models.VideoSuggestion{
				ID:          utils.GenerateRoomID()[:8],
				SuggestedBy: client.Nickname,
				VideoURL:    vote.VideoURL,
				Votes:       1,
				Voters:      map[string]bool{client.UserID: true},
			}
			room.Mu.Lock()
			room.VoteState.Suggestions = append(room.VoteState.Suggestions, sug)
			if room.VoteState.VotingUntil == 0 {
				room.StartVoting(30 * time.Second)
			}
			room.Mu.Unlock()
			h.broadcastVoteState(room)
			h.svc.UpdateRoom(ctx, room)
		} else if vote.Action == "vote" {
			room.Mu.Lock()
			for i, s := range room.VoteState.Suggestions {
				if s.ID == vote.SuggestionID {
					if !s.Voters[client.UserID] {
						s.Votes++
						s.Voters[client.UserID] = true
						room.VoteState.Suggestions[i] = s
					}
					break
				}
			}
			room.Mu.Unlock()
			h.broadcastVoteState(room)
			h.svc.UpdateRoom(ctx, room)
		}
	}
}

func (h *WebSocketHandler) readPump(client *Client, conn *websocket.Conn) {
	defer func() {
		room := client.Room
		room.Mu.Lock()
		delete(room.Clients, client)
		delete(room.Users, client.UserID)
		room.Mu.Unlock()
		conn.Close()

		leaveMsg := models.Message{
			UserID:    "system",
			Nickname:  "System",
			Text:      client.Nickname + " покинул",
			Timestamp: time.Now().Unix(),
		}
		room.AddMessage(leaveMsg)
		h.broadcastChat(room, leaveMsg)
		h.broadcastUsers(room)
		h.svc.UpdateRoom(context.Background(), room)
	}()
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		h.handleMessage(client, msg)
	}
}

func (h *WebSocketHandler) writePump(client *Client, conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()
	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

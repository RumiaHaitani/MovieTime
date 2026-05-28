package models

type User struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

type Message struct {
	UserID    string `json:"userId"`
	Nickname  string `json:"nickname"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

type Reaction struct {
	UserID    string `json:"userId"`
	Nickname  string `json:"nickname"`
	Emoji     string `json:"emoji"`
	Timestamp int64  `json:"timestamp"`
}

type VideoSuggestion struct {
	ID          string          `json:"id"`
	SuggestedBy string          `json:"suggestedBy"`
	VideoURL    string          `json:"videoUrl"`
	Votes       int             `json:"votes"`
	Voters      map[string]bool `json:"-"`
}

type VoteState struct {
	Suggestions  []VideoSuggestion `json:"suggestions"`
	VotingUntil  int64             `json:"votingUntil"`
	WinningVideo *VideoSuggestion  `json:"winningVideo,omitempty"`
}

type CreateRoomRequest struct {
	HostNickname string `json:"hostNickname"`
}

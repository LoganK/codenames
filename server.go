package codenames

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jbowens/dictionary"
)

type Server interface {
	NewServeMux(staticDir string) *http.ServeMux
	Shutdown()
}

type server struct {
	tpl *template.Template

	gameIDWords []string

	mu           sync.Mutex
	games        map[string]*Game
	defaultWords []string
	cleanupDone  chan struct{}
}

func NewServer(gameIdFile, wordFile string) (Server, error) {
	gameIDs, err := dictionary.Load(gameIdFile)
	if err != nil {
		return nil, fmt.Errorf("load gameIds %s: %v", gameIdFile, err)
	}
	gameIDs = dictionary.Filter(gameIDs, func(s string) bool { return len(s) > 3 })
	gameIDWords := gameIDs.Words()

	d, err := dictionary.Load(wordFile)
	if err != nil {
		return nil, fmt.Errorf("load words %s: %v", wordFile, err)
	}
	defaultWords := d.Words()
	sort.Strings(defaultWords)

	tpl, err := template.ParseFiles(
		filepath.Join("frontend", "index.html.tmpl"),
		filepath.Join("frontend", "analytics.html.tmpl"),
	)
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %v", err)
	}

	ticker := time.NewTicker(10 * time.Minute)
	cleanupDone := make(chan struct{})

	s := &server{
		tpl,
		gameIDWords,
		sync.Mutex{},
		make(map[string]*Game),
		defaultWords,
		cleanupDone,
	}

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-cleanupDone:
				return
			case <-ticker.C:
				s.cleanupOldGames()
			}
		}
	}()

	return s, nil
}

func (s *server) NewServeMux(staticDir string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/next-game", s.handleNextGame)
	mux.HandleFunc("/end-turn", s.handleEndTurn)
	mux.HandleFunc("/guess", s.handleGuess)
	mux.HandleFunc("/game/", s.handleRetrieveGame)
	mux.HandleFunc("/game-state", s.handleGameState)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.HandleFunc("/", s.handleIndex)

	return mux
}

func (s *server) Shutdown() {
	s.cleanupDone <- struct{}{}
}

func (s *server) getGame(gameID, stateID string) (*Game, bool) {
	g, ok := s.games[gameID]
	if ok {
		return g, ok
	}
	state, ok := decodeGameState(stateID, s.defaultWords)
	if !ok {
		return nil, false
	}
	g = newGame(gameID, state)
	s.games[gameID] = g
	return g, true
}

// GET /game/<id>
// (deprecated: use POST /game-state instead)
func (s *server) handleRetrieveGame(rw http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := req.ParseForm()
	if err != nil {
		http.Error(rw, "Error decoding query string", 400)
		return
	}

	gameID := path.Base(req.URL.Path)
	g, ok := s.getGame(gameID, req.Form.Get("state_id"))
	if ok {
		writeGame(rw, g)
		return
	}

	g = newGame(gameID, randomState(s.defaultWords))
	s.games[gameID] = g
	writeGame(rw, g)
}

// POST /game-state
func (s *server) handleGameState(rw http.ResponseWriter, req *http.Request) {
	var body struct {
		GameID  string `json:"game_id"`
		StateID string `json:"state_id"`
	}
	err := json.NewDecoder(req.Body).Decode(&body)
	if err != nil {
		http.Error(rw, "Error decoding request body", 400)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.getGame(body.GameID, body.StateID)
	if ok {
		writeGame(rw, g)
		return
	}
	g = newGame(body.GameID, randomState(s.defaultWords))
	s.games[body.GameID] = g
	writeGame(rw, g)
}

// POST /guess
func (s *server) handleGuess(rw http.ResponseWriter, req *http.Request) {
	var request struct {
		GameID  string `json:"game_id"`
		StateID string `json:"state_id"`
		Index   int    `json:"index"`
	}

	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(rw, "Error decoding", 400)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.getGame(request.GameID, request.StateID)
	if !ok {
		http.Error(rw, "No such game", 404)
		return
	}

	if err := g.Guess(request.Index); err != nil {
		http.Error(rw, err.Error(), 400)
		return
	}
	writeGame(rw, g)
}

// POST /end-turn
func (s *server) handleEndTurn(rw http.ResponseWriter, req *http.Request) {
	var request struct {
		GameID  string `json:"game_id"`
		StateID string `json:"state_id"`
	}

	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(rw, "Error decoding", 400)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.getGame(request.GameID, request.StateID)
	if !ok {
		http.Error(rw, "No such game", 404)
		return
	}

	if err := g.NextTurn(); err != nil {
		http.Error(rw, err.Error(), 400)
		return
	}
	writeGame(rw, g)
}

func (s *server) handleNextGame(rw http.ResponseWriter, req *http.Request) {
	var request struct {
		GameID  string   `json:"game_id"`
		WordSet []string `json:"word_set"`
	}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		http.Error(rw, "Error decoding", 400)
		return
	}
	wordSet := map[string]bool{}
	for _, w := range request.WordSet {
		wordSet[strings.TrimSpace(strings.ToUpper(w))] = true
	}
	if len(wordSet) > 0 && len(wordSet) < 25 {
		http.Error(rw, "Need at least 25 words", 400)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	words := s.defaultWords
	if len(wordSet) > 0 {
		words = nil
		for w := range wordSet {
			words = append(words, w)
		}
		sort.Strings(words)
	}

	g := newGame(request.GameID, randomState(words))
	s.games[request.GameID] = g
	writeGame(rw, g)
}

type statsResponse struct {
	InProgress int `json:"games_in_progress"`
}

func (s *server) handleStats(rw http.ResponseWriter, req *http.Request) {
	var inProgress int

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, g := range s.games {
		if g.WinningTeam == nil {
			inProgress++
		}
	}
	writeJSON(rw, statsResponse{inProgress})
}

func (s *server) cleanupOldGames() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, g := range s.games {
		if g.WinningTeam != nil && g.CreatedAt.Add(12*time.Hour).Before(time.Now()) {
			delete(s.games, id)
			fmt.Printf("Removed completed game %s\n", id)
			continue
		}
		if g.CreatedAt.Add(24 * time.Hour).Before(time.Now()) {
			delete(s.games, id)
			fmt.Printf("Removed expired game %s\n", id)
			continue
		}
	}
}

func writeGame(rw http.ResponseWriter, g *Game) {
	writeJSON(rw, struct {
		*Game
		StateID string `json:"state_id"`
	}{g, g.GameState.ID()})
}

func writeJSON(rw http.ResponseWriter, resp interface{}) {
	j, err := json.Marshal(resp)
	if err != nil {
		http.Error(rw, "unable to marshal response: "+err.Error(), 500)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.Write(j)
}

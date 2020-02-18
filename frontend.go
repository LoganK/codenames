package codenames

import (
	"flag"
	"math/rand"
	"net/http"
	"path/filepath"
	"strings"
)

var googleAnalyticsID = flag.String("google_analytics_id", "UA-88084599-2", "The Google Analytics tracking ID.")

type templateParameters struct {
	SelectedGameID      string
	AutogeneratedGameID string
	GoogleAnalyticsID   string
}

func (s *server) handleIndex(rw http.ResponseWriter, req *http.Request) {
	dir, id := filepath.Split(req.URL.Path)
	if dir != "" && dir != "/" {
		http.NotFound(rw, req)
		return
	}

	autogeneratedID := ""
	for {
		autogeneratedID = strings.ToLower(s.gameIDWords[rand.Intn(len(s.gameIDWords))])
		if _, ok := s.games[autogeneratedID]; !ok {
			break
		}
	}

	err := s.tpl.Execute(rw, templateParameters{
		SelectedGameID:      id,
		AutogeneratedGameID: autogeneratedID,
		GoogleAnalyticsID:   *googleAnalyticsID,
	})
	if err != nil {
		http.Error(rw, "error rendering", http.StatusInternalServerError)
	}
}

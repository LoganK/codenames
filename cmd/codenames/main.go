package main

import (
        "flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/jbowens/codenames"
)

var staticDir = flag.String("static_dir", "frontend/dist", "Directory containing the static assets for serving.")

func main() {
        flag.Parse()

	rand.Seed(time.Now().UnixNano())

	s := &http.Server{
		Addr: ":9091",
	}

	codenames, err := codenames.NewServer("assets/game-id-words.txt", "assets/original.txt")
	if err != nil {
		log.Fatal(err)
	}
	s.Handler = codenames.NewServeMux(*staticDir)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	log.Printf("Starting server. Available on http://%s:%s", hostname, s.Addr)
	log.Print(s.ListenAndServe())
	codenames.Shutdown()
}

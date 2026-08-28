package main

import (
	"fmt"
	"net/http"
	"os"
)

// User represents a user model.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// HandleGetUsers returns users list.
func HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	port := os.Getenv("PORT")
	fmt.Fprintf(w, `[{"id":"1","email":"user@example.com"}] (port: %s)`, port)
}

func main() {
	http.HandleFunc("/api/v1/users", HandleGetUsers)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	_ = http.ListenAndServe(":"+port, nil)
}

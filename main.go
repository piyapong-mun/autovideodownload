package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var db *sql.DB

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./sessions.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		log.Fatal(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS sessions (
		"id" TEXT NOT NULL PRIMARY KEY,
		"created_at" DATETIME,
		"expires_at" DATETIME
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}
}

func validateOrCreateSession(w http.ResponseWriter, r *http.Request) string {
	var sessionID string
	cookie, err := r.Cookie("session")
	if err == nil {
		sessionID = cookie.Value
		var expiresAt time.Time
		err = db.QueryRow("SELECT expires_at FROM sessions WHERE id = ?", sessionID).Scan(&expiresAt)
		if err == nil && time.Now().Before(expiresAt) {
			return sessionID
		}
	}

	sessionID = uuid.New().String()
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = db.Exec("INSERT INTO sessions (id, created_at, expires_at) VALUES (?, ?, ?)", sessionID, time.Now(), expiresAt)
	if err != nil {
		log.Printf("Failed to insert session: %v", err)
	}

	cookie = &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	// Create Folder for session
	err = os.MkdirAll(filepath.Join(".", "video", sessionID), 0755)
	if err != nil {
		log.Printf("Failed to create session folder: %v", err)
	}
	return sessionID
}

func isValidSession(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	var expiresAt time.Time
	err = db.QueryRow("SELECT expires_at FROM sessions WHERE id = ?", cookie.Value).Scan(&expiresAt)
	if err != nil {
		return false
	}
	return time.Now().Before(expiresAt)
}

type DownloadRequest struct {
	URL    string `json:"url"`
	Binary string `json:"binary"` // "windows" or "linux"
}

type Response struct {
	Message string   `json:"message"`
	Output  string   `json:"output"`
	Error   string   `json:"error,omitempty"`
	Files   []string `json:"files,omitempty"`
}

func main() {
	initDB()
	defer db.Close()

	// Clear db
	db.Exec("DELETE FROM sessions")

	// Serve the static HTML file
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		validateOrCreateSession(w, r)

		http.ServeFile(w, r, "index.html")
	})

	// Background Task
	go func() {
		for {
			// list unexpired sessions
			var sessionIDs []string
			rows, err := db.Query("SELECT id FROM sessions WHERE expires_at > ?", time.Now())
			if err != nil {
				log.Printf("Failed to query sessions: %v", err)
				return
			}
			for rows.Next() {
				var sessionID string
				err := rows.Scan(&sessionID)
				if err != nil {
					continue
				}
				sessionIDs = append(sessionIDs, sessionID)
			}

			log.Printf("Found %d active sessions", len(sessionIDs))

			// Delete all folder that are not in sessionIDs
			// List all folder in video directory
			folders, err := os.ReadDir("./video")
			if err != nil {
				log.Printf("Failed to read video directory: %v", err)
				return
			}
			for _, folder := range folders {
				if folder.IsDir() {
					if !contains(sessionIDs, folder.Name()) {
						err = os.RemoveAll(filepath.Join("./video", folder.Name()))
						if err != nil {
							log.Printf("Failed to delete folder: %v", err)
						}
					}
				}
			}
			time.Sleep(10 * time.Second)
		}

	}()

	// API Endpoint to download video
	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !isValidSession(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(Response{Error: "Unauthorized or invalid session"})
			return
		}

		var req DownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.URL == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(Response{Error: "URL is required"})
			return
		}

		sessionID := validateOrCreateSession(w, r)

		binaryName := "./video/yt-dlp" // Default Linux
		if req.Binary == "windows" {
			binaryName = "./video/yt-dlp.exe" // Windows (in same dir)
		}

		log.Printf("Starting download for URL: %s using binary: %s", req.URL, binaryName)

		// Create command to run yt-dlp. Add -P to specific output path
		cmd := exec.Command(binaryName, "-P", "./video/"+sessionID, req.URL)

		// Run and get output
		output, err := cmd.CombinedOutput()

		files := []string{}

		if err != nil {
			log.Printf("Download failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(Response{
				Error:  fmt.Sprintf("Download failed: %v", err),
				Output: string(output),
			})
			return
		}

		// list File name
		files_list, err_list := os.ReadDir("./video/" + sessionID)
		if err_list != nil {
			log.Printf("Failed to read directory: %v", err_list)
			return
		}

		for _, f := range files_list {
			if !f.IsDir() {
				files = append(files, f.Name())
			}
		}

		// output = {files: [ "file1", "file2"]}

		// Send File list name
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Message: "Download successful",
			Output:  string(output),
			Files:   files,
		})
	})

	http.HandleFunc("/api/file", func(w http.ResponseWriter, r *http.Request) {
		if !isValidSession(r) {
			http.Error(w, "Unauthorized session", http.StatusUnauthorized)
			return
		}
		cookie, _ := r.Cookie("session")
		sessionID := cookie.Value

		filename := r.URL.Query().Get("name")
		if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
			http.Error(w, "Invalid filename", http.StatusBadRequest)
			return
		}

		filePath := filepath.Join(".", "video", sessionID, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		http.ServeFile(w, r, filePath)
	})

	fmt.Println("Server is running on http://localhost:1112")
	log.Fatal(http.ListenAndServe(":1112", nil))
}

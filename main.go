package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DownloadRequest struct {
	URL    string `json:"url"`
	Binary string `json:"binary"` // "windows" or "linux"
}

type Response struct {
	Message string `json:"message"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

func main() {
	// Serve the static HTML file
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})

	// API Endpoint to download video
	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

		binaryName := "./video/yt-dlp" // Default Linux
		if req.Binary == "windows" {
			binaryName = "./video/yt-dlp.exe" // Windows (in same dir)
		}

		log.Printf("Starting download for URL: %s using binary: %s", req.URL, binaryName)

		// Create command to run yt-dlp. Add -P to specific output path
		cmd := exec.Command(binaryName, "-P", "./video", req.URL)

		// Run and get output
		output, err := cmd.CombinedOutput()

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

		// Find the generated .mp4 in ./video
		files, err := os.ReadDir("./video")
		var downloadedFile string
		if err == nil {
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".mp4") {
					downloadedFile = filepath.Join(".", "video", f.Name())
					break
				}
			}
		}

		if downloadedFile == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(Response{
				Error:  "Could not find generated .mp4 file",
				Output: string(output),
			})
			return
		}

		// Serve the file
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(downloadedFile)))
		w.Header().Set("Content-Type", "video/mp4")

		file, err := os.Open(downloadedFile)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(Response{Error: "Could not open generated file"})
			return
		}

		io.Copy(w, file)
		file.Close()

		os.Remove(downloadedFile)
		log.Printf("Sent file and deleted %s", downloadedFile)
	})

	fmt.Println("Server is running on http://localhost:1112")
	log.Fatal(http.ListenAndServe(":1112", nil))
}

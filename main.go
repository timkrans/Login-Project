package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"github.com/google/uuid"
)

var tmpl = template.Must(template.ParseGlob("templates/*"))
var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./users.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	createTables()
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/signup", signupHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/welcome", authMiddleware(welcomeHandler))
	http.HandleFunc("/", notFoundRedirectHandler)
	log.Println("Server running at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

func createTables() {
	userQuery := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL
	);`
	_, err := db.Exec(userQuery)
	if err != nil {
		log.Fatal(err)
	}

	sessionQuery := `
	CREATE TABLE IF NOT EXISTS sessions (
		session_id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		expires DATETIME NOT NULL
	);`
	_, err = db.Exec(sessionQuery)
	if err != nil {
		log.Fatal(err)
	}
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec("INSERT INTO users(username, password) VALUES (?, ?)", username, string(hashedPassword))
		if err != nil {
			http.Error(w, "Username already exists", http.StatusConflict)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	tmpl.ExecuteTemplate(w, "signup.html", nil)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		var dbPassword string
		err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&dbPassword)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(dbPassword), []byte(password)) != nil {
			tmpl.ExecuteTemplate(w, "invalid.html", nil)
			return
		}

		sessionID := uuid.NewString()
		expiresAt := time.Now().Add(24 * time.Hour)

		_, err = db.Exec("INSERT INTO sessions(session_id, username, expires) VALUES (?, ?, ?)", sessionID, username, expiresAt)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, 
			Expires:  expiresAt,
		})

		http.Redirect(w, r, "/welcome", http.StatusSeeOther)
		return
	}
	tmpl.ExecuteTemplate(w, "login.html", nil)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err == nil {
		db.Exec("DELETE FROM sessions WHERE session_id = ?", cookie.Value)
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
		})
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func notFoundRedirectHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	validPaths := map[string]bool{
		"/signup":  true,
		"/login":   true,
		"/logout":  true,
		"/welcome": true,
	}
	if len(path) >= 8 && path[:8] == "/static/" {
		http.ServeFile(w, r, path)
		return
	}
	if !validPaths[path] {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
}

func welcomeHandler(w http.ResponseWriter, r *http.Request) {
	username := getUsernameFromRequest(r)
	tmpl.ExecuteTemplate(w, "welcome.html", struct{ Username string }{Username: username})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if getUsernameFromRequest(r) == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func getUsernameFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}

	var username string
	var expires time.Time

	err = db.QueryRow("SELECT username, expires FROM sessions WHERE session_id = ?", cookie.Value).Scan(&username, &expires)
	if err != nil || time.Now().After(expires) {
		return ""
	}
	return username
}

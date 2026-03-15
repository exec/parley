package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/db"
)

var repo *db.Repository
var adminJWTSecret string
var parleyJWTSecret string
var redisClient *redis.Client

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: parley-admin <command>")
		fmt.Println("Commands:")
		fmt.Println("  serve              Start the admin HTTP server")
		fmt.Println("  create-user <name> Create a new admin user (prompts for password)")
		fmt.Println("  activate <name>    Activate an admin user")
		fmt.Println("  deactivate <name>  Deactivate an admin user")
		fmt.Println("  list-users         List all admin users")
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	var err error
	repo, err = db.NewRepositoryWithDSN(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := repo.RunMigrations(context.Background()); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	adminJWTSecret = os.Getenv("ADMIN_JWT_SECRET")
	if adminJWTSecret == "" {
		log.Fatal("ADMIN_JWT_SECRET is required")
	}
	parleyJWTSecret = os.Getenv("PARLEY_JWT_SECRET")

	// Optional Redis for kicking WS connections on ban/force-logout
	if redisHost := os.Getenv("REDIS_HOST"); redisHost != "" {
		opt := &redis.Options{Addr: redisHost + ":6379"}
		rc := redis.NewClient(opt)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := rc.Ping(ctx).Err(); err != nil {
			log.Printf("Warning: Redis unavailable (%v), WS kick will not propagate across nodes", err)
		} else {
			redisClient = rc
			log.Printf("Connected to Redis at %s for WS kick events", redisHost)
		}
		cancel()
	}

	switch os.Args[1] {
	case "serve":
		runServer()
	case "create-user":
		if len(os.Args) < 3 {
			fmt.Println("Usage: parley-admin create-user <username>")
			os.Exit(1)
		}
		cliCreateUser(os.Args[2])
	case "activate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: parley-admin activate <username>")
			os.Exit(1)
		}
		cliSetActive(os.Args[2], true)
	case "deactivate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: parley-admin deactivate <username>")
			os.Exit(1)
		}
		cliSetActive(os.Args[2], false)
	case "list-users":
		cliListUsers()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// publishKick publishes a kick event to Redis so API nodes disconnect the user's WS connections.
func publishKick(userID string) {
	if redisClient == nil {
		return
	}
	env := map[string]interface{}{
		"node_id":    "admin",
		"event_type": "kick",
		"user_id":    userID,
		"event":      "FORCE_LOGOUT",
		"data":       json.RawMessage("{}"),
	}
	data, _ := json.Marshal(env)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisClient.Publish(ctx, "parley:events", data).Err(); err != nil {
		log.Printf("Warning: failed to publish kick for user %s: %v", userID, err)
	}
}

// ─── CLI commands ──────────────────────────────────────────────────────────────

func cliCreateUser(username string) {
	fmt.Printf("Password for %s: ", username)
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)
	if password == "" {
		fmt.Println("Password cannot be empty")
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	u, err := repo.AdminCreateUser(context.Background(), username, string(hash))
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}
	fmt.Printf("Created admin user '%s' (ID: %d) — inactive. Run 'activate %s' to enable login.\n", u.Username, u.ID, u.Username)
}

func cliSetActive(username string, active bool) {
	if err := repo.AdminSetActive(context.Background(), username, active); err != nil {
		if err == db.ErrNotFound {
			fmt.Printf("Admin user '%s' not found\n", username)
			os.Exit(1)
		}
		log.Fatal(err)
	}
	state := "activated"
	if !active {
		state = "deactivated"
	}
	fmt.Printf("Admin user '%s' %s.\n", username, state)
}

func cliListUsers() {
	users, err := repo.AdminListUsers(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%-5s %-20s %-10s %-25s %-25s\n", "ID", "Username", "Active", "Created", "Last Login")
	fmt.Println(strings.Repeat("-", 90))
	for _, u := range users {
		lastLogin := "never"
		if u.LastLoginAt != nil {
			lastLogin = u.LastLoginAt.Format("2006-01-02 15:04:05")
		}
		activeStr := "no"
		if u.Active {
			activeStr = "YES"
		}
		fmt.Printf("%-5d %-20s %-10s %-25s %-25s\n", u.ID, u.Username, activeStr, u.CreatedAt.Format("2006-01-02 15:04:05"), lastLogin)
	}
}

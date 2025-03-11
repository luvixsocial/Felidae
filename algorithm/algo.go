package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"felidae/types"

	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5"
)

var (
	ctx         = context.Background()
	pgConn, _   = pgx.Connect(ctx, "postgres://user:password@localhost:5432/mydb")
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
)

func GetUserFeed(userID string, limit int) ([]int, []types.Post, error) {
	personalized, err := getPersonalizedFeed(userID, limit/2)
	if err != nil {
		log.Println("Error fetching personalized feed:", err)
	}

	randomPosts, err := getRandomPosts(limit / 2)
	if err != nil {
		log.Println("Error fetching random posts:", err)
	}

	postSet := make(map[int]bool)
	finalFeed := []int{}

	for _, post := range personalized {
		postSet[post] = true
		finalFeed = append(finalFeed, post)
	}

	for _, post := range randomPosts {
		if !postSet[post] {
			finalFeed = append(finalFeed, post)
		}
	}

	fullPosts, err := getPostsByID(finalFeed)
	if err != nil {
		return nil, nil, err
	}

	return finalFeed, fullPosts, nil
}

func getPersonalizedFeed(userID string, limit int) ([]int, error) {
	tags, err := redisClient.ZRevRange(ctx, fmt.Sprintf("user:%s:tag_scores", userID), 0, 4).Result()
	if err != nil {
		return nil, err
	}

	query := `SELECT id, tags FROM posts WHERE tags && $1 ORDER BY created_at DESC LIMIT $2`
	rows, err := pgConn.Query(ctx, query, tags, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []int
	for rows.Next() {
		var postID int
		var postTags []string
		rows.Scan(&postID, &postTags)

		score := computePersonalizedScore(userID, postID, postTags)

		redisClient.ZAdd(ctx, fmt.Sprintf("user:%s:feed", userID), &redis.Z{
			Score:  score,
			Member: postID,
		})

		posts = append(posts, postID)
	}

	return redisClient.ZRevRange(ctx, fmt.Sprintf("user:%s:feed", userID), 0, int64(limit)-1).Result()
}

func getRandomPosts(limit int) ([]int, error) {
	rows, err := pgConn.Query(ctx, "SELECT id FROM posts ORDER BY RANDOM() LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postIDs []int
	for rows.Next() {
		var postID int
		rows.Scan(&postID)
		postIDs = append(postIDs, postID)
	}

	return postIDs, nil
}

func getPostsByID(postIDs []int) ([]types.Post, error) {
	if len(postIDs) == 0 {
		return []types.Post{}, nil
	}

	query := `SELECT id, title, content, tags, created_at FROM posts WHERE id = ANY($1)`
	rows, err := pgConn.Query(ctx, query, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []types.Post
	for rows.Next() {
		var p types.Post
		rows.Scan(&p.ID, &p.Title, &p.Content, &p.Tags, &p.CreatedAt)
		posts = append(posts, p)
	}

	return posts, nil
}

func computePersonalizedScore(userID string, postID int, tags []string) float64 {
	rows, err := pgConn.Query(ctx, "SELECT post_id FROM user_interactions WHERE user_id = $1", userID)
	if err != nil {
		log.Println("Error fetching interactions:", err)
		return 0
	}
	defer rows.Close()

	interactionHistory := make(map[int]bool)
	for rows.Next() {
		var interactedPostID int
		rows.Scan(&interactedPostID)
		interactionHistory[interactedPostID] = true
	}

	interactionBoost := 0.0
	if interactionHistory[postID] {
		interactionBoost = 5.0
	}

	tagMatchScore := 0.0
	for _, tag := range tags {
		count, _ := redisClient.ZScore(ctx, fmt.Sprintf("user:%s:tag_scores", userID), tag).Result()
		tagMatchScore += count
	}

	return tagMatchScore + interactionBoost
}

func notifyUser(userID string) {
	feed, fullPosts, err := GetUserFeed(userID, 10)
	if err != nil {
		return
	}

	message := fmt.Sprintf("update:%v - %v", feed, fullPosts)
	notifyClientsForUser(userID, message)
}

func notifyClientsForUser(userID, message string) {
	fmt.Printf("Sending real-time update to user %s: %s\n", userID, message)
}

// 🎯 Testing the Feed System
func main() {
	userID := "user-123"
	limit := 10

	feed, fullPosts, err := GetUserFeed(userID, limit)
	if err != nil {
		log.Fatal("Failed to fetch feed:", err)
	}

	fmt.Println("Generated Feed IDs:", feed)
	fmt.Println("Generated Full Posts:", fullPosts)

	// Simulate real-time update
	go func() {
		for {
			time.Sleep(10 * time.Second)
			notifyUser(userID)
		}
	}()

	// Keep the program running for testing
	select {}
}
